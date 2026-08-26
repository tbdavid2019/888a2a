package a2a

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"

	"github.com/tbdavid2019/888a2a/backend/manager/store"
)

// RecoveryStore defines the persistence operations required for restart recovery.
type RecoveryStore interface {
	ListPendingWorkForRecovery(ctx context.Context) ([]*store.WorkMessage, error)
	UpdateWorkState(ctx context.Context, tenantID, workID string, expectedVersion uint64, newState string, terminalReason string) (uint64, error)
	AppendWorkEvent(ctx context.Context, event *store.WorkEventMessage) error
	GetLatestWorkEventSequence(ctx context.Context, tenantID, workID string) (uint64, error)
}

// RecoveryReport summarizes the outcome of active work recovery on Manager startup.
type RecoveryReport struct {
	TotalScanned int      `json:"totalScanned"`
	Recovered    int      `json:"recovered"`
	Skipped      int      `json:"skipped"`
	WorkIDs      []string `json:"workIds"`
}

// RecoveryService handles Manager restart recovery for pending and in-flight A2A work.
type RecoveryService struct {
	store RecoveryStore
}

// NewRecoveryService creates a new work recovery service.
func NewRecoveryService(store RecoveryStore) *RecoveryService {
	return &RecoveryService{store: store}
}

// RecoverPendingWork finds all work in SUBMITTED or WORKING state and ensures they are in a safe recoverable state.
func (rs *RecoveryService) RecoverPendingWork(ctx context.Context) (*RecoveryReport, error) {
	if rs.store == nil {
		return nil, errors.New("recovery store is required")
	}

	pendingWorks, err := rs.store.ListPendingWorkForRecovery(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "list pending work for recovery")
	}

	report := &RecoveryReport{
		TotalScanned: len(pendingWorks),
		WorkIDs:      make([]string, 0, len(pendingWorks)),
	}

	for _, w := range pendingWorks {
		reason := "manager restarted; task marked for resume"
		// Transition to SUBMITTED if it was WORKING, or keep SUBMITTED
		targetState := "SUBMITTED"

		newVer, err := rs.store.UpdateWorkState(ctx, w.TenantID, w.WorkID, w.Version, targetState, reason)
		if err != nil {
			// Version mismatch or already completed concurrently -> skip safely
			report.Skipped++
			continue
		}

		// Append recovery event to durable log
		seq, _ := rs.store.GetLatestWorkEventSequence(ctx, w.TenantID, w.WorkID)
		event := &store.WorkEventMessage{
			TenantID:       w.TenantID,
			EventID:        uuid.New().String(),
			WorkID:         w.WorkID,
			Sequence:       seq + 1,
			EventType:      "RECOVERED",
			TerminalReason: reason,
			CreatedAt:      time.Now(),
			Metadata: map[string]string{
				"context_id":       w.ContextID,
				"previous_state":   w.State,
				"previous_version": string(rune(w.Version)),
			},
		}
		_ = rs.store.AppendWorkEvent(ctx, event)

		report.Recovered++
		report.WorkIDs = append(report.WorkIDs, w.WorkID)
		_ = newVer
	}

	return report, nil
}
