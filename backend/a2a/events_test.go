package a2a

import (
	"context"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"

	"github.com/Ranxy/laelia/backend/manager/store"
)

func TestEventManager_ReplayAndLiveSubscription(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	memStore := newMemoryWorkStore()
	em := NewEventManager(memStore)

	tenantID := "tenant-1"
	workID := "work-stream-1"

	// Pre-populate historical durable events: sequences 1 and 2
	_ = memStore.AppendWorkEvent(ctx, &store.WorkEventMessage{
		TenantID:  tenantID,
		EventID:   "evt-1",
		WorkID:    workID,
		Sequence:  1,
		EventType: "SUBMITTED",
		CreatedAt: time.Now(),
		Metadata: map[string]string{
			"context_id": "ctx-1",
		},
	})
	_ = memStore.AppendWorkEvent(ctx, &store.WorkEventMessage{
		TenantID:  tenantID,
		EventID:   "evt-2",
		WorkID:    workID,
		Sequence:  2,
		EventType: "WORKING",
		CreatedAt: time.Now(),
		Metadata: map[string]string{
			"context_id": "ctx-1",
		},
	})

	// Client subscribes from sequence 0 (receives historical events 1 and 2, then waits for live event)
	seq := em.Subscribe(ctx, tenantID, workID, 0)

	receivedEvents := make(chan a2a.Event, 10)
	go func() {
		for event, err := range seq {
			if err != nil {
				return
			}
			if event != nil {
				receivedEvents <- event
			}
		}
	}()

	// Read sequence 1 (SUBMITTED)
	select {
	case e1 := <-receivedEvents:
		st, ok := e1.(*a2a.TaskStatusUpdateEvent)
		if !ok || st.Status.State != a2a.TaskStateSubmitted {
			t.Fatalf("expected event 1 SUBMITTED, got %v", e1)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for event 1")
	}

	// Read sequence 2 (WORKING)
	select {
	case e2 := <-receivedEvents:
		st, ok := e2.(*a2a.TaskStatusUpdateEvent)
		if !ok || st.Status.State != a2a.TaskStateWorking {
			t.Fatalf("expected event 2 WORKING, got %v", e2)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for event 2")
	}

	// Publish live event: sequence 3 (COMPLETED)
	info := &a2a.TaskInfo{
		TaskID:    a2a.TaskID(workID),
		ContextID: "ctx-1",
	}
	liveEvent := a2a.NewStatusUpdateEvent(info, a2a.TaskStateCompleted, nil)
	em.Publish(tenantID, workID, liveEvent, 3)

	// Read live event sequence 3
	select {
	case e3 := <-receivedEvents:
		st, ok := e3.(*a2a.TaskStatusUpdateEvent)
		if !ok || st.Status.State != a2a.TaskStateCompleted {
			t.Fatalf("expected event 3 COMPLETED, got %v", e3)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for live event 3")
	}
}

func TestEventManager_ReconnectResumeFromSequence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	memStore := newMemoryWorkStore()
	em := NewEventManager(memStore)

	tenantID := "tenant-1"
	workID := "work-stream-2"

	_ = memStore.AppendWorkEvent(ctx, &store.WorkEventMessage{
		TenantID:  tenantID,
		EventID:   "evt-1",
		WorkID:    workID,
		Sequence:  1,
		EventType: "SUBMITTED",
		CreatedAt: time.Now(),
	})
	_ = memStore.AppendWorkEvent(ctx, &store.WorkEventMessage{
		TenantID:  tenantID,
		EventID:   "evt-2",
		WorkID:    workID,
		Sequence:  2,
		EventType: "WORKING",
		CreatedAt: time.Now(),
	})
	_ = memStore.AppendWorkEvent(ctx, &store.WorkEventMessage{
		TenantID:       tenantID,
		EventID:        "evt-3",
		WorkID:         workID,
		Sequence:       3,
		EventType:      "COMPLETED",
		TerminalReason: "finished",
		CreatedAt:      time.Now(),
	})

	// Client reconnects specifying fromSequence = 2 (should ONLY get sequence 3)
	seq := em.Subscribe(ctx, tenantID, workID, 2)

	var replayed []a2a.Event
	for event, err := range seq {
		if err != nil {
			t.Fatalf("unexpected error during replay: %v", err)
		}
		if event != nil {
			replayed = append(replayed, event)
		}
	}

	if len(replayed) != 1 {
		t.Fatalf("expected exactly 1 replayed event (sequence 3), got %d", len(replayed))
	}
	st, ok := replayed[0].(*a2a.TaskStatusUpdateEvent)
	if !ok || st.Status.State != a2a.TaskStateCompleted {
		t.Errorf("expected state COMPLETED on replayed event, got %v", replayed[0])
	}
}
