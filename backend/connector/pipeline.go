package connector

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

type InboxRecord struct {
	OrganizationID       string
	InstallationID       string
	ExternalEventID      string
	ExternalEventType    string
	ExternalConversation string
	Raw                  json.RawMessage
	ReceivedAt           time.Time
}

type InboxWriter func(context.Context, InboxRecord) (bool, error)

// Receive verifies and normalizes an inbound event before it is committed to
// the durable inbox. The writer is called only after authentication succeeds;
// returning false means the external event was already acknowledged by its
// tenant-scoped idempotency key.
func Receive(ctx context.Context, adapter Connector, installation Installation, headers http.Header, raw []byte, write InboxWriter) (Envelope, bool, error) {
	if adapter == nil || write == nil {
		return Envelope{}, false, errors.New("connector and inbox writer are required")
	}
	if err := adapter.Manifest().Validate(); err != nil {
		return Envelope{}, false, err
	}
	if err := installation.Validate(); err != nil {
		return Envelope{}, false, err
	}
	verified, err := adapter.VerifyInbound(ctx, installation, headers, raw)
	if err != nil {
		return Envelope{}, false, err
	}
	if verified.Installation != installation || verified.ExternalID == "" {
		return Envelope{}, false, errors.New("connector verification returned an invalid installation or event identity")
	}
	envelope, err := adapter.Normalize(ctx, verified)
	if err != nil {
		return Envelope{}, false, err
	}
	if envelope.OrganizationID != installation.OrganizationID || envelope.InstallationID != installation.InstallationID || envelope.ExternalEventID != verified.ExternalID || envelope.EventType == "" {
		return Envelope{}, false, errors.New("connector normalization returned an invalid tenant envelope")
	}
	accepted, err := write(ctx, InboxRecord{
		OrganizationID: installation.OrganizationID, InstallationID: installation.InstallationID,
		ExternalEventID: envelope.ExternalEventID, ExternalEventType: envelope.EventType,
		ExternalConversation: envelope.ExternalConversation, Raw: append(json.RawMessage(nil), raw...), ReceivedAt: time.Now().UTC(),
	})
	if err != nil {
		return Envelope{}, false, err
	}
	return envelope, accepted, nil
}
