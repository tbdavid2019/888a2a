//nolint:revive
package common

import (
	"time"

	"connectrpc.com/connect"
	"github.com/google/cel-go/cel"
	celtypes "github.com/google/cel-go/common/types"
	"github.com/pkg/errors"
	"google.golang.org/genproto/googleapis/type/expr"
)

const celLimit = 1024 * 1024

// IAMPolicyConditionCELAttributes are the variables when evaluating IAM policy condition.
var IAMPolicyConditionCELAttributes = []cel.EnvOption{
	cel.Variable(CELAttributeResourceEnvironmentID, cel.StringType),
	cel.Variable(CELAttributeRequestTime, cel.TimestampType),
	cel.ParserExpressionSizeLimit(celLimit),
}

// ValidateIAMBindingConditionExpr validates an IAM binding condition against
// the allowed attribute set (request.time). It returns a compiled program so
// callers can reuse it; a nil or empty expression compiles to nil.
func ValidateIAMBindingConditionExpr(expression *expr.Expr) (cel.Program, error) {
	return validateCELExpr(expression, IAMPolicyConditionCELAttributes)
}

func validateCELExpr(expression *expr.Expr, conditionCELAttributes []cel.EnvOption) (cel.Program, error) {
	if expression == nil || expression.Expression == "" {
		return nil, nil
	}
	e, err := cel.NewEnv(
		conditionCELAttributes...,
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	ast, issues := e.Compile(expression.Expression)
	if issues != nil && issues.Err() != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, issues.Err())
	}
	prog, err := e.Program(ast)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return prog, nil
}

func EvalBindingCondition(expr string, requestTime time.Time) (bool, error) {
	input := map[string]any{
		CELAttributeRequestTime: requestTime,
	}
	return doEvalBindingCondition(expr, input)
}

func doEvalBindingCondition(expr string, input map[string]any) (bool, error) {
	if expr == "" {
		return true, nil
	}

	e, err := cel.NewEnv(IAMPolicyConditionCELAttributes...)
	if err != nil {
		return false, errors.Wrapf(err, "failed to new cel env")
	}
	ast, iss := e.Compile(expr)
	if iss != nil && iss.Err() != nil {
		return false, errors.Wrapf(iss.Err(), "failed to compile expr %q", expr)
	}
	// enable partial evaluation because the input only has request.time
	// but the expression can have more.
	prg, err := e.Program(ast, cel.EvalOptions(cel.OptPartialEval))
	if err != nil {
		return false, errors.Wrapf(err, "failed to construct program")
	}
	vars, err := e.PartialVars(input)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get vars")
	}
	out, _, err := prg.Eval(vars)
	if err != nil {
		return false, errors.Wrapf(err, "failed to eval cel expr")
	}
	// `out` is one of
	// - True
	// - False
	// - a residual expression.

	// return true if the result is a residual expression
	// which means that it passes "the request.time < xxx" check.
	if !celtypes.IsBool(out) {
		return true, nil
	}

	res, ok := out.Equal(celtypes.True).Value().(bool)
	if !ok {
		return false, errors.Errorf("failed to convert cel result to bool")
	}
	return res, nil
}
