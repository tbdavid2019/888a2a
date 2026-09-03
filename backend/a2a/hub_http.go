package a2a

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	standarda2a "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/google/uuid"
)

const maxHubRegistrationBody = MaxHubAgentCardBytes + 16*1024

// HubHTTPHandler exposes enrollment and peer lifecycle endpoints for external
// Agents. It does not expose provider credentials or native runtime sessions.
type HubHTTPHandler struct {
	Registry         *HubRegistry
	Mailbox          HubMailbox
	Groups           HubGroupStore
	Rate             *HubRateLimiter
	Shutdown         func(context.Context) error
	AuthorizeBrowser func(*http.Request) bool
}

type HubRateLimiter struct {
	mu      sync.Mutex
	window  time.Duration
	limit   int
	entries map[string]hubRateEntry
}

type hubRateEntry struct {
	started time.Time
	count   int
}

func NewHubRateLimiter(limit int, window time.Duration) *HubRateLimiter {
	if limit <= 0 {
		limit = 60
	}
	if window <= 0 {
		window = time.Minute
	}
	return &HubRateLimiter{limit: limit, window: window, entries: make(map[string]hubRateEntry)}
}

func (l *HubRateLimiter) Allow(key string, now time.Time) bool {
	if l == nil {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, exists := l.entries[key]
	if !exists || entry.started.IsZero() || now.Sub(entry.started) >= l.window {
		l.entries[key] = hubRateEntry{started: now, count: 1}
		l.cleanupLocked(now)
		return true
	}
	if entry.count >= l.limit {
		return false
	}
	entry.count++
	l.entries[key] = entry
	return true
}

func (l *HubRateLimiter) cleanupLocked(now time.Time) {
	if len(l.entries) <= 1024 {
		return
	}
	for k, v := range l.entries {
		if now.Sub(v.started) >= l.window {
			delete(l.entries, k)
		}
	}
}

func (h HubHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Registry == nil {
		writeHubError(w, http.StatusServiceUnavailable, "HUB_UNAVAILABLE", "Hub registry is unavailable")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/hub/v1")
	switch {
	case r.Method == http.MethodPost && path == "/agents/register":
		if h.Registry.Policy().Mode == HubModePublic && !h.allow(w, r, "register") {
			return
		}
		h.register(w, r)
	case r.Method == http.MethodGet && path == "/agents":
		h.list(w, r)
	case r.Method == http.MethodGet && path == "/status":
		h.status(w, r)
	case r.Method == http.MethodPost && path == "/admin/registration":
		h.setRegistration(w, r)
	case r.Method == http.MethodPost && path == "/admin/mode":
		h.setMode(w, r)
	case r.Method == http.MethodPost && path == "/admin/shutdown":
		h.shutdown(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(path, "/admin/agents/") && strings.HasSuffix(path, "/revoke"):
		h.revoke(w, r, strings.TrimSuffix(strings.TrimPrefix(path, "/admin/agents/"), "/revoke"))
	case r.Method == http.MethodPost && strings.HasPrefix(path, "/admin/tasks/") && strings.HasSuffix(path, "/cancel"):
		h.cancelTask(w, r, strings.TrimSuffix(strings.TrimPrefix(path, "/admin/tasks/"), "/cancel"))
	case r.Method == http.MethodGet && path == "/admin/messages":
		h.listMessages(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/agents/") && strings.Count(path, "/") == 2:
		h.lookup(w, r, strings.TrimPrefix(path, "/agents/"))
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/agents/") && strings.HasSuffix(path, "/agent-card.json"):
		agentID := strings.TrimSuffix(strings.TrimPrefix(path, "/agents/"), "/agent-card.json")
		h.card(w, r, agentID)
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
	case strings.HasPrefix(path, "/groups"):
		segments := strings.Split(strings.Trim(path, "/"), "/")
		switch {
		case len(segments) == 1 && r.Method == http.MethodPost:
			h.createGroup(w, r)
		case len(segments) == 1 && r.Method == http.MethodGet:
			h.listGroups(w, r)
		case len(segments) == 2 && segments[1] == "invitations" && r.Method == http.MethodGet:
			h.listInvitations(w, r)
		case len(segments) == 2 && segments[1] != "invitations" && r.Method == http.MethodGet:
			h.getGroup(w, r, segments[1])
		case len(segments) == 3 && segments[2] == "archive" && r.Method == http.MethodPost:
			h.archiveGroup(w, r, segments[1])
		case len(segments) == 3 && segments[2] == "leave" && r.Method == http.MethodPost:
			h.leaveGroup(w, r, segments[1])
		case len(segments) == 3 && segments[2] == "invitations" && r.Method == http.MethodPost:
			h.createInvitation(w, r, segments[1])
		case len(segments) == 3 && segments[2] == "messages" && r.Method == http.MethodPost:
			h.sendGroupMessage(w, r, segments[1])
		case len(segments) == 3 && segments[2] == "messages" && r.Method == http.MethodGet:
			h.listGroupMessages(w, r, segments[1])
		case len(segments) == 4 && segments[1] == "invitations":
			id, err := strconv.ParseUint(segments[2], 10, 64)
			if err != nil {
				writeHubError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invitation ID is invalid")
				return
			}
			switch {
			case segments[3] == "accept" && r.Method == http.MethodPost:
				h.acceptInvitation(w, r, id)
			case segments[3] == "decline" && r.Method == http.MethodPost:
				h.declineInvitation(w, r, id)
			case segments[3] == "revoke" && r.Method == http.MethodPost:
				h.revokeInvitation(w, r, id)
			default:
				writeHubError(w, http.StatusNotFound, "NOT_FOUND", "Hub route not found")
			}
		case len(segments) == 5 && segments[2] == "members" && segments[4] == "remove" && r.Method == http.MethodPost:
			h.removeMember(w, r, segments[1], segments[3])
		default:
			writeHubError(w, http.StatusNotFound, "NOT_FOUND", "Hub route not found")
		}
	default:
		writeHubError(w, http.StatusNotFound, "NOT_FOUND", "Hub route not found")
	}
}

func (h HubHTTPHandler) setRegistration(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeOperator(r) {
		writeHubError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Hub operator credentials are required")
		return
	}
	var input struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&input); err != nil || input.Enabled == nil {
		writeHubError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "enabled is required")
		return
	}
	if err := h.Registry.SetRegistrationEnabledContext(r.Context(), *input.Enabled); err != nil {
		if strings.Contains(err.Error(), "closed Hub mode") {
			writeHubError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
			return
		}
		writeHubError(w, http.StatusServiceUnavailable, "POLICY_UNAVAILABLE", "Hub registration policy is unavailable")
		return
	}
	writeHubJSON(w, http.StatusOK, map[string]any{"registrationEnabled": *input.Enabled})
}

func (h HubHTTPHandler) setMode(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeOperator(r) {
		writeHubError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Hub operator credentials are required")
		return
	}
	var input struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&input); err != nil || strings.TrimSpace(input.Mode) == "" {
		writeHubError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "mode is required")
		return
	}
	if err := h.Registry.SetModeContext(r.Context(), HubMode(strings.TrimSpace(input.Mode))); err != nil {
		status := http.StatusBadRequest
		code := "INVALID_ARGUMENT"
		if strings.Contains(err.Error(), "persist") {
			status = http.StatusServiceUnavailable
			code = "POLICY_UNAVAILABLE"
		}
		writeHubError(w, status, code, err.Error())
		return
	}
	policy := h.Registry.Policy()
	writeHubJSON(w, http.StatusOK, map[string]any{"mode": policy.Mode, "registrationEnabled": policy.RegistrationEnabled})
}

func (h HubHTTPHandler) revoke(w http.ResponseWriter, r *http.Request, agentID string) {
	if !h.authorizeOperator(r) {
		writeHubError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Hub operator credentials are required")
		return
	}
	if agentID == "" || strings.ContainsAny(agentID, "/\\") {
		writeHubError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "agent id is invalid")
		return
	}
	var input struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&input)
	if err := h.Registry.RevokeContext(r.Context(), agentID, input.Reason); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeHubError(w, http.StatusNotFound, "NOT_FOUND", "Hub Agent not found")
			return
		}
		writeHubError(w, http.StatusServiceUnavailable, "POLICY_UNAVAILABLE", "Hub revoke policy is unavailable")
		return
	}
	view, _ := h.Registry.LookupView(agentID)
	writeHubJSON(w, http.StatusOK, view)
}

func (h HubHTTPHandler) cancelTask(w http.ResponseWriter, r *http.Request, taskID string) {
	if !h.authorizeOperator(r) {
		writeHubError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Hub operator credentials are required")
		return
	}
	if h.Mailbox == nil || taskID == "" || strings.ContainsAny(taskID, "/\\") {
		writeHubError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "task ID is invalid or Hub mailbox is unavailable")
		return
	}
	policy := h.Registry.Policy()
	if err := h.Mailbox.Cancel(r.Context(), policy.HubID, taskID, time.Now().UTC()); err != nil {
		writeHubError(w, http.StatusNotFound, "NOT_FOUND", "Hub task not found or already acknowledged")
		return
	}
	writeHubJSON(w, http.StatusOK, map[string]any{"taskId": taskID, "state": "CANCELED"})
}

func (h HubHTTPHandler) listMessages(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeOperator(r) {
		writeHubError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Hub operator credentials are required")
		return
	}
	adminMailbox, ok := h.Mailbox.(HubMailboxAdmin)
	if !ok || h.Mailbox == nil {
		writeHubError(w, http.StatusServiceUnavailable, "HUB_MAILBOX_UNAVAILABLE", "Hub admin mailbox is unavailable")
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("agentId"))
	limit := 50
	if limitParam := r.URL.Query().Get("limit"); limitParam != "" {
		if parsed, err := strconv.Atoi(limitParam); err == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}
	var beforeSequence uint64
	if seqParam := r.URL.Query().Get("beforeSequence"); seqParam != "" {
		if parsed, err := strconv.ParseUint(seqParam, 10, 64); err == nil {
			beforeSequence = parsed
		}
	}
	policy := h.Registry.Policy()
	items, err := adminMailbox.ListMessagesAdmin(r.Context(), policy.HubID, agentID, beforeSequence, limit)
	if err != nil {
		writeHubError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list messages")
		return
	}
	writeHubJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h HubHTTPHandler) shutdown(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeOperator(r) {
		writeHubError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Hub operator credentials are required")
		return
	}
	if h.Shutdown == nil {
		writeHubError(w, http.StatusServiceUnavailable, "SHUTDOWN_UNAVAILABLE", "Hub shutdown control is unavailable")
		return
	}
	if err := h.Shutdown(r.Context()); err != nil {
		writeHubError(w, http.StatusServiceUnavailable, "SHUTDOWN_FAILED", "Hub shutdown failed")
		return
	}
	writeHubJSON(w, http.StatusOK, map[string]any{"shuttingDown": true})
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
	policy := h.Registry.Policy()
	if policy.Mode == HubModePublic && !h.allow(w, r, "task") {
		return
	}
	if target, exists := h.Registry.LookupView(targetAgentID); !exists {
		writeHubError(w, http.StatusNotFound, "NOT_FOUND", "target Hub Agent not found")
		return
	} else if target.State != HubAgentStateOnline {
		writeHubError(w, http.StatusConflict, "TARGET_UNAVAILABLE", "target Hub Agent is not online")
		return
	}
	var input peerTaskRequest
	r.Body = http.MaxBytesReader(w, r.Body, MaxBridgeInputBytes+4096)
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || strings.TrimSpace(input.Message) == "" || strings.TrimSpace(input.IdempotencyKey) == "" {
		writeHubError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "contextId, idempotencyKey, and message are required")
		return
	}
	if strings.HasPrefix(strings.TrimSpace(input.IdempotencyKey), "group:") {
		writeHubError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "idempotency key must not use reserved 'group:' prefix")
		return
	}
	if len([]byte(input.Message)) > MaxBridgeInputBytes {
		writeHubError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "message exceeds the Hub payload limit")
		return
	}
	if input.ContextID == "" {
		input.ContextID = uuid.NewString()
	}
	if existing, found, err := h.Mailbox.Find(r.Context(), policy.HubID, targetAgentID, caller.AgentID, input.IdempotencyKey); err != nil {
		writeHubError(w, http.StatusServiceUnavailable, "HUB_MAILBOX_UNAVAILABLE", "Hub mailbox lookup failed")
		return
	} else if found {
		writeHubJSON(w, http.StatusOK, HubInboxEnqueueResult{Item: existing, Duplicate: true})
		return
	}
	pending, err := h.Mailbox.PendingCount(r.Context(), policy.HubID)
	if err != nil {
		writeHubError(w, http.StatusServiceUnavailable, "HUB_MAILBOX_UNAVAILABLE", "Hub mailbox capacity is unavailable")
		return
	}
	if int32(pending) >= policy.MaxConcurrentTasks {
		writeHubError(w, http.StatusTooManyRequests, "CONCURRENCY_LIMIT", "Hub task concurrency limit reached")
		return
	}
	result, err := h.Mailbox.Enqueue(r.Context(), HubInboxItem{
		HubID: policy.HubID, TargetAgentID: targetAgentID, RequesterAgentID: caller.AgentID,
		TaskID: uuid.NewString(), ContextID: input.ContextID, IdempotencyKey: input.IdempotencyKey, Message: input.Message,
	})
	if err != nil {
		writeHubError(w, http.StatusConflict, "ALREADY_EXISTS", err.Error())
		return
	}
	writeHubJSON(w, http.StatusOK, result)
}

func (h HubHTTPHandler) card(w http.ResponseWriter, r *http.Request, agentID string) {
	if agentID == "" || strings.ContainsAny(agentID, "/\\") {
		writeHubError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "agent id is invalid")
		return
	}
	if h.Registry.Policy().Mode != HubModePublic {
		if _, ok := h.authenticateAgent(r); !ok {
			writeHubError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Hub Agent credentials are required")
			return
		}
	}
	view, ok := h.Registry.LookupView(agentID)
	if !ok {
		writeHubError(w, http.StatusNotFound, "NOT_FOUND", "Hub Agent not found")
		return
	}
	proto := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		proto = "https"
	}
	base := proto + "://" + r.Host
	card := standarda2a.AgentCard{
		Name:        view.DisplayName,
		Description: "Hub peer " + view.AgentID + ". Automatic execution is disabled for Hub enrollment.",
		Version:     view.Card.Version,
		SupportedInterfaces: []*standarda2a.AgentInterface{
			standarda2a.NewAgentInterface(base+"/hub/v1/agents/"+agentID+"/tasks", standarda2a.TransportProtocolHTTPJSON),
		},
		Capabilities:       standarda2a.AgentCapabilities{},
		DefaultInputModes:  []string{"text/plain"},
		DefaultOutputModes: []string{"text/plain"},
		Skills:             hubCardSkills(view.Capabilities),
		Provider:           &standarda2a.AgentProvider{Org: "888a2a Hub " + view.HubID, URL: base},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(card)
}

func hubCardSkills(capabilities []string) []standarda2a.AgentSkill {
	skills := make([]standarda2a.AgentSkill, 0, len(capabilities))
	for _, capability := range capabilities {
		capability = strings.TrimSpace(capability)
		if capability == "" {
			continue
		}
		skills = append(skills, standarda2a.AgentSkill{ID: capability, Name: capability, InputModes: []string{"text/plain"}, OutputModes: []string{"text/plain"}})
	}
	return skills
}

func (h HubHTTPHandler) lookup(w http.ResponseWriter, r *http.Request, agentID string) {
	if agentID == "" || strings.ContainsAny(agentID, "/\\") {
		writeHubError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "agent id is invalid")
		return
	}
	if h.Registry.Policy().Mode != HubModePublic {
		if _, ok := h.authenticateAgent(r); !ok {
			writeHubError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Hub Agent credentials are required")
			return
		}
	}
	view, ok := h.Registry.LookupView(agentID)
	if !ok {
		writeHubError(w, http.StatusNotFound, "NOT_FOUND", "Hub Agent not found")
		return
	}
	writeHubJSON(w, http.StatusOK, view)
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
	items, err := h.Mailbox.Poll(r.Context(), h.Registry.Policy().HubID, agentID, after, 100)
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
	if err := h.Mailbox.Acknowledge(r.Context(), h.Registry.Policy().HubID, agentID, sequence); err != nil {
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
		switch {
		case errors.Is(err, ErrHubRegistrationDisabled):
			code = "REGISTRATION_DISABLED"
		case strings.Contains(err.Error(), "bootstrap token"):
			status, code = http.StatusUnauthorized, "UNAUTHENTICATED"
		case strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "large") || strings.Contains(err.Error(), "invalid"):
			status, code = http.StatusBadRequest, "INVALID_ARGUMENT"
		}
		writeHubError(w, status, code, err.Error())
		return
	}
	policy := h.Registry.Policy()
	writeHubJSON(w, http.StatusOK, map[string]any{
		"identity": identity,
		"policy":   map[string]any{"hubId": policy.HubID, "mode": policy.Mode},
	})
}

func (h HubHTTPHandler) list(w http.ResponseWriter, r *http.Request) {
	policy := h.Registry.Policy()
	if policy.Mode != HubModePublic {
		if _, ok := h.authenticateAgent(r); !ok && !h.authorizeOperator(r) {
			writeHubError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Hub Agent credentials are required")
			return
		}
	}
	writeHubJSON(w, http.StatusOK, map[string]any{"hubId": policy.HubID, "agents": h.Registry.ListViews()})
}

func (h HubHTTPHandler) status(w http.ResponseWriter, _ *http.Request) {
	policy := h.Registry.Policy()
	writeHubJSON(w, http.StatusOK, map[string]any{
		"hubId": policy.HubID, "mode": policy.Mode,
		"registrationEnabled":    policy.RegistrationEnabled,
		"registrationTTLSeconds": policy.RegistrationTTL,
		"peerLeaseSeconds":       policy.PeerLeaseSeconds,
		"maxRegisteredAgents":    policy.MaxRegisteredAgents,
		"maxTasksPerMinute":      policy.MaxTasksPerMinute,
		"maxConcurrentTasks":     policy.MaxConcurrentTasks,
		"maxPayloadBytes":        policy.MaxPayloadBytes,
	})
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

func (h HubHTTPHandler) allow(w http.ResponseWriter, r *http.Request, operation string) bool {
	if h.Rate == nil || h.Rate.Allow(operation+":"+hubRemoteHost(r.RemoteAddr), time.Now()) {
		return true
	}
	writeHubError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Hub request rate limit exceeded")
	return false
}

func hubRemoteHost(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return host
	}
	return remoteAddr
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

func (h HubHTTPHandler) authorizeOperator(r *http.Request) bool {
	if h.Registry.AuthorizeOperator(bearerToken(r.Header.Get("Authorization"))) {
		return true
	}
	return h.AuthorizeBrowser != nil && h.AuthorizeBrowser(r)
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

func (h HubHTTPHandler) createGroup(w http.ResponseWriter, r *http.Request) {
	if h.Groups == nil {
		writeHubError(w, http.StatusServiceUnavailable, "GROUPS_UNAVAILABLE", "Hub group store is unavailable")
		return
	}
	agent, ok := h.authenticateAgent(r)
	if !ok {
		writeHubError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Hub Agent credentials are required")
		return
	}
	if !h.allow(w, r, "createGroup") {
		return
	}
	var input HubCreateGroupInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024)).Decode(&input); err != nil {
		writeHubError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "request body is invalid")
		return
	}
	if err := ValidateCreateGroup(input); err != nil {
		writeHubError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
		return
	}
	policy := h.Registry.Policy()
	group, err := h.Groups.CreateGroup(r.Context(), HubGroup{
		HubID:        policy.HubID,
		Name:         input.Name,
		OwnerAgentID: agent.AgentID,
		State:        HubGroupStateActive,
		CreatedAt:    time.Now().UTC(),
	})
	if err != nil {
		writeHubError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create group")
		return
	}
	writeHubJSON(w, http.StatusOK, group)
}

func (h HubHTTPHandler) listGroups(w http.ResponseWriter, r *http.Request) {
	if h.Groups == nil {
		writeHubError(w, http.StatusServiceUnavailable, "GROUPS_UNAVAILABLE", "Hub group store is unavailable")
		return
	}
	agent, ok := h.authenticateAgent(r)
	if !ok {
		writeHubError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Hub Agent credentials are required")
		return
	}
	groups, err := h.Groups.ListGroups(r.Context(), agent.AgentID)
	if err != nil {
		writeHubError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list groups")
		return
	}
	writeHubJSON(w, http.StatusOK, map[string]any{"groups": groups})
}

func (h HubHTTPHandler) getGroup(w http.ResponseWriter, r *http.Request, groupID string) {
	if h.Groups == nil {
		writeHubError(w, http.StatusServiceUnavailable, "GROUPS_UNAVAILABLE", "Hub group store is unavailable")
		return
	}
	agent, ok := h.authenticateAgent(r)
	if !ok {
		writeHubError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Hub Agent credentials are required")
		return
	}
	member, err := h.Groups.FindMember(r.Context(), groupID, agent.AgentID)
	if err != nil || !member.IsActive() {
		writeHubError(w, http.StatusForbidden, "PERMISSION_DENIED", "caller is not an active member of group")
		return
	}
	group, err := h.Groups.FindGroup(r.Context(), groupID)
	if err != nil {
		writeHubError(w, http.StatusNotFound, "NOT_FOUND", "group not found")
		return
	}
	members, err := h.Groups.ListMembers(r.Context(), groupID)
	if err != nil {
		writeHubError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list group members")
		return
	}
	writeHubJSON(w, http.StatusOK, map[string]any{"group": group, "members": members})
}

func (h HubHTTPHandler) archiveGroup(w http.ResponseWriter, r *http.Request, groupID string) {
	if h.Groups == nil {
		writeHubError(w, http.StatusServiceUnavailable, "GROUPS_UNAVAILABLE", "Hub group store is unavailable")
		return
	}
	agent, ok := h.authenticateAgent(r)
	if !ok {
		writeHubError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Hub Agent credentials are required")
		return
	}
	member, err := h.Groups.FindMember(r.Context(), groupID, agent.AgentID)
	if err != nil || member.Role != HubGroupRoleOwner {
		writeHubError(w, http.StatusForbidden, "PERMISSION_DENIED", "only owner can archive group")
		return
	}
	if err := h.Groups.ArchiveGroup(r.Context(), groupID, time.Now().UTC()); err != nil {
		writeHubError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to archive group")
		return
	}
	writeHubJSON(w, http.StatusOK, map[string]any{"groupId": groupID, "state": "ARCHIVED"})
}

func (h HubHTTPHandler) createInvitation(w http.ResponseWriter, r *http.Request, groupID string) {
	if h.Groups == nil {
		writeHubError(w, http.StatusServiceUnavailable, "GROUPS_UNAVAILABLE", "Hub group store is unavailable")
		return
	}
	agent, ok := h.authenticateAgent(r)
	if !ok {
		writeHubError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Hub Agent credentials are required")
		return
	}
	member, err := h.Groups.FindMember(r.Context(), groupID, agent.AgentID)
	if err != nil || !member.CanManageMembers() {
		writeHubError(w, http.StatusForbidden, "PERMISSION_DENIED", "caller cannot invite members")
		return
	}
	var input HubInviteMemberInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024)).Decode(&input); err != nil || strings.TrimSpace(input.InviteeAgentID) == "" {
		writeHubError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "inviteeAgentId is required")
		return
	}
	if _, ok := h.Registry.LookupView(input.InviteeAgentID); !ok {
		writeHubError(w, http.StatusNotFound, "NOT_FOUND", "invitee Agent not found")
		return
	}
	policy := h.Registry.Policy()
	now := time.Now().UTC()
	inv, err := h.Groups.CreateInvitation(r.Context(), HubGroupInvitation{
		HubID:          policy.HubID,
		GroupID:        groupID,
		InviterAgentID: agent.AgentID,
		InviteeAgentID: strings.TrimSpace(input.InviteeAgentID),
		State:          HubInvitationPending,
		CreatedAt:      now,
		ExpiresAt:      now.Add(24 * time.Hour),
	})
	if err != nil {
		if errors.Is(err, ErrHubGroupLimit) {
			writeHubError(w, http.StatusConflict, "GROUP_LIMIT_REACHED", "group active member limit reached")
			return
		}
		writeHubError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create invitation")
		return
	}
	writeHubJSON(w, http.StatusOK, map[string]any{"invitation": inv})
}

func (h HubHTTPHandler) listInvitations(w http.ResponseWriter, r *http.Request) {
	if h.Groups == nil {
		writeHubError(w, http.StatusServiceUnavailable, "GROUPS_UNAVAILABLE", "Hub group store is unavailable")
		return
	}
	agent, ok := h.authenticateAgent(r)
	if !ok {
		writeHubError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Hub Agent credentials are required")
		return
	}
	invs, err := h.Groups.ListInvitations(r.Context(), agent.AgentID)
	if err != nil {
		writeHubError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list invitations")
		return
	}
	writeHubJSON(w, http.StatusOK, map[string]any{"invitations": invs})
}

func (h HubHTTPHandler) acceptInvitation(w http.ResponseWriter, r *http.Request, id uint64) {
	if h.Groups == nil {
		writeHubError(w, http.StatusServiceUnavailable, "GROUPS_UNAVAILABLE", "Hub group store is unavailable")
		return
	}
	agent, ok := h.authenticateAgent(r)
	if !ok {
		writeHubError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Hub Agent credentials are required")
		return
	}
	member, err := h.Groups.AcceptInvitation(r.Context(), id, agent.AgentID, time.Now().UTC())
	if err != nil {
		if errors.Is(err, ErrHubInvitationNotFound) {
			writeHubError(w, http.StatusNotFound, "NOT_FOUND", "invitation not found")
			return
		}
		if errors.Is(err, ErrHubGroupLimit) {
			writeHubError(w, http.StatusConflict, "GROUP_LIMIT_REACHED", "group active member limit reached")
			return
		}
		writeHubError(w, http.StatusBadRequest, "INVALID_STATE", "invitation cannot be accepted")
		return
	}
	writeHubJSON(w, http.StatusOK, map[string]any{"member": member})
}

func (h HubHTTPHandler) declineInvitation(w http.ResponseWriter, r *http.Request, id uint64) {
	if h.Groups == nil {
		writeHubError(w, http.StatusServiceUnavailable, "GROUPS_UNAVAILABLE", "Hub group store is unavailable")
		return
	}
	agent, ok := h.authenticateAgent(r)
	if !ok {
		writeHubError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Hub Agent credentials are required")
		return
	}
	if err := h.Groups.DeclineInvitation(r.Context(), id, agent.AgentID, time.Now().UTC()); err != nil {
		if errors.Is(err, ErrHubInvitationNotFound) {
			writeHubError(w, http.StatusNotFound, "NOT_FOUND", "invitation not found")
			return
		}
		writeHubError(w, http.StatusBadRequest, "INVALID_STATE", "invitation cannot be declined")
		return
	}
	writeHubJSON(w, http.StatusOK, map[string]any{"status": "DECLINED"})
}

func (h HubHTTPHandler) revokeInvitation(w http.ResponseWriter, r *http.Request, id uint64) {
	if h.Groups == nil {
		writeHubError(w, http.StatusServiceUnavailable, "GROUPS_UNAVAILABLE", "Hub group store is unavailable")
		return
	}
	agent, ok := h.authenticateAgent(r)
	if !ok {
		writeHubError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Hub Agent credentials are required")
		return
	}
	if err := h.Groups.RevokeInvitation(r.Context(), id, agent.AgentID, time.Now().UTC()); err != nil {
		if errors.Is(err, ErrHubInvitationNotFound) {
			writeHubError(w, http.StatusNotFound, "NOT_FOUND", "invitation not found")
			return
		}
		writeHubError(w, http.StatusBadRequest, "INVALID_STATE", "invitation cannot be revoked")
		return
	}
	writeHubJSON(w, http.StatusOK, map[string]any{"status": "REVOKED"})
}

func (h HubHTTPHandler) leaveGroup(w http.ResponseWriter, r *http.Request, groupID string) {
	if h.Groups == nil {
		writeHubError(w, http.StatusServiceUnavailable, "GROUPS_UNAVAILABLE", "Hub group store is unavailable")
		return
	}
	agent, ok := h.authenticateAgent(r)
	if !ok {
		writeHubError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Hub Agent credentials are required")
		return
	}
	if err := h.Groups.LeaveGroup(r.Context(), groupID, agent.AgentID, time.Now().UTC()); err != nil {
		if errors.Is(err, ErrHubGroupForbidden) {
			writeHubError(w, http.StatusForbidden, "PERMISSION_DENIED", "group owner cannot leave group without archiving")
			return
		}
		if errors.Is(err, ErrHubGroupNotFound) {
			writeHubError(w, http.StatusNotFound, "NOT_FOUND", "group or member not found")
			return
		}
		writeHubError(w, http.StatusBadRequest, "INVALID_STATE", err.Error())
		return
	}
	writeHubJSON(w, http.StatusOK, map[string]any{"groupId": groupID, "status": "LEFT"})
}

func (h HubHTTPHandler) removeMember(w http.ResponseWriter, r *http.Request, groupID, targetAgentID string) {
	if h.Groups == nil {
		writeHubError(w, http.StatusServiceUnavailable, "GROUPS_UNAVAILABLE", "Hub group store is unavailable")
		return
	}
	agent, ok := h.authenticateAgent(r)
	if !ok {
		writeHubError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Hub Agent credentials are required")
		return
	}
	if err := h.Groups.RemoveMember(r.Context(), groupID, agent.AgentID, targetAgentID, time.Now().UTC()); err != nil {
		if errors.Is(err, ErrHubGroupForbidden) {
			writeHubError(w, http.StatusForbidden, "PERMISSION_DENIED", "permission denied to remove member")
			return
		}
		if errors.Is(err, ErrHubGroupNotFound) {
			writeHubError(w, http.StatusNotFound, "NOT_FOUND", "group or member not found")
			return
		}
		writeHubError(w, http.StatusBadRequest, "INVALID_STATE", err.Error())
		return
	}
	writeHubJSON(w, http.StatusOK, map[string]any{"groupId": groupID, "targetAgentId": targetAgentID, "status": "REMOVED"})
}

func deliveryOutcome(duplicate bool) string {
	if duplicate {
		return "DUPLICATE"
	}
	return "DELIVERED"
}

func (h HubHTTPHandler) sendGroupMessage(w http.ResponseWriter, r *http.Request, groupID string) {
	if h.Groups == nil {
		writeHubError(w, http.StatusServiceUnavailable, "GROUPS_UNAVAILABLE", "Hub group store is unavailable")
		return
	}
	agent, ok := h.authenticateAgent(r)
	if !ok {
		writeHubError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Hub Agent credentials are required")
		return
	}
	if !h.allow(w, r, "sendGroupMessage") {
		return
	}
	policy := h.Registry.Policy()
	limit := policy.MaxPayloadBytes
	if limit <= 0 {
		limit = 1024 * 1024
	}
	var input HubGroupMessageInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit+1024)).Decode(&input); err != nil {
		writeHubError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "request body is invalid")
		return
	}
	if err := ValidateGroupMessage(input, limit); err != nil {
		writeHubError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
		return
	}
	msg, duplicate, err := h.Groups.SendGroupMessage(r.Context(), HubGroupMessage{
		HubID:          policy.HubID,
		GroupID:        groupID,
		SenderAgentID:  agent.AgentID,
		ContextID:      input.ContextID,
		IdempotencyKey: input.IdempotencyKey,
		Message:        input.Message,
		CreatedAt:      time.Now().UTC(),
	}, int(policy.MaxConcurrentTasks))
	if err != nil {
		if errors.Is(err, ErrHubGroupForbidden) {
			writeHubError(w, http.StatusForbidden, "PERMISSION_DENIED", "caller is not a group member")
			return
		}
		if errors.Is(err, ErrHubGroupNotFound) {
			writeHubError(w, http.StatusNotFound, "NOT_FOUND", "group not found")
			return
		}
		writeHubError(w, http.StatusBadRequest, "INVALID_STATE", err.Error())
		return
	}
	writeHubJSON(w, http.StatusOK, map[string]any{
		"outcome": deliveryOutcome(duplicate),
		"message": msg,
	})
}

func (h HubHTTPHandler) listGroupMessages(w http.ResponseWriter, r *http.Request, groupID string) {
	if h.Groups == nil {
		writeHubError(w, http.StatusServiceUnavailable, "GROUPS_UNAVAILABLE", "Hub group store is unavailable")
		return
	}
	agent, ok := h.authenticateAgent(r)
	if !ok {
		writeHubError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Hub Agent credentials are required")
		return
	}
	var afterID uint64
	if afterParam := r.URL.Query().Get("afterId"); afterParam != "" {
		if parsed, err := strconv.ParseUint(afterParam, 10, 64); err == nil {
			afterID = parsed
		}
	}
	limit := 50
	if limitParam := r.URL.Query().Get("limit"); limitParam != "" {
		if parsed, err := strconv.Atoi(limitParam); err == nil && parsed > 0 && parsed <= MaxGroupHistoryPageSize {
			limit = parsed
		}
	}
	msgs, err := h.Groups.ListGroupMessages(r.Context(), groupID, agent.AgentID, afterID, limit)
	if err != nil {
		if errors.Is(err, ErrHubGroupForbidden) {
			writeHubError(w, http.StatusForbidden, "PERMISSION_DENIED", "caller is not an active group member")
			return
		}
		writeHubError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list messages")
		return
	}
	writeHubJSON(w, http.StatusOK, map[string]any{"messages": msgs})
}
