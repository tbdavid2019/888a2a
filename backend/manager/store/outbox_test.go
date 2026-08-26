package store

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDurableEventEnvelopeRoundTrip(t *testing.T) {
	event := DurableEventEnvelope{
		EventID:        "evt-1",
		Organization:   "org-1",
		AggregateType:  "conversation",
		AggregateID:    "conv-1",
		EventType:      "MESSAGE_CREATED",
		CorrelationID:  "trace-1",
		IdempotencyKey: "idem-1",
		Payload:        json.RawMessage(`{"message":"hello"}`),
		MaxAttempts:    5,
		AvailableAt:    time.Unix(100, 0).UTC(),
	}
	if err := event.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var decoded DurableEventEnvelope
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.EventID != event.EventID || decoded.Organization != event.Organization || string(decoded.Payload) != string(event.Payload) {
		t.Fatalf("round trip changed event: %#v", decoded)
	}
}

func TestDurableEventEnvelopeRejectsMissingTenantAndInvalidPayload(t *testing.T) {
	cases := []DurableEventEnvelope{
		{EventID: "evt", EventType: "TYPE", CorrelationID: "trace", Payload: json.RawMessage(`{}`), MaxAttempts: 1},
		{EventID: "evt", Organization: "org", AggregateType: "x", AggregateID: "id", EventType: "TYPE", CorrelationID: "trace", Payload: json.RawMessage(`{`), MaxAttempts: 1},
	}
	for i, event := range cases {
		if err := event.Validate(); err == nil {
			t.Fatalf("case %d: expected validation error", i)
		}
	}
}
