package a2a

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
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

func TestOpenClawBridgeLiveGateIsOptIn(t *testing.T) {
	if os.Getenv("A2A888_RUN_OPENCLAW_BRIDGE_TESTS") != "1" {
		t.Skip("set A2A888_RUN_OPENCLAW_BRIDGE_TESTS=1 to run the local OpenClaw Gateway gate")
	}
	baseURL := strings.TrimSpace(os.Getenv("A2A888_OPENCLAW_GATEWAY_URL"))
	token := strings.TrimSpace(os.Getenv("A2A888_OPENCLAW_GATEWAY_TOKEN"))
	if baseURL == "" || token == "" {
		t.Skip("A2A888_OPENCLAW_GATEWAY_URL and A2A888_OPENCLAW_GATEWAY_TOKEN are required for the live gate")
	}
	bridge, err := NewOpenClawBridge(OpenClawBridgeConfig{
		ID: "openclaw-gateway", BaseURL: baseURL, AgentID: os.Getenv("A2A888_OPENCLAW_AGENT_ID"),
		Token: func(context.Context) (string, error) { return token, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	health, err := bridge.Health(context.Background())
	if err != nil || !health.Ready {
		t.Fatalf("OpenClaw health = %+v, err=%v", health, err)
	}
	request := BridgeRequest{
		OrganizationID: "local-test", CallerID: "local-test", TaskID: "openclaw-gateway-task", ContextID: "openclaw-gateway-context", CorrelationID: "openclaw-gateway-correlation",
		BridgeID: bridge.ID(), Input: "Reply with exactly: openclaw-gateway-ok", MaxOutputBytes: 64 * 1024, Timeout: 2 * time.Minute,
	}
	result, err := ExecuteBridge(context.Background(), bridge, request, nil)
	if err != nil || result.Outcome != DeliveryOutcomeDelivered {
		t.Fatalf("OpenClaw live gate result=%+v err=%v", result, err)
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
