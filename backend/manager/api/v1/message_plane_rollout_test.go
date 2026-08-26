package v1

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/tbdavid2019/888a2a/backend/manager/component/messageplane"
)

func TestChatMessageFromPlanePayloadPreservesNativeFields(t *testing.T) {
	id := uuid.New()
	message, err := chatMessageFromPlanePayload(messageplane.Message{
		OrganizationID: "org-a", ConversationID: "conversation-a", MessageID: id.String(), MessageSeq: 7,
		Payload: []byte(`{"content":"hello","principal_id":42,"principal_name":"Ada","principal_handle":"ada","sender_type":1,"mentions":[{"id":"bob"}]}`),
	})
	require.NoError(t, err)
	require.Equal(t, id, message.ID)
	require.Equal(t, "org-a", message.OrganizationID)
	require.Equal(t, "hello", message.Content)
	require.Equal(t, 42, message.PrincipalID)
	require.Equal(t, "ada", message.PrincipalHandle)
	require.Equal(t, int64(7), message.RoomVersion)
	require.Len(t, message.Mentions, 1)
}

func TestChatMessageFromPlanePayloadRejectsInvalidIdentity(t *testing.T) {
	_, err := chatMessageFromPlanePayload(messageplane.Message{
		ConversationID: uuid.NewString(), MessageID: "not-a-uuid", Payload: []byte(`{"content":"hello"}`),
	})
	require.Error(t, err)
}
