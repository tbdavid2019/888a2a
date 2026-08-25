package orchestration

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"

	"github.com/Ranxy/laelia/backend/a2a"
	"github.com/Ranxy/laelia/backend/manager/store"
)

// JoinPolicy defines the success criteria for aggregating fan-out tasks.
type JoinPolicy string

const (
	JoinPolicyAllSuccess     JoinPolicy = "ALL_SUCCESS"
	JoinPolicyPartialFailure JoinPolicy = "PARTIAL_FAILURE"
	JoinPolicyQuorum         JoinPolicy = "QUORUM"
	JoinPolicyFirstSuccess   JoinPolicy = "FIRST_SUCCESS"
)

// FanOutTaskSpec defines a single child task in a fan-out batch.
type FanOutTaskSpec struct {
	TaskID             string
	ExecutorAgentID    string
	Input              string
	Budget             *a2a.WorkBudget
	RequestedTokens    int64
	RequestedWorkUnits int64
	Executor           func(ctx context.Context, work *store.WorkMessage) (*TaskOutput, error)
}

// TaskOutput represents execution results produced by a fan-out worker.
type TaskOutput struct {
	Output         string
	Artifacts      []*store.WorkArtifactMessage
	TokensUsed     int64
	WorkUnitsUsed  int64
	TerminalReason string
}

// TaskResult contains the deterministic outcome for one task in the fan-out array.
type TaskResult struct {
	Index           int                          `json:"index"`
	TaskID          string                       `json:"task_id"`
	ExecutorAgentID string                       `json:"executor_agent_id"`
	State           string                       `json:"state"`
	Output          string                       `json:"output"`
	Artifacts       []*store.WorkArtifactMessage `json:"artifacts,omitempty"`
	Error           string                       `json:"error,omitempty"`
	TokensUsed      int64                        `json:"tokens_used"`
	WorkUnitsUsed   int64                        `json:"work_units_used"`
	Duration        time.Duration                `json:"duration"`
}

// FanOutRequest specifies a parallel execution request across multiple peer agents.
type FanOutRequest struct {
	TenantID         string
	ParentWorkID     string
	RequesterAgentID string
	Tasks            []FanOutTaskSpec
	Policy           JoinPolicy
	MinSuccessCount  int
	Timeout          time.Duration
	MaxConcurrency   int
}

// JoinResult contains the aggregated deterministic outcome of a fan-out execution.
type JoinResult struct {
	ParentWorkID       string        `json:"parent_work_id"`
	Policy             JoinPolicy    `json:"policy"`
	Success            bool          `json:"success"`
	TotalTasks         int           `json:"total_tasks"`
	CompletedCount     int           `json:"completed_count"`
	FailedCount        int           `json:"failed_count"`
	CanceledCount      int           `json:"canceled_count"`
	TaskResults        []TaskResult  `json:"task_results"`
	TotalTokensUsed    int64         `json:"total_tokens_used"`
	TotalWorkUnitsUsed int64         `json:"total_work_units_used"`
	Duration           time.Duration `json:"duration"`
	Error              string        `json:"error,omitempty"`
}

// ExecuteFanOut coordinates parallel task execution, enforces limits, bounds concurrency,
// and returns deterministically ordered results aligned with the input task array.
func (o *Orchestrator) ExecuteFanOut(ctx context.Context, req FanOutRequest) (*JoinResult, error) {
	if len(req.Tasks) == 0 {
		return nil, errors.New("fan-out tasks cannot be empty")
	}

	tenant := req.TenantID
	if tenant == "" {
		tenant = "default"
	}

	policy := req.Policy
	if policy == "" {
		policy = JoinPolicyAllSuccess
	}

	startTime := time.Now()
	totalTasks := len(req.Tasks)

	var parent *store.WorkMessage
	if o.store != nil && req.ParentWorkID != "" {
		var err error
		parent, err = o.store.GetWork(ctx, tenant, req.ParentWorkID)
		if err != nil {
			return nil, errors.Wrapf(err, "load parent work %s for fan-out", req.ParentWorkID)
		}
		if isTerminalState(parent.State) {
			return nil, errors.Wrapf(ErrParentTerminal, "parent %s is already terminal (%s)", req.ParentWorkID, parent.State)
		}

		// Validate Fan-out limit (Task 7.2)
		if err := a2a.ValidateFanOutLimit(parent, totalTasks); err != nil {
			o.recordPolicyLimitEvent(ctx, tenant, req.ParentWorkID, "FAN_OUT_LIMIT", err.Error())
			return nil, err
		}
	}

	// Concurrency bound
	concurrency := req.MaxConcurrency
	if concurrency <= 0 {
		if parent != nil && parent.MaxConcurrency > 0 {
			concurrency = int(parent.MaxConcurrency)
		} else {
			concurrency = int(a2a.DefaultMaxConcurrency)
		}
	}
	if concurrency > totalTasks {
		concurrency = totalTasks
	}

	// Timeout context
	runCtx := ctx
	var cancel context.CancelFunc
	if req.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, req.Timeout)
	} else {
		runCtx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	// Deterministic pre-allocated results array preserving exact input order
	results := make([]TaskResult, totalTasks)
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i, spec := range req.Tasks {
		taskIndex := i
		taskSpec := spec

		tID := taskSpec.TaskID
		if tID == "" {
			tID = uuid.New().String()
		}

		results[taskIndex] = TaskResult{
			Index:           taskIndex,
			TaskID:          tID,
			ExecutorAgentID: taskSpec.ExecutorAgentID,
			State:           "SUBMITTED",
		}

		wg.Go(func() {
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-runCtx.Done():
				results[taskIndex].State = "CANCELED"
				results[taskIndex].Error = "canceled before scheduling: " + runCtx.Err().Error()
				return
			}

			childStart := time.Now()

			// Create durable child task
			childWork, delErr := o.DelegateTask(runCtx, DelegateRequest{
				TenantID:           tenant,
				ParentWorkID:       req.ParentWorkID,
				ChildWorkID:        tID,
				ContextID:          tID,
				RequesterAgentID:   req.RequesterAgentID,
				ExecutorAgentID:    taskSpec.ExecutorAgentID,
				EdgeType:           "fan_out",
				Budget:             taskSpec.Budget,
				RequestedTokens:    taskSpec.RequestedTokens,
				RequestedWorkUnits: taskSpec.RequestedWorkUnits,
				InitialState:       "WORKING",
			})
			if delErr != nil {
				results[taskIndex].State = "FAILED"
				results[taskIndex].Error = delErr.Error()
				results[taskIndex].Duration = time.Since(childStart)
				return
			}

			// Register active cancellation func
			taskCtx, taskCancel := context.WithCancel(runCtx)
			defer taskCancel()
			o.RegisterActiveTask(tID, taskCancel)
			defer o.UnregisterActiveTask(tID)

			var output *TaskOutput
			var execErr error

			if taskSpec.Executor != nil {
				output, execErr = taskSpec.Executor(taskCtx, childWork)
			} else {
				// Default mock/noop execution if no executor provided
				output = &TaskOutput{
					Output: fmt.Sprintf("Result from %s", taskSpec.ExecutorAgentID),
				}
			}

			childDuration := time.Since(childStart)
			results[taskIndex].Duration = childDuration

			if errors.Is(taskCtx.Err(), context.Canceled) || errors.Is(taskCtx.Err(), context.DeadlineExceeded) {
				results[taskIndex].State = "CANCELED"
				results[taskIndex].Error = "timed out or canceled"
				if o.store != nil {
					_, _ = o.store.UpdateWorkState(ctx, tenant, tID, childWork.Version, "CANCELED", "fan-out timeout or cancel")
				}
				return
			}

			if execErr != nil {
				results[taskIndex].State = "FAILED"
				results[taskIndex].Error = execErr.Error()
				if o.store != nil {
					_, _ = o.store.UpdateWorkState(ctx, tenant, tID, childWork.Version, "FAILED", execErr.Error())
				}
				return
			}

			results[taskIndex].State = "COMPLETED"
			if output != nil {
				results[taskIndex].Output = output.Output
				results[taskIndex].Artifacts = output.Artifacts
				results[taskIndex].TokensUsed = output.TokensUsed
				results[taskIndex].WorkUnitsUsed = output.WorkUnitsUsed

				if o.store != nil {
					_, _ = o.store.UpdateWorkState(ctx, tenant, tID, childWork.Version, "COMPLETED", output.TerminalReason)
					for _, art := range output.Artifacts {
						_ = o.store.CreateWorkArtifact(ctx, art)
					}
					_ = o.store.UpdateWorkUsage(ctx, tenant, tID, 0, 0, childDuration.Milliseconds(), output.TokensUsed, output.WorkUnitsUsed)
				}
			}
		})
	}

	wg.Wait()

	totalDuration := time.Since(startTime)
	joinRes := &JoinResult{
		ParentWorkID: req.ParentWorkID,
		Policy:       policy,
		TotalTasks:   totalTasks,
		TaskResults:  results,
		Duration:     totalDuration,
	}

	for _, r := range results {
		joinRes.TotalTokensUsed += r.TokensUsed
		joinRes.TotalWorkUnitsUsed += r.WorkUnitsUsed
		switch r.State {
		case "COMPLETED":
			joinRes.CompletedCount++
		case "FAILED":
			joinRes.FailedCount++
		case "CANCELED":
			joinRes.CanceledCount++
		default:
			joinRes.FailedCount++
		}
	}

	// Evaluate Join Policy
	switch policy {
	case JoinPolicyAllSuccess:
		joinRes.Success = (joinRes.CompletedCount == totalTasks)
		if !joinRes.Success {
			joinRes.Error = fmt.Sprintf("all-success policy failed: %d/%d completed", joinRes.CompletedCount, totalTasks)
		}
	case JoinPolicyPartialFailure:
		minSuccess := req.MinSuccessCount
		if minSuccess <= 0 {
			minSuccess = 1
		}
		joinRes.Success = (joinRes.CompletedCount >= minSuccess)
		if !joinRes.Success {
			joinRes.Error = fmt.Sprintf("partial-failure policy failed: %d completed, need %d", joinRes.CompletedCount, minSuccess)
		}
	case JoinPolicyQuorum:
		minSuccess := req.MinSuccessCount
		if minSuccess <= 0 {
			minSuccess = (totalTasks / 2) + 1
		}
		joinRes.Success = (joinRes.CompletedCount >= minSuccess)
		if !joinRes.Success {
			joinRes.Error = fmt.Sprintf("quorum policy failed: %d completed, need %d", joinRes.CompletedCount, minSuccess)
		}
	case JoinPolicyFirstSuccess:
		joinRes.Success = (joinRes.CompletedCount >= 1)
		if !joinRes.Success {
			joinRes.Error = "first-success policy failed: no tasks completed"
		}
	default:
		joinRes.Error = fmt.Sprintf("unsupported join policy %q", policy)
	}

	// Update parent usage with aggregated tokens, work units, and fan-out width
	if o.store != nil && req.ParentWorkID != "" {
		_ = o.store.UpdateWorkUsage(
			ctx,
			tenant,
			req.ParentWorkID,
			0,
			int32(totalTasks),
			totalDuration.Milliseconds(),
			joinRes.TotalTokensUsed,
			joinRes.TotalWorkUnitsUsed,
		)
	}

	// Record audit trace event for Join outcome
	if o.traceRecorder != nil && req.ParentWorkID != "" {
		decision := "ALLOWED"
		if !joinRes.Success {
			decision = "PARTIAL_OR_FAILED"
		}
		_, _ = o.traceRecorder.Record(ctx, &a2a.TraceEvent{
			TenantID:       tenant,
			WorkID:         req.ParentWorkID,
			EventType:      a2a.TraceEventTerminalOutcome,
			PolicyDecision: decision,
			TerminalReason: joinRes.Error,
			Metadata: map[string]string{
				"join_policy":     string(policy),
				"total_tasks":     fmt.Sprintf("%d", totalTasks),
				"completed_count": fmt.Sprintf("%d", joinRes.CompletedCount),
				"failed_count":    fmt.Sprintf("%d", joinRes.FailedCount),
				"canceled_count":  fmt.Sprintf("%d", joinRes.CanceledCount),
				"success":         fmt.Sprintf("%t", joinRes.Success),
			},
		})
	}

	return joinRes, nil
}
