package client

import (
	"context"
	"log/slog"
	"strings"

	pkgerrors "github.com/pkg/errors"

	"github.com/Ranxy/laelia/backend/agent/assignment"
	"github.com/Ranxy/laelia/backend/agent/state"
	"github.com/Ranxy/laelia/backend/generated-go/a2a888"
	v1pb "github.com/Ranxy/laelia/backend/generated-go/v1"
)

// Reducer returns the machine's assignment reducer.
func (c *MachineClient) Reducer() *assignment.Reducer {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.reducer
}

// LastAcknowledgedCursor loads the last acknowledged assignment cursor from
// local persistent state (or reducer if state is uninitialized).
func (c *MachineClient) LastAcknowledgedCursor() *a2a888.AssignmentCursor {
	st, err := state.Load()
	if err == nil && st != nil && st.GetLastAckCursor() != nil {
		return st.GetLastAckCursor()
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.reducer != nil {
		return c.reducer.LastAcknowledged()
	}
	return nil
}

// ApplyAssignmentEvent applies one assignment event to the machine's state,
// manages the corresponding agent runner, and produces an acknowledgement cursor.
// Applying duplicate events is idempotent and does not create duplicate runners.
func (c *MachineClient) ApplyAssignmentEvent(ctx context.Context, event *a2a888.MachineAssignmentEvent) (*a2a888.MachineAssignmentAck, error) {
	if event == nil {
		return nil, pkgerrors.Wrap(assignment.ErrInvalidEvent, "event is required")
	}

	c.mu.Lock()
	if c.reducer == nil {
		c.reducer = assignment.NewReducer(c.machineID)
	}
	red := c.reducer
	c.mu.Unlock()

	if err := red.ApplyEvent(event); err != nil {
		return nil, err
	}

	agentID := bareAgentID(event.GetAgentResourceId())
	agentName := "agents/" + agentID

	switch event.GetEventType() {
	case a2a888.AssignmentEventType_CREATE, a2a888.AssignmentEventType_CONFIG_UPDATE:
		asg := &v1pb.AgentAssignment{
			AgentName:        agentName,
			AgentDisplayName: agentID,
			AcpConfig:        configFromAssignment(event.GetConfig()),
		}
		c.spawnOrUpdate(ctx, asg)

	case a2a888.AssignmentEventType_REMOVE:
		c.stopRunner(agentID)

	default:
		return nil, pkgerrors.Wrapf(assignment.ErrInvalidEvent, "unsupported event type %d", event.GetEventType())
	}

	ack := &a2a888.MachineAssignmentAck{
		MachineResourceId: c.machineID,
		AcknowledgedThrough: &a2a888.AssignmentCursor{
			Sequence:       event.GetSequence(),
			EventId:        event.GetEventId(),
			IdempotencyKey: event.GetIdempotencyKey(),
		},
	}

	if err := red.Acknowledge(ack); err != nil {
		return nil, err
	}

	if err := state.SaveAckCursor(ack.GetAcknowledgedThrough()); err != nil {
		slog.Warn("failed to persist assignment cursor", "error", err)
	}

	return ack, nil
}

// ApplyAssignmentReplay applies an ordered batch of missing events and reconciles
// the full roster against the authoritative high watermark and revision.
func (c *MachineClient) ApplyAssignmentReplay(ctx context.Context, replay *a2a888.MachineAssignmentReplayResponse) (*a2a888.MachineAssignmentAck, error) {
	if replay == nil {
		return nil, pkgerrors.Wrap(assignment.ErrInvalidEvent, "replay response is required")
	}

	var lastAck *a2a888.MachineAssignmentAck
	for _, event := range replay.GetEvents() {
		ack, err := c.ApplyAssignmentEvent(ctx, event)
		if err != nil {
			return nil, err
		}
		lastAck = ack
	}

	if err := c.ReconcileRoster(ctx, replay.GetAuthoritativeHighWatermark(), replay.GetFullRosterRevision()); err != nil {
		return nil, err
	}

	return lastAck, nil
}

// ReconcileRoster reconciles the machine's active runners with the authoritative
// assignment reducer state, terminating zombie runners and spawning missing agents.
func (c *MachineClient) ReconcileRoster(ctx context.Context, _ uint64, authoritativeRevision string) error {
	c.mu.Lock()
	if c.reducer == nil {
		c.reducer = assignment.NewReducer(c.machineID)
	}
	red := c.reducer
	c.mu.Unlock()

	activeAssignments := red.ActiveAssignments()

	// Terminate any running agents that are not in the authoritative active roster (missed deletes).
	c.runnersMu.Lock()
	activeRunnerIDs := make([]string, 0, len(c.runners))
	for id := range c.runners {
		activeRunnerIDs = append(activeRunnerIDs, id)
	}
	c.runnersMu.Unlock()

	for _, runnerID := range activeRunnerIDs {
		if _, exists := activeAssignments[runnerID]; !exists {
			slog.Info("reconciling missed delete for agent runner", "agentID", runnerID)
			c.stopRunner(runnerID)
		}
	}

	// Spawn or converge configuration for all active assignments.
	for agentID, asg := range activeAssignments {
		agentName := "agents/" + agentID
		protoAsg := &v1pb.AgentAssignment{
			AgentName:        agentName,
			AgentDisplayName: agentID,
			AcpConfig:        configFromAssignment(asg.Config),
		}
		c.spawnOrUpdate(ctx, protoAsg)
	}

	if authoritativeRevision != "" {
		localRevision := red.FullRosterRevision()
		if localRevision != authoritativeRevision {
			slog.Warn("full roster revision mismatch after reconciliation",
				"local", localRevision, "authoritative", authoritativeRevision)
		}
	}

	return nil
}

func configFromAssignment(config *a2a888.AssignmentConfig) *v1pb.AgentACPConfig {
	if config == nil {
		return nil
	}
	// Derive provider and executable mapping from assignment config payload reference.
	ref := config.GetPayloadReference()
	providerName := "custom"
	if strings.Contains(ref, "builtin-pi") || strings.Contains(ref, "pi") {
		providerName = "builtin-pi"
	} else if strings.Contains(ref, "codex") {
		providerName = "codex"
	} else if strings.Contains(ref, "opencode") {
		providerName = "opencode"
	} else if strings.Contains(ref, "claude") {
		providerName = "claude-code"
	}

	return &v1pb.AgentACPConfig{
		Provider:   providerName,
		Executable: ref,
		Model:      config.GetRevision(),
	}
}
