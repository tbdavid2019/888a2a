package messageplane

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

// ModerationAction identifies a message mutation that is represented as an
// append-only collaboration event.
type ModerationAction string

const (
	ModerationEdit   ModerationAction = "EDIT"
	ModerationRecall ModerationAction = "RECALL"
	ModerationRedact ModerationAction = "REDACT"
)

// ModerationPolicy controls author edit/recall windows and moderator powers.
// Zero windows are fail-closed and do not grant author mutations.
type ModerationPolicy struct {
	EditWindow   time.Duration
	RecallWindow time.Duration
	ModeratorIDs []string
	LegalHold    bool
}

type ModerationMessage struct {
	OrganizationID string
	MessageID      string
	AuthorID       string
	CreatedAt      time.Time
}

// ModerationAudit is safe to persist in audit records: it contains no message
// body or raw event payload, even when legal hold retention is enabled.
type ModerationAudit struct {
	OrganizationID     string
	MessageID          string
	ActorID            string
	Action             ModerationAction
	Allowed            bool
	LegalHoldPreserved bool
	Reason             string
}

type ModerationDecision struct {
	Event CollaborationEvent
	Audit ModerationAudit
}

// EvaluateModeration authorizes a message mutation and returns the event plus
// an audit-safe decision. Tenant, actor, clock, and time-window checks happen
// before an event can be produced.
func EvaluateModeration(policy ModerationPolicy, requestedOrganizationID string, message ModerationMessage, action ModerationAction, actorID, content string, now time.Time) (ModerationDecision, error) {
	audit := ModerationAudit{OrganizationID: requestedOrganizationID, MessageID: message.MessageID, ActorID: actorID, Action: action}
	deny := func(reason string) (ModerationDecision, error) {
		audit.Reason = reason
		return ModerationDecision{Audit: audit}, fmt.Errorf("moderation denied: %s", reason)
	}
	if requestedOrganizationID == "" || message.OrganizationID == "" || requestedOrganizationID != message.OrganizationID {
		return deny("organization mismatch")
	}
	if message.MessageID == "" || message.AuthorID == "" || actorID == "" {
		return deny("message and actor identity are required")
	}
	if now.Before(message.CreatedAt) {
		return deny("message timestamp is in the future")
	}
	isModerator := slices.Contains(policy.ModeratorIDs, actorID)
	age := now.Sub(message.CreatedAt)
	eventType := EventType("")
	switch action {
	case ModerationEdit:
		if actorID != message.AuthorID || policy.EditWindow <= 0 || age > policy.EditWindow {
			return deny("only the author may edit within the edit window")
		}
		if strings.TrimSpace(content) == "" {
			return deny("edited content is required")
		}
		eventType = EventMessageEdited
	case ModerationRecall:
		if !(actorID == message.AuthorID && policy.RecallWindow > 0 && age <= policy.RecallWindow) && !isModerator {
			return deny("only the author within the recall window or a moderator may recall")
		}
		eventType = EventMessageRecalled
	case ModerationRedact:
		if !isModerator {
			return deny("only a moderator may redact")
		}
		eventType = EventMessageRedacted
	default:
		return deny("unsupported moderation action")
	}
	payload := []byte(`{}`)
	if action == ModerationEdit {
		var err error
		payload, err = json.Marshal(map[string]string{"content": content})
		if err != nil {
			return deny("edited content cannot be encoded")
		}
	}
	audit.Allowed = true
	audit.LegalHoldPreserved = policy.LegalHold && (action == ModerationRecall || action == ModerationRedact)
	audit.Reason = "allowed"
	return ModerationDecision{
		Event: CollaborationEvent{OrganizationID: message.OrganizationID, MessageID: message.MessageID, ActorID: actorID, Type: eventType, Payload: payload},
		Audit: audit,
	}, nil
}
