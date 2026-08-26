package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	models "github.com/tbdavid2019/888a2a/backend/generated-go/store"
)

func TestConversationExecutionEventsAreTenantScopedAndIdempotent(t *testing.T) {
	services, _ := requireCommandEventIntegrationStore(t)
	ctx := context.Background()
	user, err := services.CreateUser(ctx, &UserMessage{
		Email: fmt.Sprintf("execution-events-%d@example.test", time.Now().UnixNano()), Name: "Execution Events", Type: models.PrincipalType_END_USER, PasswordHash: "test",
	})
	require.NoError(t, err)
	agent, err := services.CreateAgent(ctx, &AgentMessage{
		Name: "Execution Event Agent", CreatedBy: user.ID, OwnerID: user.ID, OrganizationID: "default", WorkspaceID: "default", TokenVersion: 1,
	})
	require.NoError(t, err)
	conversation, err := services.GetOrCreateDirectConversation(ctx, agent.ID, user.ID)
	require.NoError(t, err)
	command, err := services.CreateCommand(ctx, &CommandMessage{AgentID: agent.ID, PrincipalID: user.ID, Command: "execution", Status: CommandStatusRunning})
	require.NoError(t, err)

	require.NoError(t, services.LinkCommandConversation(ctx, command.ID, conversation.ID))
	require.NoError(t, services.LinkCommandConversation(ctx, command.ID, conversation.ID))
	require.NoError(t, services.AppendCommandExecutionEvent(ctx, command.ID, "COMMAND_STEERED", `{"text":"continue"}`))
	require.NoError(t, services.AppendCommandExecutionEvent(ctx, command.ID, "COMMAND_CANCELLED", `{}`))
	require.NoError(t, services.AppendCommandExecutionEvent(ctx, command.ID, "COMMAND_COMPLETED", `{"status":"cancelled"}`))

	events, err := services.ListConversationExecutionEvents(ctx, "default", conversation.ID)
	require.NoError(t, err)
	require.Len(t, events, 4)
	require.Equal(t, []string{"COMMAND_STARTED", "COMMAND_STEERED", "COMMAND_CANCELLED", "COMMAND_COMPLETED"}, []string{
		events[0].EventType, events[1].EventType, events[2].EventType, events[3].EventType,
	})
	other, err := services.ListConversationExecutionEvents(ctx, "other-tenant", conversation.ID)
	require.NoError(t, err)
	require.Empty(t, other)
}
