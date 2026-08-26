package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/tbdavid2019/888a2a/backend/manager/migration"
)

func requireIntegrationStore(t *testing.T) *Store {
	t.Helper()
	if os.Getenv("A2A888_RUN_MIGRATION_TESTS") != "1" {
		t.Skip("set A2A888_RUN_MIGRATION_TESTS=1 to run PostgreSQL outbox integration tests")
	}
	rootURL := os.Getenv("A2A888_TEST_PG_URL")
	if rootURL == "" {
		t.Skip("set A2A888_TEST_PG_URL to a PostgreSQL URL")
	}
	root, err := sql.Open("pgx", rootURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	dbName := fmt.Sprintf("a2a888_outbox_%d", time.Now().UnixNano())
	if _, err := root.ExecContext(context.Background(), `CREATE DATABASE "`+dbName+`"`); err != nil {
		t.Skipf("test user cannot create database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = root.ExecContext(context.Background(), `DROP DATABASE IF EXISTS "`+dbName+`" WITH (FORCE)`)
	})
	u, err := url.Parse(rootURL)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + dbName
	db, err := sql.Open("pgx", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := migration.MigrateSchema(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	stores, err := New(context.Background(), u.String(), false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stores.Close() })
	return stores
}

func TestOutboxWorkerReclaimsAfterLeaseExpiry(t *testing.T) {
	stores := requireIntegrationStore(t)
	ctx := context.Background()
	event := DurableEventEnvelope{
		EventID: "integration-event-1", Organization: "default", AggregateType: "test",
		AggregateID: "aggregate-1", EventType: "TEST", CorrelationID: "trace-1",
		IdempotencyKey: "integration-idempotency-1", Payload: []byte(`{"ok":true}`), MaxAttempts: 3,
	}
	if err := stores.EnqueueOutboxEvent(ctx, event); err != nil {
		t.Fatal(err)
	}
	claimed, err := stores.ClaimOutboxEvents(ctx, "worker-a", 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("worker-a claim = %d, %v", len(claimed), err)
	}
	if _, err := stores.GetDB().ExecContext(ctx, `UPDATE a2a888_outbox_event SET claim_expires_at = now() - interval '1 second' WHERE event_id = $1`, event.EventID); err != nil {
		t.Fatal(err)
	}
	reclaimed, err := stores.ClaimOutboxEvents(ctx, "worker-b", 1)
	if err != nil || len(reclaimed) != 1 || reclaimed[0].WorkerID != "worker-b" {
		t.Fatalf("worker-b reclaim = %+v, %v", reclaimed, err)
	}
	if err := stores.AckOutboxEvent(ctx, "worker-b", event.EventID); err != nil {
		t.Fatal(err)
	}
}

func TestConnectorInboxDeduplicatesExternalEvent(t *testing.T) {
	stores := requireIntegrationStore(t)
	event := ConnectorInboxEvent{
		OrganizationID: "default", InstallationID: "integration-installation",
		ExternalEventID: "external-event-1", ExternalEventType: "message",
		RawPayload: []byte(`{"events":[{"id":"external-event-1"}]}`),
	}
	first, err := stores.RecordConnectorInbox(context.Background(), event)
	if err != nil || !first {
		t.Fatalf("first inbox insert = %v, %v", first, err)
	}
	second, err := stores.RecordConnectorInbox(context.Background(), event)
	if err != nil || second {
		t.Fatalf("duplicate inbox insert = %v, %v", second, err)
	}
	if err := stores.MarkConnectorInboxProcessed(context.Background(), event.OrganizationID, event.InstallationID, event.ExternalEventID); err != nil {
		t.Fatal(err)
	}
}

func TestOutboxDeadLetterReplayRecordsReconciliation(t *testing.T) {
	stores := requireIntegrationStore(t)
	ctx := context.Background()
	event := DurableEventEnvelope{
		EventID: "integration-event-dead-letter", Organization: "default", AggregateType: "test",
		AggregateID: "aggregate-dead-letter", EventType: "TEST", CorrelationID: "trace-dead-letter",
		IdempotencyKey: "integration-idempotency-dead-letter", Payload: []byte(`{"ok":true}`), MaxAttempts: 1,
	}
	if err := stores.EnqueueOutboxEvent(ctx, event); err != nil {
		t.Fatal(err)
	}
	claimed, err := stores.ClaimOutboxEvents(ctx, "worker-dead-letter", 1)
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim = %d, %v", len(claimed), err)
	}
	if err := stores.RetryOutboxEvent(ctx, "worker-dead-letter", event.EventID, "permanent failure", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := stores.ReplayDeadLetterOutboxEvent(ctx, "default", event.EventID, "operator-1"); err != nil {
		t.Fatal(err)
	}
	replayed, err := stores.ClaimOutboxEvents(ctx, "worker-replay", 1)
	if err != nil || len(replayed) != 1 {
		t.Fatalf("replayed claim = %d, %v", len(replayed), err)
	}
	if err := stores.AckOutboxEvent(ctx, "worker-replay", event.EventID); err != nil {
		t.Fatal(err)
	}
	if err := stores.ReplayDeadLetterOutboxEvent(ctx, "default", event.EventID, "operator-1"); !errors.Is(err, ErrOutboxNotDeadLetter) {
		t.Fatalf("second replay error = %v, want ErrOutboxNotDeadLetter", err)
	}
}
