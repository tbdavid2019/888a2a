package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
)

func TestRegisterA2ARoutesServesJSONAgentCard(t *testing.T) {
	e := echo.New()
	registerA2ARoutes(e, nil, "http://example.test", nil)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/agent-card.json", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type = %q, want application/json", got)
	}
	var card map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &card); err != nil {
		t.Fatalf("agent card is not JSON: %v", err)
	}
	if card["name"] != "888a2a Network Gateway" {
		t.Fatalf("agent card name = %v, want 888a2a Network Gateway", card["name"])
	}
}
