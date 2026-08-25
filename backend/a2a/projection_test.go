package a2a

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/Ranxy/laelia/backend/manager/store"
)

func TestProjection_FormatThreadSummary(t *testing.T) {
	convID := uuid.New()
	taskID := uuid.New()

	work := &store.WorkMessage{
		TenantID:         "tenant-1",
		WorkID:           "work-abc-1",
		A2ATaskID:        "a2a-task-999",
		RequesterAgentID: "agent-planner",
		ExecutorAgentID:  "agent-coder",
		State:            "COMPLETED",
		TerminalReason:   "Successfully generated components and passed unit tests",
	}

	AttachConversationLink(work, convID, &taskID)
	if work.SourceConversationID == nil || *work.SourceConversationID != convID {
		t.Fatalf("expected SourceConversationID to be %v, got %v", convID, work.SourceConversationID)
	}
	if work.SourceTaskID == nil || *work.SourceTaskID != taskID {
		t.Fatalf("expected SourceTaskID to be %v, got %v", taskID, work.SourceTaskID)
	}

	artifacts := []*store.WorkArtifactMessage{
		{
			ArtifactID:  "artifact-1",
			Name:        "auth_middleware.go",
			MediaType:   "text/x-go",
			ExternalURI: "https://storage.888a2a.local/artifacts/auth_middleware.go",
		},
		{
			ArtifactID:  "artifact-2",
			Description: "Benchmark results: 0 allocations per op",
		},
	}

	summary := FormatThreadSummary(work, artifacts)

	if !strings.Contains(summary, "a2a-task-999") {
		t.Errorf("summary missing A2A task ID: %s", summary)
	}
	if !strings.Contains(summary, "agent-coder") || !strings.Contains(summary, "agent-planner") {
		t.Errorf("summary missing agent identities: %s", summary)
	}
	if !strings.Contains(summary, "COMPLETED") {
		t.Errorf("summary missing state: %s", summary)
	}
	if !strings.Contains(summary, "auth_middleware.go") {
		t.Errorf("summary missing artifact name: %s", summary)
	}
	if !strings.Contains(summary, "Benchmark results") {
		t.Errorf("summary missing artifact description: %s", summary)
	}
}
