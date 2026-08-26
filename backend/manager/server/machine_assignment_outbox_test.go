package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	a2a888 "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
	v1pb "github.com/tbdavid2019/888a2a/backend/generated-go/v1"
	"github.com/tbdavid2019/888a2a/backend/manager/component/dispatcher"
	"github.com/tbdavid2019/888a2a/backend/manager/store"
)

func TestHandleMachineAssignmentOutboxEventDeliversToMachine(t *testing.T) {
	d := dispatcher.New(nil)
	defer d.Stop()
	var received *v1pb.ManagerMachineStreamMessage
	d.RegisterMachine(1, "machine-1", func(msg *v1pb.ManagerMachineStreamMessage) error {
		received = msg
		return nil
	})

	assignment := &a2a888.MachineAssignmentEvent{
		MachineResourceId: "machine-1",
		AgentResourceId:   "agent-1",
		Sequence:          1,
		EventId:           "event-1",
		IdempotencyKey:    "idempotency-1",
		EventType:         a2a888.AssignmentEventType_CREATE,
	}
	encoded, err := protojson.Marshal(assignment)
	require.NoError(t, err)
	payload, err := json.Marshal(struct {
		Assignment json.RawMessage `json:"assignment"`
	}{Assignment: encoded})
	require.NoError(t, err)

	s := &Server{dispatcher: d}
	require.NoError(t, s.handleMachineAssignmentOutboxEvent(context.Background(), store.OutboxEvent{
		DurableEventEnvelope: store.DurableEventEnvelope{
			AggregateType: "machine_assignment",
			AggregateID:   "machine-1",
			Payload:       payload,
		},
	}))
	require.NotNil(t, received)
	require.Equal(t, assignment, received.GetAssignmentEvent())
}
