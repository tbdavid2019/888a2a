package a2a

import (
	"context"
	"encoding/base64"
	"strconv"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv/taskstore"
	"github.com/google/uuid"
	"github.com/pkg/errors"

	"github.com/Ranxy/laelia/backend/manager/store"
)

// DurableTaskStoreAdapter connects the official A2A SDK taskstore.Store to the 888a2a PostgreSQL persistence.
type DurableTaskStoreAdapter struct {
	store        *store.Store
	eventManager *EventManager
}

type idempotencyContextKey struct{}

// NewDurableTaskStore creates a new task store adapter.
func NewDurableTaskStore(store *store.Store, eventManager *EventManager) *DurableTaskStoreAdapter {
	return &DurableTaskStoreAdapter{
		store:        store,
		eventManager: eventManager,
	}
}

// Create persists a new task record and ensures context persistence before acknowledgment.
func (d *DurableTaskStoreAdapter) Create(ctx context.Context, task *a2a.Task) (taskstore.TaskVersion, error) {
	if task == nil {
		return 0, errors.New("task is required")
	}

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

	// Ensure work context exists
	if _, err := d.store.EnsureWorkContext(ctx, tenant, contextID, workID); err != nil {
		return 0, errors.Wrap(err, "ensure work context in create")
	}

	requester := extractRequesterFromContext(ctx, task)
	executor := extractExecutorFromContext(ctx, task)
	idempotencyKey := extractIdempotencyKey(ctx, task)

	// Check if this work was already created with this idempotency key
	existing, err := d.store.GetWorkByIdempotencyKey(ctx, tenant, requester, idempotencyKey)
	if err == nil && existing != nil {
		return 0, taskstore.ErrTaskAlreadyExists
	}

	initialState := "SUBMITTED"
	if task.Status.State != "" {
		initialState = mapTaskStateToDurableState(task.Status.State)
	}

	now := time.Now()
	workMsg := &store.WorkMessage{
		TenantID:         tenant,
		WorkID:           workID,
		A2ATaskID:        workID,
		ContextID:        contextID,
		RequesterAgentID: requester,
		ExecutorAgentID:  executor,
		State:            initialState,
		IdempotencyKey:   idempotencyKey,
		CreatedAt:        now,
		UpdatedAt:        now,
		Version:          1,
	}

	if err := d.store.CreateWork(ctx, workMsg); err != nil {
		if errors.Is(err, store.ErrWorkAlreadyExists) {
			return 0, taskstore.ErrTaskAlreadyExists
		}
		return 0, errors.Wrap(err, "create work record")
	}

	// Persist initial work event
	initialEvent := &store.WorkEventMessage{
		TenantID:  tenant,
		EventID:   uuid.New().String(),
		WorkID:    workID,
		Sequence:  1,
		EventType: initialState,
		CreatedAt: now,
		Metadata: map[string]string{
			"context_id": contextID,
		},
	}
	_ = d.store.AppendWorkEvent(ctx, initialEvent)
	if d.eventManager != nil {
		d.eventManager.Publish(tenant, workID, task, 1)
	}

	return taskstore.TaskVersion(1), nil
}

// Update modifies an existing task with optimistic concurrency control.
func (d *DurableTaskStoreAdapter) Update(ctx context.Context, update *taskstore.UpdateRequest) (taskstore.TaskVersion, error) {
	if update == nil || update.Task == nil {
		return 0, errors.New("update request and task are required")
	}

	tenant, _ := a2a.TenantFrom(ctx)
	if tenant == "" {
		tenant = "default"
	}

	workID := string(update.Task.ID)
	existing, err := d.store.GetWorkByA2ATaskID(ctx, tenant, workID)
	if err != nil {
		if errors.Is(err, store.ErrWorkNotFound) {
			return 0, a2a.ErrTaskNotFound
		}
		return 0, errors.Wrap(err, "get work for update")
	}

	// Optimistic concurrency control check
	if update.PrevVersion != 0 && uint64(update.PrevVersion) != existing.Version {
		// Terminal state idempotency: if already terminal and requesting the same terminal state, treat as idempotent
		if isTerminalState(existing.State) {
			return taskstore.TaskVersion(existing.Version), nil
		}
		return 0, taskstore.ErrConcurrentModification
	}

	newState := mapTaskStateToDurableState(update.Task.Status.State)
	terminalReason := ""
	if update.Task.Status.Message != nil {
		var parts []string
		for _, p := range update.Task.Status.Message.Parts {
			if t := p.Text(); t != "" {
				parts = append(parts, t)
			}
		}
		terminalReason = strings.Join(parts, " ")
	}

	newVersion, err := d.store.UpdateWorkState(ctx, tenant, existing.WorkID, existing.Version, newState, terminalReason)
	if err != nil {
		if errors.Is(err, store.ErrWorkVersionMismatch) {
			// Check if already in terminal state
			current, getErr := d.store.GetWork(ctx, tenant, existing.WorkID)
			if getErr == nil && isTerminalState(current.State) {
				return taskstore.TaskVersion(current.Version), nil
			}
			return 0, taskstore.ErrConcurrentModification
		}
		return 0, errors.Wrap(err, "update work state")
	}

	// Handle artifact updates if present
	if update.Event != nil {
		if artEvent, ok := update.Event.(*a2a.TaskArtifactUpdateEvent); ok && artEvent.Artifact != nil {
			artMsg := &store.WorkArtifactMessage{
				TenantID:   tenant,
				WorkID:     existing.WorkID,
				ArtifactID: string(artEvent.Artifact.ID),
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
			}
			if len(artEvent.Artifact.Parts) > 0 {
				for _, p := range artEvent.Artifact.Parts {
					if u := p.URL(); u != "" {
						artMsg.ExternalURI = string(u)
						artMsg.MediaType = p.MediaType
					} else if text := p.Text(); text != "" {
						artMsg.Description = text
					}
				}
			}
			_ = d.store.CreateWorkArtifact(ctx, artMsg)
		}
	}

	// Append durable event
	seq, _ := d.store.GetLatestWorkEventSequence(ctx, tenant, existing.WorkID)
	nextSeq := seq + 1
	eventMsg := &store.WorkEventMessage{
		TenantID:       tenant,
		EventID:        uuid.New().String(),
		WorkID:         existing.WorkID,
		Sequence:       nextSeq,
		EventType:      newState,
		TerminalReason: terminalReason,
		CreatedAt:      time.Now(),
		Metadata: map[string]string{
			"context_id": existing.ContextID,
		},
	}
	_ = d.store.AppendWorkEvent(ctx, eventMsg)

	if d.eventManager != nil && update.Event != nil {
		d.eventManager.Publish(tenant, existing.WorkID, update.Event, nextSeq)
	}

	return taskstore.TaskVersion(newVersion), nil
}

// Get retrieves a stored task by ID.
func (d *DurableTaskStoreAdapter) Get(ctx context.Context, taskID a2a.TaskID) (*taskstore.StoredTask, error) {
	tenant, _ := a2a.TenantFrom(ctx)
	if tenant == "" {
		tenant = "default"
	}

	work, err := d.store.GetWorkByA2ATaskID(ctx, tenant, string(taskID))
	if err != nil {
		if errors.Is(err, store.ErrWorkNotFound) {
			return nil, a2a.ErrTaskNotFound
		}
		return nil, errors.Wrap(err, "get work by task id")
	}

	// Caller authorization check: if caller principal is present in context, verify access
	if caller, ok := CallerFromContext(ctx); ok && caller != nil && caller.IsAuthenticated() {
		callerID := caller.GetPrincipalID()
		if callerID != "" && work.RequesterAgentID != callerID && work.ExecutorAgentID != callerID {
			// Check if admin or owner, else deny enumeration
			if !isAuthorizedCaller(caller, work) {
				return nil, a2a.ErrTaskNotFound
			}
		}
	}

	artifacts, _ := d.store.ListWorkArtifacts(ctx, tenant, work.WorkID)

	task := projectWorkToTask(work, artifacts)
	return &taskstore.StoredTask{
		Task:    task,
		Version: taskstore.TaskVersion(work.Version),
		User:    work.RequesterAgentID,
	}, nil
}

// List lists tasks based on request parameters with cursor pagination.
func (d *DurableTaskStoreAdapter) List(ctx context.Context, req *a2a.ListTasksRequest) (*a2a.ListTasksResponse, error) {
	if req == nil {
		req = &a2a.ListTasksRequest{}
	}

	tenant := req.Tenant
	if tenant == "" {
		tenant, _ = a2a.TenantFrom(ctx)
	}
	if tenant == "" {
		tenant = "default"
	}

	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 100 {
		pageSize = 100
	}

	offset := 0
	if req.PageToken != "" {
		if decoded, err := base64.StdEncoding.DecodeString(req.PageToken); err == nil {
			if parsedOffset, err := strconv.Atoi(string(decoded)); err == nil && parsedOffset >= 0 {
				offset = parsedOffset
			}
		}
	}

	filter := store.ListWorkFilter{
		TenantID:  tenant,
		ContextID: req.ContextID,
		Limit:     pageSize,
		Offset:    offset,
	}
	if req.Status != "" {
		filter.State = mapTaskStateToDurableState(req.Status)
	}

	// Enforce peer isolation: if caller is an Agent, only list accessible work
	if caller, ok := CallerFromContext(ctx); ok && caller != nil && caller.IsAuthenticated() {
		if !isAdminCaller(caller) {
			filter.RequesterAgentID = caller.GetPrincipalID()
		}
	}

	works, totalCount, err := d.store.ListWork(ctx, filter)
	if err != nil {
		return nil, errors.Wrap(err, "list work from store")
	}

	tasks := make([]*a2a.Task, 0, len(works))
	for _, w := range works {
		var artifacts []*store.WorkArtifactMessage
		if req.IncludeArtifacts {
			artifacts, _ = d.store.ListWorkArtifacts(ctx, tenant, w.WorkID)
		}
		tasks = append(tasks, projectWorkToTask(w, artifacts))
	}

	var nextPageToken string
	if offset+len(works) < totalCount {
		nextOffset := offset + len(works)
		nextPageToken = base64.StdEncoding.EncodeToString([]byte(strconv.Itoa(nextOffset)))
	}

	return &a2a.ListTasksResponse{
		Tasks:         tasks,
		TotalSize:     totalCount,
		PageSize:      pageSize,
		NextPageToken: nextPageToken,
	}, nil
}

func projectWorkToTask(w *store.WorkMessage, artifacts []*store.WorkArtifactMessage) *a2a.Task {
	if w == nil {
		return nil
	}

	updatedAt := w.UpdatedAt
	task := &a2a.Task{
		ID:        a2a.TaskID(w.A2ATaskID),
		ContextID: w.ContextID,
		Status: a2a.TaskStatus{
			State:     mapDurableStateToTaskState(w.State),
			Timestamp: &updatedAt,
		},
	}

	if w.TerminalReason != "" {
		task.Status.Message = a2a.NewMessage(
			a2a.MessageRoleAgent,
			a2a.NewTextPart(w.TerminalReason),
		)
	}

	if len(artifacts) > 0 {
		for _, a := range artifacts {
			var parts []*a2a.Part
			if a.ExternalURI != "" {
				parts = append(parts, a2a.NewFileURLPart(a2a.URL(a.ExternalURI), a.MediaType))
			} else if a.Description != "" {
				parts = append(parts, a2a.NewTextPart(a.Description))
			}
			task.Artifacts = append(task.Artifacts, &a2a.Artifact{
				ID:    a2a.ArtifactID(a.ArtifactID),
				Parts: parts,
			})
		}
	}

	return task
}

func mapTaskStateToDurableState(s a2a.TaskState) string {
	switch s {
	case a2a.TaskStateSubmitted:
		return "SUBMITTED"
	case a2a.TaskStateWorking:
		return "WORKING"
	case a2a.TaskStateCompleted:
		return "COMPLETED"
	case a2a.TaskStateFailed:
		return "FAILED"
	case a2a.TaskStateCanceled:
		return "CANCELED"
	case a2a.TaskStateInputRequired:
		return "INPUT_REQUIRED"
	case a2a.TaskStateAuthRequired:
		return "AUTH_REQUIRED"
	case a2a.TaskStateRejected:
		return "REJECTED"
	case a2a.TaskStateUnspecified:
		fallthrough
	default:
		return "SUBMITTED"
	}
}

func mapDurableStateToTaskState(s string) a2a.TaskState {
	switch s {
	case "SUBMITTED":
		return a2a.TaskStateSubmitted
	case "WORKING":
		return a2a.TaskStateWorking
	case "COMPLETED":
		return a2a.TaskStateCompleted
	case "FAILED":
		return a2a.TaskStateFailed
	case "CANCELED":
		return a2a.TaskStateCanceled
	case "INPUT_REQUIRED":
		return a2a.TaskStateInputRequired
	case "AUTH_REQUIRED":
		return a2a.TaskStateAuthRequired
	case "REJECTED":
		return a2a.TaskStateRejected
	default:
		return a2a.TaskStateSubmitted
	}
}

func isTerminalState(s string) bool {
	return s == "COMPLETED" || s == "FAILED" || s == "CANCELED" || s == "REJECTED"
}

type callerContextKey struct{}

// WithCaller injects a CallerPrincipal into context.
func WithCaller(ctx context.Context, caller CallerPrincipal) context.Context {
	return context.WithValue(ctx, callerContextKey{}, caller)
}

// CallerFromContext extracts a CallerPrincipal from context if present.
func CallerFromContext(ctx context.Context) (CallerPrincipal, bool) {
	val, ok := ctx.Value(callerContextKey{}).(CallerPrincipal)
	return val, ok
}

func extractRequesterFromContext(ctx context.Context, task *a2a.Task) string {
	if caller, ok := CallerFromContext(ctx); ok && caller != nil {
		if id := caller.GetPrincipalID(); id != "" {
			return id
		}
	}
	if task != nil && string(task.ID) != "" {
		return "agent-" + string(task.ID)
	}
	return "anonymous"
}

func extractExecutorFromContext(ctx context.Context, _ *a2a.Task) string {
	if target, ok := ctx.Value(targetAgentContextKey{}).(string); ok && target != "" {
		return target
	}
	return "default-executor"
}

func extractIdempotencyKey(ctx context.Context, task *a2a.Task) string {
	if key, ok := ctx.Value(idempotencyContextKey{}).(string); ok && key != "" {
		return key
	}
	if task != nil && string(task.ID) != "" {
		return string(task.ID)
	}
	return uuid.New().String()
}

func isAuthorizedCaller(caller CallerPrincipal, work *store.WorkMessage) bool {
	if caller == nil {
		return false
	}
	if isAdminCaller(caller) {
		return true
	}
	cid := caller.GetPrincipalID()
	return cid == work.RequesterAgentID || cid == work.ExecutorAgentID
}

func isAdminCaller(caller CallerPrincipal) bool {
	if caller == nil {
		return false
	}
	if adminCheck, ok := caller.(interface{ IsAdmin() bool }); ok {
		return adminCheck.IsAdmin()
	}
	return false
}
