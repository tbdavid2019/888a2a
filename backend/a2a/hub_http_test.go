package a2a

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHubHTTPOpenRegistrationListAndHeartbeat(t *testing.T) {
	policy := DefaultHubPolicy()
	policy.Mode = HubModeOpen
	policy.HubID = "hub-open"
	policy.RegistrationEnabled = true
	registry, err := NewHubRegistry(policy, "bootstrap-token", nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := HubHTTPHandler{Registry: registry}

	body, err := json.Marshal(validAgentDeclaration("codex"))
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/hub/v1/agents/register", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer bootstrap-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("register status=%d body=%s", response.Code, response.Body.String())
	}
	var registered struct {
		Identity IssuedAgentIdentity `json:"identity"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &registered); err != nil {
		t.Fatal(err)
	}
	if registered.Identity.AgentID == "" || registered.Identity.AgentToken == "" {
		t.Fatalf("registration identity = %+v", registered.Identity)
	}

	list := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/hub/v1/agents", nil)
	listReq.Header.Set("X-Agent-ID", registered.Identity.AgentID)
	listReq.Header.Set("Authorization", "Bearer "+registered.Identity.AgentToken)
	handler.ServeHTTP(list, listReq)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), registered.Identity.AgentID) || strings.Contains(list.Body.String(), registered.Identity.AgentToken) {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}

	heartbeat := httptest.NewRecorder()
	heartbeatReq := httptest.NewRequest(http.MethodPost, "/hub/v1/agents/"+registered.Identity.AgentID+"/heartbeat", nil)
	heartbeatReq.Header.Set("X-Agent-ID", registered.Identity.AgentID)
	heartbeatReq.Header.Set("Authorization", "Bearer "+registered.Identity.AgentToken)
	handler.ServeHTTP(heartbeat, heartbeatReq)
	if heartbeat.Code != http.StatusOK || !strings.Contains(heartbeat.Body.String(), "ONLINE") {
		t.Fatalf("heartbeat status=%d body=%s", heartbeat.Code, heartbeat.Body.String())
	}
	disconnect := httptest.NewRecorder()
	disconnectReq := httptest.NewRequest(http.MethodPost, "/hub/v1/agents/"+registered.Identity.AgentID+"/disconnect", nil)
	disconnectReq.Header.Set("X-Agent-ID", registered.Identity.AgentID)
	disconnectReq.Header.Set("Authorization", "Bearer "+registered.Identity.AgentToken)
	handler.ServeHTTP(disconnect, disconnectReq)
	if disconnect.Code != http.StatusOK || !strings.Contains(disconnect.Body.String(), "OFFLINE") {
		t.Fatalf("disconnect status=%d body=%s", disconnect.Code, disconnect.Body.String())
	}
}

func TestHubHTTPPublicRegistrationAndOpenAuthFailure(t *testing.T) {
	policy := DefaultHubPolicy()
	policy.Mode = HubModePublic
	policy.HubID = "hub-public"
	policy.PublicConfirmed = true
	policy.RegistrationEnabled = true
	registry, err := NewHubRegistry(policy, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := HubHTTPHandler{Registry: registry}

	openPolicy := DefaultHubPolicy()
	openPolicy.Mode = HubModeOpen
	openPolicy.HubID = "hub-auth"
	openPolicy.RegistrationEnabled = true
	openRegistry, err := NewHubRegistry(openPolicy, "secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	openHandler := HubHTTPHandler{Registry: openRegistry}
	body, _ := json.Marshal(validAgentDeclaration("agy"))
	unauthorized := httptest.NewRecorder()
	openHandler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPost, "/hub/v1/agents/register", bytes.NewReader(body)))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("open auth failure status=%d body=%s", unauthorized.Code, unauthorized.Body.String())
	}

	public := httptest.NewRecorder()
	publicReq := httptest.NewRequest(http.MethodPost, "/hub/v1/agents/register", bytes.NewReader(body))
	handler.ServeHTTP(public, publicReq)
	if public.Code != http.StatusOK {
		t.Fatalf("public register status=%d body=%s", public.Code, public.Body.String())
	}
}
