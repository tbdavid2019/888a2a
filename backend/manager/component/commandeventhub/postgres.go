package commandeventhub

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

const postgresCommandEventChannel = "a2a888_command_event_wakeup"

// PostgresHub combines local command-event wakeups with PostgreSQL
// LISTEN/NOTIFY. Notifications carry no event data; the consumer always
// re-reads command_event using its last acknowledged sequence.
type PostgresHub struct {
	local     *Hub
	listener  *pq.Listener
	publisher *sql.DB
	cancel    context.CancelFunc
	done      chan struct{}
	closeOnce sync.Once
}

// NewPostgres creates a command-event hub backed by a PostgreSQL DSN.
func NewPostgres(ctx context.Context, dsn string) (*PostgresHub, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("PostgreSQL command-event hub requires a DSN")
	}
	listener := pq.NewListener(dsn, 10*time.Second, time.Minute, nil)
	if err := listener.Listen(postgresCommandEventChannel); err != nil {
		_ = listener.Close()
		return nil, errors.Wrap(err, "listen for command-event notifications")
	}
	publisher, err := sql.Open("postgres", dsn)
	if err != nil {
		_ = listener.Close()
		return nil, errors.Wrap(err, "open command-event notification publisher")
	}
	if err := publisher.PingContext(ctx); err != nil {
		_ = publisher.Close()
		_ = listener.Close()
		return nil, errors.Wrap(err, "ping command-event notification publisher")
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

func (h *PostgresHub) Subscribe(commandID uuid.UUID) chan struct{} {
	return h.local.Subscribe(commandID)
}

func (h *PostgresHub) Unsubscribe(commandID uuid.UUID, ch chan struct{}) {
	h.local.Unsubscribe(commandID, ch)
}

// NotifyCommand wakes local waiters immediately and publishes a shared wake
// for other Manager replicas. Delivery is best effort because the event log is
// authoritative and consumers always replay from their cursor.
func (h *PostgresHub) NotifyCommand(commandID uuid.UUID) {
	if h == nil || h.local == nil {
		return
	}
	h.local.NotifyCommand(commandID)
	if h.publisher == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := h.publisher.ExecContext(ctx, `SELECT pg_notify($1, $2)`, postgresCommandEventChannel, commandID.String()); err != nil {
		slog.Warn("failed to publish shared command-event notification", "command_id", commandID, "error", err)
	}
}

func (h *PostgresHub) receive(ctx context.Context) {
	defer close(h.done)
	for {
		select {
		case <-ctx.Done():
			return
		case notification, ok := <-h.listener.NotificationChannel():
			if !ok {
				return
			}
			if notification == nil {
				continue
			}
			commandID, err := uuid.Parse(notification.Extra)
			if err != nil {
				slog.Warn("ignored malformed shared command-event notification", "payload", notification.Extra, "error", err)
				continue
			}
			h.local.NotifyCommand(commandID)
		}
	}
}

// Close stops the listener and publisher.
func (h *PostgresHub) Close() error {
	if h == nil {
		return nil
	}
	var closeErr error
	h.closeOnce.Do(func() {
		if h.cancel != nil {
			h.cancel()
		}
		if h.listener != nil {
			closeErr = h.listener.Close()
		}
		if h.publisher != nil {
			if err := h.publisher.Close(); closeErr == nil {
				closeErr = err
			}
		}
		<-h.done
	})
	return closeErr
}
