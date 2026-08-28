package a2a

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenClawBridgeUsesAuthenticatedOpenResponsesEndpoint(t *testing.T) {
	var gotAuth string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":[{"type":"message","content":[{"type":"output_text","text":"hello from OpenClaw"}]}]}`))
	}))
	defer server.Close()

	bridge, err := NewOpenClawBridge(OpenClawBridgeConfig{
		ID: "openclaw-test", BaseURL: server.URL, AgentID: "default",
		Token: func(context.Context) (string, error) { return "test-token", nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	request := validBridgeRequest()
	request.BridgeID = "openclaw-test"
	result, err := ExecuteBridge(context.Background(), bridge, request, nil)
	if err != nil || result.Outcome != DeliveryOutcomeDelivered || result.Output != "hello from OpenClaw" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if gotAuth != "Bearer test-token" {
		t.Fatalf("authorization = %q, want bearer token", gotAuth)
	}
	if gotBody["model"] != "openclaw/default" || gotBody["user"] != request.ContextID {
		t.Fatalf("request body = %+v", gotBody)
	}
	metadata, ok := gotBody["metadata"].(map[string]any)
	if !ok || metadata["organization_id"] != request.OrganizationID {
		t.Fatalf("request metadata = %+v", gotBody["metadata"])
	}
}

func TestOpenClawBridgeRejectsPublicEndpoint(t *testing.T) {
	_, err := NewOpenClawBridge(OpenClawBridgeConfig{
		ID: "openclaw-public", BaseURL: "https://example.com", AgentID: "default",
	})
	if err == nil || !strings.Contains(err.Error(), "private or loopback") {
		t.Fatalf("public endpoint error = %v", err)
	}
}

func TestOpenClawBridgeMapsAuthAndRateLimitFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		want   DeliveryOutcome
	}{
		{name: "auth", status: http.StatusUnauthorized, want: DeliveryOutcomeRejected},
		{name: "rate-limit", status: http.StatusTooManyRequests, want: DeliveryOutcomeNotDelivered},
		{name: "server", status: http.StatusBadGateway, want: DeliveryOutcomeNotDelivered},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(tc.status) }))
			defer server.Close()
			bridge, err := NewOpenClawBridge(OpenClawBridgeConfig{ID: "openclaw-test", BaseURL: server.URL, Token: func(context.Context) (string, error) { return "t", nil }})
			if err != nil {
				t.Fatal(err)
			}
			request := validBridgeRequest()
			request.BridgeID = "openclaw-test"
			result, err := ExecuteBridge(context.Background(), bridge, request, nil)
			if err == nil || result.Outcome != tc.want {
				t.Fatalf("result=%+v err=%v, want %s", result, err, tc.want)
			}
		})
	}
}
