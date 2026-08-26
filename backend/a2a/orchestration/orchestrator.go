package orchestration

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"

	"github.com/tbdavid2019/888a2a/backend/a2a"
	"github.com/tbdavid2019/888a2a/backend/manager/store"
)

var (
	// ErrParentTerminal indicates delegation cannot be added to an already completed/failed/canceled parent.
	ErrParentTerminal = errors.New("cannot delegate child task from a terminal parent")
	// ErrWorkNotFound indicates the target work record was not found.
	ErrWorkNotFound = errors.New("work record not found")
)

// DelegateRequest specifies parameters for delegating a work task.
type DelegateRequest struct {
	TenantID           string
	ParentWorkID       string
	ChildWorkID        string
	ContextID          string
	RequesterAgentID   string
	ExecutorAgentID    string
	IdempotencyKey     string
	EdgeType           string
	Budget             *a2a.WorkBudget
	RequestedTokens    int64
	RequestedWorkUnits int64
	InitialState       string
	TraceID            string
	RootTraceID        string
	SpanID             string
	ParentSpanID       string
}

// OrchestratorOptions configures the Bounded Orchestrator.
type OrchestratorOptions struct {
	Store         *store.Store
	TraceRecorder *a2a.TraceRecorder
	EventManager  *a2a.EventManager
	Policy        *a2a.RuntimePolicy
}

// Orchestrator manages safe task graph creation, budget checking, cycle detection and propagation.
type Orchestrator struct {
	store         *store.Store
	traceRecorder *a2a.TraceRecorder
	eventManager  *a2a.EventManager
	cycleDetector *CycleDetector
	policy        *a2a.RuntimePolicy
	mu            sync.RWMutex
	cancelFuncs   map[string]context.CancelFunc // active task running contexts
}

// NewOrchestrator creates a new Orchestrator instance.
func NewOrchestrator(opts OrchestratorOptions) *Orchestrator {
	if opts.TraceRecorder == nil && opts.Store != nil {
		opts.TraceRecorder = a2a.NewTraceRecorder(opts.Store, opts.EventManager)
	}
	return &Orchestrator{
		store:         opts.Store,
		traceRecorder: opts.TraceRecorder,
		eventManager:  opts.EventManager,
		cycleDetector: NewCycleDetector(opts.Store),
		policy:        opts.Policy,
		cancelFuncs:   make(map[string]context.CancelFunc),
	}
}

// DelegateTask validates constraints (cycles, depth, child count, concurrency, budgets) and creates a durable work task.
func (o *Orchestrator) DelegateTask(ctx context.Context, req DelegateRequest) (*store.WorkMessage, error) {
	tenant := req.TenantID
	if tenant == "" {
		tenant = "default"
	}

	workID := req.ChildWorkID
	if workID == "" {
		workID = uuid.New().String()
	}

	contextID := req.ContextID
	if contextID == "" {
		contextID = uuid.New().String()
	}

	idempotencyKey := req.IdempotencyKey
	if idempotencyKey == "" {
		idempotencyKey = workID
	}

	edgeType := req.EdgeType
	if edgeType == "" {
		edgeType = "delegated"
	}

	var parent *store.WorkMessage
	var delegationDepth int32
	var childBudget *a2a.WorkBudget

	if req.ParentWorkID != "" {
		// 1. Cycle detection before commit
		if err := o.cycleDetector.CheckProposedEdge(ctx, tenant, req.ParentWorkID, workID); err != nil {
			o.recordPolicyLimitEvent(ctx, tenant, req.ParentWorkID, "CYCLIC_DELEGATION", err.Error())
			return nil, err
		}

		// 2. Parent state check
		if o.store != nil {
			var err error
			parent, err = o.store.GetWork(ctx, tenant, req.ParentWorkID)
			if err != nil {
				return nil, errors.Wrapf(err, "load parent work %s", req.ParentWorkID)
			}
			if isTerminalState(parent.State) {
				return nil, errors.Wrapf(ErrParentTerminal, "parent %s is already %s", req.ParentWorkID, parent.State)
			}

			delegationDepth = parent.DelegationDepth + 1

			// 3. Depth and child count limits (Task 7.2)
			if err := a2a.ValidateDelegationLimits(parent, delegationDepth); err != nil {
				o.recordPolicyLimitEvent(ctx, tenant, req.ParentWorkID, "DELEGATION_LIMIT", err.Error())
				return nil, err
			}

			// 4. Concurrency limits (Task 7.3)
			activeCount, err := o.store.CountActiveChildren(ctx, tenant, req.ParentWorkID)
			if err == nil {
				if err := a2a.ValidateConcurrencyLimit(parent, activeCount); err != nil {
					o.recordPolicyLimitEvent(ctx, tenant, req.ParentWorkID, "CONCURRENCY_LIMIT", err.Error())
					return nil, err
				}
			}

			// 5. Budget availability (Task 7.3)
			if err := a2a.ValidateBudgetAvailability(parent, req.RequestedTokens, req.RequestedWorkUnits); err != nil {
				o.recordPolicyLimitEvent(ctx, tenant, req.ParentWorkID, "BUDGET_LIMIT", err.Error())
				return nil, err
			}

			// 6. Child budget allocation bounded by parent
			childBudget = a2a.AllocateChildBudget(parent, req.Budget)
		}
	}

	if childBudget == nil {
		childBudget = a2a.EffectiveBudget(req.Budget)
	}

	initialState := req.InitialState
	if initialState == "" {
		initialState = "SUBMITTED"
	}

	now := time.Now()
	workMsg := &store.WorkMessage{
		TenantID:         tenant,
		WorkID:           workID,
		A2ATaskID:        workID,
		ContextID:        contextID,
		RequesterAgentID: req.RequesterAgentID,
		ExecutorAgentID:  req.ExecutorAgentID,
		State:            initialState,
		IdempotencyKey:   idempotencyKey,
		TraceID:          req.TraceID,
		RootTraceID:      req.RootTraceID,
		SpanID:           req.SpanID,
		ParentSpanID:     req.ParentSpanID,
		DelegationDepth:  delegationDepth,
		ParentEdgeType:   edgeType,
		MaxDepth:         childBudget.MaxDepth,
		MaxChildren:      childBudget.MaxChildren,
		MaxFanOut:        childBudget.MaxFanOut,
		MaxConcurrency:   childBudget.MaxConcurrency,
		MaxRuntimeMs:     childBudget.MaxRuntimeMs,
		MaxRetries:       childBudget.MaxRetries,
		MaxTokens:        childBudget.MaxTokens,
		MaxWorkUnits:     childBudget.MaxWorkUnits,
		CreatedAt:        now,
		UpdatedAt:        now,
		Version:          1,
	}

	if req.ParentWorkID != "" {
		workMsg.ParentWorkID = sql.NullString{String: req.ParentWorkID, Valid: true}
	}

	if o.store != nil {
		// Ensure work context exists
		if _, err := o.store.EnsureWorkContext(ctx, tenant, contextID, workID); err != nil {
			return nil, errors.Wrap(err, "ensure work context")
		}

		if err := o.store.CreateWork(ctx, workMsg); err != nil {
			return nil, errors.Wrap(err, "create child work")
		}

		// Update parent usage
		if req.ParentWorkID != "" {
			_ = o.store.UpdateWorkUsage(ctx, tenant, req.ParentWorkID, 1, 0, 0, req.RequestedTokens, req.RequestedWorkUnits)
		}
	}

	// Record DELEGATION trace event
	if o.traceRecorder != nil {
		_, _ = o.traceRecorder.Record(ctx, &a2a.TraceEvent{
			TenantID:       tenant,
			WorkID:         workID,
			TraceID:        req.TraceID,
			RootTraceID:    req.RootTraceID,
			SpanID:         req.SpanID,
			ParentSpanID:   req.ParentSpanID,
			EventType:      a2a.TraceEventDelegation,
			PolicyDecision: "ALLOWED",
			Metadata: map[string]string{
				"requester_agent_id": req.RequesterAgentID,
				"executor_agent_id":  req.ExecutorAgentID,
				"parent_work_id":     req.ParentWorkID,
				"delegation_depth":   fmt.Sprintf("%d", delegationDepth),
				"edge_type":          edgeType,
			},
		})
	}

	return workMsg, nil
}

func (o *Orchestrator) recordPolicyLimitEvent(ctx context.Context, tenantID, workID, limitType, reason string) {
	if o.traceRecorder == nil || workID == "" {
		return
	}
	_, _ = o.traceRecorder.Record(ctx, &a2a.TraceEvent{
		TenantID:       tenantID,
		WorkID:         workID,
		EventType:      a2a.TraceEventPolicyLimit,
		PolicyDecision: "DENIED",
		TerminalReason: reason,
		Metadata: map[string]string{
			"limit_type": limitType,
			"reason":     reason,
		},
	})
}

// RegisterActiveTask registers a running task cancellation function.
func (o *Orchestrator) RegisterActiveTask(workID string, cancel context.CancelFunc) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.cancelFuncs[workID] = cancel
}

// UnregisterActiveTask unregisters a finished task.
func (o *Orchestrator) UnregisterActiveTask(workID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.cancelFuncs, workID)
}

func isTerminalState(state string) bool {
	return state == "COMPLETED" || state == "FAILED" || state == "CANCELED" || state == "REJECTED"
}
