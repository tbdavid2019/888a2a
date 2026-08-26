package dispatcher

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	a2a888 "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
	v1pb "github.com/tbdavid2019/888a2a/backend/generated-go/v1"
)

func noopMachineSend(_ *v1pb.ManagerMachineStreamMessage) error { return nil }

func TestAgentReadinessAndAvailability(t *testing.T) {
	d := New(nil)
	defer d.Stop()

	// 1. Unregistered agent -> offline
	ready, reason := d.IsAgentReadyForWork(context.Background(), 10)
	assert.False(t, ready)
	assert.Equal(t, "agent is offline", reason)
	assert.Equal(t, AgentAvailabilityOffline, d.GetAgentAvailability(context.Background(), 10))

	// 2. Machine not connected -> hosting machine is offline
	sess := d.RegisterAgent(context.Background(), 10, 100, "agents/agent-10", noopSend)
	require.NotNil(t, sess)
	ready, reason = d.IsAgentReadyForWork(context.Background(), 10)
	assert.False(t, ready)
	assert.Equal(t, "hosting machine is offline", reason)
	assert.Equal(t, AgentAvailabilityOffline, d.GetAgentAvailability(context.Background(), 10))

	// 3. Register machine -> agent is ready
	mSess := d.RegisterMachine(100, "machine-100", noopMachineSend)
	require.NotNil(t, mSess)
	ready, reason = d.IsAgentReadyForWork(context.Background(), 10)
	assert.True(t, ready)
	assert.Empty(t, reason)
	assert.Equal(t, AgentAvailabilityReady, d.GetAgentAvailability(context.Background(), 10))

	// 4. Agent in-flight command -> busy
	sess.mu.Lock()
	sess.currentCmdID = "cmd-123"
	sess.mu.Unlock()

	ready, reason = d.IsAgentReadyForWork(context.Background(), 10)
	assert.False(t, ready)
	assert.Equal(t, "agent is busy with an in-flight command", reason)
	assert.Equal(t, AgentAvailabilityBusy, d.GetAgentAvailability(context.Background(), 10))

	// 5. Clear command -> ready again
	sess.mu.Lock()
	sess.currentCmdID = ""
	sess.mu.Unlock()

	ready, reason = d.IsAgentReadyForWork(context.Background(), 10)
	assert.True(t, ready)
	assert.Empty(t, reason)
	assert.Equal(t, AgentAvailabilityReady, d.GetAgentAvailability(context.Background(), 10))
}

func TestSendMachineAssignmentEventByResourceID(t *testing.T) {
	d := New(nil)
	defer d.Stop()

	var received *v1pb.ManagerMachineStreamMessage
	d.RegisterMachine(100, "machines/m1", func(msg *v1pb.ManagerMachineStreamMessage) error {
		received = msg
		return nil
	})

	event := &a2a888.MachineAssignmentEvent{
		MachineResourceId: "machines/m1",
		AgentResourceId:   "agents/a1",
		Sequence:          1,
		EventId:           "event-1",
		IdempotencyKey:    "idem-1",
		EventType:         a2a888.AssignmentEventType_CREATE,
	}
	require.NoError(t, d.SendMachineAssignmentEvent("machines/m1", event))
	require.NotNil(t, received)
	assignment := received.GetAssignmentEvent()
	require.NotNil(t, assignment)
	assert.Equal(t, event.GetEventId(), assignment.GetEventId())
	assert.Equal(t, event.GetSequence(), assignment.GetSequence())
}
