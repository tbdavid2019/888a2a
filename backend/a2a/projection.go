package a2a

import (
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/Ranxy/laelia/backend/manager/store"
)

// AttachConversationLink links an A2A work record to an existing 888a2a Conversation and Task.
func AttachConversationLink(work *store.WorkMessage, conversationID uuid.UUID, taskID *uuid.UUID) {
	if work == nil {
		return
	}
	work.SourceConversationID = &conversationID
	work.SourceTaskID = taskID
}

// FormatThreadSummary produces a concise, human-readable markdown summary for chat/thread projection.
func FormatThreadSummary(work *store.WorkMessage, artifacts []*store.WorkArtifactMessage) string {
	if work == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**A2A Task `%s`** [%s]\n", work.A2ATaskID, work.State))
	sb.WriteString(fmt.Sprintf("- **Executor**: `%s` (Requester: `%s`)\n", work.ExecutorAgentID, work.RequesterAgentID))

	if work.TerminalReason != "" {
		sb.WriteString(fmt.Sprintf("- **Outcome**: %s\n", work.TerminalReason))
	}

	if len(artifacts) > 0 {
		sb.WriteString("- **Artifacts**:\n")
		for _, a := range artifacts {
			if a.ExternalURI != "" {
				name := a.Name
				if name == "" {
					name = a.ArtifactID
				}
				sb.WriteString(fmt.Sprintf("  - [%s](%s) (%s)\n", name, a.ExternalURI, a.MediaType))
			} else {
				sb.WriteString(fmt.Sprintf("  - `%s`: %s\n", a.ArtifactID, a.Description))
			}
		}
	}

	return sb.String()
}
