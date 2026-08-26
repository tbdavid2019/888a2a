package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/pkg/errors"
)

const (
	OutboxStatusPending    = "PENDING"
	OutboxStatusClaimed    = "CLAIMED"
	OutboxStatusDelivered  = "DELIVERED"
	OutboxStatusDeadLetter = "DEAD_LETTER"
)

var (
	ErrInvalidOutboxEvent  = errors.New("invalid outbox event")
	ErrOutboxDuplicate     = errors.New("outbox event already exists")
	ErrOutboxNotClaimed    = errors.New("outbox event is not claimed by worker")
	ErrOutboxNotDeadLetter = errors.New("outbox event is not dead-lettered")
)

// DurableEventEnvelope is the stable event contract shared by outbox writers
// and consumers. Payload is opaque JSON so producers can evolve independently
// while the routing and retry identities remain durable.
type DurableEventEnvelope struct {
	EventID        string          `json:"event_id"`
	Organization   string          `json:"organization_id"`
	AggregateType  string          `json:"aggregate_type"`
	AggregateID    string          `json:"aggregate_id"`
	EventType      string          `json:"event_type"`
	CorrelationID  string          `json:"correlation_id"`
	IdempotencyKey string          `json:"idempotency_key"`
	Payload        json.RawMessage `json:"payload"`
	MaxAttempts    int             `json:"max_attempts"`
	AvailableAt    time.Time       `json:"available_at"`
}

// Validate checks the fields required for tenant-safe durable delivery.
func (e DurableEventEnvelope) Validate() error {
	if e.EventID == "" || e.Organization == "" || e.AggregateType == "" ||
		e.AggregateID == "" || e.EventType == "" || e.CorrelationID == "" {
		return ErrInvalidOutboxEvent
	}
	if len(e.Payload) == 0 || !json.Valid(e.Payload) {
		return errors.Wrap(ErrInvalidOutboxEvent, "payload must be valid JSON")
	}
	if e.MaxAttempts <= 0 {
		return errors.Wrap(ErrInvalidOutboxEvent, "max attempts must be positive")
	}
	return nil
}

// OutboxEvent is the persisted delivery record returned to a worker.
type OutboxEvent struct {
	DurableEventEnvelope
	Status      string
	Attempts    int
	LastError   string
	WorkerID    string
	ClaimedAt   sql.NullTime
	DeliveredAt sql.NullTime
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// EnqueueOutboxEvent durably records an outbound event before external delivery.
func (s *Store) EnqueueOutboxEvent(ctx context.Context, event DurableEventEnvelope) error {
	if err := event.Validate(); err != nil {
		return err
	}
	availableAt := event.AvailableAt
	if availableAt.IsZero() {
		availableAt = time.Now().UTC()
	}
	result, err := s.GetDB().ExecContext(ctx, `
		INSERT INTO a2a888_outbox_event (
			event_id, organization_id, aggregate_type, aggregate_id, event_type,
			correlation_id, idempotency_key, payload, max_attempts, available_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (organization_id, idempotency_key)
		WHERE idempotency_key <> '' DO NOTHING
	`, event.EventID, event.Organization, event.AggregateType, event.AggregateID,
		event.EventType, event.CorrelationID, event.IdempotencyKey, event.Payload,
		event.MaxAttempts, availableAt)
	if err != nil {
		return errors.Wrap(err, "enqueue outbox event")
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "check outbox enqueue result")
	}
	if rows == 0 {
		return ErrOutboxDuplicate
	}
	return nil
}

// ClaimOutboxEvents atomically claims pending events for one worker.
func (s *Store) ClaimOutboxEvents(ctx context.Context, workerID string, limit int) ([]OutboxEvent, error) {
	if workerID == "" || limit <= 0 {
		return nil, errors.Wrap(ErrInvalidOutboxEvent, "worker and positive limit are required")
	}
	rows, err := s.GetDB().QueryContext(ctx, `
		WITH claimed AS (
			SELECT event_id
			FROM a2a888_outbox_event
			WHERE available_at <= now()
			  AND (status = 'PENDING' OR (status = 'CLAIMED' AND claim_expires_at < now()))
			ORDER BY created_at, event_id
			FOR UPDATE SKIP LOCKED
			LIMIT $2
		)
		UPDATE a2a888_outbox_event AS e
		SET status = 'CLAIMED', worker_id = $1, claimed_at = now(),
			claim_expires_at = now() + interval '1 minute', updated_at = now()
		FROM claimed
		WHERE e.event_id = claimed.event_id
		RETURNING e.event_id, e.organization_id, e.aggregate_type, e.aggregate_id,
			e.event_type, e.correlation_id, e.idempotency_key, e.payload,
			e.max_attempts, e.available_at, e.status, e.attempts, e.last_error,
			e.worker_id, e.claimed_at, e.delivered_at, e.created_at, e.updated_at
	`, workerID, limit)
	if err != nil {
		return nil, errors.Wrap(err, "claim outbox events")
	}
	defer rows.Close()

	var events []OutboxEvent
	for rows.Next() {
		var event OutboxEvent
		if err := rows.Scan(
			&event.EventID, &event.Organization, &event.AggregateType, &event.AggregateID,
			&event.EventType, &event.CorrelationID, &event.IdempotencyKey, &event.Payload,
			&event.MaxAttempts, &event.AvailableAt, &event.Status, &event.Attempts,
			&event.LastError, &event.WorkerID, &event.ClaimedAt, &event.DeliveredAt,
			&event.CreatedAt, &event.UpdatedAt,
		); err != nil {
			return nil, errors.Wrap(err, "scan claimed outbox event")
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

// AckOutboxEvent marks a worker-owned event delivered exactly once.
func (s *Store) AckOutboxEvent(ctx context.Context, workerID, eventID string) error {
	result, err := s.GetDB().ExecContext(ctx, `
		UPDATE a2a888_outbox_event
		SET status = 'DELIVERED', delivered_at = now(), updated_at = now(),
			worker_id = '', claim_expires_at = NULL
		WHERE event_id = $1 AND worker_id = $2 AND status = 'CLAIMED'
	`, eventID, workerID)
	if err != nil {
		return errors.Wrap(err, "ack outbox event")
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "check outbox ack result")
	}
	if rows == 0 {
		return ErrOutboxNotClaimed
	}
	return nil
}

// RetryOutboxEvent returns a failed event to the queue or dead-letters it when
// the persisted attempt budget is exhausted.
func (s *Store) RetryOutboxEvent(ctx context.Context, workerID, eventID, lastError string, availableAt time.Time) error {
	if availableAt.IsZero() {
		availableAt = time.Now().UTC()
	}
	result, err := s.GetDB().ExecContext(ctx, `
		UPDATE a2a888_outbox_event
		SET attempts = attempts + 1,
			status = CASE WHEN attempts + 1 >= max_attempts THEN 'DEAD_LETTER' ELSE 'PENDING' END,
			last_error = $3, available_at = $4, worker_id = '', claim_expires_at = NULL, updated_at = now()
		WHERE event_id = $1 AND worker_id = $2 AND status = 'CLAIMED'
	`, eventID, workerID, lastError, availableAt)
	if err != nil {
		return errors.Wrap(err, "retry outbox event")
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "check outbox retry result")
	}
	if rows == 0 {
		return ErrOutboxNotClaimed
	}
	return nil
}

// ReplayDeadLetterOutboxEvent requeues one tenant-owned dead-letter event and
// records the operator action for reconciliation and audit consumers.
func (s *Store) ReplayDeadLetterOutboxEvent(ctx context.Context, organizationID, eventID, actorID string) error {
	if organizationID == "" || eventID == "" || actorID == "" {
		return ErrInvalidOutboxEvent
	}
	tx, err := s.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, "begin outbox replay")
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		UPDATE a2a888_outbox_event
		SET status = 'PENDING', attempts = 0, last_error = '', available_at = now(),
			worker_id = '', claimed_at = NULL, updated_at = now()
		WHERE organization_id = $1 AND event_id = $2 AND status = 'DEAD_LETTER'
	`, organizationID, eventID)
	if err != nil {
		return errors.Wrap(err, "requeue dead-letter outbox event")
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "check dead-letter replay result")
	}
	if rows == 0 {
		return ErrOutboxNotDeadLetter
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO a2a888_outbox_reconciliation (organization_id, event_id, actor_id, action)
		VALUES ($1, $2, $3, 'REPLAY')
	`, organizationID, eventID, actorID); err != nil {
		return errors.Wrap(err, "record outbox replay")
	}
	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "commit outbox replay")
	}
	return nil
}
