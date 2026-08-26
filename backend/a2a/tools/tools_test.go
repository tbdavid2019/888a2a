package tools

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"testing"
	"time"

	a2apkg "github.com/tbdavid2019/888a2a/backend/a2a"
	models "github.com/tbdavid2019/888a2a/backend/generated-go/store"
	"github.com/tbdavid2019/888a2a/backend/manager/store"
)

type testCaller struct {
	id            string
	tenant        string
	authenticated bool
}

func (c *testCaller) GetPrincipalID() string { return c.id }
func (c *testCaller) GetTenantID() string    { return c.tenant }
func (c *testCaller) IsAuthenticated() bool  { return c.authenticated }

type memDirectoryStore struct {
	agents []*store.AgentMessage
}

func (m *memDirectoryStore) ListAgents(_ context.Context, _ *store.FindAgentMessage) ([]*store.AgentMessage, error) {
	return m.agents, nil
}

func (m *memDirectoryStore) GetAgentByResourceID(_ context.Context, resourceID string) (*store.AgentMessage, error) {
	for _, a := range m.agents {
		if a.ResourceID == resourceID {
			return a, nil
		}
	}
	return nil, nil
}

type memoryWorkStore struct {
	mu        sync.RWMutex
	contexts  map[string]*store.WorkContextMessage
	works     map[string]*store.WorkMessage
	artifacts map[string][]*store.WorkArtifactMessage
	events    map[string][]*store.WorkEventMessage
}

func newMemoryWorkStore() *memoryWorkStore {
	return &memoryWorkStore{
		contexts:  make(map[string]*store.WorkContextMessage),
		works:     make(map[string]*store.WorkMessage),
		artifacts: make(map[string][]*store.WorkArtifactMessage),
		events:    make(map[string][]*store.WorkEventMessage),
	}
}

func (m *memoryWorkStore) EnsureWorkContext(_ context.Context, tenantID, contextID, rootWorkID string) (*store.WorkContextMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := tenantID + ":" + contextID
	if c, ok := m.contexts[key]; ok {
		c.UpdatedAt = time.Now()
		return c, nil
	}
	c := &store.WorkContextMessage{
		TenantID:   tenantID,
		ContextID:  contextID,
		RootWorkID: sql.NullString{String: rootWorkID, Valid: rootWorkID != ""},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Version:    1,
	}
	m.contexts[key] = c
	return c, nil
}

func (m *memoryWorkStore) CreateWork(_ context.Context, work *store.WorkMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := work.TenantID + ":" + work.WorkID
	if _, ok := m.works[key]; ok {
		return store.ErrWorkAlreadyExists
	}
	for _, w := range m.works {
		if w.TenantID == work.TenantID && w.RequesterAgentID == work.RequesterAgentID && w.IdempotencyKey == work.IdempotencyKey {
			return store.ErrWorkAlreadyExists
		}
	}
	workCopy := *work
	m.works[key] = &workCopy
	return nil
}

func (m *memoryWorkStore) GetWork(_ context.Context, tenantID, workID string) (*store.WorkMessage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := tenantID + ":" + workID
	w, ok := m.works[key]
	if !ok {
		return nil, store.ErrWorkNotFound
	}
	cloned := *w
	return &cloned, nil
}

func (m *memoryWorkStore) GetWorkByA2ATaskID(_ context.Context, tenantID, a2aTaskID string) (*store.WorkMessage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, w := range m.works {
		if w.TenantID == tenantID && w.A2ATaskID == a2aTaskID {
			cloned := *w
			return &cloned, nil
		}
	}
	return nil, store.ErrWorkNotFound
}

func (m *memoryWorkStore) GetWorkByIdempotencyKey(_ context.Context, tenantID, requesterAgentID, idempotencyKey string) (*store.WorkMessage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, w := range m.works {
		if w.TenantID == tenantID && w.RequesterAgentID == requesterAgentID && w.IdempotencyKey == idempotencyKey {
			cloned := *w
			return &cloned, nil
		}
	}
	return nil, store.ErrWorkNotFound
}

func (m *memoryWorkStore) UpdateWorkState(_ context.Context, tenantID, workID string, expectedVersion uint64, newState string, terminalReason string) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := tenantID + ":" + workID
	w, ok := m.works[key]
	if !ok {
		return 0, store.ErrWorkNotFound
	}
	if w.Version != expectedVersion {
		return 0, store.ErrWorkVersionMismatch
	}
	w.State = newState
	w.TerminalReason = terminalReason
	w.Version++
	w.UpdatedAt = time.Now()
	if newState == "COMPLETED" || newState == "FAILED" || newState == "CANCELED" || newState == "REJECTED" {
		now := time.Now()
		w.CompletedAt = &now
	}
	return w.Version, nil
}

func (m *memoryWorkStore) ListWork(_ context.Context, filter store.ListWorkFilter) ([]*store.WorkMessage, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var matching []*store.WorkMessage
	for _, w := range m.works {
		if w.TenantID != filter.TenantID {
			continue
		}
		if filter.ContextID != "" && w.ContextID != filter.ContextID {
			continue
		}
		if filter.RequesterAgentID != "" && w.RequesterAgentID != filter.RequesterAgentID {
			continue
		}
		if filter.ExecutorAgentID != "" && w.ExecutorAgentID != filter.ExecutorAgentID {
			continue
		}
		if filter.State != "" && w.State != filter.State {
			continue
		}
		cloned := *w
		matching = append(matching, &cloned)
	}

	total := len(matching)
	start := filter.Offset
	if start > total {
		start = total
	}
	end := start + filter.Limit
	if end > total {
		end = total
	}

	return matching[start:end], total, nil
}

func (m *memoryWorkStore) CreateWorkArtifact(_ context.Context, artifact *store.WorkArtifactMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := artifact.TenantID + ":" + artifact.WorkID
	m.artifacts[key] = append(m.artifacts[key], artifact)
	return nil
}

func (m *memoryWorkStore) ListWorkArtifacts(_ context.Context, tenantID, workID string) ([]*store.WorkArtifactMessage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := tenantID + ":" + workID
	return m.artifacts[key], nil
}

func (m *memoryWorkStore) AppendWorkEvent(_ context.Context, event *store.WorkEventMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := event.TenantID + ":" + event.WorkID
	m.events[key] = append(m.events[key], event)
	return nil
}

func (m *memoryWorkStore) ListWorkEvents(_ context.Context, tenantID, workID string, afterSequence uint64, limit int) ([]*store.WorkEventMessage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := tenantID + ":" + workID
	var result []*store.WorkEventMessage
	for _, e := range m.events[key] {
		if e.Sequence > afterSequence {
			result = append(result, e)
			if limit > 0 && len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (m *memoryWorkStore) GetLatestWorkEventSequence(_ context.Context, tenantID, workID string) (uint64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := tenantID + ":" + workID
	events := m.events[key]
	if len(events) == 0 {
		return 0, nil
	}
	return events[len(events)-1].Sequence, nil
}

func TestPeerTools_ListAndGetWithCapabilities(t *testing.T) {
	ctx := context.Background()

	agents := []*store.AgentMessage{
		{
			ID:          1,
			ResourceID:  "agent-sec",
			Name:        "Security Reviewer",
			Description: "Audits code for vulnerabilities",
			Enabled:     true,
			Status: &models.AgentStatus{
				State: models.AgentStatus_ONLINE,
			},
		},
		{
			ID:          2,
			ResourceID:  "agent-offline",
			Name:        "Offline Worker",
			Description: "Offline worker",
			Enabled:     true,
			Status: &models.AgentStatus{
				State: models.AgentStatus_OFFLINE,
			},
		},
	}

	skills := map[string][]a2apkg.SkillInput{
		"agent-sec": {
			{
				ID:          "skill-sast",
				Name:        "Static Analysis",
				Description: "Performs AST and tainted flow analysis",
				Tags:        []string{"security", "ast"},
				InputModes:  []string{"text/plain", "application/json"},
				OutputModes: []string{"application/json"},
			},
		},
	}

	ds := a2apkg.NewDirectoryService(&memDirectoryStore{agents: agents}, "https://api.888a2a.local", skills)
	caller := &testCaller{id: "agent-coordinator", tenant: "default", authenticated: true}

	// 1. List peers with skill tag filter
	listOut, err := PeerList(ctx, ds, caller, PeerListInput{
		Tenant:    "default",
		SkillTag:  "security",
		ReadyOnly: true,
	})
	if err != nil {
		t.Fatalf("PeerList failed: %v", err)
	}
	if listOut.TotalCount != 1 {
		t.Fatalf("expected 1 peer matching filter, got %d", listOut.TotalCount)
	}

	p := listOut.Peers[0]
	if p.AgentResourceID != "agent-sec" {
		t.Errorf("expected agent-sec, got %s", p.AgentResourceID)
	}
	if p.Readiness != a2apkg.ReadinessReady {
		t.Errorf("expected readiness READY, got %s", p.Readiness)
	}
	if len(p.Skills) != 1 || p.Skills[0].ID != "skill-sast" {
		t.Errorf("expected skill-sast, got %v", p.Skills)
	}

	formattedList := FormatPeerList(listOut)
	if !strings.Contains(formattedList, "Security Reviewer") || !strings.Contains(formattedList, "Static Analysis") {
		t.Errorf("formatted list missing expected details: %s", formattedList)
	}

	// 2. Get peer directly
	getOut, err := PeerGet(ctx, ds, caller, PeerGetInput{
		Tenant:          "default",
		AgentResourceID: "agent-sec",
	})
	if err != nil {
		t.Fatalf("PeerGet failed: %v", err)
	}
	if getOut.Peer.Name != "Security Reviewer" {
		t.Errorf("expected Security Reviewer, got %s", getOut.Peer.Name)
	}
	if getOut.Peer.Readiness != a2apkg.ReadinessReady {
		t.Errorf("expected readiness READY, got %s", getOut.Peer.Readiness)
	}

	formattedGet := FormatPeerGet(getOut)
	if !strings.Contains(formattedGet, "Static Analysis") || !strings.Contains(formattedGet, "Readiness") {
		t.Errorf("formatted get missing details: %s", formattedGet)
	}
}

func TestTaskSend_IdempotencyAndTargetWakeUp(t *testing.T) {
	ctx := context.Background()
	memStore := newMemoryWorkStore()
	em := a2apkg.NewEventManager(memStore)

	sendInput := TaskSendInput{
		TenantID:         "tenant-test",
		RequesterAgentID: "agent-coordinator",
		TargetAgentID:    "agent-specialist",
		Message:          "Analyze security posture of auth package",
		IdempotencyKey:   "idem-task-001",
		Budget: &WorkBudgetInput{
			MaxDepth:     3,
			MaxChildren:  5,
			MaxRuntimeMs: 30000,
			MaxTokens:    50000,
		},
		Trace: &TraceCorrelationInput{
			TraceID: "trace-xyz-123",
			SpanID:  "span-abc-456",
		},
	}

	// First send: should succeed and create work record
	res1, err := TaskSend(ctx, memStore, em, sendInput)
	if err != nil {
		t.Fatalf("first TaskSend failed: %v", err)
	}
	if res1.IsDuplicate {
		t.Error("first send should not be duplicate")
	}
	if res1.State != "SUBMITTED" {
		t.Errorf("expected initial state SUBMITTED, got %s", res1.State)
	}
	if res1.WorkID == "" || res1.ContextID == "" {
		t.Fatalf("expected non-empty WorkID and ContextID, got %s / %s", res1.WorkID, res1.ContextID)
	}

	// Replay with identical idempotency key: should return existing work (idempotent)
	res2, err := TaskSend(ctx, memStore, em, sendInput)
	if err != nil {
		t.Fatalf("second TaskSend failed: %v", err)
	}
	if !res2.IsDuplicate {
		t.Error("second send must be identified as duplicate/idempotent replay")
	}
	if res2.WorkID != res1.WorkID {
		t.Errorf("expected same WorkID %s, got %s", res1.WorkID, res2.WorkID)
	}

	formatted := FormatTaskSendResult(res1)
	if !strings.Contains(formatted, "A2A Task Sent Successfully") || !strings.Contains(formatted, res1.WorkID) {
		t.Errorf("unexpected formatted output: %s", formatted)
	}
	if !strings.Contains(formatted, "Do NOT poll") {
		t.Errorf("formatted output must remind agent not to poll: %s", formatted)
	}
}

func TestTaskManage_LifecycleAndDelegatedReviewReply(t *testing.T) {
	ctx := context.Background()
	memStore := newMemoryWorkStore()
	em := a2apkg.NewEventManager(memStore)

	// Step 1: Coordinator delegates a review task
	originContextID := "ctx-delegated-review-100"
	parentWorkID := "work-parent-root"

	sendRes, err := TaskSend(ctx, memStore, em, TaskSendInput{
		TenantID:         "default",
		RequesterAgentID: "agent-coordinator",
		TargetAgentID:    "agent-reviewer",
		Message:          "Review pull request #42 for security flaws",
		ContextID:        originContextID,
		ParentWorkID:     parentWorkID,
		IdempotencyKey:   "idem-rev-42",
		Budget: &WorkBudgetInput{
			MaxDepth:     2,
			MaxRuntimeMs: 15000,
		},
	})
	if err != nil {
		t.Fatalf("TaskSend failed: %v", err)
	}

	reviewWorkID := sendRes.WorkID

	// Step 2: Target agent fetches task via TaskGet
	getRes, err := TaskGet(ctx, memStore, TaskGetInput{
		TenantID: "default",
		WorkID:   reviewWorkID,
	})
	if err != nil {
		t.Fatalf("TaskGet failed: %v", err)
	}
	if getRes.ContextID != originContextID || getRes.ParentWorkID != parentWorkID {
		t.Errorf("task should carry origin context %s and parent %s, got %s / %s",
			originContextID, parentWorkID, getRes.ContextID, getRes.ParentWorkID)
	}
	if getRes.DelegationDepth != 1 {
		t.Errorf("expected delegation depth 1, got %d", getRes.DelegationDepth)
	}

	// Step 3: Reviewer subscribes or works, then submits review reply with artifacts
	replyRes, err := TaskReply(ctx, memStore, em, TaskReplyInput{
		TenantID:      "default",
		WorkID:        reviewWorkID,
		CallerAgentID: "agent-reviewer",
		State:         "COMPLETED",
		Message:       "Security review completed: 0 vulnerabilities found; LGTM",
		Artifacts: []ArtifactInput{
			{
				ArtifactID:  "art-sec-report",
				Name:        "security_audit_report.json",
				MediaType:   "application/json",
				ExternalURI: "https://artifacts.888a2a.local/reports/sec_42.json",
				Description: "Full static analysis summary with 0 criticals",
				SizeBytes:   1420,
			},
		},
	})
	if err != nil {
		t.Fatalf("TaskReply failed: %v", err)
	}
	if replyRes.State != "COMPLETED" {
		t.Errorf("expected state COMPLETED, got %s", replyRes.State)
	}
	if len(replyRes.Artifacts) != 1 {
		t.Fatalf("expected 1 attached artifact, got %d", len(replyRes.Artifacts))
	}

	// Step 4: Verify task in originating context reflects the completed review and artifacts
	originTask, err := TaskGet(ctx, memStore, TaskGetInput{
		TenantID: "default",
		WorkID:   reviewWorkID,
	})
	if err != nil {
		t.Fatalf("TaskGet for originTask failed: %v", err)
	}
	if originTask.State != "COMPLETED" {
		t.Errorf("expected COMPLETED state, got %s", originTask.State)
	}
	if !strings.Contains(originTask.TerminalReason, "0 vulnerabilities found") {
		t.Errorf("expected terminal reason to contain review message, got %s", originTask.TerminalReason)
	}
	if len(originTask.Artifacts) != 1 || originTask.Artifacts[0].Name != "security_audit_report.json" {
		t.Errorf("expected artifact security_audit_report.json in origin task, got %v", originTask.Artifacts)
	}

	// Step 5: Query via TaskList
	listRes, err := TaskList(ctx, memStore, TaskListInput{
		TenantID:         "default",
		ContextID:        originContextID,
		IncludeArtifacts: true,
	})
	if err != nil {
		t.Fatalf("TaskList failed: %v", err)
	}
	if listRes.TotalCount != 1 || len(listRes.Tasks) != 1 {
		t.Fatalf("expected 1 task in origin context, got total=%d len=%d", listRes.TotalCount, len(listRes.Tasks))
	}

	// Step 6: Test TaskCancel idempotency
	cancelRes, err := TaskCancel(ctx, memStore, em, TaskCancelInput{
		TenantID: "default",
		WorkID:   reviewWorkID,
		Reason:   "User canceled",
	})
	if err != nil {
		t.Fatalf("TaskCancel on completed task should not error: %v", err)
	}
	if !cancelRes.AlreadyDone {
		t.Error("cancel on already-completed task should report AlreadyDone=true")
	}
}
