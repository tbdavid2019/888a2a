package a2a

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	card := httptest.NewRecorder()
	cardReq := httptest.NewRequest(http.MethodGet, "/hub/v1/agents/"+publicIdentity.Identity.AgentID+"/agent-card.json", nil)
	handler.ServeHTTP(card, cardReq)
	if card.Code != http.StatusOK || !strings.Contains(card.Body.String(), "supportedInterfaces") || strings.Contains(card.Body.String(), publicIdentity.Identity.AgentToken) {
		t.Fatalf("public card status=%d body=%s", card.Code, card.Body.String())
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
	mailbox := NewMemoryHubMailbox()
	shutdownCalled := false
	handler := HubHTTPHandler{Registry: registry, Mailbox: mailbox, Shutdown: func(context.Context) error { shutdownCalled = true; return nil }}
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
	queued, err := mailbox.Enqueue(context.Background(), HubInboxItem{HubID: policy.HubID, TargetAgentID: response.Identity.AgentID, RequesterAgentID: "requester", TaskID: "task-cancel", ContextID: "ctx", IdempotencyKey: "cancel-key", Message: "cancel me"})
	if err != nil {
		t.Fatal(err)
	}
	cancel := httptest.NewRecorder()
	cancelReq := httptest.NewRequest(http.MethodPost, "/hub/v1/admin/tasks/"+queued.Item.TaskID+"/cancel", nil)
	cancelReq.Header.Set("Authorization", "Bearer operator-token")
	handler.ServeHTTP(cancel, cancelReq)
	if cancel.Code != http.StatusOK || !strings.Contains(cancel.Body.String(), "CANCELED") {
		t.Fatalf("cancel status=%d body=%s", cancel.Code, cancel.Body.String())
	}
	shutdown := httptest.NewRecorder()
	shutdownReq := httptest.NewRequest(http.MethodPost, "/hub/v1/admin/shutdown", nil)
	shutdownReq.Header.Set("Authorization", "Bearer operator-token")
	handler.ServeHTTP(shutdown, shutdownReq)
	if shutdown.Code != http.StatusOK || !shutdownCalled {
		t.Fatalf("shutdown status=%d called=%v body=%s", shutdown.Code, shutdownCalled, shutdown.Body.String())
	}
}

func TestHubHTTPOperatorCanChangeMode(t *testing.T) {
	policy := DefaultHubPolicy()
	policy.Mode = HubModePublic
	policy.HubID = "hub-mode-switch"
	policy.PublicConfirmed = true
	policy.RegistrationEnabled = true
	registry, err := NewHubRegistry(policy, "bootstrap-token", nil)
	if err != nil {
		t.Fatal(err)
	}
	registry.SetOperatorToken("operator-token")
	handler := HubHTTPHandler{Registry: registry}

	changeMode := func(mode string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/hub/v1/admin/mode", strings.NewReader(`{"mode":"`+mode+`"}`))
		req.Header.Set("Authorization", "Bearer operator-token")
		handler.ServeHTTP(recorder, req)
		return recorder
	}

	if response := changeMode("closed"); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"mode":"closed"`) {
		t.Fatalf("closed mode status=%d body=%s", response.Code, response.Body.String())
	}
	registration := httptest.NewRecorder()
	registrationRequest := httptest.NewRequest(http.MethodPost, "/hub/v1/admin/registration", strings.NewReader(`{"enabled":true}`))
	registrationRequest.Header.Set("Authorization", "Bearer operator-token")
	handler.ServeHTTP(registration, registrationRequest)
	if registration.Code != http.StatusBadRequest {
		t.Fatalf("closed registration status=%d body=%s", registration.Code, registration.Body.String())
	}
	if response := changeMode("open"); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"mode":"open"`) {
		t.Fatalf("open mode status=%d body=%s", response.Code, response.Body.String())
	}
	if response := changeMode("public"); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"mode":"public"`) {
		t.Fatalf("public mode status=%d body=%s", response.Code, response.Body.String())
	}

	status := httptest.NewRecorder()
	handler.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/hub/v1/status", nil))
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"mode":"public"`) {
		t.Fatalf("status code=%d body=%s", status.Code, status.Body.String())
	}
}

func TestHubHTTPOperatorCannotChangeModeWithoutCredentials(t *testing.T) {
	registry, err := NewHubRegistry(DefaultHubPolicy(), "bootstrap-token", nil)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/hub/v1/admin/mode", strings.NewReader(`{"mode":"closed"}`))
	HubHTTPHandler{Registry: registry}.ServeHTTP(response, req)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestHubHTTPAuthenticatedBrowserOperatorCanChangeMode(t *testing.T) {
	policy := DefaultHubPolicy()
	policy.HubID = "hub-browser-mode"
	registry, err := NewHubRegistry(policy, "bootstrap-token", nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := HubHTTPHandler{
		Registry: registry,
		AuthorizeBrowser: func(*http.Request) bool {
			return true
		},
	}
	response := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/hub/v1/admin/mode", strings.NewReader(`{"mode":"closed"}`))
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"mode":"closed"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestHubHTTPEnforcesPendingTaskConcurrency(t *testing.T) {
	policy := DefaultHubPolicy()
	policy.Mode = HubModePublic
	policy.HubID = "hub-concurrency"
	policy.PublicConfirmed = true
	policy.RegistrationEnabled = true
	policy.MaxConcurrentTasks = 1
	registry, err := NewHubRegistry(policy, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	mailbox := NewMemoryHubMailbox()
	handler := HubHTTPHandler{Registry: registry, Mailbox: mailbox}
	body, _ := json.Marshal(validAgentDeclaration("concurrency"))
	register := httptest.NewRecorder()
	handler.ServeHTTP(register, httptest.NewRequest(http.MethodPost, "/hub/v1/agents/register", bytes.NewReader(body)))
	var response struct {
		Identity IssuedAgentIdentity `json:"identity"`
	}
	if err := json.Unmarshal(register.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	heartbeat := httptest.NewRequest(http.MethodPost, "/hub/v1/agents/"+response.Identity.AgentID+"/heartbeat", nil)
	heartbeat.Header.Set("X-Agent-ID", response.Identity.AgentID)
	heartbeat.Header.Set("Authorization", "Bearer "+response.Identity.AgentToken)
	handler.ServeHTTP(httptest.NewRecorder(), heartbeat)
	post := func(key string) *httptest.ResponseRecorder {
		record := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/hub/v1/agents/"+response.Identity.AgentID+"/tasks", strings.NewReader(`{"message":"work","idempotencyKey":"`+key+`"}`))
		req.Header.Set("X-Agent-ID", response.Identity.AgentID)
		req.Header.Set("Authorization", "Bearer "+response.Identity.AgentToken)
		handler.ServeHTTP(record, req)
		return record
	}
	first := post("one")
	second := post("two")
	if first.Code != http.StatusOK || second.Code != http.StatusTooManyRequests || !strings.Contains(second.Body.String(), "CONCURRENCY_LIMIT") {
		t.Fatalf("concurrency statuses=%d/%d bodies=%s/%s", first.Code, second.Code, first.Body.String(), second.Body.String())
	}
}

func TestHubHTTPRejectsCrossHubPeerCredentials(t *testing.T) {
	newHandler := func(hubID string) HubHTTPHandler {
		policy := DefaultHubPolicy()
		policy.Mode = HubModePublic
		policy.HubID = hubID
		policy.PublicConfirmed = true
		policy.RegistrationEnabled = true
		registry, err := NewHubRegistry(policy, "", nil)
		if err != nil {
			t.Fatal(err)
		}
		mailbox := NewMemoryHubMailbox()
		return HubHTTPHandler{Registry: registry, Mailbox: mailbox}
	}
	firstHub := newHandler("hub-one")
	secondHub := newHandler("hub-two")
	body, _ := json.Marshal(validAgentDeclaration("cross-hub"))
	register := httptest.NewRecorder()
	firstHub.ServeHTTP(register, httptest.NewRequest(http.MethodPost, "/hub/v1/agents/register", bytes.NewReader(body)))
	var first struct {
		Identity IssuedAgentIdentity `json:"identity"`
	}
	if err := json.Unmarshal(register.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	secondBody, err := json.Marshal(validAgentDeclaration("cross-hub-target"))
	if err != nil {
		t.Fatal(err)
	}
	secondRegister := httptest.NewRecorder()
	secondHub.ServeHTTP(secondRegister, httptest.NewRequest(http.MethodPost, "/hub/v1/agents/register", bytes.NewReader(secondBody)))
	var second struct {
		Identity IssuedAgentIdentity `json:"identity"`
	}
	if err := json.Unmarshal(secondRegister.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/hub/v1/agents/"+second.Identity.AgentID+"/tasks", strings.NewReader(`{"message":"cross hub","idempotencyKey":"cross-hub-task"}`))
	req.Header.Set("X-Agent-ID", first.Identity.AgentID)
	req.Header.Set("Authorization", "Bearer "+first.Identity.AgentToken)
	rec := httptest.NewRecorder()
	secondHub.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("cross-Hub request status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHubRateLimiterEvictsExpiredEntries(t *testing.T) {
	limiter := NewHubRateLimiter(10, time.Minute)
	now := time.Now().UTC()
	for i := 0; i < 1100; i++ {
		limiter.Allow(fmt.Sprintf("ip-%d", i), now)
	}
	if len(limiter.entries) < 1100 {
		t.Fatalf("expected at least 1100 entries before expiry, got %d", len(limiter.entries))
	}
	future := now.Add(2 * time.Minute)
	limiter.Allow("new-ip", future)
	if len(limiter.entries) > 1024 {
		t.Fatalf("expected entries to be cleaned up below 1025, got %d", len(limiter.entries))
	}
}

func TestHubHTTPRejectsGroupPrefixInDirectTask(t *testing.T) {
	policy := DefaultHubPolicy()
	policy.Mode = HubModePublic
	policy.HubID = "hub-test"
	policy.PublicConfirmed = true
	policy.RegistrationEnabled = true
	registry, err := NewHubRegistry(policy, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := HubHTTPHandler{Registry: registry, Mailbox: NewMemoryHubMailbox()}
	regBody, _ := json.Marshal(validAgentDeclaration("sender-peer"))
	regRec := httptest.NewRecorder()
	handler.ServeHTTP(regRec, httptest.NewRequest(http.MethodPost, "/hub/v1/agents/register", bytes.NewReader(regBody)))
	var sender struct {
		Identity IssuedAgentIdentity `json:"identity"`
	}
	_ = json.Unmarshal(regRec.Body.Bytes(), &sender)

	tgtBody, _ := json.Marshal(validAgentDeclaration("target-peer"))
	tgtRec := httptest.NewRecorder()
	handler.ServeHTTP(tgtRec, httptest.NewRequest(http.MethodPost, "/hub/v1/agents/register", bytes.NewReader(tgtBody)))
	var target struct {
		Identity IssuedAgentIdentity `json:"identity"`
	}
	_ = json.Unmarshal(tgtRec.Body.Bytes(), &target)

	// Heartbeat target so it is online
	hbReq := httptest.NewRequest(http.MethodPost, "/hub/v1/agents/"+target.Identity.AgentID+"/heartbeat", nil)
	hbReq.Header.Set("X-Agent-ID", target.Identity.AgentID)
	hbReq.Header.Set("Authorization", "Bearer "+target.Identity.AgentToken)
	hbRec := httptest.NewRecorder()
	handler.ServeHTTP(hbRec, hbReq)
	if hbRec.Code != http.StatusOK {
		t.Fatalf("target heartbeat failed: status=%d body=%s", hbRec.Code, hbRec.Body.String())
	}

	taskPayload := `{"contextId":"c1","idempotencyKey":"group:grp-1:msg-1","message":"hello"}`
	taskReq := httptest.NewRequest(http.MethodPost, "/hub/v1/agents/"+target.Identity.AgentID+"/tasks", strings.NewReader(taskPayload))
	taskReq.Header.Set("X-Agent-ID", sender.Identity.AgentID)
	taskReq.Header.Set("Authorization", "Bearer "+sender.Identity.AgentToken)
	taskRec := httptest.NewRecorder()
	handler.ServeHTTP(taskRec, taskReq)
	if taskRec.Code != http.StatusBadRequest || !strings.Contains(taskRec.Body.String(), "group:") {
		t.Fatalf("expected 400 for reserved group: prefix, got code=%d body=%s", taskRec.Code, taskRec.Body.String())
	}
}

func TestHubHTTPCardRespectsForwardedProto(t *testing.T) {
	policy := DefaultHubPolicy()
	policy.Mode = HubModePublic
	policy.HubID = "hub-test"
	policy.PublicConfirmed = true
	policy.RegistrationEnabled = true
	registry, err := NewHubRegistry(policy, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	handler := HubHTTPHandler{Registry: registry}
	regBody, _ := json.Marshal(validAgentDeclaration("proto-peer"))
	regRec := httptest.NewRecorder()
	handler.ServeHTTP(regRec, httptest.NewRequest(http.MethodPost, "/hub/v1/agents/register", bytes.NewReader(regBody)))
	var reg struct {
		Identity IssuedAgentIdentity `json:"identity"`
	}
	_ = json.Unmarshal(regRec.Body.Bytes(), &reg)

	req := httptest.NewRequest(http.MethodGet, "/hub/v1/agents/"+reg.Identity.AgentID+"/agent-card.json", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("card status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "https://") {
		t.Fatalf("expected card to use https, got %s", rec.Body.String())
	}
}

func TestHubHTTPAdminListMessages(t *testing.T) {
	policy := DefaultHubPolicy()
	policy.Mode = HubModePublic
	policy.HubID = "hub-test"
	policy.PublicConfirmed = true
	policy.RegistrationEnabled = true
	registry, err := NewHubRegistry(policy, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	registry.SetOperatorToken("super-secret-operator")
	mailbox := NewMemoryHubMailbox()
	handler := HubHTTPHandler{Registry: registry, Mailbox: mailbox}

	// 1. Unauthorized access without token
	unauthReq := httptest.NewRequest(http.MethodGet, "/hub/v1/admin/messages", nil)
	unauthRec := httptest.NewRecorder()
	handler.ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthorized admin access, got %d", unauthRec.Code)
	}

	// 2. Authorized access with operator token
	authReq := httptest.NewRequest(http.MethodGet, "/hub/v1/admin/messages", nil)
	authReq.Header.Set("Authorization", "Bearer super-secret-operator")
	authRec := httptest.NewRecorder()
	handler.ServeHTTP(authRec, authReq)
	if authRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for operator token, got %d body=%s", authRec.Code, authRec.Body.String())
	}

	// 3. Enqueue item and verify listing
	_, _ = mailbox.Enqueue(context.Background(), HubInboxItem{
		HubID: policy.HubID, TargetAgentID: "target-1", RequesterAgentID: "sender-1",
		TaskID: "task-123", ContextID: "ctx-123", IdempotencyKey: "key-123",
		Message: "test admin message body",
	})
	listReq := httptest.NewRequest(http.MethodGet, "/hub/v1/admin/messages?agentId=target-1", nil)
	listReq.Header.Set("Authorization", "Bearer super-secret-operator")
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", listRec.Code)
	}
	if !strings.Contains(listRec.Body.String(), "test admin message body") || !strings.Contains(listRec.Body.String(), "task-123") {
		t.Fatalf("expected message body in admin list, got %s", listRec.Body.String())
	}
}
