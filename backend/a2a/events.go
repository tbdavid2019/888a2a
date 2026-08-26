package a2a

import (
	"context"
	"iter"
	"sync"

	"github.com/a2aproject/a2a-go/v2/a2a"

	"github.com/tbdavid2019/888a2a/backend/manager/store"
)

// EventStore is the subset of store needed for persisting and retrieving events.
type EventStore interface {
	AppendWorkEvent(ctx context.Context, event *store.WorkEventMessage) error
	ListWorkEvents(ctx context.Context, tenantID, workID string, afterSequence uint64, limit int) ([]*store.WorkEventMessage, error)
	GetLatestWorkEventSequence(ctx context.Context, tenantID, workID string) (uint64, error)
}

type eventItem struct {
	sequence uint64
	event    a2a.Event
}

type workChannel struct {
	subscribers map[chan eventItem]struct{}
	mu          sync.RWMutex
}

// EventManager coordinates durable event logging and live stream distribution.
type EventManager struct {
	store    EventStore
	channels map[string]*workChannel // key: tenant:workID
	mu       sync.RWMutex
}

// NewEventManager creates a new event manager.
func NewEventManager(store EventStore) *EventManager {
	return &EventManager{
		store:    store,
		channels: make(map[string]*workChannel),
	}
}

func channelKey(tenantID, workID string) string {
	if tenantID == "" {
		tenantID = "default"
	}
	return tenantID + ":" + workID
}

func (em *EventManager) getOrCreateChannel(tenantID, workID string) *workChannel {
	key := channelKey(tenantID, workID)
	em.mu.Lock()
	defer em.mu.Unlock()

	ch, ok := em.channels[key]
	if !ok {
		ch = &workChannel{
			subscribers: make(map[chan eventItem]struct{}),
		}
		em.channels[key] = ch
	}
	return ch
}

// Publish broadcasts an event to all live subscribers for a work record.
func (em *EventManager) Publish(tenantID, workID string, event a2a.Event, sequence uint64) {
	key := channelKey(tenantID, workID)
	em.mu.RLock()
	ch, ok := em.channels[key]
	em.mu.RUnlock()

	if !ok || ch == nil {
		return
	}

	ch.mu.RLock()
	defer ch.mu.RUnlock()

	item := eventItem{sequence: sequence, event: event}
	for sub := range ch.subscribers {
		select {
		case sub <- item:
		default:
			// Non-blocking drop if subscriber is too slow
		}
	}
}

// Subscribe streams events starting after fromSequence: first durable historical events, then live events.
func (em *EventManager) Subscribe(ctx context.Context, tenantID, workID string, fromSequence uint64) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		if tenantID == "" {
			tenantID = "default"
		}

		lastYieldedSeq := fromSequence

		// Step 1: Replay historical durable events from store
		if em.store != nil {
			histEvents, err := em.store.ListWorkEvents(ctx, tenantID, workID, fromSequence, 500)
			if err != nil {
				yield(nil, err)
				return
			}

			for _, e := range histEvents {
				event := mapWorkEventToA2AEvent(e)
				if event != nil {
					if !yield(event, nil) {
						return
					}
					lastYieldedSeq = e.Sequence
					if isTerminalEvent(event) {
						return
					}
				}
			}
		}

		// Step 2: Subscribe to live events
		ch := em.getOrCreateChannel(tenantID, workID)
		subChan := make(chan eventItem, 64)

		ch.mu.Lock()
		ch.subscribers[subChan] = struct{}{}
		ch.mu.Unlock()

		defer func() {
			ch.mu.Lock()
			delete(ch.subscribers, subChan)
			ch.mu.Unlock()
			close(subChan)
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case item, ok := <-subChan:
				if !ok {
					return
				}
				if item.sequence <= lastYieldedSeq {
					continue
				}
				if !yield(item.event, nil) {
					return
				}
				lastYieldedSeq = item.sequence

				// Check if this event reached a terminal state
				if isTerminalEvent(item.event) {
					return
				}
			}
		}
	}
}

func isTerminalEvent(event a2a.Event) bool {
	if event == nil {
		return false
	}
	switch e := event.(type) {
	case *a2a.TaskStatusUpdateEvent:
		return e.Status.State == a2a.TaskStateCompleted ||
			e.Status.State == a2a.TaskStateFailed ||
			e.Status.State == a2a.TaskStateCanceled ||
			e.Status.State == a2a.TaskStateRejected
	case *a2a.Task:
		return e.Status.State == a2a.TaskStateCompleted ||
			e.Status.State == a2a.TaskStateFailed ||
			e.Status.State == a2a.TaskStateCanceled ||
			e.Status.State == a2a.TaskStateRejected
	}
	return false
}

func mapWorkEventToA2AEvent(e *store.WorkEventMessage) a2a.Event {
	if e == nil {
		return nil
	}

	taskID := a2a.TaskID(e.WorkID)
	contextID := ""
	if e.Metadata != nil {
		contextID = e.Metadata["context_id"]
	}

	info := &a2a.TaskInfo{
		TaskID:    taskID,
		ContextID: contextID,
	}

	switch e.EventType {
	case "STATUS_UPDATE", "SUBMITTED", "WORKING", "COMPLETED", "FAILED", "CANCELED", "REJECTED":
		state := mapDurableStateToTaskState(e.EventType)
		if e.EventType == "STATUS_UPDATE" && e.PolicyDecision != "" {
			state = mapDurableStateToTaskState(e.PolicyDecision)
		}
		var msg *a2a.Message
		if e.TerminalReason != "" {
			msg = a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart(e.TerminalReason))
		}
		return a2a.NewStatusUpdateEvent(info, state, msg)

	case "ARTIFACT_UPDATE":
		var artifactID a2a.ArtifactID
		var parts []*a2a.Part
		if e.Metadata != nil {
			artifactID = a2a.ArtifactID(e.Metadata["artifact_id"])
			if uri, ok := e.Metadata["external_uri"]; ok && uri != "" {
				parts = append(parts, a2a.NewFileURLPart(a2a.URL(uri), e.Metadata["media_type"]))
			} else if text, ok := e.Metadata["text"]; ok && text != "" {
				parts = append(parts, a2a.NewTextPart(text))
			}
		}
		if artifactID != "" {
			return a2a.NewArtifactUpdateEvent(info, artifactID, parts...)
		}
		return a2a.NewArtifactEvent(info, parts...)

	case "MESSAGE":
		text := ""
		if e.Metadata != nil {
			text = e.Metadata["text"]
		}
		return a2a.NewMessageForTask(a2a.MessageRoleAgent, info, a2a.NewTextPart(text))
	}

	return a2a.NewStatusUpdateEvent(info, a2a.TaskStateWorking, nil)
}
