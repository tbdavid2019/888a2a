package a2a

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

const maxHubRegistrationBody = MaxHubAgentCardBytes + 16*1024

// HubHTTPHandler exposes enrollment and peer lifecycle endpoints for external
// Agents. It does not expose provider credentials or native runtime sessions.
type HubHTTPHandler struct {
	Registry *HubRegistry
	Mailbox  HubMailbox
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
	case r.Method == http.MethodPost && strings.HasPrefix(path, "/agents/") && strings.HasSuffix(path, "/tasks"):
		agentID := strings.TrimSuffix(strings.TrimPrefix(path, "/agents/"), "/tasks")
		h.sendPeerTask(w, r, agentID)
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/agents/") && strings.HasSuffix(path, "/inbox"):
		agentID := strings.TrimSuffix(strings.TrimPrefix(path, "/agents/"), "/inbox")
		h.pollInbox(w, r, agentID)
	case r.Method == http.MethodPost && strings.HasPrefix(path, "/agents/") && strings.Contains(path, "/inbox/") && strings.HasSuffix(path, "/ack"):
		prefix := strings.TrimPrefix(path, "/agents/")
		parts := strings.Split(strings.TrimSuffix(prefix, "/ack"), "/inbox/")
		if len(parts) != 2 {
			writeHubError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "inbox acknowledgment path is invalid")
			return
		}
		sequence, err := strconv.ParseUint(parts[1], 10, 64)
		if err != nil {
			writeHubError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "inbox sequence is invalid")
			return
		}
		h.ackInbox(w, r, parts[0], sequence)
	default:
		writeHubError(w, http.StatusNotFound, "NOT_FOUND", "Hub route not found")
	}
}

type peerTaskRequest struct {
	ContextID      string `json:"contextId"`
	IdempotencyKey string `json:"idempotencyKey"`
	Message        string `json:"message"`
}

func (h HubHTTPHandler) sendPeerTask(w http.ResponseWriter, r *http.Request, targetAgentID string) {
	if h.Mailbox == nil {
		writeHubError(w, http.StatusServiceUnavailable, "HUB_MAILBOX_UNAVAILABLE", "Hub mailbox is unavailable")
		return
	}
	caller, ok := h.authenticateAgent(r)
	if !ok {
		writeHubError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Hub Agent credentials are required")
		return
	}
	if target, exists := h.Registry.LookupView(targetAgentID); !exists || target.State == HubAgentStateRevoked || target.State == HubAgentStateExpired {
		writeHubError(w, http.StatusNotFound, "NOT_FOUND", "target Hub Agent not found")
		return
	}
	var input peerTaskRequest
	r.Body = http.MaxBytesReader(w, r.Body, MaxBridgeInputBytes+4096)
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || strings.TrimSpace(input.Message) == "" || strings.TrimSpace(input.IdempotencyKey) == "" {
		writeHubError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "contextId, idempotencyKey, and message are required")
		return
	}
	if len([]byte(input.Message)) > MaxBridgeInputBytes {
		writeHubError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "message exceeds the Hub payload limit")
		return
	}
	if input.ContextID == "" {
		input.ContextID = uuid.NewString()
	}
	result, err := h.Mailbox.Enqueue(r.Context(), HubInboxItem{
		HubID: h.Registry.policy.HubID, TargetAgentID: targetAgentID, RequesterAgentID: caller.AgentID,
		TaskID: uuid.NewString(), ContextID: input.ContextID, IdempotencyKey: input.IdempotencyKey, Message: input.Message,
	})
	if err != nil {
		writeHubError(w, http.StatusConflict, "ALREADY_EXISTS", err.Error())
		return
	}
	writeHubJSON(w, http.StatusOK, result)
}

func (h HubHTTPHandler) pollInbox(w http.ResponseWriter, r *http.Request, agentID string) {
	if h.Mailbox == nil {
		writeHubError(w, http.StatusServiceUnavailable, "HUB_MAILBOX_UNAVAILABLE", "Hub mailbox is unavailable")
		return
	}
	caller, ok := h.authenticateAgent(r)
	if !ok || caller.AgentID != agentID {
		writeHubError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Hub Agent credentials are invalid")
		return
	}
	after := uint64(0)
	if raw := r.URL.Query().Get("afterSequence"); raw != "" {
		var err error
		after, err = strconv.ParseUint(raw, 10, 64)
		if err != nil {
			writeHubError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "afterSequence is invalid")
			return
		}
	}
	items, err := h.Mailbox.Poll(r.Context(), h.Registry.policy.HubID, agentID, after, 100)
	if err != nil {
		writeHubError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
		return
	}
	writeHubJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h HubHTTPHandler) ackInbox(w http.ResponseWriter, r *http.Request, agentID string, sequence uint64) {
	if h.Mailbox == nil {
		writeHubError(w, http.StatusServiceUnavailable, "HUB_MAILBOX_UNAVAILABLE", "Hub mailbox is unavailable")
		return
	}
	caller, ok := h.authenticateAgent(r)
	if !ok || caller.AgentID != agentID {
		writeHubError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Hub Agent credentials are invalid")
		return
	}
	if err := h.Mailbox.Acknowledge(r.Context(), h.Registry.policy.HubID, agentID, sequence); err != nil {
		writeHubError(w, http.StatusNotFound, "NOT_FOUND", "Hub inbox item not found")
		return
	}
	writeHubJSON(w, http.StatusOK, map[string]any{"acknowledged": true, "sequence": sequence})
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
