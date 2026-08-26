package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"

	models "github.com/tbdavid2019/888a2a/backend/generated-go/store"
	"github.com/tbdavid2019/888a2a/backend/manager/component/commandeventhub"
	"github.com/tbdavid2019/888a2a/backend/manager/migration"
)

func requireCommandEventIntegrationStore(t *testing.T) (*Store, string) {
	t.Helper()
	if os.Getenv("A2A888_RUN_MIGRATION_TESTS") != "1" {
		t.Skip("set A2A888_RUN_MIGRATION_TESTS=1 to run command-event integration tests")
	}
	rootURL := os.Getenv("A2A888_TEST_PG_URL")
	if rootURL == "" {
		t.Skip("set A2A888_TEST_PG_URL to a PostgreSQL URL")
	}
	root, err := sql.Open("pgx", rootURL)
	require.NoError(t, err)
	t.Cleanup(func() { _ = root.Close() })
	databaseName := fmt.Sprintf("a2a888_command_events_%d", time.Now().UnixNano())
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
	services, err := New(context.Background(), databaseURL.String(), true)
	require.NoError(t, err)
	t.Cleanup(func() { _ = services.Close() })
	return services, databaseURL.String()
}

func TestCommandEventReplayAfterPeerWakeAndDisconnect(t *testing.T) {
	services, databaseURL := requireCommandEventIntegrationStore(t)
	ctx := context.Background()
	user, err := services.CreateUser(ctx, &UserMessage{
		Email: fmt.Sprintf("command-events-%d@example.test", time.Now().UnixNano()), Name: "Command Events", Type: models.PrincipalType_END_USER, PasswordHash: "test",
	})
	require.NoError(t, err)
	agent, err := services.CreateAgent(ctx, &AgentMessage{
		Name: "Command Event Agent", CreatedBy: user.ID, OwnerID: user.ID, OrganizationID: "default", WorkspaceID: "default", TokenVersion: 1,
	})
	require.NoError(t, err)
	command, err := services.CreateCommand(ctx, &CommandMessage{AgentID: agent.ID, PrincipalID: user.ID, Command: "integration", Status: CommandStatusPending})
	require.NoError(t, err)

	first, err := commandeventhub.NewPostgres(ctx, databaseURL)
	require.NoError(t, err)
	t.Cleanup(func() { _ = first.Close() })
	second, err := commandeventhub.NewPostgres(ctx, databaseURL)
	require.NoError(t, err)
	t.Cleanup(func() { _ = second.Close() })
	services.SetCommandEventNotifier(first)
	waiter := second.Subscribe(command.ID)
	t.Cleanup(func() { second.Unsubscribe(command.ID, waiter) })

	for sequence := int32(1); sequence <= 3; sequence++ {
		require.NoError(t, services.AppendCommandEvent(ctx, &CommandEventMessage{
			CommandID: command.ID, SeqNo: sequence, EventType: 1, Summary: fmt.Sprintf("event-%d", sequence), PayloadJSON: "{}",
		}))
	}
	select {
	case <-waiter:
	case <-time.After(5 * time.Second):
		t.Fatal("peer replica did not receive command-event wake")
	}

	// A single shared wake is enough: the receiving replica replays every
	// durable row after its cursor, including events dropped by a slow local
	// live subscriber.
	replayed, err := services.GetCommandEvents(ctx, command.ID, 0)
	require.NoError(t, err)
	require.Len(t, replayed, 3)
	for index, event := range replayed {
		require.Equal(t, int32(index+1), event.SeqNo)
	}

	second.Unsubscribe(command.ID, waiter)
	require.NoError(t, services.AppendCommandEvent(ctx, &CommandEventMessage{
		CommandID: command.ID, SeqNo: 4, EventType: 1, Summary: "event-4", PayloadJSON: "{}",
	}))
	disconnectedReplay, err := services.GetCommandEvents(ctx, command.ID, 3)
	require.NoError(t, err)
	require.Len(t, disconnectedReplay, 1)
	require.Equal(t, int32(4), disconnectedReplay[0].SeqNo)
}
