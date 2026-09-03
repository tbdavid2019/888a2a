package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	a2agateway "github.com/tbdavid2019/888a2a/backend/a2a"
)

func TestRegisterHubRoutesServesOpenRegistration(t *testing.T) {
	policy := a2agateway.DefaultHubPolicy()
	policy.Mode = a2agateway.HubModeOpen
	policy.HubID = "hub-route-test"
	policy.RegistrationEnabled = true
	registry, err := a2agateway.NewHubRegistry(policy, "bootstrap", nil)
	if err != nil {
		t.Fatal(err)
	}
	e := echo.New()
	registerHubRoutes(e, registry, nil, nil, nil)
	body, _ := json.Marshal(a2agateway.AgentDeclaration{
		DisplayName: "route-agent", ProviderFamily: "agy", TransportID: "agy-cli",
		Capabilities: []string{"text"}, RegistrationIdempotencyKey: "route-key",
	})
	req := httptest.NewRequest(http.MethodPost, "/hub/v1/agents/register", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer bootstrap")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("route status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRegisterHubRoutesServesGroups(t *testing.T) {
	policy := a2agateway.DefaultHubPolicy()
	policy.Mode = a2agateway.HubModeOpen
	policy.HubID = "hub-route-grp-test"
	policy.RegistrationEnabled = true
	registry, err := a2agateway.NewHubRegistry(policy, "bootstrap", nil)
	if err != nil {
		t.Fatal(err)
	}
	mailbox := a2agateway.NewMemoryHubMailbox()
	groupStore := a2agateway.NewMemoryHubGroupStore(mailbox)
	e := echo.New()
	e.Any("/hub/v1/*", echo.WrapHandler(a2agateway.HubHTTPHandler{
		Registry: registry, Mailbox: mailbox, Groups: groupStore,
	}))

	// Register agent
	body, _ := json.Marshal(a2agateway.AgentDeclaration{
		DisplayName: "route-agent", ProviderFamily: "agy", TransportID: "agy-cli",
		Capabilities: []string{"text"}, RegistrationIdempotencyKey: "grp-route-key",
	})
	req := httptest.NewRequest(http.MethodPost, "/hub/v1/agents/register", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer bootstrap")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("reg status=%d body=%s", rec.Code, rec.Body.String())
	}
	var reg struct {
		Identity a2agateway.IssuedAgentIdentity `json:"identity"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &reg)

	// Create group
	grpBody, _ := json.Marshal(a2agateway.HubCreateGroupInput{Name: "Echo Team"})
	grpReq := httptest.NewRequest(http.MethodPost, "/hub/v1/groups", bytes.NewReader(grpBody))
	grpReq.Header.Set("X-Agent-ID", reg.Identity.AgentID)
	grpReq.Header.Set("Authorization", "Bearer "+reg.Identity.AgentToken)
	grpRec := httptest.NewRecorder()
	e.ServeHTTP(grpRec, grpReq)
	if grpRec.Code != http.StatusOK {
		t.Fatalf("create group status=%d body=%s", grpRec.Code, grpRec.Body.String())
	}
}
