package a2a

import (
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/tbdavid2019/888a2a/backend/manager/store"
)

// AttachConversationLink links an A2A work record to an existing 888a2a Conversation and Task.
func AttachConversationLink(work *store.WorkMessage, conversationID uuid.UUID, taskID *uuid.UUID) {
	if work == nil {
		return
	}
	work.SourceConversationID = &conversationID
	work.SourceTaskID = taskID
}

// FormatThreadSummary produces a concise, audit-safe markdown summary for chat/thread projection.
func FormatThreadSummary(work *store.WorkMessage, artifacts []*store.WorkArtifactMessage) string {
	if work == nil {
		return ""
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "**A2A Task `%s`** [%s]\n", sanitizeSafeString(work.A2ATaskID), sanitizeSafeString(work.State))
	fmt.Fprintf(&sb, "- **Executor**: `%s` (Requester: `%s`)\n", sanitizeSafeString(work.ExecutorAgentID), sanitizeSafeString(work.RequesterAgentID))

	if work.TraceID != "" {
		fmt.Fprintf(&sb, "- **Trace**: `%s`", sanitizeSafeString(work.TraceID))
		if work.SpanID != "" {
			fmt.Fprintf(&sb, " (Span: `%s`)", sanitizeSafeString(work.SpanID))
		}
		sb.WriteString("\n")
	}

	if work.TerminalReason != "" {
		fmt.Fprintf(&sb, "- **Outcome**: %s\n", sanitizeSafeString(work.TerminalReason))
	}

	if len(artifacts) > 0 {
		sb.WriteString("- **Artifacts**:\n")
		for _, a := range artifacts {
			name := a.Name
			if name == "" {
				name = a.ArtifactID
			}
			name = sanitizeSafeString(name)
			if a.ExternalURI != "" {
				fmt.Fprintf(&sb, "  - [%s](%s) (%s)\n", name, sanitizeSafeString(a.ExternalURI), sanitizeSafeString(a.MediaType))
			} else {
				fmt.Fprintf(&sb, "  - `%s`: %s\n", name, sanitizeSafeString(a.Description))
			}
		}
	}

	return sb.String()
}

// FormatDelegationSummary produces an audit-safe markdown summary when an A2A task is delegated.
func FormatDelegationSummary(work *store.WorkMessage) string {
	if work == nil {
		return ""
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "📋 **A2A Task Delegated**: `%s`\n", sanitizeSafeString(work.A2ATaskID))
	fmt.Fprintf(&sb, "- **Target Executor**: `%s`\n", sanitizeSafeString(work.ExecutorAgentID))
	fmt.Fprintf(&sb, "- **Requester**: `%s`\n", sanitizeSafeString(work.RequesterAgentID))
	fmt.Fprintf(&sb, "- **Context ID**: `%s`\n", sanitizeSafeString(work.ContextID))

	if work.ParentWorkID.Valid && work.ParentWorkID.String != "" {
		fmt.Fprintf(&sb, "- **Parent Task**: `%s` (Depth: %d)\n", sanitizeSafeString(work.ParentWorkID.String), work.DelegationDepth)
	}

	if work.TraceID != "" {
		fmt.Fprintf(&sb, "- **Trace ID**: `%s`\n", sanitizeSafeString(work.TraceID))
	}

	if work.MaxDepth > 0 || work.MaxRuntimeMs > 0 || work.MaxTokens > 0 || work.MaxChildren > 0 {
		fmt.Fprintf(&sb, "- **Budget Limits**: MaxDepth=%d, MaxChildren=%d, MaxRuntimeMs=%d, MaxTokens=%d\n",
			work.MaxDepth, work.MaxChildren, work.MaxRuntimeMs, work.MaxTokens)
	}

	return sb.String()
}

// FormatStatusUpdateSummary produces an audit-safe markdown summary for a state change event.
func FormatStatusUpdateSummary(work *store.WorkMessage, event *store.WorkEventMessage) string {
	if work == nil {
		return ""
	}

	state := work.State
	seq := uint64(0)
	if event != nil {
		if event.EventType != "" {
			state = event.EventType
		}
		seq = event.Sequence
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "🔄 **A2A Task Status**: `%s` → [%s]", sanitizeSafeString(work.A2ATaskID), sanitizeSafeString(state))
	if seq > 0 {
		fmt.Fprintf(&sb, " (Seq #%d)", seq)
	}
	sb.WriteString("\n")

	fmt.Fprintf(&sb, "- **Executor**: `%s`\n", sanitizeSafeString(work.ExecutorAgentID))
	if work.TraceID != "" {
		fmt.Fprintf(&sb, "- **Trace ID**: `%s`\n", sanitizeSafeString(work.TraceID))
	}
	if event != nil && event.TerminalReason != "" {
		fmt.Fprintf(&sb, "- **Note**: %s\n", sanitizeSafeString(event.TerminalReason))
	}

	return sb.String()
}

// FormatResultSummary produces an audit-safe markdown summary when an A2A task reaches a terminal state.
func FormatResultSummary(work *store.WorkMessage, artifacts []*store.WorkArtifactMessage) string {
	if work == nil {
		return ""
	}

	var icon string
	switch work.State {
	case "COMPLETED":
		icon = "✅"
	case "FAILED":
		icon = "❌"
	case "CANCELED":
		icon = "🚫"
	default:
		icon = "🏁"
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s **A2A Task `%s` %s**\n", icon, sanitizeSafeString(work.A2ATaskID), sanitizeSafeString(work.State))
	fmt.Fprintf(&sb, "- **Executor**: `%s` (Requester: `%s`)\n", sanitizeSafeString(work.ExecutorAgentID), sanitizeSafeString(work.RequesterAgentID))

	if work.TraceID != "" {
		fmt.Fprintf(&sb, "- **Trace ID**: `%s`\n", sanitizeSafeString(work.TraceID))
	}

	if work.TerminalReason != "" {
		fmt.Fprintf(&sb, "- **Outcome**: %s\n", sanitizeSafeString(work.TerminalReason))
	}

	if len(artifacts) > 0 {
		fmt.Fprintf(&sb, "- **Artifacts** (%d):\n", len(artifacts))
		for _, a := range artifacts {
			name := a.Name
			if name == "" {
				name = a.ArtifactID
			}
			name = sanitizeSafeString(name)

			if a.ExternalURI != "" {
				meta := sanitizeSafeString(a.MediaType)
				if a.SizeBytes > 0 {
					meta += fmt.Sprintf(", %d bytes", a.SizeBytes)
				}
				fmt.Fprintf(&sb, "  - [%s](%s) (%s)\n", name, sanitizeSafeString(a.ExternalURI), meta)
			} else {
				fmt.Fprintf(&sb, "  - `%s`: %s\n", name, sanitizeSafeString(a.Description))
			}
		}
	}

	return sb.String()
}

// sanitizeSafeString ensures output excludes potential secrets or model scratchpads.
func sanitizeSafeString(s string) string {
	if s == "" {
		return ""
	}
	// Strip XML-like thought tags if present
	clean := s
	if idx := strings.Index(clean, "<thought>"); idx != -1 {
		if endIdx := strings.Index(clean, "</thought>"); endIdx != -1 {
			clean = clean[:idx] + clean[endIdx+len("</thought>"):]
		} else {
			clean = clean[:idx]
		}
	}
	if idx := strings.Index(clean, "<thinking>"); idx != -1 {
		if endIdx := strings.Index(clean, "</thinking>"); endIdx != -1 {
			clean = clean[:idx] + clean[endIdx+len("</thinking>"):]
		} else {
			clean = clean[:idx]
		}
	}
	return strings.TrimSpace(clean)
}
