package tools

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/google/uuid"
	"github.com/pkg/errors"

	a2apkg "github.com/Ranxy/laelia/backend/a2a"
	"github.com/Ranxy/laelia/backend/manager/store"
)

// WorkBudgetInput specifies optional resource and fanout bounds for delegated work.
type WorkBudgetInput struct {
	MaxDepth       int32 `json:"maxDepth,omitempty"`
	MaxChildren    int32 `json:"maxChildren,omitempty"`
	MaxFanOut      int32 `json:"maxFanOut,omitempty"`
	MaxConcurrency int32 `json:"maxConcurrency,omitempty"`
	MaxRuntimeMs   int64 `json:"maxRuntimeMs,omitempty"`
	MaxRetries     int32 `json:"maxRetries,omitempty"`
	MaxTokens      int64 `json:"maxTokens,omitempty"`
	MaxWorkUnits   int64 `json:"maxWorkUnits,omitempty"`
}

// TraceCorrelationInput specifies audit-safe distributed trace correlation identifiers.
type TraceCorrelationInput struct {
	TraceID      string `json:"traceId,omitempty"`
	RootTraceID  string `json:"rootTraceId,omitempty"`
	SpanID       string `json:"spanId,omitempty"`
	ParentSpanID string `json:"parentSpanId,omitempty"`
}

// TaskSendInput defines parameters for sending an A2A task to a peer agent.
type TaskSendInput struct {
	TenantID             string                 `json:"tenant,omitempty"`
	RequesterAgentID     string                 `json:"requesterAgentId"`
	TargetAgentID        string                 `json:"targetAgentId"`
	Message              string                 `json:"message"`
	ContextID            string                 `json:"contextId,omitempty"`
	ParentWorkID         string                 `json:"parentWorkId,omitempty"`
	ParentEdgeType       string                 `json:"parentEdgeType,omitempty"`
	IdempotencyKey       string                 `json:"idempotencyKey,omitempty"`
	Budget               *WorkBudgetInput       `json:"budget,omitempty"`
	Trace                *TraceCorrelationInput `json:"trace,omitempty"`
	SourceConversationID string                 `json:"sourceConversationId,omitempty"`
	SourceTaskID         string                 `json:"sourceTaskId,omitempty"`
}

// TaskSendResult contains the created or existing work record information.
type TaskSendResult struct {
	TenantID         string                 `json:"tenant"`
	WorkID           string                 `json:"workId"`
	A2ATaskID        string                 `json:"a2aTaskId"`
	ContextID        string                 `json:"contextId"`
	State            string                 `json:"state"`
	RequesterAgentID string                 `json:"requesterAgentId"`
	ExecutorAgentID  string                 `json:"executorAgentId"`
	ParentWorkID     string                 `json:"parentWorkId,omitempty"`
	IdempotencyKey   string                 `json:"idempotencyKey"`
	Trace            *TraceCorrelationInput `json:"trace,omitempty"`
	Budget           *WorkBudgetInput       `json:"budget,omitempty"`
	CreatedAt        time.Time              `json:"createdAt"`
	IsDuplicate      bool                   `json:"isDuplicate"`
}

// TaskSend creates a durable A2A work task idempotently and publishes the initial event to wake up the target.
func TaskSend(ctx context.Context, s WorkStore, em *a2apkg.EventManager, in TaskSendInput) (*TaskSendResult, error) {
	if s == nil {
		return nil, errors.New("store is required")
	}

	tenant := strings.TrimSpace(in.TenantID)
	if tenant == "" {
		tenant = "default"
	}

	requester := strings.TrimSpace(in.RequesterAgentID)
	if requester == "" {
		return nil, errors.New("requesterAgentId is required")
	}

	target := strings.TrimSpace(in.TargetAgentID)
	if target == "" {
		return nil, errors.New("targetAgentId is required")
	}

	idempotencyKey := strings.TrimSpace(in.IdempotencyKey)
	if idempotencyKey == "" {
		idempotencyKey = uuid.New().String()
	}

	// Idempotency check: if already submitted with this key, return existing work
	existing, err := s.GetWorkByIdempotencyKey(ctx, tenant, requester, idempotencyKey)
	if err == nil && existing != nil {
		return &TaskSendResult{
			TenantID:         existing.TenantID,
			WorkID:           existing.WorkID,
			A2ATaskID:        existing.A2ATaskID,
			ContextID:        existing.ContextID,
			State:            existing.State,
			RequesterAgentID: existing.RequesterAgentID,
			ExecutorAgentID:  existing.ExecutorAgentID,
			ParentWorkID:     existing.ParentWorkID.String,
			IdempotencyKey:   existing.IdempotencyKey,
			Trace: &TraceCorrelationInput{
				TraceID:      existing.TraceID,
				RootTraceID:  existing.RootTraceID,
				SpanID:       existing.SpanID,
				ParentSpanID: existing.ParentSpanID,
			},
			Budget: &WorkBudgetInput{
				MaxDepth:       existing.MaxDepth,
				MaxChildren:    existing.MaxChildren,
				MaxFanOut:      existing.MaxFanOut,
				MaxConcurrency: existing.MaxConcurrency,
				MaxRuntimeMs:   existing.MaxRuntimeMs,
				MaxRetries:     existing.MaxRetries,
				MaxTokens:      existing.MaxTokens,
				MaxWorkUnits:   existing.MaxWorkUnits,
			},
			CreatedAt:   existing.CreatedAt,
			IsDuplicate: true,
		}, nil
	}

	workID := uuid.New().String()
	contextID := strings.TrimSpace(in.ContextID)
	if contextID == "" {
		contextID = uuid.New().String()
	}

	// Ensure work context exists
	if _, err := s.EnsureWorkContext(ctx, tenant, contextID, workID); err != nil {
		return nil, errors.Wrap(err, "ensure work context in task send")
	}

	var convUUID *uuid.UUID
	if in.SourceConversationID != "" {
		if parsed, err := uuid.Parse(in.SourceConversationID); err == nil {
			convUUID = &parsed
		}
	}

	var taskUUID *uuid.UUID
	if in.SourceTaskID != "" {
		if parsed, err := uuid.Parse(in.SourceTaskID); err == nil {
			taskUUID = &parsed
		}
	}

	var parentWorkNull sql.NullString
	delegationDepth := int32(0)
	if in.ParentWorkID != "" {
		parentWorkNull = sql.NullString{String: in.ParentWorkID, Valid: true}
		parentWork, err := s.GetWork(ctx, tenant, in.ParentWorkID)
		if err == nil && parentWork != nil {
			delegationDepth = parentWork.DelegationDepth + 1
		} else {
			delegationDepth = 1
		}
	}

	edgeType := in.ParentEdgeType
	if edgeType == "" {
		edgeType = "delegated"
	}

	workMsg := &store.WorkMessage{
		TenantID:             tenant,
		WorkID:               workID,
		A2ATaskID:            workID,
		ContextID:            contextID,
		RequesterAgentID:     requester,
		ExecutorAgentID:      target,
		SourceConversationID: convUUID,
		SourceTaskID:         taskUUID,
		State:                "SUBMITTED",
		IdempotencyKey:       idempotencyKey,
		ParentWorkID:         parentWorkNull,
		ParentEdgeType:       edgeType,
		DelegationDepth:      delegationDepth,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
		Version:              1,
	}

	if in.Budget != nil {
		workMsg.MaxDepth = in.Budget.MaxDepth
		workMsg.MaxChildren = in.Budget.MaxChildren
		workMsg.MaxFanOut = in.Budget.MaxFanOut
		workMsg.MaxConcurrency = in.Budget.MaxConcurrency
		workMsg.MaxRuntimeMs = in.Budget.MaxRuntimeMs
		workMsg.MaxRetries = in.Budget.MaxRetries
		workMsg.MaxTokens = in.Budget.MaxTokens
		workMsg.MaxWorkUnits = in.Budget.MaxWorkUnits
	}

	if in.Trace != nil {
		workMsg.TraceID = in.Trace.TraceID
		workMsg.RootTraceID = in.Trace.RootTraceID
		workMsg.SpanID = in.Trace.SpanID
		workMsg.ParentSpanID = in.Trace.ParentSpanID
	}

	if err := s.CreateWork(ctx, workMsg); err != nil {
		if errors.Is(err, store.ErrWorkAlreadyExists) {
			// Race condition check: retrieve existing
			existing, getErr := s.GetWorkByIdempotencyKey(ctx, tenant, requester, idempotencyKey)
			if getErr == nil && existing != nil {
				return &TaskSendResult{
					TenantID:         existing.TenantID,
					WorkID:           existing.WorkID,
					A2ATaskID:        existing.A2ATaskID,
					ContextID:        existing.ContextID,
					State:            existing.State,
					RequesterAgentID: existing.RequesterAgentID,
					ExecutorAgentID:  existing.ExecutorAgentID,
					ParentWorkID:     existing.ParentWorkID.String,
					IdempotencyKey:   existing.IdempotencyKey,
					CreatedAt:        existing.CreatedAt,
					IsDuplicate:      true,
				}, nil
			}
		}
		return nil, errors.Wrap(err, "create work record in task send")
	}

	// Persist initial SUBMITTED work event
	initialEvent := &store.WorkEventMessage{
		TenantID:     tenant,
		EventID:      uuid.New().String(),
		WorkID:       workID,
		Sequence:     1,
		EventType:    "SUBMITTED",
		TraceID:      workMsg.TraceID,
		RootTraceID:  workMsg.RootTraceID,
		SpanID:       workMsg.SpanID,
		ParentSpanID: workMsg.ParentSpanID,
		CreatedAt:    time.Now(),
		Metadata: map[string]string{
			"context_id":  contextID,
			"requester":   requester,
			"target":      target,
			"message":     in.Message,
			"parent_work": in.ParentWorkID,
		},
	}
	_ = s.AppendWorkEvent(ctx, initialEvent)

	// Publish event to wake up target agent without polling
	if em != nil {
		a2aTask := &a2a.Task{
			ID:        a2a.TaskID(workID),
			ContextID: contextID,
			Status: a2a.TaskStatus{
				State: a2a.TaskStateSubmitted,
			},
		}
		if in.Message != "" {
			a2aTask.Status.Message = a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart(in.Message))
		}
		em.Publish(tenant, workID, a2aTask, 1)
	}

	return &TaskSendResult{
		TenantID:         tenant,
		WorkID:           workID,
		A2ATaskID:        workID,
		ContextID:        contextID,
		State:            "SUBMITTED",
		RequesterAgentID: requester,
		ExecutorAgentID:  target,
		ParentWorkID:     in.ParentWorkID,
		IdempotencyKey:   idempotencyKey,
		Trace:            in.Trace,
		Budget:           in.Budget,
		CreatedAt:        workMsg.CreatedAt,
		IsDuplicate:      false,
	}, nil
}

// FormatTaskSendResult renders human-readable output for TaskSend.
func FormatTaskSendResult(res *TaskSendResult) string {
	if res == nil {
		return "Failed to send task.\n"
	}

	var sb strings.Builder
	if res.IsDuplicate {
		sb.WriteString("**A2A Task Already Exists** (idempotent replay):\n")
	} else {
		sb.WriteString("**A2A Task Sent Successfully**:\n")
	}

	fmt.Fprintf(&sb, "- **Task ID**: `%s` (Work ID: `%s`)\n", res.A2ATaskID, res.WorkID)
	fmt.Fprintf(&sb, "- **Context ID**: `%s`\n", res.ContextID)
	fmt.Fprintf(&sb, "- **Target Executor**: `%s`\n", res.ExecutorAgentID)
	fmt.Fprintf(&sb, "- **Requester**: `%s`\n", res.RequesterAgentID)
	fmt.Fprintf(&sb, "- **State**: `%s`\n", res.State)
	fmt.Fprintf(&sb, "- **Idempotency Key**: `%s`\n", res.IdempotencyKey)

	if res.ParentWorkID != "" {
		fmt.Fprintf(&sb, "- **Parent Task**: `%s`\n", res.ParentWorkID)
	}
	if res.Trace != nil && res.Trace.TraceID != "" {
		fmt.Fprintf(&sb, "- **Trace ID**: `%s` (Span: `%s`)\n", res.Trace.TraceID, res.Trace.SpanID)
	}
	if res.Budget != nil {
		fmt.Fprintf(&sb, "- **Budget**: MaxDepth=%d, MaxChildren=%d, MaxFanOut=%d, MaxRuntimeMs=%d, MaxTokens=%d\n",
			res.Budget.MaxDepth, res.Budget.MaxChildren, res.Budget.MaxFanOut, res.Budget.MaxRuntimeMs, res.Budget.MaxTokens)
	}

	sb.WriteString("\nTarget wake-up is asynchronous. Do NOT poll or block waiting on peer process.\n")
	return sb.String()
}
