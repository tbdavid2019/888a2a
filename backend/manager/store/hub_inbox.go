package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
)

type HubInboxMessage struct {
	Sequence         uint64
	HubID            string
	TargetAgentID    string
	RequesterAgentID string
	TaskID           string
	ContextID        string
	IdempotencyKey   string
	Message          string
	CreatedAt        time.Time
	AcknowledgedAt   sql.NullTime
}

func (s *Store) CreateHubInboxItem(ctx context.Context, item *HubInboxMessage) (*HubInboxMessage, bool, error) {
	if item == nil || item.HubID == "" || item.TargetAgentID == "" || item.RequesterAgentID == "" || item.TaskID == "" || item.ContextID == "" || item.IdempotencyKey == "" || item.Message == "" {
		return nil, false, errors.New("Hub inbox item is incomplete")
	}
	if _, err := uuid.Parse(item.TaskID); err != nil {
		return nil, false, errors.New("Hub inbox task ID is invalid")
	}
	var created HubInboxMessage
	err := s.GetDB().QueryRowContext(ctx, `INSERT INTO a2a888_hub_inbox
		(hub_id, target_agent_id, requester_agent_id, task_id, context_id, idempotency_key, message)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (hub_id, target_agent_id, requester_agent_id, idempotency_key) DO NOTHING
		RETURNING sequence, hub_id, target_agent_id, requester_agent_id, task_id, context_id, idempotency_key, message, created_at, acknowledged_at`,
		item.HubID, item.TargetAgentID, item.RequesterAgentID, item.TaskID, item.ContextID, item.IdempotencyKey, item.Message).Scan(
		&created.Sequence, &created.HubID, &created.TargetAgentID, &created.RequesterAgentID, &created.TaskID,
		&created.ContextID, &created.IdempotencyKey, &created.Message, &created.CreatedAt, &created.AcknowledgedAt)
	if errors.Is(err, sql.ErrNoRows) {
		existing, getErr := s.getHubInboxByIdempotency(ctx, item.HubID, item.TargetAgentID, item.RequesterAgentID, item.IdempotencyKey)
		return existing, getErr == nil, getErr
	}
	if err != nil {
		return nil, false, err
	}
	return &created, false, nil
}

func (s *Store) getHubInboxByIdempotency(ctx context.Context, hubID, target, requester, key string) (*HubInboxMessage, error) {
	var item HubInboxMessage
	err := s.GetDB().QueryRowContext(ctx, `SELECT sequence, hub_id, target_agent_id, requester_agent_id, task_id, context_id, idempotency_key, message, created_at, acknowledged_at
		FROM a2a888_hub_inbox WHERE hub_id=$1 AND target_agent_id=$2 AND requester_agent_id=$3 AND idempotency_key=$4`, hubID, target, requester, key).Scan(
		&item.Sequence, &item.HubID, &item.TargetAgentID, &item.RequesterAgentID, &item.TaskID,
		&item.ContextID, &item.IdempotencyKey, &item.Message, &item.CreatedAt, &item.AcknowledgedAt)
	return &item, err
}

func (s *Store) ListHubInbox(ctx context.Context, hubID, targetAgentID string, afterSequence uint64, limit int) ([]*HubInboxMessage, error) {
	if limit <= 0 || limit > 100 {
		return nil, errors.New("Hub inbox limit is invalid")
	}
	rows, err := s.GetDB().QueryContext(ctx, `SELECT sequence, hub_id, target_agent_id, requester_agent_id, task_id, context_id, idempotency_key, message, created_at, acknowledged_at
		FROM a2a888_hub_inbox WHERE hub_id=$1 AND target_agent_id=$2 AND sequence>$3 AND state='PENDING'
		ORDER BY sequence LIMIT $4`, hubID, targetAgentID, afterSequence, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []*HubInboxMessage
	for rows.Next() {
		item := new(HubInboxMessage)
		if err := rows.Scan(&item.Sequence, &item.HubID, &item.TargetAgentID, &item.RequesterAgentID, &item.TaskID, &item.ContextID, &item.IdempotencyKey, &item.Message, &item.CreatedAt, &item.AcknowledgedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) AcknowledgeHubInbox(ctx context.Context, hubID, targetAgentID string, sequence uint64, now time.Time) error {
	result, err := s.GetDB().ExecContext(ctx, `UPDATE a2a888_hub_inbox SET state='ACKNOWLEDGED', acknowledged_at=$4
		WHERE hub_id=$1 AND target_agent_id=$2 AND sequence=$3 AND state='PENDING'`, hubID, targetAgentID, sequence, now)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		var exists bool
		if scanErr := s.GetDB().QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM a2a888_hub_inbox WHERE hub_id=$1 AND target_agent_id=$2 AND sequence=$3)`, hubID, targetAgentID, sequence).Scan(&exists); scanErr != nil || !exists {
			return errors.Wrap(ErrHubAgentAlreadyExists, "Hub inbox item not found")
		}
	}
	return nil
}
