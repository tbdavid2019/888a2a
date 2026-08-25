package client

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/Ranxy/laelia/backend/agent/assignment"
	daemonsrv "github.com/Ranxy/laelia/backend/agent/daemon"
	"github.com/Ranxy/laelia/backend/agent/state"
	"github.com/Ranxy/laelia/backend/generated-go/a2a888"
)

func newTestMachineClient(t *testing.T, machineID string) *MachineClient {
	tempHome := t.TempDir()
	t.Setenv("A2A888_HOME", tempHome)

	c := &MachineClient{
		machineID: machineID,
		runners:   make(map[string]*agentRunner),
		reducer:   assignment.NewReducer(machineID),
		capacity:  newCapacityTracker(16),
		daemon:    &daemonsrv.Server{},
	}
	_ = state.Save(&state.State{
		MachineID: machineID,
		CreatedAt: time.Now(),
	})
	return c
}

func testAssignmentEvent(machineID string, seq uint64, eventID, idempKey, agentID string,
	eventType a2a888.AssignmentEventType, rev string) *a2a888.MachineAssignmentEvent {
	e := &a2a888.MachineAssignmentEvent{
		MachineResourceId: machineID,
		AgentResourceId:   agentID,
		Sequence:          seq,
		EventId:           eventID,
		IdempotencyKey:    idempKey,
		EventType:         eventType,
		CreatedAt:         timestamppb.New(time.Now()),
	}
	if rev != "" {
		e.Config = &a2a888.AssignmentConfig{
			Revision:         rev,
			PayloadReference: "pi/" + rev,
			PayloadDigest:    "sha256:123456",
		}
	}
	return e
}

func TestApplyAssignmentEvent_CreateConfigRemove(t *testing.T) {
	c := newTestMachineClient(t, "machine-1")
	ctx := context.Background()

	// 1. CREATE event for agent-1
	e1 := testAssignmentEvent("machine-1", 1, "evt-1", "key-1", "agent-1", a2a888.AssignmentEventType_CREATE, "v1")
	ack1, err := c.ApplyAssignmentEvent(ctx, e1)
	require.NoError(t, err)
	require.NotNil(t, ack1)
	assert.Equal(t, uint64(1), ack1.GetAcknowledgedThrough().GetSequence())

	c.runnersMu.Lock()
	r1, exists1 := c.runners["agent-1"]
	c.runnersMu.Unlock()
	require.True(t, exists1)
	require.NotNil(t, r1)

	// 2. Duplicate CREATE event is idempotent (no duplicate runner created)
	ack1Dup, err := c.ApplyAssignmentEvent(ctx, e1)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), ack1Dup.GetAcknowledgedThrough().GetSequence())

	c.runnersMu.Lock()
	assert.Equal(t, 1, len(c.runners))
	c.runnersMu.Unlock()

	// 3. CONFIG_UPDATE event for agent-1
	e2 := testAssignmentEvent("machine-1", 2, "evt-2", "key-2", "agent-1", a2a888.AssignmentEventType_CONFIG_UPDATE, "v2")
	ack2, err := c.ApplyAssignmentEvent(ctx, e2)
	require.NoError(t, err)
	assert.Equal(t, uint64(2), ack2.GetAcknowledgedThrough().GetSequence())

	// 4. REMOVE event for agent-1
	e3 := testAssignmentEvent("machine-1", 3, "evt-3", "key-3", "agent-1", a2a888.AssignmentEventType_REMOVE, "")
	ack3, err := c.ApplyAssignmentEvent(ctx, e3)
	require.NoError(t, err)
	assert.Equal(t, uint64(3), ack3.GetAcknowledgedThrough().GetSequence())

	c.runnersMu.Lock()
	_, existsAfterRemove := c.runners["agent-1"]
	c.runnersMu.Unlock()
	assert.False(t, existsAfterRemove)
}

func TestApplyAssignmentReplayAndReconciliation(t *testing.T) {
	c := newTestMachineClient(t, "machine-1")
	ctx := context.Background()

	// Add an untracked zombie runner that shouldn't exist
	c.runnersMu.Lock()
	c.runners["zombie-agent"] = &agentRunner{
		machine:   c,
		agentName: "agents/zombie-agent",
		agentID:   "zombie-agent",
	}
	c.runnersMu.Unlock()

	events := []*a2a888.MachineAssignmentEvent{
		testAssignmentEvent("machine-1", 1, "evt-1", "key-1", "agent-1", a2a888.AssignmentEventType_CREATE, "v1"),
		testAssignmentEvent("machine-1", 2, "evt-2", "key-2", "agent-2", a2a888.AssignmentEventType_CREATE, "v1"),
		testAssignmentEvent("machine-1", 3, "evt-3", "key-3", "agent-3", a2a888.AssignmentEventType_CREATE, "v1"),
		testAssignmentEvent("machine-1", 4, "evt-4", "key-4", "agent-2", a2a888.AssignmentEventType_REMOVE, ""),
	}

	replay := &a2a888.MachineAssignmentReplayResponse{
		MachineResourceId:          "machine-1",
		Events:                     events,
		AuthoritativeHighWatermark: 4,
		FullRosterRevision:         c.reducer.FullRosterRevision(),
	}

	ack, err := c.ApplyAssignmentReplay(ctx, replay)
	require.NoError(t, err)
	require.NotNil(t, ack)
	assert.Equal(t, uint64(4), ack.GetAcknowledgedThrough().GetSequence())

	c.runnersMu.Lock()
	defer c.runnersMu.Unlock()

	// zombie-agent was reaped
	assert.NotContains(t, c.runners, "zombie-agent")
	// agent-2 was removed
	assert.NotContains(t, c.runners, "agent-2")
	// agent-1 and agent-3 exist
	assert.Contains(t, c.runners, "agent-1")
	assert.Contains(t, c.runners, "agent-3")
	assert.Equal(t, 2, len(c.runners))
}

func TestReconcileRosterConvergesStaleConfigs(t *testing.T) {
	c := newTestMachineClient(t, "machine-1")
	ctx := context.Background()

	e1 := testAssignmentEvent("machine-1", 1, "evt-1", "key-1", "agent-1", a2a888.AssignmentEventType_CREATE, "v1")
	_, err := c.ApplyAssignmentEvent(ctx, e1)
	require.NoError(t, err)

	// Reconcile with expected state
	err = c.ReconcileRoster(ctx, 1, c.reducer.FullRosterRevision())
	require.NoError(t, err)

	c.runnersMu.Lock()
	assert.Contains(t, c.runners, "agent-1")
	c.runnersMu.Unlock()
}
