package messageplane

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tbdavid2019/888a2a/backend/common"
)

func TestWuKongIMAdapterUsesInternalBusinessEndpoints(t *testing.T) {
	paths := make(chan string, 8)
	handlerErrors := make(chan error, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths <- r.URL.Path
		switch r.URL.Path {
		case "/message/send":
			var request wuKongSendRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				handlerErrors <- err
				return
			}
			if request.ClientMessageNo != "client-1" || request.FromUID != "user-1" || request.Payload != base64.StdEncoding.EncodeToString([]byte(`{"type":1}`)) {
				handlerErrors <- fmt.Errorf("unexpected send request: %+v", request)
				return
			}
			_, _ = w.Write([]byte(`{"message_id":42,"message_seq":7,"client_msg_no":"client-1"}`))
		case "/channel/messagesync":
			_, _ = w.Write([]byte(`[{"message_id":42,"message_seq":7,"client_msg_no":"client-1","from_uid":"user-1","payload":"eyJ0eXBlIjoxfQ=="}]`))
		case "/channel/subscriber_add":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/readyz":
			_, _ = w.Write([]byte("ok"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	adapter, err := NewWuKongIMAdapter(WuKongIMConfig{
		BaseURL:  server.URL,
		LoginUID: func(string, string) string { return "user-1" },
		Credentials: func(_ context.Context, org, conversation string) (ConnectionCredentials, error) {
			return ConnectionCredentials{OrganizationID: org, ConversationID: conversation, Token: "short-lived"}, nil
		},
	})
	require.NoError(t, err)
	ctx := common.SetOrganizationIDToContext(context.Background(), "org-1")
	credentials, err := adapter.IssueCredentials(ctx, "org-1", "conversation-1")
	require.NoError(t, err)
	require.Equal(t, "short-lived", credentials.Token)
	message, err := adapter.Append(ctx, MessageInput{OrganizationID: "org-1", ConversationID: "conversation-1", ClientMessageNo: "client-1", SenderID: "user-1", Payload: []byte(`{"type":1}`)})
	require.NoError(t, err)
	require.Equal(t, "42", message.MessageID)
	require.Equal(t, uint64(7), message.MessageSeq)
	history, err := adapter.History(ctx, HistoryRequest{OrganizationID: "org-1", ConversationID: "conversation-1", After: Cursor{OrganizationID: "org-1", ConversationID: "conversation-1", MessageSeq: 6}, Limit: 10})
	require.NoError(t, err)
	require.Len(t, history.Messages, 1)
	require.Equal(t, uint64(7), history.NextCursor.MessageSeq)
	require.NoError(t, adapter.ProjectMembership(ctx, MembershipProjection{OrganizationID: "org-1", ConversationID: "conversation-1", PrincipalID: "user-2", Role: "member"}))
	health, err := adapter.Health(ctx)
	require.NoError(t, err)
	require.True(t, health.Healthy)
	close(paths)
	seen := make(map[string]bool)
	for path := range paths {
		seen[path] = true
	}
	require.Equal(t, map[string]bool{"/message/send": true, "/channel/messagesync": true, "/channel/subscriber_add": true, "/readyz": true}, seen)
	select {
	case err := <-handlerErrors:
		require.NoError(t, err)
	default:
	}
}

func TestWuKongIMAdapterRejectsPublicOrMismatchedTenants(t *testing.T) {
	_, err := NewWuKongIMAdapter(WuKongIMConfig{BaseURL: "http://8.8.8.8:5001"})
	require.Error(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	adapter, err := NewWuKongIMAdapter(WuKongIMConfig{BaseURL: server.URL})
	require.NoError(t, err)
	_, err = adapter.Health(common.SetOrganizationIDToContext(context.Background(), "org-a"))
	require.NoError(t, err)
	_, err = adapter.IssueCredentials(common.SetOrganizationIDToContext(context.Background(), "org-a"), "org-b", "conversation")
	require.Error(t, err)
}

func TestRawNumberStringRejectsNonIntegerMessageIDs(t *testing.T) {
	_, err := rawNumberString(json.RawMessage(`1.5`))
	require.Error(t, err)
	_, err = rawNumberString(json.RawMessage(`"42"`))
	require.NoError(t, err)
}

func TestWuKongIMHealthFallsBackToCurrentReleaseEndpoint(t *testing.T) {
	paths := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths <- r.URL.Path
		if r.URL.Path == "/readyz" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()
	adapter, err := NewWuKongIMAdapter(WuKongIMConfig{BaseURL: server.URL})
	require.NoError(t, err)
	health, err := adapter.Health(context.Background())
	require.NoError(t, err)
	require.True(t, health.Healthy)
	require.Equal(t, "/readyz", <-paths)
	require.Equal(t, "/health", <-paths)
}
