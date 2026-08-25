package dispatcher

import (
	"context"

	pkgerrors "github.com/pkg/errors"

	"github.com/Ranxy/laelia/backend/generated-go/a2a888"
)

// AgentAvailabilityStatus represents the live readiness of an Agent for work dispatch.
type AgentAvailabilityStatus string

const (
	AgentAvailabilityReady     AgentAvailabilityStatus = "READY"
	AgentAvailabilityBusy      AgentAvailabilityStatus = "BUSY"
	AgentAvailabilitySaturated AgentAvailabilityStatus = "SATURATED"
	AgentAvailabilityStopped   AgentAvailabilityStatus = "STOPPED"
	AgentAvailabilityOffline   AgentAvailabilityStatus = "OFFLINE"
)

// HandleAssignmentReplay processes a Machine's request for missing assignment events.
func (d *Dispatcher) HandleAssignmentReplay(ctx context.Context, req *a2a888.MachineAssignmentReplayRequest) (*a2a888.MachineAssignmentReplayResponse, error) {
	if d.store == nil {
		return nil, pkgerrors.New("store not available")
	}
	return d.store.GetMachineAssignmentReplay(ctx, req)
}

// HandleAssignmentAck processes a cumulative assignment acknowledgement from a Machine.
func (d *Dispatcher) HandleAssignmentAck(ctx context.Context, ack *a2a888.MachineAssignmentAck) error {
	if d.store == nil {
		return pkgerrors.New("store not available")
	}
	return d.store.AcknowledgeMachineAssignment(ctx, ack)
}

// DispatchAssignmentEvent records an assignment event transactionally and pushes
// it to the connected Machine.
func (d *Dispatcher) DispatchAssignmentEvent(ctx context.Context, event *a2a888.MachineAssignmentEvent) (*a2a888.MachineAssignmentEvent, error) {
	if d.store == nil {
		return nil, pkgerrors.New("store not available")
	}
	recorded, err := d.store.RecordMachineAssignmentEvent(ctx, event)
	if err != nil {
		return nil, err
	}
	return recorded, nil
}

// IsAgentReadyForWork evaluates live connection, administrative state, and current
// concurrency to report whether an Agent can accept new tasks.
func (d *Dispatcher) IsAgentReadyForWork(ctx context.Context, agentID int) (bool, string) {
	if d.agentStopped(ctx, agentID) {
		return false, "agent is stopped or disabled"
	}

	sess, ok := d.registry.getAgent(agentID)
	if !ok {
		return false, "agent is offline"
	}

	if sess.machineID != 0 && !d.IsMachineConnected(sess.machineID) {
		return false, "hosting machine is offline"
	}

	sess.mu.Lock()
	hasCmd := sess.currentCmdID != ""
	sess.mu.Unlock()

	if hasCmd {
		return false, "agent is busy with an in-flight command"
	}

	return true, ""
}

// GetAgentAvailability returns the status string for Agent Directory inputs.
func (d *Dispatcher) GetAgentAvailability(ctx context.Context, agentID int) AgentAvailabilityStatus {
	ready, reason := d.IsAgentReadyForWork(ctx, agentID)
	if ready {
		return AgentAvailabilityReady
	}
	switch reason {
	case "agent is stopped or disabled":
		return AgentAvailabilityStopped
	case "agent is busy with an in-flight command":
		return AgentAvailabilityBusy
	default:
		return AgentAvailabilityOffline
	}
}
