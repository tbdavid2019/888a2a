package a2a

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

var ErrHubInboxNotFound = errors.New("Hub inbox item not found")
var ErrHubTaskConcurrencyLimit = errors.New("Hub task concurrency limit reached")

// HubInboxItem is a durable peer mailbox entry. It carries task data only;
// authentication material and native provider session IDs never belong here.
type HubInboxItem struct {
	Sequence         uint64     `json:"sequence"`
	HubID            string     `json:"hubId"`
	TargetAgentID    string     `json:"targetAgentId"`
	RequesterAgentID string     `json:"requesterAgentId"`
	TaskID           string     `json:"taskId"`
	ContextID        string     `json:"contextId"`
	IdempotencyKey   string     `json:"idempotencyKey"`
	Message          string     `json:"message"`
	CreatedAt        time.Time  `json:"createdAt"`
	AcknowledgedAt   *time.Time `json:"acknowledgedAt,omitempty"`
}

type HubInboxEnqueueResult struct {
	Item      HubInboxItem `json:"item"`
	Duplicate bool         `json:"duplicate"`
}

// HubMailbox is the persistence boundary for peer delivery. Implementations
// must make Enqueue idempotent on hub/target/requester/idempotency key.
type HubMailbox interface {
	Enqueue(context.Context, HubInboxItem) (HubInboxEnqueueResult, error)
	Find(context.Context, string, string, string, string) (HubInboxItem, bool, error)
	PendingCount(context.Context, string) (int, error)
	Poll(context.Context, string, string, uint64, int) ([]HubInboxItem, error)
	Acknowledge(context.Context, string, string, uint64) error
	Cancel(context.Context, string, string, time.Time) error
}

type MemoryHubMailbox struct {
	mu      sync.Mutex
	nextSeq uint64
	items   []HubInboxItem
}

func NewMemoryHubMailbox() *MemoryHubMailbox { return &MemoryHubMailbox{nextSeq: 1} }

func (m *MemoryHubMailbox) Enqueue(_ context.Context, item HubInboxItem) (HubInboxEnqueueResult, error) {
	if item.HubID == "" || item.TargetAgentID == "" || item.RequesterAgentID == "" || item.TaskID == "" || item.ContextID == "" || item.IdempotencyKey == "" || item.Message == "" {
		return HubInboxEnqueueResult{}, errors.New("Hub inbox item identity and message are required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.items {
		if existing.HubID == item.HubID && existing.TargetAgentID == item.TargetAgentID && existing.RequesterAgentID == item.RequesterAgentID && existing.IdempotencyKey == item.IdempotencyKey {
			return HubInboxEnqueueResult{Item: existing, Duplicate: true}, nil
		}
	}
	item.Sequence = m.nextSeq
	m.nextSeq++
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now().UTC()
	}
	m.items = append(m.items, item)
	return HubInboxEnqueueResult{Item: item}, nil
}

func (m *MemoryHubMailbox) Find(_ context.Context, hubID, targetAgentID, requesterAgentID, idempotencyKey string) (HubInboxItem, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, item := range m.items {
		if item.HubID == hubID && item.TargetAgentID == targetAgentID && item.RequesterAgentID == requesterAgentID && item.IdempotencyKey == idempotencyKey {
			return item, true, nil
		}
	}
	return HubInboxItem{}, false, nil
}

func (m *MemoryHubMailbox) PendingCount(_ context.Context, hubID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, item := range m.items {
		if item.HubID == hubID && item.AcknowledgedAt == nil {
			count++
		}
	}
	return count, nil
}

func (m *MemoryHubMailbox) Poll(_ context.Context, hubID, targetAgentID string, afterSequence uint64, limit int) ([]HubInboxItem, error) {
	if hubID == "" || targetAgentID == "" {
		return nil, errors.New("Hub and target Agent IDs are required")
	}
	if limit <= 0 || limit > 100 {
		return nil, errors.New("inbox limit must be between 1 and 100")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	items := make([]HubInboxItem, 0, limit)
	for _, item := range m.items {
		if item.HubID == hubID && item.TargetAgentID == targetAgentID && item.Sequence > afterSequence && item.AcknowledgedAt == nil {
			items = append(items, item)
			if len(items) == limit {
				break
			}
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Sequence < items[j].Sequence })
	return items, nil
}

func (m *MemoryHubMailbox) Acknowledge(_ context.Context, hubID, targetAgentID string, sequence uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.items {
		if m.items[i].HubID == hubID && m.items[i].TargetAgentID == targetAgentID && m.items[i].Sequence == sequence {
			if m.items[i].AcknowledgedAt == nil {
				now := time.Now().UTC()
				m.items[i].AcknowledgedAt = &now
			}
			return nil
		}
	}
	return fmt.Errorf("%w: sequence %d", ErrHubInboxNotFound, sequence)
}

func (m *MemoryHubMailbox) Cancel(_ context.Context, hubID, taskID string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.items {
		if m.items[i].HubID == hubID && m.items[i].TaskID == taskID {
			if m.items[i].AcknowledgedAt == nil {
				m.items[i].AcknowledgedAt = &now
			}
			return nil
		}
	}
	return ErrHubInboxNotFound
}
