package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tbdavid2019/888a2a/backend/common"
)

func TestMessageCursorsResumePerDeviceAndAgent(t *testing.T) {
	services := requireIntegrationStore(t)
	ctx := context.Background()
	user, err := services.CreateUser(ctx, &UserMessage{
		Email: "cursor-user@example.test", Name: "Cursor User", Type: 1, PasswordHash: "test",
	})
	require.NoError(t, err)
	agent, err := services.CreateAgent(ctx, &AgentMessage{
		Name: "Cursor Agent", CreatedBy: user.ID, OwnerID: user.ID, OrganizationID: "default", WorkspaceID: "default", TokenVersion: 1,
	})
	require.NoError(t, err)
	conversation, err := services.GetOrCreateDirectConversation(ctx, agent.ID, user.ID)
	require.NoError(t, err)

	created := make([]*ChatMessage, 0, 3)
	for _, content := range []string{"one", "two", "three"} {
		message, _, createErr := services.CreateChatMessageBumpVersion(ctx, &ChatMessage{
			ConversationID: conversation.ID, PrincipalID: user.ID, Role: 1, Content: content, SenderType: SenderTypeUser,
		})
		require.NoError(t, createErr)
		created = append(created, message)
	}

	deviceA, deviceB := "browser-a", "browser-b"
	readA, err := services.UpsertUserDeviceReadCursor(ctx, user.ID, deviceA, conversation.ID, created[0].RoomVersion)
	require.NoError(t, err)
	require.Equal(t, created[0].RoomVersion, readA)
	readB, err := services.UpsertUserDeviceReadCursor(ctx, user.ID, deviceB, conversation.ID, 0)
	require.NoError(t, err)
	require.Equal(t, int64(0), readB)
	readA, err = services.UpsertUserDeviceReadCursor(ctx, user.ID, deviceA, conversation.ID, 0)
	require.NoError(t, err)
	require.Equal(t, created[0].RoomVersion, readA, "a stale device acknowledgement must not rewind")
	readA, found, err := services.GetUserDeviceReadCursor(ctx, user.ID, deviceA, conversation.ID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, created[0].RoomVersion, readA)
	readB, found, err = services.GetUserDeviceReadCursor(ctx, user.ID, deviceB, conversation.ID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, int64(0), readB, "one device must not inherit another device cursor")

	messages, _, err := services.ListConversationMessages(ctx, conversation.ID, readA, 0, 10, 0)
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.Equal(t, "two", messages[0].Content)
	require.Equal(t, "three", messages[1].Content)

	processed, err := services.UpsertCursor(ctx, agent.ID, conversation.ID, 2)
	require.NoError(t, err)
	require.Equal(t, int64(2), processed)
	processed, found, err = services.GetCursor(ctx, agent.ID, conversation.ID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, int64(2), processed)
	processed, err = services.UpsertCursor(ctx, agent.ID, conversation.ID, 1)
	require.NoError(t, err)
	require.Equal(t, int64(2), processed, "an out-of-order Agent acknowledgement must not rewind")

	// A tenant-bound context must not reuse the default tenant's cursor key.
	otherTenant := common.SetOrganizationIDToContext(ctx, "other-tenant")
	_, found, err = services.GetUserDeviceReadCursor(otherTenant, user.ID, deviceA, conversation.ID)
	require.NoError(t, err)
	require.False(t, found)
}
