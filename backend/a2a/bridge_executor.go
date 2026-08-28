package a2a

import (
	"context"
	"errors"
	"iter"
	"strings"
	"sync"
	"time"

	sdk "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
)

// BridgeAgentExecutor adapts one explicitly configured AgentBridge to the
// official A2A executor contract. The bridge is selected by the caller; this
// type never falls back to another provider or transport.
type BridgeAgentExecutor struct {
	bridge   AgentBridge
	bindings *BindingRegistry

	mu       sync.Mutex
	sessions map[sdk.TaskID]BridgeSession
}

var _ a2asrv.AgentExecutor = (*BridgeAgentExecutor)(nil)

// NewBridgeAgentExecutor creates an A2A executor backed by a configured bridge.
func NewBridgeAgentExecutor(agentID string, bridge AgentBridge) (*BridgeAgentExecutor, error) {
	if strings.TrimSpace(agentID) == "" {
		return nil, errors.New("agent id is required")
	}
	if bridge == nil {
		return nil, errors.New("agent bridge is required")
	}
	return &BridgeAgentExecutor{bridge: bridge, bindings: NewBindingRegistry(nil), sessions: make(map[sdk.TaskID]BridgeSession)}, nil
}

// Execute forwards one A2A message to the configured bridge and translates
// bounded bridge events into A2A artifacts and terminal status updates.
func (e *BridgeAgentExecutor) Execute(ctx context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[sdk.Event, error] {
	return func(yield func(sdk.Event, error) bool) {
		if execCtx == nil || execCtx.Message == nil {
			yield(nil, errors.New("A2A bridge execution requires a message"))
			return
		}
		request, err := e.requestFromContext(ctx, execCtx)
		if err != nil {
			yield(nil, err)
			return
		}
		if execCtx.StoredTask == nil && !yield(sdk.NewSubmittedTask(execCtx, execCtx.Message), nil) {
			return
		}
		if !yield(sdk.NewStatusUpdateEvent(execCtx, sdk.TaskStateWorking, nil), nil) {
			return
		}

		if err := e.bridge.Preflight(ctx, request); err != nil {
			yield(sdk.NewStatusUpdateEvent(execCtx, sdk.TaskStateRejected, sdk.NewMessageForTask(sdk.MessageRoleAgent, execCtx, sdk.NewTextPart("bridge preflight rejected"))), nil)
			return
		}
		session, err := e.bridge.Start(ctx, request)
		if err != nil || session == nil {
			reason := "bridge did not start"
			if err != nil {
				reason = "bridge did not start: " + err.Error()
			}
			yield(sdk.NewStatusUpdateEvent(execCtx, sdk.TaskStateFailed, sdk.NewMessageForTask(sdk.MessageRoleAgent, execCtx, sdk.NewTextPart(reason))), nil)
			return
		}
		binding, err := e.bindings.Start(request, request.Timeout)
		if err != nil {
			_ = session.Stop(context.Background())
			yield(sdk.NewStatusUpdateEvent(execCtx, sdk.TaskStateRejected, sdk.NewMessageForTask(sdk.MessageRoleAgent, execCtx, sdk.NewTextPart("bridge binding rejected"))), nil)
			return
		}
		e.remember(sdk.TaskID(request.TaskID), session)
		defer func() {
			e.forget(sdk.TaskID(request.TaskID))
			e.bindings.Stop(binding.BindingID, request.OrganizationID)
			_ = session.Stop(context.Background())
		}()

		var artifactID sdk.ArtifactID
		result, invokeErr := session.Invoke(ctx, request, func(event BridgeEvent) error {
			if event.Text == "" {
				return nil
			}
			var artifact *sdk.TaskArtifactUpdateEvent
			if artifactID == "" {
				artifact = sdk.NewArtifactEvent(execCtx, sdk.NewTextPart(event.Text))
				artifactID = artifact.Artifact.ID
			} else {
				artifact = sdk.NewArtifactUpdateEvent(execCtx, artifactID, sdk.NewTextPart(event.Text))
			}
			if !yield(artifact, nil) {
				_ = session.Cancel(context.Background())
			}
			return nil
		})
		if invokeErr != nil && result.Outcome == "" {
			result.Outcome = DeliveryOutcomeUnknown
		}
		state := sdk.TaskStateCompleted
		reason := result.Output
		switch result.Outcome {
		case DeliveryOutcomeRejected:
			state = sdk.TaskStateRejected
		case DeliveryOutcomeNotDelivered, DeliveryOutcomeUnknown:
			state = sdk.TaskStateFailed
		}
		if reason == "" {
			reason = result.Reason
		}
		if invokeErr != nil && reason == "" {
			reason = "bridge invocation failed"
		}
		if reason == "" {
			reason = "bridge completed"
		}
		if !yield(sdk.NewStatusUpdateEvent(execCtx, state, sdk.NewMessageForTask(sdk.MessageRoleAgent, execCtx, sdk.NewTextPart(reason))), nil) {
			return
		}
	}
}

// Cancel cancels only the bridge session associated with this A2A task.
func (e *BridgeAgentExecutor) Cancel(ctx context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[sdk.Event, error] {
	return func(yield func(sdk.Event, error) bool) {
		if execCtx == nil {
			return
		}
		e.mu.Lock()
		session := e.sessions[execCtx.TaskID]
		e.mu.Unlock()
		if session != nil {
			if err := session.Cancel(ctx); err != nil {
				yield(nil, err)
				return
			}
		}
		if execCtx.StoredTask == nil || !execCtx.StoredTask.Status.State.Terminal() {
			yield(sdk.NewStatusUpdateEvent(execCtx, sdk.TaskStateCanceled, sdk.NewMessageForTask(sdk.MessageRoleAgent, execCtx, sdk.NewTextPart("task canceled by request"))), nil)
		}
	}
}

func (e *BridgeAgentExecutor) requestFromContext(ctx context.Context, execCtx *a2asrv.ExecutorContext) (BridgeRequest, error) {
	callerID := ""
	organizationID := strings.TrimSpace(execCtx.Tenant)
	if caller, ok := CallerFromContext(ctx); ok && caller != nil {
		callerID = caller.GetPrincipalID()
		if organizationID == "" {
			organizationID = caller.GetTenantID()
		}
	}
	if callerID == "" && execCtx.User != nil && execCtx.User.Authenticated {
		callerID = execCtx.User.Name
	}
	if callerID == "" {
		return BridgeRequest{}, errors.New("authenticated A2A caller is required")
	}
	if organizationID == "" {
		return BridgeRequest{}, errors.New("A2A organization is required")
	}
	input := make([]string, 0, len(execCtx.Message.Parts))
	for _, part := range execCtx.Message.Parts {
		if text := part.Text(); text != "" {
			input = append(input, text)
		}
	}
	if len(input) == 0 {
		return BridgeRequest{}, errors.New("A2A message must contain text")
	}
	taskID := string(execCtx.TaskID)
	if taskID == "" {
		return BridgeRequest{}, errors.New("A2A task id is required")
	}
	timeout := 5 * time.Minute
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return BridgeRequest{}, context.DeadlineExceeded
		}
		if remaining < timeout {
			timeout = remaining
		}
	}
	return BridgeRequest{
		OrganizationID: organizationID,
		CallerID:       callerID,
		TaskID:         taskID,
		ContextID:      execCtx.ContextID,
		CorrelationID:  execCtx.Message.ID,
		BridgeID:       e.bridge.ID(),
		Input:          strings.Join(input, " "),
		MaxOutputBytes: MaxBridgeOutputBytes,
		Timeout:        timeout,
	}, nil
}

func (e *BridgeAgentExecutor) remember(taskID sdk.TaskID, session BridgeSession) {
	e.mu.Lock()
	e.sessions[taskID] = session
	e.mu.Unlock()
}

func (e *BridgeAgentExecutor) forget(taskID sdk.TaskID) {
	e.mu.Lock()
	delete(e.sessions, taskID)
	e.mu.Unlock()
}
