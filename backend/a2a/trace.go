package a2a

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"

	"github.com/Ranxy/laelia/backend/manager/store"
)

// Standard Agent Network audit and trace event types.
const (
	TraceEventDiscovery       = "DISCOVERY"
	TraceEventDelegation      = "DELEGATION"
	TraceEventProviderSession = "PROVIDER_SESSION"
	TraceEventPermission      = "PERMISSION"
	TraceEventBudget          = "BUDGET"
	TraceEventPolicyLimit     = "POLICY_LIMIT"
	TraceEventRetry           = "RETRY"
	TraceEventCancellation    = "CANCELLATION"
	TraceEventTerminalOutcome = "TERMINAL_OUTCOME"
)

// TraceEvent represents an audit-safe lifecycle or policy record.
type TraceEvent struct {
	TenantID       string            `json:"tenant_id"`
	EventID        string            `json:"event_id"`
	WorkID         string            `json:"work_id"`
	TraceID        string            `json:"trace_id"`
	RootTraceID    string            `json:"root_trace_id"`
	SpanID         string            `json:"span_id"`
	ParentSpanID   string            `json:"parent_span_id"`
	EventType      string            `json:"event_type"`
	ProviderID     string            `json:"provider_id"`
	SessionID      string            `json:"session_id"`
	PolicyDecision string            `json:"policy_decision"`
	RetryCount     int32             `json:"retry_count"`
	TerminalReason string            `json:"terminal_reason"`
	Metadata       map[string]string `json:"metadata"`
	CreatedAt      time.Time         `json:"created_at"`
	Sequence       uint64            `json:"sequence"`
}

// TraceRecorder manages recording durable, audit-safe events.
type TraceRecorder struct {
	store        *store.Store
	eventManager *EventManager
}

// NewTraceRecorder creates a new trace recorder.
func NewTraceRecorder(store *store.Store, eventManager *EventManager) *TraceRecorder {
	return &TraceRecorder{
		store:        store,
		eventManager: eventManager,
	}
}

var (
	sensitiveKeyPatterns = []string{
		"token", "secret", "password", "key", "auth", "authorization",
		"bearer", "credential", "cookie", "cert", "private",
	}

	hiddenReasoningKeyPatterns = []string{
		"thought", "thinking", "hidden_reasoning", "prompt", "raw_prompt",
		"cot", "chain_of_thought", "internal_thought",
	}

	bearerRegex = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9\-\._~\+\/]+=*`)
	secretRegex = regexp.MustCompile(`(?i)(api[_\-]?key|secret|password|token)\s*[:=]\s*[^\s,;]+`)
)

// SanitizeMetadata scrubs credentials and hidden reasoning from metadata key-value pairs.
func SanitizeMetadata(raw map[string]string) map[string]string {
	if raw == nil {
		return make(map[string]string)
	}
	clean := make(map[string]string, len(raw))

	for k, v := range raw {
		kLower := strings.ToLower(k)

		// Exclude hidden reasoning keys entirely
		if matchesAnyPattern(kLower, hiddenReasoningKeyPatterns) {
			continue
		}

		// Exclude credential and secret keys entirely
		if matchesAnyPattern(kLower, sensitiveKeyPatterns) {
			continue
		}

		// Sanitize value content against credential leakage
		sanitizedVal := sanitizeValue(v)
		clean[k] = sanitizedVal
	}

	return clean
}

func matchesAnyPattern(key string, patterns []string) bool {
	for _, p := range patterns {
		if strings.Contains(key, p) {
			return true
		}
	}
	return false
}

func sanitizeValue(val string) string {
	res := bearerRegex.ReplaceAllString(val, "Bearer [REDACTED]")
	res = secretRegex.ReplaceAllString(res, "$1=[REDACTED]")
	return res
}

// Record sanitizes and durably appends an audit event to the store.
func (r *TraceRecorder) Record(ctx context.Context, event *TraceEvent) (*TraceEvent, error) {
	if event == nil {
		return nil, errors.New("event cannot be nil")
	}

	if event.TenantID == "" {
		event.TenantID = "default"
	}
	if event.EventID == "" {
		event.EventID = uuid.New().String()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}

	// Always sanitize metadata before writing
	event.Metadata = SanitizeMetadata(event.Metadata)
	event.TerminalReason = sanitizeValue(event.TerminalReason)

	if r.store != nil && event.WorkID != "" {
		latestSeq, err := r.store.GetLatestWorkEventSequence(ctx, event.TenantID, event.WorkID)
		if err != nil {
			return nil, errors.Wrap(err, "get latest work event sequence")
		}
		event.Sequence = latestSeq + 1

		msg := &store.WorkEventMessage{
			TenantID:       event.TenantID,
			EventID:        event.EventID,
			WorkID:         event.WorkID,
			Sequence:       event.Sequence,
			TraceID:        event.TraceID,
			RootTraceID:    event.RootTraceID,
			SpanID:         event.SpanID,
			ParentSpanID:   event.ParentSpanID,
			EventType:      event.EventType,
			ProviderID:     event.ProviderID,
			SessionID:      event.SessionID,
			PolicyDecision: event.PolicyDecision,
			RetryCount:     event.RetryCount,
			TerminalReason: event.TerminalReason,
			Metadata:       event.Metadata,
			CreatedAt:      event.CreatedAt,
		}

		if err := r.store.AppendWorkEvent(ctx, msg); err != nil {
			return nil, errors.Wrap(err, "append work trace event")
		}
	}

	return event, nil
}

// ListEvents retrieves durable trace events for a work ID.
func (r *TraceRecorder) ListEvents(ctx context.Context, tenantID, workID string, afterSequence uint64, limit int) ([]*TraceEvent, error) {
	if r.store == nil {
		return nil, nil
	}
	if tenantID == "" {
		tenantID = "default"
	}

	msgs, err := r.store.ListWorkEvents(ctx, tenantID, workID, afterSequence, limit)
	if err != nil {
		return nil, errors.Wrap(err, "list work trace events")
	}

	events := make([]*TraceEvent, 0, len(msgs))
	for _, m := range msgs {
		events = append(events, &TraceEvent{
			TenantID:       m.TenantID,
			EventID:        m.EventID,
			WorkID:         m.WorkID,
			Sequence:       m.Sequence,
			TraceID:        m.TraceID,
			RootTraceID:    m.RootTraceID,
			SpanID:         m.SpanID,
			ParentSpanID:   m.ParentSpanID,
			EventType:      m.EventType,
			ProviderID:     m.ProviderID,
			SessionID:      m.SessionID,
			PolicyDecision: m.PolicyDecision,
			RetryCount:     m.RetryCount,
			TerminalReason: m.TerminalReason,
			Metadata:       m.Metadata,
			CreatedAt:      m.CreatedAt,
		})
	}
	return events, nil
}
