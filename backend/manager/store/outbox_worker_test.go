package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tbdavid2019/888a2a/backend/manager/component/tenantqueue"
)

type fakeOutboxRepository struct {
	events   []OutboxEvent
	acked    []string
	retried  []string
	lastTime time.Time
}

func (f *fakeOutboxRepository) ClaimOutboxEvents(_ context.Context, _ string, _ int) ([]OutboxEvent, error) {
	return f.events, nil
}

func (f *fakeOutboxRepository) AckOutboxEvent(_ context.Context, _, eventID string) error {
	f.acked = append(f.acked, eventID)
	return nil
}

func (f *fakeOutboxRepository) RetryOutboxEvent(_ context.Context, _, eventID, _ string, availableAt time.Time) error {
	f.retried = append(f.retried, eventID)
	f.lastTime = availableAt
	return nil
}

func TestOutboxWorkerAcknowledgesSuccessfulDelivery(t *testing.T) {
	repo := &fakeOutboxRepository{events: []OutboxEvent{{DurableEventEnvelope: DurableEventEnvelope{EventID: "evt-1"}}}}
	worker := &OutboxWorker{
		Repository: repo,
		WorkerID:   "worker-1",
		BatchSize:  10,
		Handle:     func(context.Context, OutboxEvent) error { return nil },
	}
	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(repo.acked) != 1 || repo.acked[0] != "evt-1" {
		t.Fatalf("acked events = %v, want evt-1", repo.acked)
	}
}

func TestOutboxWorkerRetriesFailedDelivery(t *testing.T) {
	repo := &fakeOutboxRepository{events: []OutboxEvent{{
		DurableEventEnvelope: DurableEventEnvelope{EventID: "evt-2"},
		Attempts:             1,
	}}}
	now := time.Unix(100, 0)
	worker := &OutboxWorker{
		Repository: repo,
		WorkerID:   "worker-1",
		BatchSize:  10,
		Now:        func() time.Time { return now },
		RetryDelay: func(int) time.Duration { return 5 * time.Second },
		Handle:     func(context.Context, OutboxEvent) error { return errors.New("temporary") },
	}
	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(repo.retried) != 1 || !repo.lastTime.Equal(now.Add(5*time.Second)) {
		t.Fatalf("retry = %v at %v", repo.retried, repo.lastTime)
	}
	if len(repo.acked) != 0 {
		t.Fatalf("failed event was acknowledged: %v", repo.acked)
	}
}

func TestOutboxWorkerUsesFairTenantQueue(t *testing.T) {
	repo := &fakeOutboxRepository{events: []OutboxEvent{
		{DurableEventEnvelope: DurableEventEnvelope{EventID: "a-1", Organization: "org-a"}},
		{DurableEventEnvelope: DurableEventEnvelope{EventID: "a-2", Organization: "org-a"}},
		{DurableEventEnvelope: DurableEventEnvelope{EventID: "b-1", Organization: "org-b"}},
	}}
	var delivered []string
	worker := &OutboxWorker{
		Repository: repo,
		WorkerID:   "worker-1",
		BatchSize:  10,
		Queue:      tenantqueue.NewQueue(2, 3),
		Handle: func(_ context.Context, event OutboxEvent) error {
			delivered = append(delivered, event.EventID)
			return nil
		},
	}

	if err := worker.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	want := []string{"a-1", "b-1", "a-2"}
	if len(delivered) != len(want) {
		t.Fatalf("delivered = %v, want %v", delivered, want)
	}
	for i := range want {
		if delivered[i] != want[i] {
			t.Fatalf("delivered = %v, want %v", delivered, want)
		}
	}
}
