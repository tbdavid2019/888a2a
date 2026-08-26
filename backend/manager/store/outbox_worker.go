package store

import (
	"context"
	"time"

	"github.com/tbdavid2019/888a2a/backend/manager/component/tenantqueue"
)

// OutboxRepository is the small persistence boundary needed by the worker.
// Keeping it separate makes claim/ack/retry behavior testable without hiding
// the PostgreSQL transaction semantics of Store.
type OutboxRepository interface {
	ClaimOutboxEvents(context.Context, string, int) ([]OutboxEvent, error)
	AckOutboxEvent(context.Context, string, string) error
	RetryOutboxEvent(context.Context, string, string, string, time.Time) error
}

type OutboxHandler func(context.Context, OutboxEvent) error

// OutboxWorker delivers at-least-once events. A handler must be idempotent;
// the worker acknowledges only after the handler returns successfully.
type OutboxWorker struct {
	Repository OutboxRepository
	WorkerID   string
	BatchSize  int
	RetryDelay func(attempts int) time.Duration
	Handle     OutboxHandler
	Now        func() time.Time
	Limiter    *tenantqueue.Limiter
	Queue      *tenantqueue.Queue
}

// Run polls until the context is cancelled. A failed batch does not stop the
// worker; individual event failures are returned to the durable retry path.
func (w *OutboxWorker) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = time.Second
	}
	if err := w.RunOnce(ctx); err != nil {
		return err
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := w.RunOnce(ctx); err != nil {
				return err
			}
		}
	}
}

func (w *OutboxWorker) RunOnce(ctx context.Context) error {
	if w == nil || w.Repository == nil || w.WorkerID == "" || w.BatchSize <= 0 || w.Handle == nil {
		return ErrInvalidOutboxEvent
	}
	events, err := w.Repository.ClaimOutboxEvents(ctx, w.WorkerID, w.BatchSize)
	if err != nil {
		return err
	}
	now := time.Now
	if w.Now != nil {
		now = w.Now
	}
	delay := func(attempts int) time.Duration { return time.Second }
	if w.RetryDelay != nil {
		delay = w.RetryDelay
	}
	if w.Queue != nil {
		for _, event := range events {
			if w.Queue.Enqueue(tenantqueue.Item{OrganizationID: event.Organization, Value: event}) {
				continue
			}
			if retryErr := w.Repository.RetryOutboxEvent(ctx, w.WorkerID, event.EventID, "organization queue limit reached", now().Add(delay(event.Attempts+1))); retryErr != nil {
				return retryErr
			}
		}
		events = events[:0]
		for {
			item, ok := w.Queue.Dequeue()
			if !ok {
				break
			}
			event, ok := item.Value.(OutboxEvent)
			if !ok {
				return ErrInvalidOutboxEvent
			}
			events = append(events, event)
		}
	}
	for _, event := range events {
		var release func()
		if w.Limiter != nil {
			var admitted bool
			release, admitted = w.Limiter.TryAcquire(event.Organization)
			if !admitted {
				if retryErr := w.Repository.RetryOutboxEvent(ctx, w.WorkerID, event.EventID, "organization worker limit reached", now().Add(delay(event.Attempts+1))); retryErr != nil {
					return retryErr
				}
				continue
			}
		}
		if err := w.Handle(ctx, event); err != nil {
			if release != nil {
				release()
			}
			if retryErr := w.Repository.RetryOutboxEvent(ctx, w.WorkerID, event.EventID, err.Error(), now().Add(delay(event.Attempts+1))); retryErr != nil {
				return retryErr
			}
			continue
		}
		if release != nil {
			release()
		}
		if err := w.Repository.AckOutboxEvent(ctx, w.WorkerID, event.EventID); err != nil {
			return err
		}
	}
	return nil
}
