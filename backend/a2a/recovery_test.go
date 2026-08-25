package a2a

import (
	"context"
	"testing"
	"time"

	"github.com/Ranxy/laelia/backend/manager/store"
)

func TestRecoveryService_RecoverPendingWork(t *testing.T) {
	ctx := context.Background()
	memStore := newMemoryWorkStore()

	// 1. Task in SUBMITTED state (pending execution before crash)
	_ = memStore.CreateWork(ctx, &store.WorkMessage{
		TenantID:         "tenant-1",
		WorkID:           "work-pending-1",
		A2ATaskID:        "work-pending-1",
		ContextID:        "ctx-1",
		RequesterAgentID: "agent-1",
		ExecutorAgentID:  "agent-2",
		State:            "SUBMITTED",
		IdempotencyKey:   "idem-1",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
		Version:          1,
	})

	// 2. Task in WORKING state (interrupted by manager restart)
	_ = memStore.CreateWork(ctx, &store.WorkMessage{
		TenantID:         "tenant-1",
		WorkID:           "work-working-2",
		A2ATaskID:        "work-working-2",
		ContextID:        "ctx-2",
		RequesterAgentID: "agent-1",
		ExecutorAgentID:  "agent-3",
		State:            "WORKING",
		IdempotencyKey:   "idem-2",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
		Version:          1,
	})

	// 3. Task in COMPLETED state (should NOT be recovered)
	_ = memStore.CreateWork(ctx, &store.WorkMessage{
		TenantID:         "tenant-1",
		WorkID:           "work-done-3",
		A2ATaskID:        "work-done-3",
		ContextID:        "ctx-3",
		RequesterAgentID: "agent-1",
		ExecutorAgentID:  "agent-2",
		State:            "COMPLETED",
		IdempotencyKey:   "idem-3",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
		Version:          2,
	})

	recoverySvc := NewRecoveryService(memStore)
	report, err := recoverySvc.RecoverPendingWork(ctx)
	if err != nil {
		t.Fatalf("RecoverPendingWork failed: %v", err)
	}

	if report.TotalScanned != 2 {
		t.Errorf("expected 2 pending tasks scanned, got %d", report.TotalScanned)
	}
	if report.Recovered != 2 {
		t.Errorf("expected 2 tasks recovered, got %d", report.Recovered)
	}

	// Verify working task transitioned to SUBMITTED and recorded RECOVERED event
	recoveredWork, err := memStore.GetWork(ctx, "tenant-1", "work-working-2")
	if err != nil {
		t.Fatalf("GetWork failed: %v", err)
	}
	if recoveredWork.State != "SUBMITTED" {
		t.Errorf("expected state SUBMITTED, got %s", recoveredWork.State)
	}
	if recoveredWork.Version != 2 {
		t.Errorf("expected version 2 after recovery, got %d", recoveredWork.Version)
	}

	// Check recovery event log
	events, err := memStore.ListWorkEvents(ctx, "tenant-1", "work-working-2", 0, 10)
	if err != nil {
		t.Fatalf("ListWorkEvents failed: %v", err)
	}
	if len(events) != 1 || events[0].EventType != "RECOVERED" {
		t.Fatalf("expected RECOVERED event, got %v", events)
	}
}
