package client

import (
	"sync"
	"time"
)

// AgentAvailabilityState indicates an agent's readiness to accept new tasks.
type AgentAvailabilityState string

const (
	AgentStateReady     AgentAvailabilityState = "READY"
	AgentStateBusy      AgentAvailabilityState = "BUSY"
	AgentStateSaturated AgentAvailabilityState = "SATURATED"
	AgentStateStopped   AgentAvailabilityState = "STOPPED"
	AgentStateOffline   AgentAvailabilityState = "OFFLINE"
)

// AgentRuntimeAvailability records runtime readiness metrics for one agent.
type AgentRuntimeAvailability struct {
	AgentID          string                 `json:"agent_id"`
	State            AgentAvailabilityState `json:"state"`
	InFlightCommands int                    `json:"in_flight_commands"`
	MaxConcurrency   int                    `json:"max_concurrency"`
	LastActiveAt     time.Time              `json:"last_active_at"`
}

// MachineCapacityReport contains machine-level capacity and per-agent availability.
type MachineCapacityReport struct {
	MachineID         string                               `json:"machine_id"`
	MaxCapacity       int                                  `json:"max_capacity"`
	ActiveRunners     int                                  `json:"active_runners"`
	BusyRunners       int                                  `json:"busy_runners"`
	AgentAvailability map[string]*AgentRuntimeAvailability `json:"agent_availability"`
	ReportedAt        time.Time                            `json:"reported_at"`
}

// capacityTracker manages machine capacity limits and agent busy state.
type capacityTracker struct {
	mu          sync.RWMutex
	maxCapacity int
	busyAgents  map[string]int
	lastActive  map[string]time.Time
}

func newCapacityTracker(defaultMaxCapacity int) *capacityTracker {
	if defaultMaxCapacity <= 0 {
		defaultMaxCapacity = 16
	}
	return &capacityTracker{
		maxCapacity: defaultMaxCapacity,
		busyAgents:  make(map[string]int),
		lastActive:  make(map[string]time.Time),
	}
}

func (t *capacityTracker) setMaxCapacity(max int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if max > 0 {
		t.maxCapacity = max
	}
}

func (t *capacityTracker) markBusy(agentID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.busyAgents[agentID]++
	t.lastActive[agentID] = time.Now()
}

func (t *capacityTracker) markIdle(agentID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.busyAgents[agentID] > 0 {
		t.busyAgents[agentID]--
		if t.busyAgents[agentID] == 0 {
			delete(t.busyAgents, agentID)
		}
	}
	t.lastActive[agentID] = time.Now()
}

func (t *capacityTracker) isAgentBusy(agentID string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.busyAgents[agentID] > 0
}

// SetMaxCapacity sets the maximum concurrency capacity for this machine.
func (c *MachineClient) SetMaxCapacity(limit int) {
	if c.capacity != nil {
		c.capacity.setMaxCapacity(limit)
	}
}

// MarkAgentBusy marks an agent as currently executing a turn.
func (c *MachineClient) MarkAgentBusy(agentID string) {
	if c.capacity != nil {
		c.capacity.markBusy(bareAgentID(agentID))
	}
}

// MarkAgentIdle marks an agent as finished with its turn.
func (c *MachineClient) MarkAgentIdle(agentID string) {
	if c.capacity != nil {
		c.capacity.markIdle(bareAgentID(agentID))
	}
}

// IsAgentAvailable reports whether an agent runner is active and ready for work.
func (c *MachineClient) IsAgentAvailable(agentID string) bool {
	bareID := bareAgentID(agentID)
	c.runnersMu.Lock()
	_, running := c.runners[bareID]
	c.runnersMu.Unlock()

	if !running {
		return false
	}
	if c.capacity == nil {
		return true
	}
	return !c.capacity.isAgentBusy(bareID)
}

// GetCapacityReport generates a live capacity and availability report.
func (c *MachineClient) GetCapacityReport() *MachineCapacityReport {
	c.runnersMu.Lock()
	activeCount := len(c.runners)
	runnerIDs := make([]string, 0, activeCount)
	for id := range c.runners {
		runnerIDs = append(runnerIDs, id)
	}
	c.runnersMu.Unlock()

	maxCap := 16
	busyCount := 0
	availMap := make(map[string]*AgentRuntimeAvailability, len(runnerIDs))

	if c.capacity != nil {
		c.capacity.mu.RLock()
		maxCap = c.capacity.maxCapacity
		busyCount = len(c.capacity.busyAgents)
		for _, id := range runnerIDs {
			inFlight := c.capacity.busyAgents[id]
			st := AgentStateReady
			if inFlight > 0 {
				st = AgentStateBusy
			}
			availMap[id] = &AgentRuntimeAvailability{
				AgentID:          id,
				State:            st,
				InFlightCommands: inFlight,
				MaxConcurrency:   1,
				LastActiveAt:     c.capacity.lastActive[id],
			}
		}
		c.capacity.mu.RUnlock()
	} else {
		for _, id := range runnerIDs {
			availMap[id] = &AgentRuntimeAvailability{
				AgentID:          id,
				State:            AgentStateReady,
				InFlightCommands: 0,
				MaxConcurrency:   1,
			}
		}
	}

	return &MachineCapacityReport{
		MachineID:         c.machineID,
		MaxCapacity:       maxCap,
		ActiveRunners:     activeCount,
		BusyRunners:       busyCount,
		AgentAvailability: availMap,
		ReportedAt:        time.Now(),
	}
}
