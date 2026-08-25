package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/Ranxy/laelia/backend/generated-go/a2a888"
)

func TestEventTypeConversions(t *testing.T) {
	assert.Equal(t, "CREATE", eventTypeToString(a2a888.AssignmentEventType_CREATE))
	assert.Equal(t, "CONFIG_UPDATE", eventTypeToString(a2a888.AssignmentEventType_CONFIG_UPDATE))
	assert.Equal(t, "REMOVE", eventTypeToString(a2a888.AssignmentEventType_REMOVE))
	assert.Equal(t, "", eventTypeToString(a2a888.AssignmentEventType_ASSIGNMENT_EVENT_TYPE_UNSPECIFIED))

	assert.Equal(t, a2a888.AssignmentEventType_CREATE, stringToEventType("CREATE"))
	assert.Equal(t, a2a888.AssignmentEventType_CONFIG_UPDATE, stringToEventType("CONFIG_UPDATE"))
	assert.Equal(t, a2a888.AssignmentEventType_REMOVE, stringToEventType("REMOVE"))
	assert.Equal(t, a2a888.AssignmentEventType_ASSIGNMENT_EVENT_TYPE_UNSPECIFIED, stringToEventType("UNKNOWN"))
}

func TestComputeFullRosterRevisionDeterministic(t *testing.T) {
	now := time.Now()
	events := []*a2a888.MachineAssignmentEvent{
		{
			MachineResourceId: "machine-1",
			AgentResourceId:   "agent-1",
			Sequence:          1,
			EventType:         a2a888.AssignmentEventType_CREATE,
			Config: &a2a888.AssignmentConfig{
				Revision: "rev-1",
			},
			CreatedAt: timestamppb.New(now),
		},
		{
			MachineResourceId: "machine-1",
			AgentResourceId:   "agent-2",
			Sequence:          2,
			EventType:         a2a888.AssignmentEventType_CREATE,
			Config: &a2a888.AssignmentConfig{
				Revision: "rev-2",
			},
			CreatedAt: timestamppb.New(now),
		},
		{
			MachineResourceId: "machine-1",
			AgentResourceId:   "agent-1",
			Sequence:          3,
			EventType:         a2a888.AssignmentEventType_CONFIG_UPDATE,
			Config: &a2a888.AssignmentConfig{
				Revision: "rev-1b",
			},
			CreatedAt: timestamppb.New(now),
		},
		{
			MachineResourceId: "machine-1",
			AgentResourceId:   "agent-2",
			Sequence:          4,
			EventType:         a2a888.AssignmentEventType_REMOVE,
			CreatedAt:         timestamppb.New(now),
		},
	}

	rev1 := ComputeFullRosterRevision(events)
	rev2 := ComputeFullRosterRevision(events)
	require.NotEmpty(t, rev1)
	assert.Equal(t, rev1, rev2)
	assert.True(t, len(rev1) > 10)

	// Removing agent-1 leaves empty active roster.
	events = append(events, &a2a888.MachineAssignmentEvent{
		MachineResourceId: "machine-1",
		AgentResourceId:   "agent-1",
		Sequence:          5,
		EventType:         a2a888.AssignmentEventType_REMOVE,
		CreatedAt:         timestamppb.New(now),
	})
	revEmpty := ComputeFullRosterRevision(events)
	assert.NotEqual(t, rev1, revEmpty)
}
