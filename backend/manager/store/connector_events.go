package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/pkg/errors"
)

const (
	ConnectorInboxReceived  = "RECEIVED"
	ConnectorInboxProcessed = "PROCESSED"
	ConnectorInboxFailed    = "FAILED"
)

var ErrInvalidConnectorEvent = errors.New("invalid connector event")

// ConnectorInboxEvent is the durable identity recorded before webhook work is
// dispatched. The external event identity is unique per tenant installation.
type ConnectorInboxEvent struct {
	OrganizationID       string
	InstallationID       string
	ExternalEventID      string
	ExternalEventType    string
	ExternalConversation string
	RawPayload           json.RawMessage
	Status               string
	Attempts             int
	LastError            string
	ReceivedAt           time.Time
	ProcessedAt          sql.NullTime
}

func (e ConnectorInboxEvent) Validate() error {
	if e.OrganizationID == "" || e.InstallationID == "" || e.ExternalEventID == "" ||
		e.ExternalEventType == "" || len(e.RawPayload) == 0 || !json.Valid(e.RawPayload) {
		return ErrInvalidConnectorEvent
	}
	return nil
}

// RecordConnectorInbox persists an inbound platform event before asynchronous
// normalization. It returns false when the same installation already recorded
// the external event, making webhook redelivery idempotent.
func (s *Store) RecordConnectorInbox(ctx context.Context, event ConnectorInboxEvent) (bool, error) {
	if err := event.Validate(); err != nil {
		return false, err
	}
	result, err := s.GetDB().ExecContext(ctx, `
		INSERT INTO a2a888_connector_inbox (
			organization_id, installation_id, external_event_id, external_event_type,
			external_conversation, raw_payload
		) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (organization_id, installation_id, external_event_id) DO NOTHING
	`, event.OrganizationID, event.InstallationID, event.ExternalEventID,
		event.ExternalEventType, event.ExternalConversation, event.RawPayload)
	if err != nil {
		return false, errors.Wrap(err, "record connector inbox event")
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, errors.Wrap(err, "check connector inbox result")
	}
	return rows == 1, nil
}

func (s *Store) MarkConnectorInboxProcessed(ctx context.Context, organizationID, installationID, externalEventID string) error {
	result, err := s.GetDB().ExecContext(ctx, `
		UPDATE a2a888_connector_inbox
		SET status = 'PROCESSED', processed_at = now(), updated_at = now()
		WHERE organization_id = $1 AND installation_id = $2 AND external_event_id = $3
	`, organizationID, installationID, externalEventID)
	if err != nil {
		return errors.Wrap(err, "mark connector inbox event processed")
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "check connector inbox update result")
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}
