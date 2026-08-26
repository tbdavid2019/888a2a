package commandeventhub

import (
	"sync"

	"github.com/google/uuid"
)

// Hub is the local side of the command-event wake-up boundary. A wake is only
// a hint; consumers must re-read the durable event log from their cursor.
type Hub struct {
	mu      sync.Mutex
	waiters map[uuid.UUID]map[chan struct{}]struct{}
}

// New returns an empty local command-event hub.
func New() *Hub {
	return &Hub{waiters: make(map[uuid.UUID]map[chan struct{}]struct{})}
}

// Subscribe registers a command-event waiter.
func (h *Hub) Subscribe(commandID uuid.UUID) chan struct{} {
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.waiters[commandID] == nil {
		h.waiters[commandID] = make(map[chan struct{}]struct{})
	}
	h.waiters[commandID][ch] = struct{}{}
	return ch
}

// Unsubscribe removes a command-event waiter.
func (h *Hub) Unsubscribe(commandID uuid.UUID, ch chan struct{}) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if waiters := h.waiters[commandID]; waiters != nil {
		delete(waiters, ch)
		if len(waiters) == 0 {
			delete(h.waiters, commandID)
		}
	}
}

// NotifyCommand wakes all local waiters without blocking the publisher.
func (h *Hub) NotifyCommand(commandID uuid.UUID) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.waiters[commandID] {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
