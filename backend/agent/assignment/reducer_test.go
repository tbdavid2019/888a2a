package assignment

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/Ranxy/laelia/backend/generated-go/a2a888"
)

func TestReducerRejectsZeroAndGapSequence(t *testing.T) {
	reducer := NewReducer("machine-1")

	if err := reducer.ApplyEvent(event("machine-1", 0, "event-0", "key-0", a2a888.AssignmentEventType_CREATE, "agent-1", "r1")); err == nil {
		t.Fatal("ApplyEvent accepted a zero sequence")
	}
	if err := reducer.ApplyEvent(event("machine-1", 2, "event-2", "key-2", a2a888.AssignmentEventType_CREATE, "agent-1", "r1")); err == nil {
		t.Fatal("ApplyEvent accepted a sequence gap")
	}
}

func TestReducerRejectsWrongMachine(t *testing.T) {
	reducer := NewReducer("machine-1")

	if err := reducer.ApplyEvent(event("machine-2", 1, "event-1", "key-1", a2a888.AssignmentEventType_CREATE, "agent-2", "r1")); err == nil {
		t.Fatal("ApplyEvent accepted an event for another machine")
	}
}

func TestReducerRejectsZeroAckAfterEvent(t *testing.T) {
	reducer := NewReducer("machine-1")
	if err := reducer.ApplyEvent(event("machine-1", 1, "event-1", "key-1", a2a888.AssignmentEventType_CREATE, "agent-1", "r1")); err != nil {
		t.Fatalf("ApplyEvent: %v", err)
	}
	zeroAck := &a2a888.MachineAssignmentAck{
		MachineResourceId:   "machine-1",
		AcknowledgedThrough: &a2a888.AssignmentCursor{},
	}
	if err := reducer.Acknowledge(zeroAck); err == nil {
		t.Fatal("Acknowledge accepted a zero cursor after applying an event")
	}
}

func TestReducerRejectsWrongMachineAck(t *testing.T) {
	reducer := NewReducer("machine-1")
	e := event("machine-1", 1, "event-1", "key-1", a2a888.AssignmentEventType_CREATE, "agent-1", "r1")
	if err := reducer.ApplyEvent(e); err != nil {
		t.Fatalf("ApplyEvent: %v", err)
	}
	if err := reducer.Acknowledge(ack("machine-2", e)); err == nil {
		t.Fatal("Acknowledge accepted a cursor for another machine")
	}
}

func TestReducerRejectsConflictingDuplicateEvent(t *testing.T) {
	reducer := NewReducer("machine-1")
	e := event("machine-1", 1, "event-1", "key-1", a2a888.AssignmentEventType_CREATE, "agent-1", "r1")
	if err := reducer.ApplyEvent(e); err != nil {
		t.Fatalf("ApplyEvent: %v", err)
	}
	conflict := proto.CloneOf(e)
	conflict.EventId = "event-1-replaced"
	if err := reducer.ApplyEvent(conflict); err == nil {
		t.Fatal("ApplyEvent accepted a conflicting duplicate")
	}
}

func TestReducerDuplicateEventIsIdempotent(t *testing.T) {
	reducer := NewReducer("machine-1")
	e := event("machine-1", 1, "event-1", "key-1", a2a888.AssignmentEventType_CREATE, "agent-1", "r1")

	if err := reducer.ApplyEvent(e); err != nil {
		t.Fatalf("ApplyEvent: %v", err)
	}
	if err := reducer.ApplyEvent(e); err != nil {
		t.Fatalf("duplicate ApplyEvent: %v", err)
	}
	if got := reducer.LastSequence(); got != 1 {
		t.Fatalf("LastSequence() = %d, want 1", got)
	}
}

func TestReducerDuplicateAckIsIdempotentAndRegressionRejected(t *testing.T) {
	reducer := NewReducer("machine-1")
	e1 := event("machine-1", 1, "event-1", "key-1", a2a888.AssignmentEventType_CREATE, "agent-1", "r1")
	e2 := event("machine-1", 2, "event-2", "key-2", a2a888.AssignmentEventType_CONFIG_UPDATE, "agent-1", "r2")
	for _, e := range []*a2a888.MachineAssignmentEvent{e1, e2} {
		if err := reducer.ApplyEvent(e); err != nil {
			t.Fatalf("ApplyEvent: %v", err)
		}
	}
	ack2 := ack("machine-1", e2)
	if err := reducer.Acknowledge(ack2); err != nil {
		t.Fatalf("Acknowledge: %v", err)
	}
	if err := reducer.Acknowledge(ack2); err != nil {
		t.Fatalf("duplicate Acknowledge: %v", err)
	}
	if err := reducer.Acknowledge(ack("machine-1", e1)); err == nil {
		t.Fatal("Acknowledge accepted a regressing cursor")
	}
}

func TestReducerAppliesOrderedReplay(t *testing.T) {
	reducer := NewReducer("machine-1")
	events := []*a2a888.MachineAssignmentEvent{
		event("machine-1", 1, "event-1", "key-1", a2a888.AssignmentEventType_CREATE, "agent-1", "r1"),
		event("machine-1", 2, "event-2", "key-2", a2a888.AssignmentEventType_CONFIG_UPDATE, "agent-1", "r2"),
		event("machine-1", 3, "event-3", "key-3", a2a888.AssignmentEventType_REMOVE, "agent-1", ""),
	}

	if err := reducer.ApplyReplay(events); err != nil {
		t.Fatalf("ApplyReplay: %v", err)
	}
	if got := reducer.LastSequence(); got != 3 {
		t.Fatalf("LastSequence() = %d, want 3", got)
	}
	if _, ok := reducer.Assignment("agent-1"); ok {
		t.Fatal("removed assignment remains active")
	}
}

func event(machine string, sequence uint64, eventID, idempotencyKey string, eventType a2a888.AssignmentEventType, agent, revision string) *a2a888.MachineAssignmentEvent {
	e := &a2a888.MachineAssignmentEvent{
		MachineResourceId: machine,
		AgentResourceId:   agent,
		Sequence:          sequence,
		EventId:           eventID,
		IdempotencyKey:    idempotencyKey,
		EventType:         eventType,
		CreatedAt:         timestamppb.New(time.Unix(1700000000+int64(sequence), 0)),
	}
	if revision != "" {
		e.Config = &a2a888.AssignmentConfig{
			Revision:         revision,
			PayloadReference: "payload/" + revision,
			PayloadDigest:    deterministicDigest("payload/" + revision),
		}
	}
	return e
}

func deterministicDigest(payload string) string {
	digest := sha256.Sum256([]byte(payload))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func ack(machine string, e *a2a888.MachineAssignmentEvent) *a2a888.MachineAssignmentAck {
	return &a2a888.MachineAssignmentAck{
		MachineResourceId: machine,
		AcknowledgedThrough: &a2a888.AssignmentCursor{
			Sequence:       e.GetSequence(),
			EventId:        e.GetEventId(),
			IdempotencyKey: e.GetIdempotencyKey(),
		},
	}
}
