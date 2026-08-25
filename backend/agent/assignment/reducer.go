package assignment

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"strings"

	pkgerrors "github.com/pkg/errors"
	"google.golang.org/protobuf/proto"

	"github.com/Ranxy/laelia/backend/generated-go/a2a888"
)

var (
	ErrInvalidEvent       = errors.New("invalid assignment event")
	ErrMachineMismatch    = errors.New("assignment machine mismatch")
	ErrSequenceGap        = errors.New("assignment sequence gap")
	ErrSequenceRegression = errors.New("assignment sequence regression")
	ErrAckRegression      = errors.New("assignment acknowledgement regression")
	ErrAckMismatch        = errors.New("assignment acknowledgement mismatch")
)

// Assignment is the reducer's current state for one Agent resource.
type Assignment struct {
	AgentResourceID string
	Config          *a2a888.AssignmentConfig
}

// Reducer validates and applies the ordered assignment log for one Machine.
// It deliberately has no persistence or transport responsibilities.
type Reducer struct {
	machineResourceID string
	lastSequence      uint64
	lastAcknowledged  *a2a888.AssignmentCursor
	eventsBySequence  map[uint64]*a2a888.MachineAssignmentEvent
	eventIDs          map[string]uint64
	idempotencyKeys   map[string]uint64
	assignments       map[string]Assignment
}

func NewReducer(machineResourceID string) *Reducer {
	return &Reducer{
		machineResourceID: machineResourceID,
		eventsBySequence:  make(map[uint64]*a2a888.MachineAssignmentEvent),
		eventIDs:          make(map[string]uint64),
		idempotencyKeys:   make(map[string]uint64),
		assignments:       make(map[string]Assignment),
	}
}

// ApplyEvent validates and applies one event. Reapplying the exact event is
// idempotent; a changed event at an existing sequence is rejected.
func (r *Reducer) ApplyEvent(event *a2a888.MachineAssignmentEvent) error {
	if err := validateEvent(event, r.machineResourceID); err != nil {
		return err
	}
	sequence := event.GetSequence()
	if sequence <= r.lastSequence {
		previous, ok := r.eventsBySequence[sequence]
		if !ok {
			return pkgerrors.Wrapf(ErrSequenceRegression, "sequence %d is not retained", sequence)
		}
		if !proto.Equal(previous, event) {
			return pkgerrors.Wrapf(ErrInvalidEvent, "sequence %d identity or payload changed", sequence)
		}
		return nil
	}
	if sequence != r.lastSequence+1 {
		return pkgerrors.Wrapf(ErrSequenceGap, "got %d after %d", sequence, r.lastSequence)
	}
	if previous, ok := r.eventIDs[event.GetEventId()]; ok && previous != sequence {
		return pkgerrors.Wrapf(ErrInvalidEvent, "event id %q was used at sequence %d", event.GetEventId(), previous)
	}
	if previous, ok := r.idempotencyKeys[event.GetIdempotencyKey()]; ok && previous != sequence {
		return pkgerrors.Wrapf(ErrInvalidEvent, "idempotency key %q was used at sequence %d", event.GetIdempotencyKey(), previous)
	}

	agentID := event.GetAgentResourceId()
	switch event.GetEventType() {
	case a2a888.AssignmentEventType_CREATE:
		if _, exists := r.assignments[agentID]; exists {
			return pkgerrors.Wrapf(ErrInvalidEvent, "Agent %q already exists", agentID)
		}
		r.assignments[agentID] = Assignment{AgentResourceID: agentID, Config: cloneConfig(event.GetConfig())}
	case a2a888.AssignmentEventType_CONFIG_UPDATE:
		if _, exists := r.assignments[agentID]; !exists {
			return pkgerrors.Wrapf(ErrInvalidEvent, "Agent %q does not exist", agentID)
		}
		r.assignments[agentID] = Assignment{AgentResourceID: agentID, Config: cloneConfig(event.GetConfig())}
	case a2a888.AssignmentEventType_REMOVE:
		if _, exists := r.assignments[agentID]; !exists {
			return pkgerrors.Wrapf(ErrInvalidEvent, "Agent %q does not exist", agentID)
		}
		delete(r.assignments, agentID)
	case a2a888.AssignmentEventType_ASSIGNMENT_EVENT_TYPE_UNSPECIFIED:
		fallthrough
	default:
		return pkgerrors.Wrapf(ErrInvalidEvent, "unsupported event type %d", event.GetEventType())
	}

	stored := proto.CloneOf(event)
	r.eventsBySequence[sequence] = stored
	r.eventIDs[event.GetEventId()] = sequence
	r.idempotencyKeys[event.GetIdempotencyKey()] = sequence
	r.lastSequence = sequence
	return nil
}

// ApplyReplay applies an ordered replay response's events.
func (r *Reducer) ApplyReplay(events []*a2a888.MachineAssignmentEvent) error {
	for _, event := range events {
		if err := r.ApplyEvent(event); err != nil {
			return err
		}
	}
	return nil
}

// Acknowledge validates a cumulative Machine acknowledgement. Repeating the
// exact cursor is idempotent, while moving it backwards is rejected.
func (r *Reducer) Acknowledge(ack *a2a888.MachineAssignmentAck) error {
	if ack == nil || strings.TrimSpace(ack.GetMachineResourceId()) == "" {
		return pkgerrors.Wrap(ErrInvalidEvent, "acknowledgement machine is required")
	}
	if ack.GetMachineResourceId() != r.machineResourceID {
		return ErrMachineMismatch
	}
	cursor := ack.GetAcknowledgedThrough()
	if cursor == nil {
		return pkgerrors.Wrap(ErrInvalidEvent, "acknowledgement cursor is required")
	}
	if cursor.GetSequence() == 0 {
		if r.lastSequence != 0 {
			return pkgerrors.Wrap(ErrAckMismatch, "zero cursor cannot acknowledge applied events")
		}
		if cursor.GetEventId() != "" || cursor.GetIdempotencyKey() != "" {
			return pkgerrors.Wrap(ErrAckMismatch, "zero cursor cannot have event identity")
		}
	} else {
		if cursor.GetSequence() > r.lastSequence {
			return pkgerrors.Wrapf(ErrAckMismatch, "sequence %d is ahead of %d", cursor.GetSequence(), r.lastSequence)
		}
		event, ok := r.eventsBySequence[cursor.GetSequence()]
		if !ok || event.GetEventId() != cursor.GetEventId() || event.GetIdempotencyKey() != cursor.GetIdempotencyKey() {
			return ErrAckMismatch
		}
	}
	if r.lastAcknowledged != nil {
		if cursor.GetSequence() < r.lastAcknowledged.GetSequence() {
			return ErrAckRegression
		}
		if cursor.GetSequence() == r.lastAcknowledged.GetSequence() && !proto.Equal(r.lastAcknowledged, cursor) {
			return ErrAckMismatch
		}
	}
	r.lastAcknowledged = proto.CloneOf(cursor)
	return nil
}

func (r *Reducer) LastSequence() uint64 { return r.lastSequence }

func (r *Reducer) LastAcknowledged() *a2a888.AssignmentCursor {
	if r.lastAcknowledged == nil {
		return nil
	}
	return proto.CloneOf(r.lastAcknowledged)
}

func (r *Reducer) Assignment(agentResourceID string) (Assignment, bool) {
	assignment, ok := r.assignments[agentResourceID]
	if !ok {
		return Assignment{}, false
	}
	assignment.Config = cloneConfig(assignment.Config)
	return assignment, true
}

func (r *Reducer) ActiveAssignments() map[string]Assignment {
	out := make(map[string]Assignment, len(r.assignments))
	for k, v := range r.assignments {
		out[k] = Assignment{
			AgentResourceID: v.AgentResourceID,
			Config:          cloneConfig(v.Config),
		}
	}
	return out
}

func (r *Reducer) ActiveAgentIDs() []string {
	ids := make([]string, 0, len(r.assignments))
	for id := range r.assignments {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

func (r *Reducer) FullRosterRevision() string {
	ids := r.ActiveAgentIDs()
	var sb strings.Builder
	for _, id := range ids {
		asg := r.assignments[id]
		sb.WriteString(id)
		sb.WriteString("=")
		if asg.Config != nil {
			sb.WriteString(asg.Config.GetRevision())
		}
		sb.WriteString(";")
	}
	hash := sha256.Sum256([]byte(sb.String()))
	return "sha256:" + hex.EncodeToString(hash[:])
}

func validateEvent(event *a2a888.MachineAssignmentEvent, machineResourceID string) error {
	if event == nil {
		return pkgerrors.Wrap(ErrInvalidEvent, "event is required")
	}
	if strings.TrimSpace(machineResourceID) == "" || event.GetMachineResourceId() != machineResourceID {
		return ErrMachineMismatch
	}
	if strings.TrimSpace(event.GetAgentResourceId()) == "" || strings.TrimSpace(event.GetEventId()) == "" || strings.TrimSpace(event.GetIdempotencyKey()) == "" {
		return pkgerrors.Wrap(ErrInvalidEvent, "machine, Agent, event and idempotency identities are required")
	}
	if event.GetSequence() == 0 {
		return pkgerrors.Wrap(ErrInvalidEvent, "sequence must be positive")
	}
	if event.GetCreatedAt() == nil || event.GetCreatedAt().CheckValid() != nil {
		return pkgerrors.Wrap(ErrInvalidEvent, "created_at must be a valid timestamp")
	}
	switch event.GetEventType() {
	case a2a888.AssignmentEventType_CREATE, a2a888.AssignmentEventType_CONFIG_UPDATE:
		if event.GetConfig() == nil || strings.TrimSpace(event.GetConfig().GetRevision()) == "" || strings.TrimSpace(event.GetConfig().GetPayloadReference()) == "" || strings.TrimSpace(event.GetConfig().GetPayloadDigest()) == "" {
			return pkgerrors.Wrap(ErrInvalidEvent, "config revision, payload reference and payload digest are required")
		}
	case a2a888.AssignmentEventType_REMOVE:
		if event.GetConfig() != nil {
			return pkgerrors.Wrap(ErrInvalidEvent, "remove cannot carry config")
		}
	case a2a888.AssignmentEventType_ASSIGNMENT_EVENT_TYPE_UNSPECIFIED:
		fallthrough
	default:
		return pkgerrors.Wrapf(ErrInvalidEvent, "unsupported event type %d", event.GetEventType())
	}
	return nil
}

func cloneConfig(config *a2a888.AssignmentConfig) *a2a888.AssignmentConfig {
	if config == nil {
		return nil
	}
	return proto.CloneOf(config)
}
