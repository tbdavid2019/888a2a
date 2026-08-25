package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMachineCapacityAndAvailability(t *testing.T) {
	c := newTestMachineClient(t, "machine-cap-1")
	c.SetMaxCapacity(4)

	// Initially no runners -> agent not available
	assert.False(t, c.IsAgentAvailable("agent-1"))

	// Add agent-1 and agent-2 runners
	c.runnersMu.Lock()
	c.runners["agent-1"] = &agentRunner{machine: c, agentID: "agent-1", agentName: "agents/agent-1"}
	c.runners["agent-2"] = &agentRunner{machine: c, agentID: "agent-2", agentName: "agents/agent-2"}
	c.runnersMu.Unlock()

	// Both agents available initially
	assert.True(t, c.IsAgentAvailable("agent-1"))
	assert.True(t, c.IsAgentAvailable("agent-2"))

	rep := c.GetCapacityReport()
	require.NotNil(t, rep)
	assert.Equal(t, "machine-cap-1", rep.MachineID)
	assert.Equal(t, 4, rep.MaxCapacity)
	assert.Equal(t, 2, rep.ActiveRunners)
	assert.Equal(t, 0, rep.BusyRunners)
	assert.Equal(t, AgentStateReady, rep.AgentAvailability["agent-1"].State)
	assert.Equal(t, AgentStateReady, rep.AgentAvailability["agent-2"].State)

	// Mark agent-1 as busy
	c.MarkAgentBusy("agent-1")
	assert.False(t, c.IsAgentAvailable("agent-1"))
	assert.True(t, c.IsAgentAvailable("agent-2"))

	rep = c.GetCapacityReport()
	assert.Equal(t, 1, rep.BusyRunners)
	assert.Equal(t, AgentStateBusy, rep.AgentAvailability["agent-1"].State)
	assert.Equal(t, AgentStateReady, rep.AgentAvailability["agent-2"].State)

	// Mark agent-1 as idle
	c.MarkAgentIdle("agent-1")
	assert.True(t, c.IsAgentAvailable("agent-1"))

	rep = c.GetCapacityReport()
	assert.Equal(t, 0, rep.BusyRunners)
	assert.Equal(t, AgentStateReady, rep.AgentAvailability["agent-1"].State)
}
