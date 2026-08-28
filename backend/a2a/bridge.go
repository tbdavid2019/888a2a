package a2a

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	// MaxBridgeInputBytes bounds data sent to a local Provider bridge.
	MaxBridgeInputBytes = 1 << 20
	// MaxBridgeOutputBytes bounds data accepted from a local Provider bridge.
	MaxBridgeOutputBytes = 4 << 20
	maxBridgeTimeout     = 10 * time.Minute
)

// DeliveryOutcome is deliberately narrower than an A2A task state. It says
// whether the bridge knows the local runtime accepted or completed a request.
type DeliveryOutcome string

const (
	DeliveryOutcomeDelivered    DeliveryOutcome = "delivered"
	DeliveryOutcomeRejected     DeliveryOutcome = "rejected"
	DeliveryOutcomeNotDelivered DeliveryOutcome = "not_delivered"
	DeliveryOutcomeUnknown      DeliveryOutcome = "outcome_unknown"
)

// BridgeRequest is the complete identity and resource budget for one bridge
// invocation. A bridge must not infer any of these values from process state.
type BridgeRequest struct {
	OrganizationID string
	CallerID       string
	TaskID         string
	ContextID      string
	CorrelationID  string
	BridgeID       string
	Input          string
	MaxOutputBytes int
	Timeout        time.Duration
}

// BridgeEvent is an ordered, bounded event emitted by a running bridge.
type BridgeEvent struct {
	Sequence uint64
	Kind     string
	Text     string
	Terminal bool
}

// BridgeResult reports the final bridge delivery outcome and bounded output.
type BridgeResult struct {
	Outcome DeliveryOutcome
	Output  string
	Reason  string
	Events  []BridgeEvent
}

// BridgeHealth is a read-only bridge readiness report.
type BridgeHealth struct {
	Ready  bool
	Detail string
}

// AgentBridge is the common boundary for ACP, Gateway, CLI and MCP bridges.
// Implementations own only their runtime session; routing and cross-transport
// retry decisions remain outside this interface.
type AgentBridge interface {
	ID() string
	Preflight(context.Context, BridgeRequest) error
	Start(context.Context, BridgeRequest) (BridgeSession, error)
	Health(context.Context) (BridgeHealth, error)
}

// BridgeSession is one isolated runtime invocation. Invoke may stream ordered
// events through emit. A caller must not invoke the session after Stop.
type BridgeSession interface {
	Invoke(context.Context, BridgeRequest, func(BridgeEvent) error) (BridgeResult, error)
	Cancel(context.Context) error
	Stop(context.Context) error
}

// ExecuteBridge validates identity and budgets, runs one bridge session, and
// always stops that session. It never retries a possibly executed invocation.
func ExecuteBridge(ctx context.Context, bridge AgentBridge, request BridgeRequest, emit func(BridgeEvent) error) (BridgeResult, error) {
	if bridge == nil {
		return BridgeResult{Outcome: DeliveryOutcomeRejected}, fmt.Errorf("bridge is required")
	}
	if err := ValidateBridgeRequest(request, bridge.ID()); err != nil {
		return BridgeResult{Outcome: DeliveryOutcomeRejected, Reason: "invalid bridge request"}, err
	}
	if err := bridge.Preflight(ctx, request); err != nil {
		return BridgeResult{Outcome: DeliveryOutcomeRejected, Reason: "bridge preflight rejected"}, err
	}
	session, err := bridge.Start(ctx, request)
	if err != nil {
		return BridgeResult{Outcome: DeliveryOutcomeNotDelivered, Reason: "bridge did not start"}, err
	}
	if session == nil {
		return BridgeResult{Outcome: DeliveryOutcomeNotDelivered, Reason: "bridge returned no session"}, fmt.Errorf("bridge returned nil session")
	}
	defer func() { _ = session.Stop(ctx) }()
	result, err := session.Invoke(ctx, request, func(event BridgeEvent) error {
		if emit == nil {
			return nil
		}
		return emit(event)
	})
	if err != nil {
		if result.Outcome == "" {
			result.Outcome = DeliveryOutcomeUnknown
		}
		return result, err
	}
	if err := ValidateBridgeResult(result); err != nil {
		return BridgeResult{Outcome: DeliveryOutcomeUnknown, Reason: "invalid bridge result"}, err
	}
	return result, nil
}

// ValidateBridgeRequest rejects missing identity and unsafe budgets before a
// local process, Gateway, or MCP call can begin.
func ValidateBridgeRequest(request BridgeRequest, bridgeID string) error {
	for name, value := range map[string]string{
		"organization_id": request.OrganizationID,
		"caller_id":       request.CallerID,
		"task_id":         request.TaskID,
		"context_id":      request.ContextID,
		"correlation_id":  request.CorrelationID,
		"bridge_id":       request.BridgeID,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
		if strings.ContainsAny(value, "\r\n") || len(value) > 256 {
			return fmt.Errorf("%s is invalid", name)
		}
	}
	if bridgeID != "" && request.BridgeID != bridgeID {
		return fmt.Errorf("bridge_id does not match bridge")
	}
	if len([]byte(request.Input)) > MaxBridgeInputBytes {
		return fmt.Errorf("bridge input exceeds %d bytes", MaxBridgeInputBytes)
	}
	if request.MaxOutputBytes <= 0 || request.MaxOutputBytes > MaxBridgeOutputBytes {
		return fmt.Errorf("max output bytes must be between 1 and %d", MaxBridgeOutputBytes)
	}
	if request.Timeout <= 0 || request.Timeout > maxBridgeTimeout {
		return fmt.Errorf("bridge timeout must be between 1ns and %s", maxBridgeTimeout)
	}
	return nil
}

// ValidateBridgeResult rejects malformed or out-of-order bridge evidence.
func ValidateBridgeResult(result BridgeResult) error {
	switch result.Outcome {
	case DeliveryOutcomeDelivered, DeliveryOutcomeRejected, DeliveryOutcomeNotDelivered, DeliveryOutcomeUnknown:
	default:
		return fmt.Errorf("invalid bridge outcome %q", result.Outcome)
	}
	if len([]byte(result.Output)) > MaxBridgeOutputBytes {
		return fmt.Errorf("bridge output exceeds %d bytes", MaxBridgeOutputBytes)
	}
	var previous uint64
	for i, event := range result.Events {
		if event.Sequence == 0 || event.Sequence <= previous {
			return fmt.Errorf("bridge event %d has non-monotonic sequence", i)
		}
		if len([]byte(event.Text)) > MaxBridgeOutputBytes {
			return fmt.Errorf("bridge event %d exceeds output limit", i)
		}
		previous = event.Sequence
	}
	return nil
}
