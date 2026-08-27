package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/pkg/errors"
)

func (s *Store) EnqueueConnectorDelivery(ctx context.Context, organizationID, installationID, conversationID, idempotencyKey string, payload any, availableAt time.Time) error {
	if organizationID == "" || installationID == "" || conversationID == "" || idempotencyKey == "" {
		return errors.New("connector delivery tenant, installation, conversation, and idempotency key are required")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "marshal connector delivery")
	}
	sum := sha256.Sum256([]byte(organizationID + "\x00" + installationID + "\x00" + idempotencyKey))
	eventID := "connector-delivery-" + hex.EncodeToString(sum[:])
	return s.EnqueueOutboxEvent(ctx, DurableEventEnvelope{
		EventID: eventID, Organization: organizationID, AggregateType: "connector_installation",
		AggregateID: installationID, EventType: "CONNECTOR_DELIVERY", CorrelationID: conversationID,
		IdempotencyKey: "connector/" + installationID + "/" + idempotencyKey, Payload: body,
		MaxAttempts: 8, AvailableAt: availableAt,
	})
}

func (s *Store) RecordConnectorDivergence(ctx context.Context, organizationID, installationID, sourceRef, destinationRef, externalEventID, reason string) error {
	if organizationID == "" || installationID == "" || sourceRef == "" || destinationRef == "" || externalEventID == "" || reason == "" {
		return errors.New("connector divergence tenant, installation, source, destination, event, and reason are required")
	}
	_, err := s.GetDB().ExecContext(ctx, `
		INSERT INTO a2a888_connector_divergence (organization_id,installation_id,source_ref,destination_ref,external_event_id,reason)
		VALUES ($1,$2,$3,$4,$5,$6)
	`, organizationID, installationID, sourceRef, destinationRef, externalEventID, reason)
	return errors.Wrap(err, "record connector divergence")
}
