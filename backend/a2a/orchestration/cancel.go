package orchestration

import (
	"context"
	"sync"
	"time"

	a2asdk "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/pkg/errors"

	"github.com/tbdavid2019/888a2a/backend/a2a"
	"github.com/tbdavid2019/888a2a/backend/manager/store"
)

// CancellationResult summarizes the outcome of a tree cancellation.
type CancellationResult struct {
	RootWorkID         string    `json:"root_work_id"`
	CancelledWorkIDs   []string  `json:"cancelled_work_ids"`
	AlreadyTerminalIDs []string  `json:"already_terminal_ids"`
	TotalAffected      int       `json:"total_affected"`
	Timestamp          time.Time `json:"timestamp"`
}

// CancelWorkTree propagates root cancellation to all queued and running descendants,
// cancelling running processes and transitioning every active descendant to CANCELED.
func (o *Orchestrator) CancelWorkTree(ctx context.Context, tenantID, rootWorkID, reason string) (*CancellationResult, error) {
	if tenantID == "" {
		tenantID = "default"
	}
	if reason == "" {
		reason = "cancelled by operator"
	}

	result := &CancellationResult{
		RootWorkID: rootWorkID,
		Timestamp:  time.Now(),
	}

	if o.store == nil {
		return result, nil
	}

	root, err := o.store.GetWork(ctx, tenantID, rootWorkID)
	if err != nil {
		if errors.Is(err, store.ErrWorkNotFound) {
			return nil, ErrWorkNotFound
		}
		return nil, errors.Wrapf(err, "load root work %s for cancel", rootWorkID)
	}

	// 1. Gather all descendants in the task tree
	descendants, err := o.store.ListDescendants(ctx, tenantID, rootWorkID)
	if err != nil {
		return nil, errors.Wrapf(err, "list descendants for cancel %s", rootWorkID)
	}

	allNodes := append([]*store.WorkMessage{root}, descendants...)

	var cancelWg sync.WaitGroup
	var mu sync.Mutex

	for _, node := range allNodes {
		wID := node.WorkID
		wVer := node.Version
		wState := node.State

		// Trigger running runtime cancellation if registered
		o.mu.RLock()
		cancelFn, hasCancel := o.cancelFuncs[wID]
		o.mu.RUnlock()
		if hasCancel && cancelFn != nil {
			cancelFn()
		}

		if isTerminalState(wState) {
			mu.Lock()
			result.AlreadyTerminalIDs = append(result.AlreadyTerminalIDs, wID)
			mu.Unlock()
			continue
		}

		cancelWg.Add(1)
		go func(workID string, version uint64) {
			defer cancelWg.Done()

			// Transition state to CANCELED in store
			_, updateErr := o.store.UpdateWorkState(ctx, tenantID, workID, version, "CANCELED", reason)
			if updateErr != nil && !errors.Is(updateErr, store.ErrWorkVersionMismatch) {
				// Retry with fresh version if mismatch
				if current, getErr := o.store.GetWork(ctx, tenantID, workID); getErr == nil && !isTerminalState(current.State) {
					_, _ = o.store.UpdateWorkState(ctx, tenantID, workID, current.Version, "CANCELED", reason)
				}
			}

			// Record CANCELLATION trace event
			if o.traceRecorder != nil {
				_, _ = o.traceRecorder.Record(ctx, &a2a.TraceEvent{
					TenantID:       tenantID,
					WorkID:         workID,
					EventType:      a2a.TraceEventCancellation,
					PolicyDecision: "ALLOWED",
					TerminalReason: reason,
					Metadata: map[string]string{
						"cancelled_by": "root_propagation",
						"root_work_id": rootWorkID,
						"reason":       reason,
					},
				})
			}

			mu.Lock()
			result.CancelledWorkIDs = append(result.CancelledWorkIDs, workID)
			mu.Unlock()
		}(wID, wVer)
	}

	cancelWg.Wait()
	result.TotalAffected = len(result.CancelledWorkIDs) + len(result.AlreadyTerminalIDs)

	// Publish terminal status via eventManager if available
	if o.eventManager != nil {
		for _, wID := range result.CancelledWorkIDs {
			now := time.Now()
			ev := &a2asdk.TaskStatusUpdateEvent{
				TaskID: a2asdk.TaskID(wID),
				Status: a2asdk.TaskStatus{
					State:     a2asdk.TaskStateCanceled,
					Timestamp: &now,
					Message: a2asdk.NewMessage(
						a2asdk.MessageRoleAgent,
						a2asdk.NewTextPart(reason),
					),
				},
			}
			o.eventManager.Publish(tenantID, wID, ev, 0)
		}
	}

	return result, nil
}

// EnsureTerminalState verifies that every descendant in the tree is in an observable terminal state.
func (o *Orchestrator) EnsureTerminalState(ctx context.Context, tenantID, rootWorkID string) (bool, error) {
	if o.store == nil {
		return true, nil
	}
	if tenantID == "" {
		tenantID = "default"
	}

	root, err := o.store.GetWork(ctx, tenantID, rootWorkID)
	if err != nil {
		return false, err
	}
	if !isTerminalState(root.State) {
		return false, errors.Errorf("root %s is in non-terminal state %s", rootWorkID, root.State)
	}

	descendants, err := o.store.ListDescendants(ctx, tenantID, rootWorkID)
	if err != nil {
		return false, err
	}
	for _, d := range descendants {
		if !isTerminalState(d.State) {
			return false, errors.Errorf("descendant %s is in non-terminal state %s", d.WorkID, d.State)
		}
	}
	return true, nil
}
