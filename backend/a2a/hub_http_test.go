package a2a

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
	handler := HubHTTPHandler{Registry: registry, Mailbox: NewMemoryHubMailbox()}

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

	// Reconnect before exercising peer-ID mailbox delivery.
	heartbeatReq = httptest.NewRequest(http.MethodPost, "/hub/v1/agents/"+registered.Identity.AgentID+"/heartbeat", nil)
	heartbeatReq.Header.Set("X-Agent-ID", registered.Identity.AgentID)
	heartbeatReq.Header.Set("Authorization", "Bearer "+registered.Identity.AgentToken)
	handler.ServeHTTP(httptest.NewRecorder(), heartbeatReq)
	taskBody := strings.NewReader(`{"contextId":"ctx-task","idempotencyKey":"task-key","message":"hello peer"}`)
	send := httptest.NewRecorder()
	sendReq := httptest.NewRequest(http.MethodPost, "/hub/v1/agents/"+registered.Identity.AgentID+"/tasks", taskBody)
	sendReq.Header.Set("X-Agent-ID", registered.Identity.AgentID)
	sendReq.Header.Set("Authorization", "Bearer "+registered.Identity.AgentToken)
	handler.ServeHTTP(send, sendReq)
	if send.Code != http.StatusOK {
		t.Fatalf("send status=%d body=%s", send.Code, send.Body.String())
	}
	inbox := httptest.NewRecorder()
	inboxReq := httptest.NewRequest(http.MethodGet, "/hub/v1/agents/"+registered.Identity.AgentID+"/inbox", nil)
	inboxReq.Header.Set("X-Agent-ID", registered.Identity.AgentID)
	inboxReq.Header.Set("Authorization", "Bearer "+registered.Identity.AgentToken)
	handler.ServeHTTP(inbox, inboxReq)
	if inbox.Code != http.StatusOK || !strings.Contains(inbox.Body.String(), "hello peer") {
		t.Fatalf("inbox status=%d body=%s", inbox.Code, inbox.Body.String())
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
	var publicIdentity struct {
		Identity IssuedAgentIdentity `json:"identity"`
	}
	if err := json.Unmarshal(public.Body.Bytes(), &publicIdentity); err != nil {
		t.Fatal(err)
	}
	lookup := httptest.NewRecorder()
	lookupReq := httptest.NewRequest(http.MethodGet, "/hub/v1/agents/"+publicIdentity.Identity.AgentID, nil)
	handler.ServeHTTP(lookup, lookupReq)
	if lookup.Code != http.StatusOK || !strings.Contains(lookup.Body.String(), "providerFamily") || strings.Contains(lookup.Body.String(), "agentCardJson") {
		t.Fatalf("public lookup status=%d body=%s", lookup.Code, lookup.Body.String())
	}
}

func TestHubHTTPPublicRateLimitReturns429(t *testing.T) {
	policy := DefaultHubPolicy()
	policy.Mode = HubModePublic
	policy.HubID = "hub-rate"
	policy.PublicConfirmed = true
	policy.RegistrationEnabled = true
	registry, err := NewHubRegistry(policy, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := HubHTTPHandler{Registry: registry, Rate: NewHubRateLimiter(1, time.Hour)}
	body, _ := json.Marshal(validAgentDeclaration("rate"))
	first := httptest.NewRecorder()
	firstReq := httptest.NewRequest(http.MethodPost, "/hub/v1/agents/register", bytes.NewReader(body))
	handler.ServeHTTP(first, firstReq)
	second := httptest.NewRecorder()
	secondReq := httptest.NewRequest(http.MethodPost, "/hub/v1/agents/register", bytes.NewReader(body))
	handler.ServeHTTP(second, secondReq)
	if first.Code != http.StatusOK || second.Code != http.StatusTooManyRequests {
		t.Fatalf("rate limit statuses=%d/%d bodies=%s/%s", first.Code, second.Code, first.Body.String(), second.Body.String())
	}
}

func TestHubHTTPOperatorCanDisableRegistrationAndRevokePeer(t *testing.T) {
	policy := DefaultHubPolicy()
	policy.Mode = HubModePublic
	policy.HubID = "hub-operator"
	policy.PublicConfirmed = true
	policy.RegistrationEnabled = true
	registry, err := NewHubRegistry(policy, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	registry.SetOperatorToken("operator-token")
	handler := HubHTTPHandler{Registry: registry}
	body, _ := json.Marshal(validAgentDeclaration("operator"))
	register := httptest.NewRecorder()
	handler.ServeHTTP(register, httptest.NewRequest(http.MethodPost, "/hub/v1/agents/register", bytes.NewReader(body)))
	var response struct {
		Identity IssuedAgentIdentity `json:"identity"`
	}
	if err := json.Unmarshal(register.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}

	disable := httptest.NewRecorder()
	disableReq := httptest.NewRequest(http.MethodPost, "/hub/v1/admin/registration", strings.NewReader(`{"enabled":false}`))
	disableReq.Header.Set("Authorization", "Bearer operator-token")
	handler.ServeHTTP(disable, disableReq)
	if disable.Code != http.StatusOK {
		t.Fatalf("disable status=%d body=%s", disable.Code, disable.Body.String())
	}
	rejected := httptest.NewRecorder()
	handler.ServeHTTP(rejected, httptest.NewRequest(http.MethodPost, "/hub/v1/agents/register", bytes.NewReader(body)))
	if rejected.Code != http.StatusForbidden {
		t.Fatalf("registration after disable status=%d body=%s", rejected.Code, rejected.Body.String())
	}

	revoke := httptest.NewRecorder()
	revokeReq := httptest.NewRequest(http.MethodPost, "/hub/v1/admin/agents/"+response.Identity.AgentID+"/revoke", strings.NewReader(`{"reason":"test"}`))
	revokeReq.Header.Set("Authorization", "Bearer operator-token")
	handler.ServeHTTP(revoke, revokeReq)
	if revoke.Code != http.StatusOK {
		t.Fatalf("revoke status=%d body=%s", revoke.Code, revoke.Body.String())
	}
	if _, err := registry.Authenticate(response.Identity.AgentID, response.Identity.AgentToken); err == nil {
		t.Fatal("revoked peer token must be rejected")
	}
}
