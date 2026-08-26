package a2a

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/tbdavid2019/888a2a/backend/manager/store"
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
		TraceID:          "trace-proj-001",
		SpanID:           "span-proj-002",
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
	if !strings.Contains(summary, "trace-proj-001") {
		t.Errorf("summary missing trace ID: %s", summary)
	}
	if !strings.Contains(summary, "auth_middleware.go") {
		t.Errorf("summary missing artifact name: %s", summary)
	}
	if !strings.Contains(summary, "Benchmark results") {
		t.Errorf("summary missing artifact description: %s", summary)
	}
}

func TestProjection_FormatDelegationAndStatusSummaries(t *testing.T) {
	work := &store.WorkMessage{
		TenantID:         "tenant-1",
		WorkID:           "work-del-1",
		A2ATaskID:        "a2a-delegated-100",
		ContextID:        "ctx-del-root",
		RequesterAgentID: "agent-coord",
		ExecutorAgentID:  "agent-specialist",
		State:            "SUBMITTED",
		ParentWorkID:     sql.NullString{String: "work-parent-root", Valid: true},
		DelegationDepth:  1,
		TraceID:          "trace-del-001",
		MaxDepth:         3,
		MaxChildren:      5,
		MaxRuntimeMs:     60000,
	}

	delSummary := FormatDelegationSummary(work)
	if !strings.Contains(delSummary, "A2A Task Delegated") || !strings.Contains(delSummary, "a2a-delegated-100") {
		t.Errorf("delegation summary missing task identity: %s", delSummary)
	}
	if !strings.Contains(delSummary, "agent-specialist") || !strings.Contains(delSummary, "agent-coord") {
		t.Errorf("delegation summary missing peer identities: %s", delSummary)
	}
	if !strings.Contains(delSummary, "work-parent-root") || !strings.Contains(delSummary, "Depth: 1") {
		t.Errorf("delegation summary missing parent context: %s", delSummary)
	}
	if !strings.Contains(delSummary, "trace-del-001") {
		t.Errorf("delegation summary missing trace ID: %s", delSummary)
	}

	// Status update event
	event := &store.WorkEventMessage{
		TenantID:       "tenant-1",
		WorkID:         "work-del-1",
		Sequence:       2,
		EventType:      "WORKING",
		TerminalReason: "Starting security audit scan",
	}

	statusSummary := FormatStatusUpdateSummary(work, event)
	if !strings.Contains(statusSummary, "WORKING") || !strings.Contains(statusSummary, "Seq #2") {
		t.Errorf("status summary missing state or sequence: %s", statusSummary)
	}
	if !strings.Contains(statusSummary, "Starting security audit scan") {
		t.Errorf("status summary missing note: %s", statusSummary)
	}
}

func TestProjection_FormatResultSummary_NoHiddenReasoning(t *testing.T) {
	work := &store.WorkMessage{
		TenantID:         "tenant-1",
		WorkID:           "work-res-1",
		A2ATaskID:        "a2a-res-200",
		RequesterAgentID: "agent-coord",
		ExecutorAgentID:  "agent-reviewer",
		State:            "COMPLETED",
		TerminalReason:   "All 12 checks passed. <thought>internal model reasoning should be stripped</thought> LGTM!",
		TraceID:          "trace-audit-999",
	}

	artifacts := []*store.WorkArtifactMessage{
		{
			ArtifactID:  "art-report",
			Name:        "report.pdf",
			MediaType:   "application/pdf",
			ExternalURI: "https://artifacts.888a2a.local/reports/report.pdf",
			SizeBytes:   204800,
		},
	}

	resSummary := FormatResultSummary(work, artifacts)

	if !strings.Contains(resSummary, "COMPLETED") || !strings.Contains(resSummary, "a2a-res-200") {
		t.Errorf("result summary missing task ID or state: %s", resSummary)
	}
	if !strings.Contains(resSummary, "trace-audit-999") {
		t.Errorf("result summary missing trace ID: %s", resSummary)
	}
	if !strings.Contains(resSummary, "report.pdf") || !strings.Contains(resSummary, "204800 bytes") {
		t.Errorf("result summary missing artifact info: %s", resSummary)
	}

	// Verify hidden reasoning is stripped
	if strings.Contains(resSummary, "internal model reasoning") || strings.Contains(resSummary, "<thought>") {
		t.Errorf("result summary must NOT expose hidden reasoning: %s", resSummary)
	}
	if !strings.Contains(resSummary, "LGTM!") {
		t.Errorf("result summary missing sanitized outcome: %s", resSummary)
	}
}
