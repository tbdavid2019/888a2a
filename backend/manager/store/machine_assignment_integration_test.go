package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	a2a888 "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
)

func TestMachineAssignmentOutboxReplayAndAck(t *testing.T) {
	stores := requireIntegrationStore(t)
	ctx := context.Background()

	machine, err := stores.CreateMachine(ctx, &MachineMessage{Name: "assignment-integration-machine"})
	require.NoError(t, err)
	require.NotEmpty(t, machine.ResourceID)

	events := []*a2a888.MachineAssignmentEvent{
		{
			MachineResourceId: machine.ResourceID,
			AgentResourceId:   "agent-integration-1",
			EventId:           "assignment-create-1",
			IdempotencyKey:    "assignment-idempotency-create-1",
			EventType:         a2a888.AssignmentEventType_CREATE,
			Config: &a2a888.AssignmentConfig{
				Revision:         "revision-1",
				PayloadReference: "runtime/agent-integration-1",
				PayloadDigest:    "sha256:assignment-1",
			},
			CreatedAt: timestamppb.New(time.Now().UTC()),
		},
		{
			MachineResourceId: machine.ResourceID,
			AgentResourceId:   "agent-integration-1",
			EventId:           "assignment-update-1",
			IdempotencyKey:    "assignment-idempotency-update-1",
			EventType:         a2a888.AssignmentEventType_CONFIG_UPDATE,
			Config: &a2a888.AssignmentConfig{
				Revision:         "revision-2",
				PayloadReference: "runtime/agent-integration-1",
				PayloadDigest:    "sha256:assignment-2",
			},
			CreatedAt: timestamppb.New(time.Now().UTC()),
		},
		{
			MachineResourceId: machine.ResourceID,
			AgentResourceId:   "agent-integration-1",
			EventId:           "assignment-remove-1",
			IdempotencyKey:    "assignment-idempotency-remove-1",
			EventType:         a2a888.AssignmentEventType_REMOVE,
			CreatedAt:         timestamppb.New(time.Now().UTC()),
		},
	}

	for index, event := range events {
		recorded, recordErr := stores.RecordMachineAssignmentEvent(ctx, event)
		require.NoError(t, recordErr, "record event %d", index)
		require.Equal(t, uint64(index+1), recorded.GetSequence())
	}

	// Retrying the same request is idempotent and does not add another outbox
	// delivery intent.
	recorded, err := stores.RecordMachineAssignmentEvent(ctx, events[0])
	require.NoError(t, err)
	require.Equal(t, uint64(1), recorded.GetSequence())

	claimed, err := stores.ClaimOutboxEvents(ctx, "assignment-integration-worker", 10)
	require.NoError(t, err)
	require.Len(t, claimed, 3)

	replay, err := stores.GetMachineAssignmentReplay(ctx, &a2a888.MachineAssignmentReplayRequest{
		MachineResourceId: machine.ResourceID,
	})
	require.NoError(t, err)
	require.Len(t, replay.GetEvents(), 3)
	require.Equal(t, uint64(3), replay.GetAuthoritativeHighWatermark())
	require.Equal(t, a2a888.AssignmentEventType_CREATE, replay.GetEvents()[0].GetEventType())
	require.Equal(t, a2a888.AssignmentEventType_CONFIG_UPDATE, replay.GetEvents()[1].GetEventType())
	require.Equal(t, a2a888.AssignmentEventType_REMOVE, replay.GetEvents()[2].GetEventType())

	last := replay.GetEvents()[2]
	require.NoError(t, stores.AcknowledgeMachineAssignment(ctx, &a2a888.MachineAssignmentAck{
		MachineResourceId: machine.ResourceID,
		AcknowledgedThrough: &a2a888.AssignmentCursor{
			Sequence:       last.GetSequence(),
			EventId:        last.GetEventId(),
			IdempotencyKey: last.GetIdempotencyKey(),
		},
	}))

	postAck, err := stores.GetMachineAssignmentReplay(ctx, &a2a888.MachineAssignmentReplayRequest{
		MachineResourceId: machine.ResourceID,
		LastAcknowledged: &a2a888.AssignmentCursor{
			Sequence:       last.GetSequence(),
			EventId:        last.GetEventId(),
			IdempotencyKey: last.GetIdempotencyKey(),
		},
	})
	require.NoError(t, err)
	require.Empty(t, postAck.GetEvents())

	for _, event := range claimed {
		require.NoError(t, stores.AckOutboxEvent(ctx, "assignment-integration-worker", event.EventID))
	}
}
