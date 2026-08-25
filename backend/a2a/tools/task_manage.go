package tools

import (
	"context"
	"fmt"
	"iter"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/google/uuid"
	"github.com/pkg/errors"

	a2apkg "github.com/Ranxy/laelia/backend/a2a"
	"github.com/Ranxy/laelia/backend/manager/store"
)

// TaskGetInput defines input parameters for retrieving a task.
type TaskGetInput struct {
	TenantID     string `json:"tenant,omitempty"`
	WorkID       string `json:"workId"`
	CallerAgentID string `json:"callerAgentId,omitempty"`
}

// WorkUsageOutput describes resource consumption.
type WorkUsageOutput struct {
	Depth       int32 `json:"depth"`
	Children    int32 `json:"children"`
	FanOut      int32 `json:"fanOut"`
	RuntimeMs   int64 `json:"runtimeMs"`
	Tokens      int64 `json:"tokens"`
	WorkUnits   int64 `json:"workUnits"`
}

// TaskGetResult contains comprehensive task information.
type TaskGetResult struct {
	TenantID         string                     `json:"tenant"`
	WorkID           string                     `json:"workId"`
	A2ATaskID        string                     `json:"a2aTaskId"`
	ContextID        string                     `json:"contextId"`
	RequesterAgentID string                     `json:"requesterAgentId"`
	ExecutorAgentID  string                     `json:"executorAgentId"`
	State            string                     `json:"state"`
	TerminalReason   string                     `json:"terminalReason,omitempty"`
	ParentWorkID     string                     `json:"parentWorkId,omitempty"`
	DelegationDepth  int32                      `json:"delegationDepth"`
	IdempotencyKey   string                     `json:"idempotencyKey"`
	Trace            *TraceCorrelationInput     `json:"trace,omitempty"`
	Budget           *WorkBudgetInput           `json:"budget,omitempty"`
	Usage            *WorkUsageOutput           `json:"usage,omitempty"`
	Artifacts        []*store.WorkArtifactMessage `json:"artifacts,omitempty"`
	CreatedAt        time.Time                  `json:"createdAt"`
	UpdatedAt        time.Time                  `json:"updatedAt"`
	StartedAt        *time.Time                 `json:"startedAt,omitempty"`
	CompletedAt      *time.Time                 `json:"completedAt,omitempty"`
	Version          uint64                     `json:"version"`
}

// TaskGet retrieves a work task by ID.
func TaskGet(ctx context.Context, s WorkStore, in TaskGetInput) (*TaskGetResult, error) {
	if s == nil {
		return nil, errors.New("store is required")
	}
	if in.WorkID == "" {
		return nil, errors.New("workId is required")
	}

	tenant := in.TenantID
	if tenant == "" {
		tenant = "default"
	}

	work, err := s.GetWork(ctx, tenant, in.WorkID)
	if err != nil {
		// Fallback to A2A task ID lookup
		if errors.Is(err, store.ErrWorkNotFound) {
			work, err = s.GetWorkByA2ATaskID(ctx, tenant, in.WorkID)
		}
		if err != nil {
			return nil, errors.Wrap(err, "get task from store")
		}
	}

	artifacts, _ := s.ListWorkArtifacts(ctx, tenant, work.WorkID)

	return mapWorkToResult(work, artifacts), nil
}

// TaskListInput defines parameters for querying tasks.
type TaskListInput struct {
	TenantID         string `json:"tenant,omitempty"`
	ContextID        string `json:"contextId,omitempty"`
	RequesterAgentID string `json:"requesterAgentId,omitempty"`
	ExecutorAgentID  string `json:"executorAgentId,omitempty"`
	State            string `json:"state,omitempty"`
	Limit            int    `json:"limit,omitempty"`
	Offset           int    `json:"offset,omitempty"`
	IncludeArtifacts bool   `json:"includeArtifacts,omitempty"`
}

// TaskListResult contains task search results and pagination details.
type TaskListResult struct {
	Tasks      []*TaskGetResult `json:"tasks"`
	TotalCount int              `json:"totalCount"`
	Limit      int              `json:"limit"`
	Offset     int              `json:"offset"`
}

// TaskList queries tasks matching the filter.
func TaskList(ctx context.Context, s WorkStore, in TaskListInput) (*TaskListResult, error) {
	if s == nil {
		return nil, errors.New("store is required")
	}

	tenant := in.TenantID
	if tenant == "" {
		tenant = "default"
	}

	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	filter := store.ListWorkFilter{
		TenantID:         tenant,
		ContextID:        in.ContextID,
		RequesterAgentID: in.RequesterAgentID,
		ExecutorAgentID:  in.ExecutorAgentID,
		State:            in.State,
		Limit:            limit,
		Offset:           in.Offset,
	}

	works, totalCount, err := s.ListWork(ctx, filter)
	if err != nil {
		return nil, errors.Wrap(err, "list work from store")
	}

	results := make([]*TaskGetResult, 0, len(works))
	for _, w := range works {
		var artifacts []*store.WorkArtifactMessage
		if in.IncludeArtifacts {
			artifacts, _ = s.ListWorkArtifacts(ctx, tenant, w.WorkID)
		}
		results = append(results, mapWorkToResult(w, artifacts))
	}

	return &TaskListResult{
		Tasks:      results,
		TotalCount: totalCount,
		Limit:      limit,
		Offset:     in.Offset,
	}, nil
}

// TaskSubscribeInput defines parameters for subscribing to task events.
type TaskSubscribeInput struct {
	TenantID     string `json:"tenant,omitempty"`
	WorkID       string `json:"workId"`
	FromSequence uint64 `json:"fromSequence,omitempty"`
}

// TaskSubscribe streams events for a task.
func TaskSubscribe(ctx context.Context, em *a2apkg.EventManager, in TaskSubscribeInput) (iter.Seq2[a2a.Event, error], error) {
	if em == nil {
		return nil, errors.New("event manager is required")
	}
	if in.WorkID == "" {
		return nil, errors.New("workId is required")
	}

	tenant := in.TenantID
	if tenant == "" {
		tenant = "default"
	}

	return em.Subscribe(ctx, tenant, in.WorkID, in.FromSequence), nil
}

// TaskCancelInput defines parameters for canceling a task.
type TaskCancelInput struct {
	TenantID      string `json:"tenant,omitempty"`
	WorkID        string `json:"workId"`
	CallerAgentID string `json:"callerAgentId,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

// TaskCancelResult contains the outcome of a cancellation.
type TaskCancelResult struct {
	TenantID       string `json:"tenant"`
	WorkID         string `json:"workId"`
	State          string `json:"state"`
	TerminalReason string `json:"terminalReason"`
	Version        uint64 `json:"version"`
	AlreadyDone    bool   `json:"alreadyDone"`
}

// TaskCancel idempotently transitions a task to CANCELED.
func TaskCancel(ctx context.Context, s WorkStore, em *a2apkg.EventManager, in TaskCancelInput) (*TaskCancelResult, error) {
	if s == nil {
		return nil, errors.New("store is required")
	}
	if in.WorkID == "" {
		return nil, errors.New("workId is required")
	}

	tenant := in.TenantID
	if tenant == "" {
		tenant = "default"
	}

	reason := in.Reason
	if reason == "" {
		reason = "task canceled by request"
	}

	work, err := s.GetWork(ctx, tenant, in.WorkID)
	if err != nil {
		if errors.Is(err, store.ErrWorkNotFound) {
			work, err = s.GetWorkByA2ATaskID(ctx, tenant, in.WorkID)
		}
		if err != nil {
			return nil, errors.Wrap(err, "get task for cancel")
		}
	}

	// Idempotency: if already in a terminal state, return current terminal state
	if isTerminal(work.State) {
		return &TaskCancelResult{
			TenantID:       work.TenantID,
			WorkID:         work.WorkID,
			State:          work.State,
			TerminalReason: work.TerminalReason,
			Version:        work.Version,
			AlreadyDone:    true,
		}, nil
	}

	newVersion, err := s.UpdateWorkState(ctx, tenant, work.WorkID, work.Version, "CANCELED", reason)
	if err != nil {
		if errors.Is(err, store.ErrWorkVersionMismatch) {
			current, getErr := s.GetWork(ctx, tenant, work.WorkID)
			if getErr == nil && isTerminal(current.State) {
				return &TaskCancelResult{
					TenantID:       current.TenantID,
					WorkID:         current.WorkID,
					State:          current.State,
					TerminalReason: current.TerminalReason,
					Version:        current.Version,
					AlreadyDone:    true,
				}, nil
			}
		}
		return nil, errors.Wrap(err, "update work state to canceled")
	}

	// Append durable work event
	seq, _ := s.GetLatestWorkEventSequence(ctx, tenant, work.WorkID)
	nextSeq := seq + 1
	event := &store.WorkEventMessage{
		TenantID:       tenant,
		EventID:        uuid.New().String(),
		WorkID:         work.WorkID,
		Sequence:       nextSeq,
		EventType:      "CANCELED",
		TerminalReason: reason,
		CreatedAt:      time.Now(),
		Metadata: map[string]string{
			"context_id": work.ContextID,
			"canceledBy": in.CallerAgentID,
		},
	}
	_ = s.AppendWorkEvent(ctx, event)

	if em != nil {
		cancelMsg := a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart(reason))
		execCtx := &a2a.TaskInfo{
			TaskID:    a2a.TaskID(work.A2ATaskID),
			ContextID: work.ContextID,
		}
		em.Publish(tenant, work.WorkID, a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCanceled, cancelMsg), nextSeq)
	}

	return &TaskCancelResult{
		TenantID:       tenant,
		WorkID:         work.WorkID,
		State:          "CANCELED",
		TerminalReason: reason,
		Version:        newVersion,
		AlreadyDone:    false,
	}, nil
}

// ArtifactInput defines an artifact to attach when replying to a task.
type ArtifactInput struct {
	ArtifactID  string `json:"artifactId,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MediaType   string `json:"mediaType,omitempty"`
	ExternalURI string `json:"externalUri,omitempty"`
	FileID      string `json:"fileId,omitempty"`
	Digest      string `json:"digest,omitempty"`
	SizeBytes   int64  `json:"sizeBytes,omitempty"`
}

// TaskReplyInput defines parameters for an executor or reviewer replying to a task.
type TaskReplyInput struct {
	TenantID      string                 `json:"tenant,omitempty"`
	WorkID        string                 `json:"workId"`
	CallerAgentID string                 `json:"callerAgentId,omitempty"`
	State         string                 `json:"state,omitempty"` // Default: COMPLETED
	Message       string                 `json:"message,omitempty"`
	Artifacts     []ArtifactInput        `json:"artifacts,omitempty"`
	Trace         *TraceCorrelationInput `json:"trace,omitempty"`
}

// TaskReplyResult contains the result of replying to a task.
type TaskReplyResult struct {
	TenantID       string                       `json:"tenant"`
	WorkID         string                       `json:"workId"`
	A2ATaskID      string                       `json:"a2aTaskId"`
	ContextID      string                       `json:"contextId"`
	State          string                       `json:"state"`
	TerminalReason string                       `json:"terminalReason,omitempty"`
	Artifacts      []*store.WorkArtifactMessage `json:"artifacts,omitempty"`
	Version        uint64                       `json:"version"`
	UpdatedAt      time.Time                    `json:"updatedAt"`
}

// TaskReply records a result, status update, and/or artifacts for a delegated task.
func TaskReply(ctx context.Context, s WorkStore, em *a2apkg.EventManager, in TaskReplyInput) (*TaskReplyResult, error) {
	if s == nil {
		return nil, errors.New("store is required")
	}
	if in.WorkID == "" {
		return nil, errors.New("workId is required")
	}

	tenant := in.TenantID
	if tenant == "" {
		tenant = "default"
	}

	newState := in.State
	if newState == "" {
		newState = "COMPLETED"
	}

	work, err := s.GetWork(ctx, tenant, in.WorkID)
	if err != nil {
		if errors.Is(err, store.ErrWorkNotFound) {
			work, err = s.GetWorkByA2ATaskID(ctx, tenant, in.WorkID)
		}
		if err != nil {
			return nil, errors.Wrap(err, "get task for reply")
		}
	}

	// Persist artifacts
	var savedArtifacts []*store.WorkArtifactMessage
	for _, art := range in.Artifacts {
		artID := art.ArtifactID
		if artID == "" {
			artID = uuid.New().String()
		}

		var fileUUID *uuid.UUID
		if art.FileID != "" {
			if parsed, err := uuid.Parse(art.FileID); err == nil {
				fileUUID = &parsed
			}
		}

		artMsg := &store.WorkArtifactMessage{
			TenantID:    tenant,
			WorkID:      work.WorkID,
			ArtifactID:  artID,
			Name:        art.Name,
			Description: art.Description,
			MediaType:   art.MediaType,
			ExternalURI: art.ExternalURI,
			FileID:      fileUUID,
			Digest:      art.Digest,
			SizeBytes:   art.SizeBytes,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		if err := s.CreateWorkArtifact(ctx, artMsg); err != nil {
			return nil, errors.Wrap(err, "save work artifact")
		}
		savedArtifacts = append(savedArtifacts, artMsg)
	}

	// Update work state
	newVersion, err := s.UpdateWorkState(ctx, tenant, work.WorkID, work.Version, newState, in.Message)
	if err != nil {
		return nil, errors.Wrap(err, "update work state in reply")
	}

	// Append durable event
	seq, _ := s.GetLatestWorkEventSequence(ctx, tenant, work.WorkID)
	nextSeq := seq + 1

	eventMeta := map[string]string{
		"context_id": work.ContextID,
		"repliedBy":  in.CallerAgentID,
	}
	if in.Message != "" {
		eventMeta["message"] = in.Message
	}

	event := &store.WorkEventMessage{
		TenantID:       tenant,
		EventID:        uuid.New().String(),
		WorkID:         work.WorkID,
		Sequence:       nextSeq,
		EventType:      newState,
		TerminalReason: in.Message,
		CreatedAt:      time.Now(),
		Metadata:       eventMeta,
	}
	if in.Trace != nil {
		event.TraceID = in.Trace.TraceID
		event.RootTraceID = in.Trace.RootTraceID
		event.SpanID = in.Trace.SpanID
		event.ParentSpanID = in.Trace.ParentSpanID
	}
	_ = s.AppendWorkEvent(ctx, event)

	// Publish A2A events
	if em != nil {
		execCtx := &a2a.TaskInfo{
			TaskID:    a2a.TaskID(work.A2ATaskID),
			ContextID: work.ContextID,
		}

		// Publish artifact events if present
		for _, art := range savedArtifacts {
			var parts []*a2a.Part
			if art.ExternalURI != "" {
				parts = append(parts, a2a.NewFileURLPart(a2a.URL(art.ExternalURI), art.MediaType))
			} else if art.Description != "" {
				parts = append(parts, a2a.NewTextPart(art.Description))
			}
			artEvent := a2a.NewArtifactUpdateEvent(execCtx, a2a.ArtifactID(art.ArtifactID), parts...)
			em.Publish(tenant, work.WorkID, artEvent, nextSeq)
		}

		// Publish status update / terminal event
		var respMsg *a2a.Message
		if in.Message != "" {
			respMsg = a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart(in.Message))
		}
		statusEvent := a2a.NewStatusUpdateEvent(execCtx, mapDurableStateToSDKState(newState), respMsg)
		em.Publish(tenant, work.WorkID, statusEvent, nextSeq)
	}

	allArtifacts, _ := s.ListWorkArtifacts(ctx, tenant, work.WorkID)

	return &TaskReplyResult{
		TenantID:       tenant,
		WorkID:         work.WorkID,
		A2ATaskID:      work.A2ATaskID,
		ContextID:      work.ContextID,
		State:          newState,
		TerminalReason: in.Message,
		Artifacts:      allArtifacts,
		Version:        newVersion,
		UpdatedAt:      time.Now(),
	}, nil
}

func mapWorkToResult(w *store.WorkMessage, artifacts []*store.WorkArtifactMessage) *TaskGetResult {
	if w == nil {
		return nil
	}

	res := &TaskGetResult{
		TenantID:         w.TenantID,
		WorkID:           w.WorkID,
		A2ATaskID:        w.A2ATaskID,
		ContextID:        w.ContextID,
		RequesterAgentID: w.RequesterAgentID,
		ExecutorAgentID:  w.ExecutorAgentID,
		State:            w.State,
		TerminalReason:   w.TerminalReason,
		ParentWorkID:     w.ParentWorkID.String,
		DelegationDepth:  w.DelegationDepth,
		IdempotencyKey:   w.IdempotencyKey,
		CreatedAt:        w.CreatedAt,
		UpdatedAt:        w.UpdatedAt,
		StartedAt:        w.StartedAt,
		CompletedAt:      w.CompletedAt,
		Version:          w.Version,
		Artifacts:        artifacts,
	}

	if w.TraceID != "" {
		res.Trace = &TraceCorrelationInput{
			TraceID:      w.TraceID,
			RootTraceID:  w.RootTraceID,
			SpanID:       w.SpanID,
			ParentSpanID: w.ParentSpanID,
		}
	}

	if w.MaxDepth > 0 || w.MaxTokens > 0 || w.MaxRuntimeMs > 0 || w.MaxChildren > 0 {
		res.Budget = &WorkBudgetInput{
			MaxDepth:       w.MaxDepth,
			MaxChildren:    w.MaxChildren,
			MaxFanOut:      w.MaxFanOut,
			MaxConcurrency: w.MaxConcurrency,
			MaxRuntimeMs:   w.MaxRuntimeMs,
			MaxRetries:     w.MaxRetries,
			MaxTokens:      w.MaxTokens,
			MaxWorkUnits:   w.MaxWorkUnits,
		}
	}

	res.Usage = &WorkUsageOutput{
		Depth:     w.DelegationDepth,
		Children:  w.UsedChildren,
		FanOut:    w.UsedFanOut,
		RuntimeMs: w.UsedRuntimeMs,
		Tokens:    w.UsedTokens,
		WorkUnits: w.UsedWorkUnits,
	}

	return res
}

func isTerminal(state string) bool {
	return state == "COMPLETED" || state == "FAILED" || state == "CANCELED" || state == "REJECTED"
}

func mapDurableStateToSDKState(s string) a2a.TaskState {
	switch s {
	case "SUBMITTED":
		return a2a.TaskStateSubmitted
	case "WORKING":
		return a2a.TaskStateWorking
	case "COMPLETED":
		return a2a.TaskStateCompleted
	case "FAILED":
		return a2a.TaskStateFailed
	case "CANCELED":
		return a2a.TaskStateCanceled
	case "INPUT_REQUIRED":
		return a2a.TaskStateInputRequired
	case "AUTH_REQUIRED":
		return a2a.TaskStateAuthRequired
	case "REJECTED":
		return a2a.TaskStateRejected
	default:
		return a2a.TaskStateSubmitted
	}
}

// FormatTaskGet formats a TaskGetResult into human-readable text.
func FormatTaskGet(r *TaskGetResult) string {
	if r == nil {
		return "Task not found.\n"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("### A2A Task `%s` [%s]\n", r.A2ATaskID, r.State))
	sb.WriteString(fmt.Sprintf("- **Context ID**: `%s` (Work ID: `%s`)\n", r.ContextID, r.WorkID))
	sb.WriteString(fmt.Sprintf("- **Executor**: `%s` (Requester: `%s`)\n", r.ExecutorAgentID, r.RequesterAgentID))

	if r.ParentWorkID != "" {
		sb.WriteString(fmt.Sprintf("- **Parent Task**: `%s` (Depth: %d)\n", r.ParentWorkID, r.DelegationDepth))
	}
	if r.TerminalReason != "" {
		sb.WriteString(fmt.Sprintf("- **Outcome**: %s\n", r.TerminalReason))
	}
	if r.Trace != nil && r.Trace.TraceID != "" {
		sb.WriteString(fmt.Sprintf("- **Trace**: `%s` (Span: `%s`)\n", r.Trace.TraceID, r.Trace.SpanID))
	}
	if len(r.Artifacts) > 0 {
		sb.WriteString(fmt.Sprintf("- **Artifacts** (%d):\n", len(r.Artifacts)))
		for _, a := range r.Artifacts {
			name := a.Name
			if name == "" {
				name = a.ArtifactID
			}
			if a.ExternalURI != "" {
				sb.WriteString(fmt.Sprintf("  - [%s](%s) (%s)\n", name, a.ExternalURI, a.MediaType))
			} else {
				sb.WriteString(fmt.Sprintf("  - `%s`: %s\n", name, a.Description))
			}
		}
	}
	return sb.String()
}

// FormatTaskList formats a TaskListResult into human-readable text.
func FormatTaskList(r *TaskListResult) string {
	if r == nil || len(r.Tasks) == 0 {
		return "No tasks found.\n"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Tasks (%d total, showing %d):\n", r.TotalCount, len(r.Tasks)))
	for _, t := range r.Tasks {
		sb.WriteString(fmt.Sprintf("- `%s` [%s] executor=`%s` context=`%s`\n", t.A2ATaskID, t.State, t.ExecutorAgentID, t.ContextID))
		if t.TerminalReason != "" {
			sb.WriteString(fmt.Sprintf("  Outcome: %s\n", t.TerminalReason))
		}
	}
	return sb.String()
}

// FormatTaskCancelResult renders human-readable cancellation output.
func FormatTaskCancelResult(r *TaskCancelResult) string {
	if r == nil {
		return "Failed to cancel task.\n"
	}
	if r.AlreadyDone {
		return fmt.Sprintf("Task `%s` is already terminal (%s).\n", r.WorkID, r.State)
	}
	return fmt.Sprintf("Task `%s` canceled successfully: %s\n", r.WorkID, r.TerminalReason)
}

// FormatTaskReplyResult renders human-readable task reply output.
func FormatTaskReplyResult(r *TaskReplyResult) string {
	if r == nil {
		return "Failed to reply to task.\n"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Task `%s` updated to state [%s].\n", r.A2ATaskID, r.State))
	if r.TerminalReason != "" {
		sb.WriteString(fmt.Sprintf("- Outcome: %s\n", r.TerminalReason))
	}
	if len(r.Artifacts) > 0 {
		sb.WriteString(fmt.Sprintf("- Artifacts attached: %d\n", len(r.Artifacts)))
	}
	return sb.String()
}
