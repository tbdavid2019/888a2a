package messageplane

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/tbdavid2019/888a2a/backend/common"
)

// TestWuKongIMExternalReadinessGate is the opt-in production boundary check.
// It skips when no controlled WuKongIM endpoint is configured; a fake server
// cannot prove the selected engine is deployed and ready.
func TestWuKongIMExternalReadinessGate(t *testing.T) {
	baseURL := os.Getenv("A2A888_WUKONGIM_URL")
	if baseURL == "" {
		t.Skip("set A2A888_WUKONGIM_URL to a controlled internal WuKongIM endpoint")
	}
	adapter, err := NewWuKongIMAdapter(WuKongIMConfig{BaseURL: baseURL})
	require.NoError(t, err)
	health, err := adapter.Health(context.Background())
	require.NoError(t, err)
	require.True(t, health.Healthy)
}

// TestWuKongIMExternalMessagePlaneGate exercises the vendor business API
// against a controlled endpoint. It is deliberately opt-in because it writes
// messages to the configured test tenant/channel.
func TestWuKongIMExternalMessagePlaneGate(t *testing.T) {
	baseURL := os.Getenv("A2A888_WUKONGIM_URL")
	if baseURL == "" {
		t.Skip("set A2A888_WUKONGIM_URL to a controlled internal WuKongIM endpoint")
	}
	adapter, err := NewWuKongIMAdapter(WuKongIMConfig{
		BaseURL:  baseURL,
		LoginUID: func(string, string) string { return "a2a888-live-gate" },
	})
	require.NoError(t, err)
	ctx := common.SetOrganizationIDToContext(context.Background(), "default")
	// WuKongIM v2 limits channel identifiers more strictly than the 888a2a
	// resource name; use a short unique channel so the gate tests the adapter
	// protocol rather than vendor identifier validation.
	conversationID := "g-" + uuid.NewString()
	require.NoError(t, adapter.ProjectMembership(ctx, MembershipProjection{
		OrganizationID: "default", ConversationID: conversationID, PrincipalID: "a2a888-live-gate", Role: "member",
	}))

	first, err := adapter.Append(ctx, MessageInput{
		OrganizationID: "default", ConversationID: conversationID,
		ClientMessageNo: "live-1", SenderID: "a2a888-live-gate", Payload: []byte(`{"n":1}`),
	})
	require.NoError(t, err)
	second, err := adapter.Append(ctx, MessageInput{
		OrganizationID: "default", ConversationID: conversationID,
		ClientMessageNo: "live-2", SenderID: "a2a888-live-gate", Payload: []byte(`{"n":2}`),
	})
	require.NoError(t, err)
	require.Greater(t, second.MessageSeq, first.MessageSeq)
	retry, err := adapter.Append(ctx, MessageInput{
		OrganizationID: "default", ConversationID: conversationID,
		ClientMessageNo: "live-1", SenderID: "a2a888-live-gate", Payload: []byte(`{"n":1}`),
	})
	require.NoError(t, err)
	require.Equal(t, first.MessageID, retry.MessageID)
	require.Equal(t, first.MessageSeq, retry.MessageSeq)
	history, err := adapter.History(ctx, HistoryRequest{
		OrganizationID: "default", ConversationID: conversationID, Limit: 10,
	})
	require.NoError(t, err)
	require.Len(t, history.Messages, 2)
	require.Equal(t, first.MessageSeq, history.Messages[0].MessageSeq)
	require.Equal(t, second.MessageSeq, history.Messages[1].MessageSeq)
}
