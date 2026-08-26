package state

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/tbdavid2019/888a2a/backend/manager/store"
)

type HeartbeatUpdate struct {
	AgentID         int
	LastHeartbeatAt int64
	SessionID       string
}

// heartbeatWriter is the narrow store dependency the buffer flushes to. *store.Store
// satisfies it; tests may pass a fake. Keeping it an interface (rather than
// *store.Store) lets the buffer be unit-tested without a Postgres connection.
type heartbeatWriter interface {
	TouchAgentHeartbeats(ctx context.Context, updates []store.AgentHeartbeat) error
}

type HeartbeatBuffer struct {
	mu       sync.Mutex
	updates  map[int]*HeartbeatUpdate
	store    heartbeatWriter
	interval time.Duration

	// startMu guards the single-flight Start so a second call cannot overwrite
	// cancel and leak the first flush goroutine.
	startMu sync.Mutex
	cancel  context.CancelFunc
	// done is closed when the flush goroutine exits after its final flush;
	// Stop waits on it so the caller never races the last batch.
	done chan struct{}
}

func NewHeartbeatBuffer(store *store.Store, interval time.Duration) *HeartbeatBuffer {
	if interval == 0 {
		interval = 10 * time.Second
	}
	return &HeartbeatBuffer{
		updates:  make(map[int]*HeartbeatUpdate),
		store:    store,
		interval: interval,
	}
}

func (b *HeartbeatBuffer) Record(update *HeartbeatUpdate) {
	b.mu.Lock()
	b.updates[update.AgentID] = update
	b.mu.Unlock()
}

func (b *HeartbeatBuffer) GetLatest(agentID int) *HeartbeatUpdate {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.updates[agentID]
}

// Start launches the flush ticker. It is idempotent: a second call (e.g. if
// the server wiring ever double-starts it) returns immediately instead of
// overwriting b.cancel and leaving the first goroutine running with no way
// to stop it. The goroutine exits when ctx is cancelled, does a final flush,
// and closes done so Stop can join it.
func (b *HeartbeatBuffer) Start(ctx context.Context) {
	b.startMu.Lock()
	if b.cancel != nil {
		// Already running.
		b.startMu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	b.cancel = cancel
	b.done = make(chan struct{})
	b.startMu.Unlock()

	go func() {
		defer close(b.done)
		ticker := time.NewTicker(b.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				b.flush()
				return
			case <-ticker.C:
				b.flush()
			}
		}
	}()
}

// Stop cancels the flush loop and blocks until the final flush has been
// written, so a caller that closes the store right after Stop cannot race the
// last batch. It is safe to call before Start or more than once.
func (b *HeartbeatBuffer) Stop() {
	b.startMu.Lock()
	cancel := b.cancel
	done := b.done
	b.startMu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	<-done
}

func (b *HeartbeatBuffer) flush() {
	b.mu.Lock()
	snapshot := b.updates
	b.updates = make(map[int]*HeartbeatUpdate)
	b.mu.Unlock()

	if len(snapshot) == 0 {
		return
	}

	// Bound the DB write so a hung Postgres does not block the flush loop (and
	// thus shutdown's final flush) indefinitely.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	updates := make([]store.AgentHeartbeat, 0, len(snapshot))
	for _, update := range snapshot {
		updates = append(updates, store.AgentHeartbeat{
			AgentID:         update.AgentID,
			LastHeartbeatAt: update.LastHeartbeatAt,
		})
	}

	if err := b.store.TouchAgentHeartbeats(ctx, updates); err != nil {
		slog.Error("failed to batch update agent heartbeats", "count", len(updates), "error", err)
	}
}
