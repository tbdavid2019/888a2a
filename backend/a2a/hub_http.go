package a2a

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

const maxHubRegistrationBody = MaxHubAgentCardBytes + 16*1024

// HubHTTPHandler exposes enrollment and peer lifecycle endpoints for external
// Agents. It does not expose provider credentials or native runtime sessions.
type HubHTTPHandler struct {
	Registry *HubRegistry
}

func (h HubHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Registry == nil {
		writeHubError(w, http.StatusServiceUnavailable, "HUB_UNAVAILABLE", "Hub registry is unavailable")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/hub/v1")
	switch {
	case r.Method == http.MethodPost && path == "/agents/register":
		h.register(w, r)
	case r.Method == http.MethodGet && path == "/agents":
		h.list(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(path, "/agents/") && strings.HasSuffix(path, "/heartbeat"):
		agentID := strings.TrimSuffix(strings.TrimPrefix(path, "/agents/"), "/heartbeat")
		h.heartbeat(w, r, agentID)
	case r.Method == http.MethodPost && strings.HasPrefix(path, "/agents/") && strings.HasSuffix(path, "/disconnect"):
		agentID := strings.TrimSuffix(strings.TrimPrefix(path, "/agents/"), "/disconnect")
		h.disconnect(w, r, agentID)
	default:
		writeHubError(w, http.StatusNotFound, "NOT_FOUND", "Hub route not found")
	}
}

func (h HubHTTPHandler) disconnect(w http.ResponseWriter, r *http.Request, agentID string) {
	if agentID == "" || strings.ContainsAny(agentID, "/\\") {
		writeHubError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "agent id is invalid")
		return
	}
	if agent, ok := h.authenticateAgent(r); !ok || agent.AgentID != agentID {
		writeHubError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Hub Agent credentials are invalid")
		return
	}
	updated, err := h.Registry.DisconnectContext(r.Context(), agentID, bearerToken(r.Header.Get("Authorization")))
	if err != nil {
		writeHubError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Hub Agent credentials are invalid")
		return
	}
	writeHubJSON(w, http.StatusOK, updated.View())
}

func (h HubHTTPHandler) register(w http.ResponseWriter, r *http.Request) {
	var declaration AgentDeclaration
	r.Body = http.MaxBytesReader(w, r.Body, maxHubRegistrationBody)
	if err := json.NewDecoder(r.Body).Decode(&declaration); err != nil {
		writeHubError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid Agent declaration")
		return
	}
	identity, err := h.Registry.RegisterContext(r.Context(), bearerToken(r.Header.Get("Authorization")), declaration)
	if err != nil {
		status, code := http.StatusForbidden, "PERMISSION_DENIED"
		if errors.Is(err, ErrHubRegistrationDisabled) {
			code = "REGISTRATION_DISABLED"
		} else if strings.Contains(err.Error(), "bootstrap token") {
			status, code = http.StatusUnauthorized, "UNAUTHENTICATED"
		} else if strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "large") || strings.Contains(err.Error(), "invalid") {
			status, code = http.StatusBadRequest, "INVALID_ARGUMENT"
		}
		writeHubError(w, status, code, err.Error())
		return
	}
	writeHubJSON(w, http.StatusOK, map[string]any{
		"identity": identity,
		"policy":   map[string]any{"hubId": h.Registry.policy.HubID, "mode": h.Registry.policy.Mode},
	})
}

func (h HubHTTPHandler) list(w http.ResponseWriter, r *http.Request) {
	if h.Registry.policy.Mode != HubModePublic {
		if _, ok := h.authenticateAgent(r); !ok {
			writeHubError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Hub Agent credentials are required")
			return
		}
	}
	writeHubJSON(w, http.StatusOK, map[string]any{"hubId": h.Registry.policy.HubID, "agents": h.Registry.ListViews()})
}

func (h HubHTTPHandler) heartbeat(w http.ResponseWriter, r *http.Request, agentID string) {
	if agentID == "" || strings.ContainsAny(agentID, "/\\") {
		writeHubError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "agent id is invalid")
		return
	}
	agent, ok := h.authenticateAgent(r)
	if !ok || agent.AgentID != agentID {
		writeHubError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Hub Agent credentials are invalid")
		return
	}
	updated, err := h.Registry.HeartbeatContext(r.Context(), agentID, bearerToken(r.Header.Get("Authorization")))
	if err != nil {
		writeHubError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Hub Agent credentials are invalid")
		return
	}
	writeHubJSON(w, http.StatusOK, updated.View())
}

func (h HubHTTPHandler) authenticateAgent(r *http.Request) (*RegisteredAgent, bool) {
	agentID := strings.TrimSpace(r.Header.Get("X-Agent-ID"))
	token := bearerToken(r.Header.Get("Authorization"))
	if agentID == "" || token == "" {
		return nil, false
	}
	agent, err := h.Registry.Authenticate(agentID, token)
	return agent, err == nil
}

func bearerToken(header string) string {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return parts[1]
}

func writeHubJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeHubError(w http.ResponseWriter, status int, code, message string) {
	writeHubJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
