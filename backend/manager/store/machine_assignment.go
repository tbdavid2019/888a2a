package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"time"

	pkgerrors "github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/tbdavid2019/888a2a/backend/agent/assignment"
	"github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
)

// MachineAssignmentState represents the durable state and roster tracking for
// one Machine.
type MachineAssignmentState struct {
	MachineResourceID     string
	HighWatermark         uint64
	LastAckSequence       uint64
	LastAckEventID        string
	LastAckIdempotencyKey string
	FullRosterRevision    string
	UpdatedAt             time.Time
}

// RecordMachineAssignmentEvent persists an assignment event transactionally with
// monotonic sequence allocation and idempotency verification.
func (s *Store) RecordMachineAssignmentEvent(ctx context.Context, event *a2a888.MachineAssignmentEvent) (*a2a888.MachineAssignmentEvent, error) {
	if event == nil {
		return nil, pkgerrors.Wrap(assignment.ErrInvalidEvent, "event is required")
	}
	machineID := strings.TrimSpace(event.GetMachineResourceId())
	agentID := strings.TrimSpace(event.GetAgentResourceId())
	eventID := strings.TrimSpace(event.GetEventId())
	idempKey := strings.TrimSpace(event.GetIdempotencyKey())

	if machineID == "" || agentID == "" || eventID == "" || idempKey == "" {
		return nil, pkgerrors.Wrap(assignment.ErrInvalidEvent, "machine, Agent, event, and idempotency identities are required")
	}

	eventTypeStr := eventTypeToString(event.GetEventType())
	if eventTypeStr == "" {
		return nil, pkgerrors.Wrapf(assignment.ErrInvalidEvent, "unsupported event type %d", event.GetEventType())
	}

	var configRev, configRef, configDigest string
	if event.GetEventType() == a2a888.AssignmentEventType_CREATE || event.GetEventType() == a2a888.AssignmentEventType_CONFIG_UPDATE {
		cfg := event.GetConfig()
		if cfg == nil || strings.TrimSpace(cfg.GetRevision()) == "" || strings.TrimSpace(cfg.GetPayloadReference()) == "" || strings.TrimSpace(cfg.GetPayloadDigest()) == "" {
			return nil, pkgerrors.Wrap(assignment.ErrInvalidEvent, "config revision, payload reference and payload digest are required")
		}
		configRev = cfg.GetRevision()
		configRef = cfg.GetPayloadReference()
		configDigest = cfg.GetPayloadDigest()
	} else if event.GetEventType() == a2a888.AssignmentEventType_REMOVE {
		if event.GetConfig() != nil {
			return nil, pkgerrors.Wrap(assignment.ErrInvalidEvent, "remove cannot carry config")
		}
	}

	tx, err := s.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Check if an event with this event_id or idempotency_key already exists for this machine.
	var existingSeq uint64
	var existingEventID, existingIdempKey, existingAgentID, existingType string
	var existingRev, existingRef, existingDigest string
	var existingCreatedAt time.Time

	err = tx.QueryRowContext(ctx, `
		SELECT sequence, event_id, idempotency_key, agent_resource_id, event_type,
		       config_revision, config_payload_reference, config_payload_digest, created_at
		FROM a2a888_machine_assignment_event
		WHERE machine_resource_id = $1 AND (event_id = $2 OR idempotency_key = $3)
		LIMIT 1
	`, machineID, eventID, idempKey).Scan(
		&existingSeq, &existingEventID, &existingIdempKey, &existingAgentID, &existingType,
		&existingRev, &existingRef, &existingDigest, &existingCreatedAt,
	)

	if err == nil {
		// Existing event found: verify it is an exact match for idempotency.
		if existingEventID == eventID && existingIdempKey == idempKey &&
			existingAgentID == agentID && existingType == eventTypeStr &&
			existingRev == configRev && existingRef == configRef && existingDigest == configDigest {
			if err := tx.Commit(); err != nil {
				return nil, err
			}
			return buildAssignmentEventProto(machineID, existingSeq, existingEventID, existingIdempKey, existingAgentID,
				stringToEventType(existingType), existingRev, existingRef, existingDigest, existingCreatedAt), nil
		}
		return nil, pkgerrors.Wrap(assignment.ErrInvalidEvent, "conflicting event with existing idempotency key or event ID")
	} else if err != sql.ErrNoRows {
		return nil, err
	}

	// Compute next monotonic sequence for this machine.
	var maxSeq uint64
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(sequence), 0)
		FROM a2a888_machine_assignment_event
		WHERE machine_resource_id = $1
	`, machineID).Scan(&maxSeq); err != nil {
		return nil, err
	}

	nextSeq := maxSeq + 1
	if event.GetSequence() > 0 {
		if event.GetSequence() <= maxSeq {
			return nil, pkgerrors.Wrapf(assignment.ErrSequenceRegression, "sequence %d is <= current high watermark %d", event.GetSequence(), maxSeq)
		}
		if event.GetSequence() != nextSeq {
			return nil, pkgerrors.Wrapf(assignment.ErrSequenceGap, "sequence %d got after %d", event.GetSequence(), maxSeq)
		}
	}

	createdAt := time.Now()
	if event.GetCreatedAt() != nil && event.GetCreatedAt().CheckValid() == nil {
		createdAt = event.GetCreatedAt().AsTime()
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO a2a888_machine_assignment_event (
			machine_resource_id, sequence, event_id, idempotency_key, agent_resource_id,
			event_type, config_revision, config_payload_reference, config_payload_digest, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, machineID, nextSeq, eventID, idempKey, agentID, eventTypeStr, configRev, configRef, configDigest, createdAt); err != nil {
		return nil, err
	}

	// Fetch all events for the machine to compute active roster and full roster revision.
	allEvents, err := listMachineAssignmentEventsTx(ctx, tx, machineID, 0, 0)
	if err != nil {
		return nil, err
	}

	fullRosterRev := ComputeFullRosterRevision(allEvents)

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO a2a888_machine_assignment_state (
			machine_resource_id, high_watermark, full_roster_revision, updated_at
		) VALUES ($1, $2, $3, now())
		ON CONFLICT (machine_resource_id)
		DO UPDATE SET high_watermark = EXCLUDED.high_watermark,
		              full_roster_revision = EXCLUDED.full_roster_revision,
		              updated_at = now()
	`, machineID, nextSeq, fullRosterRev); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return buildAssignmentEventProto(machineID, nextSeq, eventID, idempKey, agentID,
		event.GetEventType(), configRev, configRef, configDigest, createdAt), nil
}

// GetMachineAssignmentEvents returns ordered assignment events for a Machine starting
// after afterSequence.
func (s *Store) GetMachineAssignmentEvents(ctx context.Context, machineResourceID string, afterSequence uint64, limit int) ([]*a2a888.MachineAssignmentEvent, error) {
	tx, err := s.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	events, err := listMachineAssignmentEventsTx(ctx, tx, machineResourceID, afterSequence, limit)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return events, nil
}

// GetMachineAssignmentReplay generates the replay response containing missing
// events in order and authoritative high watermark and roster revision.
func (s *Store) GetMachineAssignmentReplay(ctx context.Context, req *a2a888.MachineAssignmentReplayRequest) (*a2a888.MachineAssignmentReplayResponse, error) {
	if req == nil || strings.TrimSpace(req.GetMachineResourceId()) == "" {
		return nil, pkgerrors.Wrap(assignment.ErrInvalidEvent, "machine_resource_id is required")
	}
	machineID := req.GetMachineResourceId()

	tx, err := s.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var afterSeq uint64
	cursor := req.GetLastAcknowledged()
	if cursor != nil && cursor.GetSequence() > 0 {
		var seq uint64
		var eventID, idempKey string
		err := tx.QueryRowContext(ctx, `
			SELECT sequence, event_id, idempotency_key
			FROM a2a888_machine_assignment_event
			WHERE machine_resource_id = $1 AND sequence = $2
		`, machineID, cursor.GetSequence()).Scan(&seq, &eventID, &idempKey)
		if err == sql.ErrNoRows {
			return nil, pkgerrors.Wrapf(assignment.ErrAckMismatch, "sequence %d does not exist for machine %q", cursor.GetSequence(), machineID)
		} else if err != nil {
			return nil, err
		}
		if eventID != cursor.GetEventId() || idempKey != cursor.GetIdempotencyKey() {
			return nil, pkgerrors.Wrapf(assignment.ErrAckMismatch, "cursor identity mismatch at sequence %d", cursor.GetSequence())
		}
		afterSeq = cursor.GetSequence()
	}

	events, err := listMachineAssignmentEventsTx(ctx, tx, machineID, afterSeq, 0)
	if err != nil {
		return nil, err
	}

	var highWatermark uint64
	var fullRosterRevision string
	err = tx.QueryRowContext(ctx, `
		SELECT high_watermark, full_roster_revision
		FROM a2a888_machine_assignment_state
		WHERE machine_resource_id = $1
	`, machineID).Scan(&highWatermark, &fullRosterRevision)
	if err == sql.ErrNoRows {
		// Calculate dynamically if state row is not yet written.
		allEvents, err := listMachineAssignmentEventsTx(ctx, tx, machineID, 0, 0)
		if err != nil {
			return nil, err
		}
		if len(allEvents) > 0 {
			highWatermark = allEvents[len(allEvents)-1].GetSequence()
		}
		fullRosterRevision = ComputeFullRosterRevision(allEvents)
	} else if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &a2a888.MachineAssignmentReplayResponse{
		MachineResourceId:          machineID,
		Events:                     events,
		AuthoritativeHighWatermark: highWatermark,
		FullRosterRevision:         fullRosterRevision,
	}, nil
}

// AcknowledgeMachineAssignment updates the persistent assignment cursor for a Machine.
func (s *Store) AcknowledgeMachineAssignment(ctx context.Context, ack *a2a888.MachineAssignmentAck) error {
	if ack == nil || strings.TrimSpace(ack.GetMachineResourceId()) == "" {
		return pkgerrors.Wrap(assignment.ErrInvalidEvent, "machine_resource_id is required")
	}
	machineID := ack.GetMachineResourceId()
	cursor := ack.GetAcknowledgedThrough()
	if cursor == nil {
		return pkgerrors.Wrap(assignment.ErrInvalidEvent, "acknowledged_through cursor is required")
	}

	tx, err := s.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if cursor.GetSequence() == 0 {
		if cursor.GetEventId() != "" || cursor.GetIdempotencyKey() != "" {
			return pkgerrors.Wrap(assignment.ErrAckMismatch, "zero cursor cannot have event identity")
		}
	} else {
		var storedEventID, storedIdempKey string
		err := tx.QueryRowContext(ctx, `
			SELECT event_id, idempotency_key
			FROM a2a888_machine_assignment_event
			WHERE machine_resource_id = $1 AND sequence = $2
		`, machineID, cursor.GetSequence()).Scan(&storedEventID, &storedIdempKey)
		if err == sql.ErrNoRows {
			return pkgerrors.Wrapf(assignment.ErrAckMismatch, "sequence %d not found for machine %q", cursor.GetSequence(), machineID)
		} else if err != nil {
			return err
		}
		if storedEventID != cursor.GetEventId() || storedIdempKey != cursor.GetIdempotencyKey() {
			return pkgerrors.Wrap(assignment.ErrAckMismatch, "cursor event ID or idempotency key mismatch")
		}
	}

	var currentAckSeq uint64
	var currentAckEventID, currentAckIdempKey string
	err = tx.QueryRowContext(ctx, `
		SELECT last_ack_sequence, last_ack_event_id, last_ack_idempotency_key
		FROM a2a888_machine_assignment_state
		WHERE machine_resource_id = $1
	`, machineID).Scan(&currentAckSeq, &currentAckEventID, &currentAckIdempKey)

	if err == nil {
		if cursor.GetSequence() < currentAckSeq {
			return assignment.ErrAckRegression
		}
		if cursor.GetSequence() == currentAckSeq {
			if cursor.GetEventId() != currentAckEventID || cursor.GetIdempotencyKey() != currentAckIdempKey {
				return assignment.ErrAckMismatch
			}
			return tx.Commit() // Idempotent duplicate ack.
		}
	} else if err != sql.ErrNoRows {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO a2a888_machine_assignment_state (
			machine_resource_id, last_ack_sequence, last_ack_event_id, last_ack_idempotency_key, updated_at
		) VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (machine_resource_id)
		DO UPDATE SET last_ack_sequence = EXCLUDED.last_ack_sequence,
		              last_ack_event_id = EXCLUDED.last_ack_event_id,
		              last_ack_idempotency_key = EXCLUDED.last_ack_idempotency_key,
		              updated_at = now()
	`, machineID, cursor.GetSequence(), cursor.GetEventId(), cursor.GetIdempotencyKey()); err != nil {
		return err
	}

	return tx.Commit()
}

// GetMachineAssignmentState retrieves the recorded state for a Machine.
func (s *Store) GetMachineAssignmentState(ctx context.Context, machineResourceID string) (*MachineAssignmentState, error) {
	var state MachineAssignmentState
	state.MachineResourceID = machineResourceID

	err := s.GetDB().QueryRowContext(ctx, `
		SELECT high_watermark, last_ack_sequence, last_ack_event_id, last_ack_idempotency_key, full_roster_revision, updated_at
		FROM a2a888_machine_assignment_state
		WHERE machine_resource_id = $1
	`, machineResourceID).Scan(
		&state.HighWatermark,
		&state.LastAckSequence,
		&state.LastAckEventID,
		&state.LastAckIdempotencyKey,
		&state.FullRosterRevision,
		&state.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &state, nil
}

// ComputeFullRosterRevision computes a deterministic hash of the active Agent roster.
func ComputeFullRosterRevision(events []*a2a888.MachineAssignmentEvent) string {
	active := make(map[string]string)
	for _, e := range events {
		agentID := e.GetAgentResourceId()
		switch e.GetEventType() {
		case a2a888.AssignmentEventType_CREATE, a2a888.AssignmentEventType_CONFIG_UPDATE:
			if e.GetConfig() != nil {
				active[agentID] = e.GetConfig().GetRevision()
			}
		case a2a888.AssignmentEventType_REMOVE:
			delete(active, agentID)
		default:
			continue
		}
	}

	keys := make([]string, 0, len(active))
	for k := range active {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(active[k])
		sb.WriteString(";")
	}

	hash := sha256.Sum256([]byte(sb.String()))
	return "sha256:" + hex.EncodeToString(hash[:])
}

func listMachineAssignmentEventsTx(ctx context.Context, tx *sql.Tx, machineResourceID string, afterSeq uint64, limit int) ([]*a2a888.MachineAssignmentEvent, error) {
	query := `
		SELECT sequence, event_id, idempotency_key, agent_resource_id, event_type,
		       config_revision, config_payload_reference, config_payload_digest, created_at
		FROM a2a888_machine_assignment_event
		WHERE machine_resource_id = $1 AND sequence > $2
		ORDER BY sequence ASC
	`
	args := []any{machineResourceID, afterSeq}
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*a2a888.MachineAssignmentEvent
	for rows.Next() {
		var seq uint64
		var eventID, idempKey, agentID, eventTypeStr string
		var configRev, configRef, configDigest string
		var createdAt time.Time

		if err := rows.Scan(
			&seq, &eventID, &idempKey, &agentID, &eventTypeStr,
			&configRev, &configRef, &configDigest, &createdAt,
		); err != nil {
			return nil, err
		}

		events = append(events, buildAssignmentEventProto(
			machineResourceID, seq, eventID, idempKey, agentID,
			stringToEventType(eventTypeStr), configRev, configRef, configDigest, createdAt,
		))
	}
	return events, rows.Err()
}

func buildAssignmentEventProto(machineID string, seq uint64, eventID, idempKey, agentID string,
	eventType a2a888.AssignmentEventType, rev, ref, digest string, createdAt time.Time) *a2a888.MachineAssignmentEvent {
	e := &a2a888.MachineAssignmentEvent{
		MachineResourceId: machineID,
		AgentResourceId:   agentID,
		Sequence:          seq,
		EventId:           eventID,
		IdempotencyKey:    idempKey,
		EventType:         eventType,
		CreatedAt:         timestamppb.New(createdAt),
	}
	if rev != "" || ref != "" || digest != "" {
		e.Config = &a2a888.AssignmentConfig{
			Revision:         rev,
			PayloadReference: ref,
			PayloadDigest:    digest,
		}
	}
	return e
}

func eventTypeToString(t a2a888.AssignmentEventType) string {
	switch t {
	case a2a888.AssignmentEventType_CREATE:
		return "CREATE"
	case a2a888.AssignmentEventType_CONFIG_UPDATE:
		return "CONFIG_UPDATE"
	case a2a888.AssignmentEventType_REMOVE:
		return "REMOVE"
	default:
		return ""
	}
}

func stringToEventType(s string) a2a888.AssignmentEventType {
	switch s {
	case "CREATE":
		return a2a888.AssignmentEventType_CREATE
	case "CONFIG_UPDATE":
		return a2a888.AssignmentEventType_CONFIG_UPDATE
	case "REMOVE":
		return a2a888.AssignmentEventType_REMOVE
	default:
		return a2a888.AssignmentEventType_ASSIGNMENT_EVENT_TYPE_UNSPECIFIED
	}
}
