package messageplane

import (
	"encoding/json"
	"fmt"
)

// EventType is an append-only collaboration mutation.
type EventType string

const (
	EventMessageCreated   EventType = "MESSAGE_CREATED"
	EventMessageEdited    EventType = "MESSAGE_EDITED"
	EventMessageRecalled  EventType = "MESSAGE_RECALLED"
	EventMessageRedacted  EventType = "MESSAGE_REDACTED"
	EventReactionAdded    EventType = "REACTION_ADDED"
	EventReactionRemoved  EventType = "REACTION_REMOVED"
	EventThreadLinked     EventType = "THREAD_LINKED"
	EventCommandStarted   EventType = "COMMAND_STARTED"
	EventCommandSteered   EventType = "COMMAND_STEERED"
	EventCommandCancelled EventType = "COMMAND_CANCELLED"
	EventCommandCompleted EventType = "COMMAND_COMPLETED"
)

// CollaborationEvent is the durable mutation envelope. Payload is immutable;
// visible message state is derived by applying events in sequence order.
type CollaborationEvent struct {
	OrganizationID string
	ConversationID string
	MessageID      string
	ActorID        string
	Type           EventType
	Payload        []byte
}

// MessageView is the user-visible projection of one message.
type MessageView struct {
	MessageID    string
	Content      string
	Recalled     bool
	Redacted     bool
	ThreadRootID string
	Reactions    map[string]map[string]bool
}

// ProjectEvents deterministically applies append-only collaboration events.
// Unknown event types fail closed so a new mutation cannot silently produce an
// incorrect visible projection.
func ProjectEvents(events []CollaborationEvent) (map[string]*MessageView, error) {
	return projectEvents(events, false)
}

// ProjectEventsForLegalHold returns the same visible projection for an
// authorized compliance reader. Recalled or redacted content is retained
// only in this explicitly authorized result; ordinary readers receive empty
// content for those events.
func ProjectEventsForLegalHold(events []CollaborationEvent) (map[string]*MessageView, error) {
	return projectEvents(events, true)
}

func projectEvents(events []CollaborationEvent, legalHoldAccess bool) (map[string]*MessageView, error) {
	views := make(map[string]*MessageView)
	retainedContent := make(map[string]string)
	for _, event := range events {
		if event.MessageID == "" || event.Type == "" {
			return nil, fmt.Errorf("collaboration event identity is required")
		}
		view := views[event.MessageID]
		if view == nil {
			view = &MessageView{MessageID: event.MessageID, Reactions: make(map[string]map[string]bool)}
			views[event.MessageID] = view
		}
		var payload struct {
			Content      string `json:"content"`
			Emoji        string `json:"emoji"`
			ThreadRootID string `json:"thread_root_id"`
		}
		if len(event.Payload) > 0 && !json.Valid(event.Payload) {
			return nil, fmt.Errorf("event %s payload is not valid JSON", event.Type)
		}
		if len(event.Payload) > 0 {
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				return nil, fmt.Errorf("decode event %s: %w", event.Type, err)
			}
		}
		switch event.Type {
		case EventMessageCreated:
			view.Content = payload.Content
			retainedContent[event.MessageID] = payload.Content
		case EventMessageEdited:
			if !view.Recalled && !view.Redacted {
				view.Content = payload.Content
				retainedContent[event.MessageID] = payload.Content
			}
		case EventMessageRecalled:
			view.Recalled = true
			view.Content = retainedContent[event.MessageID]
			if !legalHoldAccess {
				view.Content = ""
			}
		case EventMessageRedacted:
			view.Redacted = true
			view.Content = retainedContent[event.MessageID]
			if !legalHoldAccess {
				view.Content = ""
			}
		case EventReactionAdded, EventReactionRemoved:
			if payload.Emoji == "" || event.ActorID == "" {
				return nil, fmt.Errorf("reaction event requires emoji and actor")
			}
			actors := view.Reactions[payload.Emoji]
			if actors == nil {
				actors = make(map[string]bool)
				view.Reactions[payload.Emoji] = actors
			}
			if event.Type == EventReactionAdded {
				actors[event.ActorID] = true
			} else {
				delete(actors, event.ActorID)
			}
		case EventThreadLinked:
			if payload.ThreadRootID == "" {
				return nil, fmt.Errorf("thread event requires thread_root_id")
			}
			view.ThreadRootID = payload.ThreadRootID
		case EventCommandStarted, EventCommandSteered, EventCommandCancelled, EventCommandCompleted:
			// Command lifecycle events are retained in the append-only log; the
			// message projection does not expose internal command state here.
		default:
			return nil, fmt.Errorf("unsupported collaboration event type %q", event.Type)
		}
	}
	return views, nil
}
