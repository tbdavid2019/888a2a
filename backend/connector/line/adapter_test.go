package line

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tbdavid2019/888a2a/backend/connector"
)

const lineFixtureBody = `{"events":[{"type":"message","webhookEventId":"event-1","timestamp":1692251666727,"source":{"type":"group","userId":"user-1","groupId":"group-1"},"message":{"id":"message-1","type":"text","text":"hello"}}]}`

func lineSignature(body, secret string) string {
	hash := hmac.New(sha256.New, []byte(secret))
	_, _ = hash.Write([]byte(body))
	return base64.StdEncoding.EncodeToString(hash.Sum(nil))
}

func TestVerifyInboundUsesExactRawBodyAndRejectsBadSignature(t *testing.T) {
	adapter := Adapter{ChannelSecret: "channel-secret"}
	installation := connector.Installation{OrganizationID: "org-a", InstallationID: "line-a", Kind: "line"}
	headers := http.Header{"X-Line-Signature": []string{lineSignature(lineFixtureBody, adapter.ChannelSecret)}}
	verified, err := adapter.VerifyInbound(context.Background(), installation, headers, []byte(lineFixtureBody))
	if err != nil || verified.ExternalID != "event-1" || string(verified.Raw) != lineFixtureBody {
		t.Fatalf("verified = %+v, err = %v", verified, err)
	}
	headers.Set("X-Line-Signature", "bad")
	if _, err := adapter.VerifyInbound(context.Background(), installation, headers, []byte(lineFixtureBody)); err == nil {
		t.Fatal("bad LINE signature was accepted")
	}
}

func TestNormalizePreservesGroupAndEventIdentity(t *testing.T) {
	adapter := Adapter{ChannelSecret: "channel-secret"}
	installation := connector.Installation{OrganizationID: "org-a", InstallationID: "line-a", Kind: "line"}
	verified, err := adapter.VerifyInbound(context.Background(), installation, http.Header{"X-Line-Signature": []string{lineSignature(lineFixtureBody, adapter.ChannelSecret)}}, []byte(lineFixtureBody))
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := adapter.Normalize(context.Background(), verified)
	if err != nil || envelope.ExternalConversation != "group-1" || envelope.ExternalEventID != "event-1" || envelope.Text != "hello" {
		t.Fatalf("envelope = %+v, err = %v", envelope, err)
	}
}

func TestDeliverUsesReplyTokenAndRetryKeyWithoutExposingSecretInBody(t *testing.T) {
	var receivedPath, receivedAuth, receivedRetry, receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		receivedPath = request.URL.Path
		receivedAuth = request.Header.Get("Authorization")
		receivedRetry = request.Header.Get("X-Line-Retry-Key")
		body, _ := io.ReadAll(request.Body)
		receivedBody = string(body)
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	adapter := Adapter{AccessToken: "access-token", APIBaseURL: server.URL}
	_, err := adapter.Deliver(context.Background(), connector.Installation{OrganizationID: "org-a", InstallationID: "line-a", Kind: "line"}, connector.Outbound{ConversationID: "user-1", ReplyTo: "reply-1", Text: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if receivedPath != "/v2/bot/message/reply" || receivedAuth != "Bearer access-token" || receivedRetry == "" || !strings.Contains(receivedBody, `"replyToken":"reply-1"`) || strings.Contains(receivedBody, "access-token") {
		t.Fatalf("path=%q auth=%q retry=%q body=%q", receivedPath, receivedAuth, receivedRetry, receivedBody)
	}
}
