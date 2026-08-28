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
	registerHubRoutes(e, registry, nil, nil)
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
