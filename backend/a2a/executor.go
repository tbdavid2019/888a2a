package a2a

import (
	"context"
	"fmt"
	"iter"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
)

// ExecutionHandler is a pluggable function to execute work for an agent.
type ExecutionHandler func(ctx context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error]

// AgentExecutorAdapter implements the official a2asrv.AgentExecutor interface.
type AgentExecutorAdapter struct {
	agentID string
	handler ExecutionHandler
}

// NewAgentExecutor creates a new AgentExecutorAdapter.
func NewAgentExecutor(agentID string, handler ExecutionHandler) *AgentExecutorAdapter {
	return &AgentExecutorAdapter{
		agentID: agentID,
		handler: handler,
	}
}

// Execute executes work and yields ordered A2A events.
func (a *AgentExecutorAdapter) Execute(ctx context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	if a.handler != nil {
		return a.handler(ctx, execCtx)
	}

	return func(yield func(a2a.Event, error) bool) {
		// If task was not yet created in store, emit submitted
		if execCtx.StoredTask == nil {
			if !yield(a2a.NewSubmittedTask(execCtx, execCtx.Message), nil) {
				return
			}
		}

		// Emit working status
		if !yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateWorking, nil), nil) {
			return
		}

		// Extract text from request
		input := ""
		if execCtx.Message != nil {
			var parts []string
			for _, part := range execCtx.Message.Parts {
				if text := part.Text(); text != "" {
					parts = append(parts, text)
				}
			}
			input = strings.Join(parts, " ")
		}

		// Emit response message
		respText := fmt.Sprintf("[%s] processed: %s", a.agentID, input)
		respMsg := a2a.NewMessageForTask(
			a2a.MessageRoleAgent,
			execCtx,
			a2a.NewTextPart(respText),
		)

		// Emit completion
		yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCompleted, respMsg), nil)
	}
}

// Cancel cancels the task idempotently.
func (a *AgentExecutorAdapter) Cancel(_ context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		// If already in terminal state, yield nothing or current status
		if execCtx.StoredTask != nil {
			state := execCtx.StoredTask.Status.State
			if state == a2a.TaskStateCompleted || state == a2a.TaskStateFailed || state == a2a.TaskStateCanceled || state == a2a.TaskStateRejected {
				return
			}
		}

		cancelMsg := a2a.NewMessage(
			a2a.MessageRoleAgent,
			a2a.NewTextPart("task canceled by request"),
		)
		yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCanceled, cancelMsg), nil)
	}
}
