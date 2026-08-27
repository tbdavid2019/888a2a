package approval

import (
	"context"
	"encoding/json"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/tbdavid2019/888a2a/backend/a2a"
	a2a888 "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
	"github.com/tbdavid2019/888a2a/backend/manager/store"
)

// NewChecker adapts durable organization approvals to the ACP executor
// callback. It persists only normalized action parameters and waits on the
// same request from any Manager replica.
func NewChecker(approvalStore *store.ApprovalStore, binding store.ApprovalBinding) a2a.ApprovalChecker {
	return func(ctx context.Context, permission a2a.PermissionRequest) (a2a.ApprovalCheckResult, error) {
		if approvalStore == nil {
			return a2a.ApprovalCheckResult{Decision: a2a.DecisionDeny, Reason: "approval store is not configured"}, errors.New("approval store is not configured")
		}
		if binding.OrganizationID == "" || binding.RequesterPrincipal == "" || binding.PolicyName == "" || binding.PolicyVersion == "" {
			return a2a.ApprovalCheckResult{Decision: a2a.DecisionDeny, Reason: "approval binding is incomplete"}, errors.New("approval binding is incomplete")
		}
		parameters, err := json.Marshal(map[string]string{
			"action_kind": string(permission.ActionKind), "command": permission.Command,
			"mcp_server": permission.MCPServer, "mcp_tool": permission.MCPTool,
			"target_path": permission.TargetPath,
		})
		if err != nil {
			return a2a.ApprovalCheckResult{Decision: a2a.DecisionDeny, Reason: "approval parameters are invalid"}, err
		}
		action := &a2a888.BoundAction{
			OrganizationId: binding.OrganizationID, WorkspaceId: binding.WorkspaceID,
			ResourceName: permission.TargetPath, AgentId: binding.ExecutingAgentID,
			ActionType: string(permission.ActionKind), Destination: permission.ToolName,
			NormalizedParametersJson: string(parameters), TaskId: permission.WorkID,
		}
		intentHash, err := store.ApprovalIntentHash(action)
		if err != nil {
			return a2a.ApprovalCheckResult{Decision: a2a.DecisionDeny, Reason: "approval intent is invalid"}, err
		}
		action.IntentHash = intentHash
		timeout := binding.Timeout
		if timeout <= 0 {
			timeout = 5 * time.Minute
		}
		request := &a2a888.ApprovalRequest{
			Name: store.ApprovalRequestName(binding, intentHash), OrganizationId: binding.OrganizationID,
			WorkspaceId: binding.WorkspaceID, PolicyName: binding.PolicyName, PolicyVersion: binding.PolicyVersion,
			RequesterPrincipalId: binding.RequesterPrincipal, ExecutingAgentId: binding.ExecutingAgentID,
			ExecutingAgentOwnerId: binding.ExecutingAgentOwner, Action: action,
			RequiredApprovals: binding.RequiredApprovals, ExpiresAt: timestamppb.New(time.Now().UTC().Add(timeout)),
		}
		if err := approvalStore.CreateRequest(ctx, request); err != nil {
			return a2a.ApprovalCheckResult{Decision: a2a.DecisionDeny, Reason: "approval request could not be created"}, err
		}
		result, err := approvalStore.WaitForDecision(ctx, binding.OrganizationID, request.Name, intentHash)
		if err != nil {
			return a2a.ApprovalCheckResult{Decision: a2a.DecisionDeny, Reason: result.Reason}, err
		}
		if result.Decision == store.ApprovalWaitAllow {
			return a2a.ApprovalCheckResult{Decision: a2a.DecisionAllow, Reason: result.Reason}, nil
		}
		return a2a.ApprovalCheckResult{Decision: a2a.DecisionDeny, Reason: result.Reason}, nil
	}
}
