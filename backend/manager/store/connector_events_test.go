package store

import (
	"encoding/json"
	"testing"
)

func TestConnectorInboxEventValidation(t *testing.T) {
	event := ConnectorInboxEvent{
		OrganizationID:    "org-1",
		InstallationID:    "line-install-1",
		ExternalEventID:   "webhook-1",
		ExternalEventType: "message",
		RawPayload:        json.RawMessage(`{"events":[]}`),
	}
	if err := event.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	event.RawPayload = json.RawMessage(`{`)
	if err := event.Validate(); err == nil {
		t.Fatal("expected invalid JSON payload to fail")
	}
}
