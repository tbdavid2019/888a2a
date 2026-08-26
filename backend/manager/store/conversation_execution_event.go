package store

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
)

// ConversationExecutionEvent is the conversation-visible lifecycle record for
// an Agent command. Command output remains in command_event; this ledger is the
// compact tenant-scoped index used by conversation activity and audit readers.
type ConversationExecutionEvent struct {
	ID             int64
	OrganizationID string
	ConversationID uuid.UUID
	CommandID      uuid.UUID
	EventType      string
	PayloadJSON    string
	CreatedAt      time.Time
}

var validConversationExecutionEvents = map[string]bool{
	"COMMAND_STARTED":   true,
	"COMMAND_STEERED":   true,
	"COMMAND_CANCELLED": true,
	"COMMAND_COMPLETED": true,
}

// AppendCommandExecutionEvent records a command lifecycle event for every
// conversation linked to the command. Start, cancel, and completion are
// idempotent; steer is repeatable because each steer is meaningful.
func (s *Store) AppendCommandExecutionEvent(ctx context.Context, commandID uuid.UUID, eventType, payloadJSON string) error {
	if commandID == uuid.Nil || !validConversationExecutionEvents[eventType] {
		return errors.New("command execution event identity or type is invalid")
	}
	if strings.TrimSpace(payloadJSON) == "" {
		payloadJSON = "{}"
	}
	_, err := s.GetDB().ExecContext(ctx, `
		INSERT INTO a2a888_conversation_execution_event (
			organization_id, conversation_id, command_id, event_type, payload_json
		)
		SELECT organization_id, conversation_id, $1, $2, $3::jsonb
		FROM command_conversation
		WHERE command_id = $1
		  AND ($2 = 'COMMAND_STEERED' OR NOT EXISTS (
			SELECT 1 FROM a2a888_conversation_execution_event existing
			WHERE existing.organization_id = command_conversation.organization_id
			  AND existing.conversation_id = command_conversation.conversation_id
			  AND existing.command_id = $1
			  AND existing.event_type = $2
		))
	`, commandID, eventType, payloadJSON)
	return errors.Wrap(err, "append conversation execution event")
}

// ListConversationExecutionEvents returns lifecycle events in durable insert
// order and enforces the Organization boundary at the query edge.
func (s *Store) ListConversationExecutionEvents(ctx context.Context, organizationID string, conversationID uuid.UUID) ([]*ConversationExecutionEvent, error) {
	if strings.TrimSpace(organizationID) == "" || conversationID == uuid.Nil {
		return nil, errors.New("organization_id and conversation_id are required")
	}
	rows, err := s.GetDB().QueryContext(ctx, `
		SELECT id, organization_id, conversation_id, command_id, event_type, payload_json, created_at
		FROM a2a888_conversation_execution_event
		WHERE organization_id = $1 AND conversation_id = $2
		ORDER BY id ASC
	`, organizationID, conversationID)
	if err != nil {
		return nil, errors.Wrap(err, "list conversation execution events")
	}
	defer rows.Close()
	var events []*ConversationExecutionEvent
	for rows.Next() {
		event := &ConversationExecutionEvent{}
		if err := rows.Scan(&event.ID, &event.OrganizationID, &event.ConversationID, &event.CommandID, &event.EventType, &event.PayloadJSON, &event.CreatedAt); err != nil {
			return nil, errors.Wrap(err, "scan conversation execution event")
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, "iterate conversation execution events")
	}
	return events, nil
}
