package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"

	a2a888 "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
	"github.com/tbdavid2019/888a2a/backend/manager/component/tenantqueue"
	"github.com/tbdavid2019/888a2a/backend/manager/store"
)

type machineAssignmentOutboxPayload struct {
	Assignment json.RawMessage `json:"assignment"`
}

func (s *Server) handleMachineAssignmentOutboxEvent(_ context.Context, event store.OutboxEvent) error {
	if event.AggregateType != "machine_assignment" || event.AggregateID == "" {
		return errors.New("unsupported outbox event for machine assignment handler")
	}

	var payload machineAssignmentOutboxPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return errors.Wrap(err, "decode machine assignment outbox payload")
	}
	if len(payload.Assignment) == 0 {
		return errors.New("machine assignment outbox payload is missing assignment")
	}

	var assignment a2a888.MachineAssignmentEvent
	if err := protojson.Unmarshal(payload.Assignment, &assignment); err != nil {
		return errors.Wrap(err, "decode machine assignment event")
	}
	if assignment.GetMachineResourceId() == "" {
		assignment.MachineResourceId = event.AggregateID
	}
	if assignment.GetMachineResourceId() != event.AggregateID {
		return errors.New("machine assignment aggregate does not match event identity")
	}
	if s.dispatcher == nil {
		return errors.New("machine assignment dispatcher is unavailable")
	}
	return s.dispatcher.SendMachineAssignmentEvent(event.AggregateID, &assignment)
}

func (s *Server) newMachineAssignmentOutboxWorker() *store.OutboxWorker {
	hostname, _ := os.Hostname()
	workerID := fmt.Sprintf("manager-%s-%d", strings.TrimSpace(hostname), os.Getpid())
	return &store.OutboxWorker{
		Repository: s.store,
		WorkerID:   workerID,
		BatchSize:  64,
		Queue:      tenantqueue.NewQueue(128, 512),
		Limiter:    tenantqueue.NewLimiter(32, 128),
		RetryDelay: func(attempts int) time.Duration {
			if attempts < 1 {
				attempts = 1
			}
			return time.Duration(1<<min(attempts, 6)) * time.Second
		},
		Handle: s.handleMachineAssignmentOutboxEvent,
	}
}
