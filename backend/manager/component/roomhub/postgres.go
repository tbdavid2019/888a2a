package roomhub

import (
	"context"
	"database/sql"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/pkg/errors"
)

const postgresRoomChannel = "a2a888_room_wakeup"

// PostgresHub combines local waiters with PostgreSQL LISTEN/NOTIFY so a
// conversation write on one Manager replica wakes readers on every replica.
type PostgresHub struct {
	local     *Hub
	listener  *pq.Listener
	publisher *sql.DB
	cancel    context.CancelFunc
	done      chan struct{}
	closeOnce sync.Once
}

// NewPostgres creates a shared room notifier. The listener and publisher use
// separate connections because PostgreSQL notifications are delivered only to
// sessions that are listening, while writes must use a normal SQL connection.
func NewPostgres(ctx context.Context, dsn string) (*PostgresHub, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("PostgreSQL room notifier requires a DSN")
	}
	listener := pq.NewListener(dsn, 10*time.Second, time.Minute, nil)
	if err := listener.Listen(postgresRoomChannel); err != nil {
		_ = listener.Close()
		return nil, errors.Wrap(err, "listen for room notifications")
	}
	publisher, err := sql.Open("postgres", dsn)
	if err != nil {
		_ = listener.Close()
		return nil, errors.Wrap(err, "open room notification publisher")
	}
	if err := publisher.PingContext(ctx); err != nil {
		_ = publisher.Close()
		_ = listener.Close()
		return nil, errors.Wrap(err, "ping room notification publisher")
	}
	runCtx, cancel := context.WithCancel(ctx)
	h := &PostgresHub{
		local:     New(),
		listener:  listener,
		publisher: publisher,
		cancel:    cancel,
		done:      make(chan struct{}),
	}
	go h.receive(runCtx)
	return h, nil
}

func (h *PostgresHub) Subscribe(conversationID uuid.UUID) chan struct{} {
	return h.local.Subscribe(conversationID)
}

func (h *PostgresHub) Unsubscribe(conversationID uuid.UUID, ch chan struct{}) {
	h.local.Unsubscribe(conversationID, ch)
}

// NotifyConversation wakes local readers immediately and publishes a shared
// notification for other Manager replicas. Notification delivery is best
// effort because readers always re-check the durable conversation version.
func (h *PostgresHub) NotifyConversation(conversationID uuid.UUID) {
	if h == nil || h.local == nil {
		return
	}
	h.local.NotifyConversation(conversationID)
	if h.publisher == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := h.publisher.ExecContext(ctx, `SELECT pg_notify($1, $2)`, postgresRoomChannel, conversationID.String()); err != nil {
		slog.Warn("failed to publish shared room notification", "conversation_id", conversationID, "error", err)
	}
}

func (h *PostgresHub) receive(ctx context.Context) {
	defer close(h.done)
	notifications := h.listener.NotificationChannel()
	for {
		select {
		case <-ctx.Done():
			return
		case notification, ok := <-notifications:
			if !ok {
				return
			}
			if notification == nil {
				continue
			}
			conversationID, err := uuid.Parse(notification.Extra)
			if err != nil {
				slog.Warn("ignored malformed shared room notification", "payload", notification.Extra, "error", err)
				continue
			}
			h.local.NotifyConversation(conversationID)
		}
	}
}

// Close stops the listener and publisher before the Store closes its database.
func (h *PostgresHub) Close() error {
	if h == nil {
		return nil
	}
	var err error
	h.closeOnce.Do(func() {
		if h.cancel != nil {
			h.cancel()
		}
		if h.listener != nil {
			err = h.listener.Close()
		}
		if h.publisher != nil {
			if closeErr := h.publisher.Close(); err == nil {
				err = closeErr
			}
		}
		<-h.done
	})
	return err
}
