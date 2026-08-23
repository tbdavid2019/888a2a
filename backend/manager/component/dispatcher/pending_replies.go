package dispatcher

import (
	"sync"

	"google.golang.org/protobuf/proto"
)

// pendingReplies correlates request/response round trips over the bidi streams:
// a unary RPC registers a buffered channel keyed by request_id, the matching
// stream reply is delivered into it, and the unary RPC unblocks. Each pending
// set is typed, so a late or duplicated reply can never resolve the wrong RPC.
type pendingReplies[T proto.Message] struct {
	mu sync.Mutex
	m  map[string]chan T
}

func newPendingReplies[T proto.Message]() *pendingReplies[T] {
	return &pendingReplies[T]{m: make(map[string]chan T)}
}

// register creates a response channel keyed by requestID for an in-flight
// round trip. cancel must be called if the caller gives up waiting, to avoid
// leaking the entry.
func (p *pendingReplies[T]) register(requestID string) chan T {
	ch := make(chan T, 1)
	p.mu.Lock()
	p.m[requestID] = ch
	p.mu.Unlock()
	return ch
}

// cancel removes a pending entry without delivering a result. Safe to call
// after the reply arrived (it is a no-op in that case).
func (p *pendingReplies[T]) cancel(requestID string) {
	p.mu.Lock()
	delete(p.m, requestID)
	p.mu.Unlock()
}

// complete delivers a reply to the waiting caller and removes the pending
// entry. Called from the bidi receive loops when the machine app replies.
// Unknown request ids (late replies, already-cancelled callers) are dropped
// silently.
func (p *pendingReplies[T]) complete(requestID string, msg T) {
	p.mu.Lock()
	ch, ok := p.m[requestID]
	if ok {
		delete(p.m, requestID)
	}
	p.mu.Unlock()
	if ok {
		select {
		case ch <- msg:
		default:
		}
	}
}
