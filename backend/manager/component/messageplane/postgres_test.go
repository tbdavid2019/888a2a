package messageplane

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"

	"github.com/tbdavid2019/888a2a/backend/common"
	"github.com/tbdavid2019/888a2a/backend/manager/migration"
)

func requireMessagePlaneDatabase(t *testing.T) *sql.DB {
	t.Helper()
	if os.Getenv("A2A888_RUN_MIGRATION_TESTS") != "1" {
		t.Skip("set A2A888_RUN_MIGRATION_TESTS=1 to run MessagePlane PostgreSQL integration tests")
	}
	rootURL := os.Getenv("A2A888_TEST_PG_URL")
	if rootURL == "" {
		t.Skip("set A2A888_TEST_PG_URL to a PostgreSQL URL")
	}
	root, err := sql.Open("pgx", rootURL)
	require.NoError(t, err)
	t.Cleanup(func() { _ = root.Close() })
	databaseName := fmt.Sprintf("a2a888_message_plane_%d", time.Now().UnixNano())
	if _, err := root.ExecContext(context.Background(), `CREATE DATABASE "`+databaseName+`"`); err != nil {
		t.Skipf("test user cannot create database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = root.ExecContext(context.Background(), `DROP DATABASE IF EXISTS "`+databaseName+`" WITH (FORCE)`)
	})
	databaseURL, err := url.Parse(rootURL)
	require.NoError(t, err)
	databaseURL.Path = "/" + databaseName
	db, err := sql.Open("pgx", databaseURL.String())
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, migration.MigrateSchema(context.Background(), db))
	return db
}

func TestPostgresPlaneConcurrentAppendAndRetry(t *testing.T) {
	db := requireMessagePlaneDatabase(t)
	plane, err := NewPostgresPlane(db)
	require.NoError(t, err)
	ctx := common.SetOrganizationIDToContext(context.Background(), "default")
	const count = 12
	inputs := make([]MessageInput, count)
	for i := range inputs {
		inputs[i] = MessageInput{OrganizationID: "default", ConversationID: "conversation-1", ClientMessageNo: fmt.Sprintf("client-%d", i), SenderID: "user-1", Payload: []byte(fmt.Sprintf(`{"index":%d}`, i))}
	}
	messages := make([]Message, count)
	appendErrors := make([]error, count)
	var wg sync.WaitGroup
	for i := range inputs {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			messages[index], appendErrors[index] = plane.Append(ctx, inputs[index])
		}(i)
	}
	wg.Wait()
	for _, appendErr := range appendErrors {
		require.NoError(t, appendErr)
	}
	sequences := make([]int, count)
	for i, message := range messages {
		require.NotEmpty(t, message.MessageID)
		sequences[i] = int(message.MessageSeq)
	}
	sort.Ints(sequences)
	for i, sequence := range sequences {
		require.Equal(t, i+1, sequence)
	}
	retry, err := plane.Append(ctx, inputs[0])
	require.NoError(t, err)
	require.Equal(t, messages[0].MessageID, retry.MessageID)
	require.Equal(t, messages[0].MessageSeq, retry.MessageSeq)
	history, err := plane.History(ctx, HistoryRequest{OrganizationID: "default", ConversationID: "conversation-1", Limit: 100})
	require.NoError(t, err)
	require.Len(t, history.Messages, count)
	require.Equal(t, uint64(count), history.NextCursor.MessageSeq)
	_, err = plane.History(common.SetOrganizationIDToContext(context.Background(), "other"), HistoryRequest{OrganizationID: "default", ConversationID: "conversation-1", Limit: 10})
	require.Error(t, err)
}

func TestPostgresPlaneDualProjectionParity(t *testing.T) {
	db := requireMessagePlaneDatabase(t)
	plane, err := NewPostgresPlane(db)
	require.NoError(t, err)
	ctx := common.SetOrganizationIDToContext(context.Background(), "default")
	payload := []byte(`{"content":"hello","attachments":[{"id":"file-1"}],"mentions":[{"id":"user-2"}],"thread_root_id":"root-1","reactions":[{"emoji":"👍"}]}`)
	message, err := plane.Append(ctx, MessageInput{
		OrganizationID: "default", ConversationID: "conversation-parity", ClientMessageNo: "client-parity", SenderID: "user-1", Payload: payload,
	})
	require.NoError(t, err)

	var content, attachments, mentions, reactions string
	var threadRoot sql.NullString
	err = db.QueryRowContext(ctx, `
		SELECT content, attachments::text, mentions::text, thread_root_id, reactions::text
		FROM a2a888_message_projection
		WHERE organization_id = $1 AND message_id = $2
	`, "default", message.MessageID).Scan(&content, &attachments, &mentions, &threadRoot, &reactions)
	require.NoError(t, err)
	require.Equal(t, "hello", content)
	require.True(t, threadRoot.Valid)
	require.Equal(t, "root-1", threadRoot.String)
	for _, projected := range []string{attachments, mentions, reactions} {
		var value any
		require.NoError(t, json.Unmarshal([]byte(projected), &value))
	}

	projectionCursor, err := plane.AdvanceProjectionCursor(ctx, "device", "browser-1", Cursor{OrganizationID: "default", ConversationID: "conversation-parity", MessageSeq: message.MessageSeq})
	require.NoError(t, err)
	require.Equal(t, message.MessageSeq, projectionCursor.MessageSeq)
	projectionCursor, err = plane.AdvanceProjectionCursor(ctx, "device", "browser-1", Cursor{OrganizationID: "default", ConversationID: "conversation-parity", MessageSeq: 0})
	require.NoError(t, err)
	require.Equal(t, message.MessageSeq, projectionCursor.MessageSeq)
}

func TestPostgresPlaneReconcileRepairsDriftAndQuarantinesUnknownMembership(t *testing.T) {
	db := requireMessagePlaneDatabase(t)
	plane, err := NewPostgresPlane(db)
	require.NoError(t, err)
	ctx := common.SetOrganizationIDToContext(context.Background(), "default")
	message, err := plane.Append(ctx, MessageInput{
		OrganizationID: "default", ConversationID: "conversation-reconcile", ClientMessageNo: "client-reconcile", SenderID: "user-1", Payload: []byte(`{"content":"repair me"}`),
	})
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `DELETE FROM a2a888_message_projection WHERE organization_id = $1 AND message_id = $2`, "default", message.MessageID)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
		INSERT INTO a2a888_message_membership (organization_id, conversation_id, principal_id, role)
		VALUES ('default', 'conversation-reconcile', 'stale-user', 'member')
	`)
	require.NoError(t, err)

	report, err := plane.ReconcileConversation(ctx, "default", "conversation-reconcile", []MembershipProjection{{OrganizationID: "default", ConversationID: "conversation-reconcile", PrincipalID: "current-user", Role: "owner"}})
	require.NoError(t, err)
	require.Equal(t, 2, report.Repaired)
	require.Equal(t, 1, report.Quarantined)
	var content string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT content FROM a2a888_message_projection WHERE organization_id = 'default' AND message_id = $1`, message.MessageID).Scan(&content))
	require.Equal(t, "repair me", content)
	var repairedRole string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT role FROM a2a888_message_membership WHERE organization_id = 'default' AND conversation_id = 'conversation-reconcile' AND principal_id = 'current-user'`).Scan(&repairedRole))
	require.Equal(t, "owner", repairedRole)
	var quarantined int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT count(*) FROM a2a888_message_reconciliation WHERE organization_id = 'default' AND conversation_id = 'conversation-reconcile' AND action = 'QUARANTINED'`).Scan(&quarantined))
	require.Equal(t, 1, quarantined)
}
