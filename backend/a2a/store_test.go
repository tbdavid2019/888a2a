package a2a

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv/taskstore"
	"github.com/google/uuid"

	"github.com/Ranxy/laelia/backend/manager/store"
)

type memoryWorkStore struct {
	mu        sync.RWMutex
	contexts  map[string]*store.WorkContextMessage // key: tenant:contextID
	works     map[string]*store.WorkMessage        // key: tenant:workID
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
	// Check unique index on (tenant, requester, idempotency_key)
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
	if isTerminalState(newState) && w.CompletedAt == nil {
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

func (m *memoryWorkStore) ListPendingWorkForRecovery(_ context.Context) ([]*store.WorkMessage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*store.WorkMessage
	for _, w := range m.works {
		if w.State == "SUBMITTED" || w.State == "WORKING" {
			cloned := *w
			result = append(result, &cloned)
		}
	}
	return result, nil
}

type testTaskStoreAdapter struct {
	memStore     *memoryWorkStore
	eventManager *EventManager
}

func newTestTaskStoreAdapter() *testTaskStoreAdapter {
	mem := newMemoryWorkStore()
	em := NewEventManager(mem)
	return &testTaskStoreAdapter{
		memStore:     mem,
		eventManager: em,
	}
}

func (t *testTaskStoreAdapter) Create(ctx context.Context, task *a2a.Task) (taskstore.TaskVersion, error) {
	tenant, _ := a2a.TenantFrom(ctx)
	if tenant == "" {
		tenant = "default"
	}
	workID := string(task.ID)
	if workID == "" {
		workID = uuid.New().String()
		task.ID = a2a.TaskID(workID)
	}
	contextID := task.ContextID
	if contextID == "" {
		contextID = uuid.New().String()
		task.ContextID = contextID
	}

	_, _ = t.memStore.EnsureWorkContext(ctx, tenant, contextID, workID)

	requester := extractRequesterFromContext(ctx, task)
	executor := extractExecutorFromContext(ctx, task)
	idempotencyKey := extractIdempotencyKey(ctx, task)

	existing, err := t.memStore.GetWorkByIdempotencyKey(ctx, tenant, requester, idempotencyKey)
	if err == nil && existing != nil {
		return 0, taskstore.ErrTaskAlreadyExists
	}

	now := time.Now()
	workMsg := &store.WorkMessage{
		TenantID:         tenant,
		WorkID:           workID,
		A2ATaskID:        workID,
		ContextID:        contextID,
		RequesterAgentID: requester,
		ExecutorAgentID:  executor,
		State:            "SUBMITTED",
		IdempotencyKey:   idempotencyKey,
		CreatedAt:        now,
		UpdatedAt:        now,
		Version:          1,
	}

	if err := t.memStore.CreateWork(ctx, workMsg); err != nil {
		return 0, taskstore.ErrTaskAlreadyExists
	}

	_ = t.memStore.AppendWorkEvent(ctx, &store.WorkEventMessage{
		TenantID:  tenant,
		EventID:   uuid.New().String(),
		WorkID:    workID,
		Sequence:  1,
		EventType: "SUBMITTED",
		CreatedAt: now,
	})

	return 1, nil
}

func (t *testTaskStoreAdapter) Update(ctx context.Context, update *taskstore.UpdateRequest) (taskstore.TaskVersion, error) {
	tenant, _ := a2a.TenantFrom(ctx)
	if tenant == "" {
		tenant = "default"
	}

	workID := string(update.Task.ID)
	existing, err := t.memStore.GetWorkByA2ATaskID(ctx, tenant, workID)
	if err != nil {
		return 0, a2a.ErrTaskNotFound
	}

	if update.PrevVersion != 0 && uint64(update.PrevVersion) != existing.Version {
		if isTerminalState(existing.State) {
			return taskstore.TaskVersion(existing.Version), nil
		}
		return 0, taskstore.ErrConcurrentModification
	}

	newState := mapTaskStateToDurableState(update.Task.Status.State)
	newVer, err := t.memStore.UpdateWorkState(ctx, tenant, existing.WorkID, existing.Version, newState, "")
	if err != nil {
		return 0, taskstore.ErrConcurrentModification
	}

	seq, _ := t.memStore.GetLatestWorkEventSequence(ctx, tenant, existing.WorkID)
	nextSeq := seq + 1
	_ = t.memStore.AppendWorkEvent(ctx, &store.WorkEventMessage{
		TenantID:  tenant,
		EventID:   uuid.New().String(),
		WorkID:    existing.WorkID,
		Sequence:  nextSeq,
		EventType: newState,
		CreatedAt: time.Now(),
	})

	if t.eventManager != nil && update.Event != nil {
		t.eventManager.Publish(tenant, existing.WorkID, update.Event, nextSeq)
	}

	return taskstore.TaskVersion(newVer), nil
}

func (t *testTaskStoreAdapter) Get(ctx context.Context, taskID a2a.TaskID) (*taskstore.StoredTask, error) {
	tenant, _ := a2a.TenantFrom(ctx)
	if tenant == "" {
		tenant = "default"
	}

	work, err := t.memStore.GetWorkByA2ATaskID(ctx, tenant, string(taskID))
	if err != nil {
		return nil, a2a.ErrTaskNotFound
	}

	if caller, ok := CallerFromContext(ctx); ok && caller != nil && caller.IsAuthenticated() {
		if !isAuthorizedCaller(caller, work) {
			return nil, a2a.ErrTaskNotFound
		}
	}

	artifacts, _ := t.memStore.ListWorkArtifacts(ctx, tenant, work.WorkID)
	task := projectWorkToTask(work, artifacts)
	return &taskstore.StoredTask{
		Task:    task,
		Version: taskstore.TaskVersion(work.Version),
		User:    work.RequesterAgentID,
	}, nil
}

func (t *testTaskStoreAdapter) List(ctx context.Context, req *a2a.ListTasksRequest) (*a2a.ListTasksResponse, error) {
	tenant := req.Tenant
	if tenant == "" {
		tenant, _ = a2a.TenantFrom(ctx)
	}
	if tenant == "" {
		tenant = "default"
	}

	filter := store.ListWorkFilter{
		TenantID:  tenant,
		ContextID: req.ContextID,
		Limit:     req.PageSize,
	}
	if filter.Limit <= 0 {
		filter.Limit = 50
	}

	if caller, ok := CallerFromContext(ctx); ok && caller != nil && caller.IsAuthenticated() {
		if !isAdminCaller(caller) {
			filter.RequesterAgentID = caller.GetPrincipalID()
		}
	}

	works, totalCount, _ := t.memStore.ListWork(ctx, filter)
	tasks := make([]*a2a.Task, 0, len(works))
	for _, w := range works {
		tasks = append(tasks, projectWorkToTask(w, nil))
	}

	return &a2a.ListTasksResponse{
		Tasks:     tasks,
		TotalSize: totalCount,
		PageSize:  filter.Limit,
	}, nil
}

func TestDurableTaskStore_IdempotencyAndWorkCreation(t *testing.T) {
	ctx := context.Background()
	ts := newTestTaskStoreAdapter()

	taskID := a2a.TaskID("task-12345")
	task := &a2a.Task{
		ID:        taskID,
		ContextID: "ctx-1",
		Status: a2a.TaskStatus{
			State: a2a.TaskStateSubmitted,
		},
	}

	ctx = context.WithValue(ctx, idempotencyContextKey{}, "idem-abc-123")
	caller := &fakeCaller{id: "agent-caller-1", tenant: "default", authenticated: true}
	ctx = WithCaller(ctx, caller)

	// First creation succeeds
	ver, err := ts.Create(ctx, task)
	if err != nil {
		t.Fatalf("first Create failed: %v", err)
	}
	if ver != 1 {
		t.Errorf("expected version 1, got %d", ver)
	}

	// Lost response retry: repeated creation with same idempotency key returns ErrTaskAlreadyExists
	_, err = ts.Create(ctx, task)
	if err != taskstore.ErrTaskAlreadyExists {
		t.Fatalf("expected ErrTaskAlreadyExists on retry, got %v", err)
	}

	// Stored task is queryable and matches original
	stored, err := ts.Get(ctx, taskID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if stored.Task.ID != taskID {
		t.Errorf("expected task ID %q, got %q", taskID, stored.Task.ID)
	}
}

func TestDurableTaskStore_PeerIsolation(t *testing.T) {
	ctx := context.Background()
	ts := newTestTaskStoreAdapter()

	task1 := &a2a.Task{ID: "task-alice-1", ContextID: "ctx-1"}
	task2 := &a2a.Task{ID: "task-bob-1", ContextID: "ctx-2"}

	ctxAlice := WithCaller(ctx, &fakeCaller{id: "agent-alice", tenant: "default", authenticated: true})
	ctxBob := WithCaller(ctx, &fakeCaller{id: "agent-bob", tenant: "default", authenticated: true})

	_, _ = ts.Create(ctxAlice, task1)
	_, _ = ts.Create(ctxBob, task2)

	// Bob should NOT be able to Get Alice's task
	_, err := ts.Get(ctxBob, "task-alice-1")
	if err != a2a.ErrTaskNotFound {
		t.Fatalf("expected Bob to get ErrTaskNotFound for Alice's task, got %v", err)
	}

	// Bob's List should ONLY return Bob's tasks (peer isolation)
	listResp, err := ts.List(ctxBob, &a2a.ListTasksRequest{})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(listResp.Tasks) != 1 || listResp.Tasks[0].ID != "task-bob-1" {
		t.Fatalf("expected Bob to see only his 1 task, got %d tasks", len(listResp.Tasks))
	}
}

func TestDurableTaskStore_OptimisticConcurrencyAndTerminalIdempotency(t *testing.T) {
	ctx := context.Background()
	ts := newTestTaskStoreAdapter()

	task := &a2a.Task{ID: "task-occ-1", ContextID: "ctx-occ"}
	_, err := ts.Create(ctx, task)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Update to WORKING at version 1 -> version 2
	up1 := &taskstore.UpdateRequest{
		Task: &a2a.Task{
			ID:     "task-occ-1",
			Status: a2a.TaskStatus{State: a2a.TaskStateWorking},
		},
		PrevVersion: 1,
	}
	ver, err := ts.Update(ctx, up1)
	if err != nil {
		t.Fatalf("Update 1 failed: %v", err)
	}
	if ver != 2 {
		t.Fatalf("expected version 2, got %d", ver)
	}

	// Concurrent update with stale version 1 should fail
	upStale := &taskstore.UpdateRequest{
		Task: &a2a.Task{
			ID:     "task-occ-1",
			Status: a2a.TaskStatus{State: a2a.TaskStateWorking},
		},
		PrevVersion: 1,
	}
	_, err = ts.Update(ctx, upStale)
	if err != taskstore.ErrConcurrentModification {
		t.Fatalf("expected ErrConcurrentModification for stale version, got %v", err)
	}

	// Update to COMPLETED at version 2 -> version 3
	upComplete := &taskstore.UpdateRequest{
		Task: &a2a.Task{
			ID:     "task-occ-1",
			Status: a2a.TaskStatus{State: a2a.TaskStateCompleted},
		},
		PrevVersion: 2,
	}
	ver, err = ts.Update(ctx, upComplete)
	if err != nil {
		t.Fatalf("Update complete failed: %v", err)
	}
	if ver != 3 {
		t.Fatalf("expected version 3, got %d", ver)
	}

	// Repeated terminal update on completed task is idempotent (does not corrupt)
	upDuplicateCancel := &taskstore.UpdateRequest{
		Task: &a2a.Task{
			ID:     "task-occ-1",
			Status: a2a.TaskStatus{State: a2a.TaskStateCanceled},
		},
		PrevVersion: 1, // Stale version
	}
	ver, err = ts.Update(ctx, upDuplicateCancel)
	if err != nil {
		t.Fatalf("terminal idempotency update should not error, got %v", err)
	}
	if ver != 3 {
		t.Errorf("expected version to remain 3, got %d", ver)
	}
}
