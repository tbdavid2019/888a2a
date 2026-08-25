package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pkg/errors"
)

var (
	// ErrWorkNotFound indicates no work record matching the specified criteria was found.
	ErrWorkNotFound = errors.New("work record not found")
	// ErrWorkContextNotFound indicates no work context matching the specified criteria was found.
	ErrWorkContextNotFound = errors.New("work context not found")
	// ErrWorkAlreadyExists indicates a work record already exists for the unique key.
	ErrWorkAlreadyExists = errors.New("work record already exists")
	// ErrWorkVersionMismatch indicates an optimistic concurrency conflict on update.
	ErrWorkVersionMismatch = errors.New("work version mismatch")
	// ErrWorkArtifactNotFound indicates no work artifact matching the specified key was found.
	ErrWorkArtifactNotFound = errors.New("work artifact not found")
)

// WorkContextMessage represents a row in a2a888_work_context.
type WorkContextMessage struct {
	TenantID   string
	ContextID  string
	RootWorkID sql.NullString
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Version    uint64
}

// WorkMessage represents a row in a2a888_work.
type WorkMessage struct {
	TenantID             string
	WorkID               string
	A2ATaskID            string
	ContextID            string
	RequesterAgentID     string
	ExecutorAgentID      string
	SourceConversationID *uuid.UUID
	SourceTaskID         *uuid.UUID
	State                string
	TerminalReason       string
	IdempotencyKey       string
	TraceID              string
	RootTraceID          string
	SpanID               string
	ParentSpanID         string
	ParentWorkID         sql.NullString
	ParentEdgeType       string
	DelegationDepth      int32
	RetryCount           int32
	MaxDepth             int32
	MaxChildren          int32
	MaxFanOut            int32
	MaxConcurrency       int32
	MaxRuntimeMs         int64
	MaxRetries           int32
	MaxTokens            int64
	MaxWorkUnits         int64
	UsedChildren         int32
	UsedFanOut           int32
	UsedRuntimeMs        int64
	UsedTokens           int64
	UsedWorkUnits        int64
	CreatedAt            time.Time
	UpdatedAt            time.Time
	StartedAt            *time.Time
	CompletedAt          *time.Time
	Version              uint64
}

// WorkArtifactMessage represents a row in a2a888_work_artifact.
type WorkArtifactMessage struct {
	TenantID    string
	WorkID      string
	ArtifactID  string
	Name        string
	Description string
	MediaType   string
	ExternalURI string
	FileID      *uuid.UUID
	Digest      string
	SizeBytes   int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// WorkEventMessage represents a row in a2a888_work_event.
type WorkEventMessage struct {
	TenantID       string
	EventID        string
	WorkID         string
	Sequence       uint64
	TraceID        string
	RootTraceID    string
	SpanID         string
	ParentSpanID   string
	EventType      string
	ProviderID     string
	SessionID      string
	PolicyDecision string
	RetryCount     int32
	TerminalReason string
	Metadata       map[string]string
	CreatedAt      time.Time
}

// ListWorkFilter specifies filter criteria for ListWork.
type ListWorkFilter struct {
	TenantID         string
	ContextID        string
	RequesterAgentID string
	ExecutorAgentID  string
	State            string
	Limit            int
	Offset           int
}

const insertWorkContextSQL = `
INSERT INTO a2a888_work_context (tenant_id, context_id, root_work_id, created_at, updated_at, version)
VALUES ($1, $2, $3, now(), now(), 1)
ON CONFLICT (tenant_id, context_id) DO UPDATE
SET updated_at = now()
RETURNING tenant_id, context_id, root_work_id, created_at, updated_at, version;
`

const getWorkContextSQL = `
SELECT tenant_id, context_id, root_work_id, created_at, updated_at, version
FROM a2a888_work_context
WHERE tenant_id = $1 AND context_id = $2;
`

// EnsureWorkContext inserts or updates the work context record and returns it.
func (s *Store) EnsureWorkContext(ctx context.Context, tenantID, contextID, rootWorkID string) (*WorkContextMessage, error) {
	if tenantID == "" {
		tenantID = "default"
	}
	if contextID == "" {
		return nil, errors.New("context_id cannot be empty")
	}

	var rootWorkNull sql.NullString
	if rootWorkID != "" {
		rootWorkNull = sql.NullString{String: rootWorkID, Valid: true}
	}

	row := s.dbConnManager.GetDB().QueryRowContext(ctx, insertWorkContextSQL, tenantID, contextID, rootWorkNull)
	var res WorkContextMessage
	if err := row.Scan(&res.TenantID, &res.ContextID, &res.RootWorkID, &res.CreatedAt, &res.UpdatedAt, &res.Version); err != nil {
		return nil, errors.Wrap(err, "ensure work context")
	}
	return &res, nil
}

// GetWorkContext retrieves a work context record.
func (s *Store) GetWorkContext(ctx context.Context, tenantID, contextID string) (*WorkContextMessage, error) {
	if tenantID == "" {
		tenantID = "default"
	}
	row := s.dbConnManager.GetDB().QueryRowContext(ctx, getWorkContextSQL, tenantID, contextID)
	var res WorkContextMessage
	if err := row.Scan(&res.TenantID, &res.ContextID, &res.RootWorkID, &res.CreatedAt, &res.UpdatedAt, &res.Version); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrWorkContextNotFound
		}
		return nil, errors.Wrap(err, "get work context")
	}
	return &res, nil
}

const insertWorkSQL = `
INSERT INTO a2a888_work (
	tenant_id, work_id, a2a_task_id, context_id, requester_agent_id, executor_agent_id,
	source_conversation_id, source_task_id, state, terminal_reason, idempotency_key,
	trace_id, root_trace_id, span_id, parent_span_id, parent_work_id, parent_edge_type,
	delegation_depth, retry_count, max_depth, max_children, max_fan_out, max_concurrency,
	max_runtime_ms, max_retries, max_tokens, max_work_units, used_children, used_fan_out,
	used_runtime_ms, used_tokens, used_work_units, created_at, updated_at, started_at,
	completed_at, version
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17,
	$18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31, $32,
	now(), now(), $33, $34, 1
);
`

// CreateWork inserts a new work record. Returns ErrWorkAlreadyExists if idempotency key or task ID conflicts.
func (s *Store) CreateWork(ctx context.Context, work *WorkMessage) error {
	if work == nil {
		return errors.New("work cannot be nil")
	}
	if work.TenantID == "" {
		work.TenantID = "default"
	}
	if work.State == "" {
		work.State = "SUBMITTED"
	}
	if work.ParentEdgeType == "" {
		work.ParentEdgeType = "delegated"
	}

	_, err := s.dbConnManager.GetDB().ExecContext(
		ctx,
		insertWorkSQL,
		work.TenantID,
		work.WorkID,
		work.A2ATaskID,
		work.ContextID,
		work.RequesterAgentID,
		work.ExecutorAgentID,
		work.SourceConversationID,
		work.SourceTaskID,
		work.State,
		work.TerminalReason,
		work.IdempotencyKey,
		work.TraceID,
		work.RootTraceID,
		work.SpanID,
		work.ParentSpanID,
		work.ParentWorkID,
		work.ParentEdgeType,
		work.DelegationDepth,
		work.RetryCount,
		work.MaxDepth,
		work.MaxChildren,
		work.MaxFanOut,
		work.MaxConcurrency,
		work.MaxRuntimeMs,
		work.MaxRetries,
		work.MaxTokens,
		work.MaxWorkUnits,
		work.UsedChildren,
		work.UsedFanOut,
		work.UsedRuntimeMs,
		work.UsedTokens,
		work.UsedWorkUnits,
		work.StartedAt,
		work.CompletedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrWorkAlreadyExists
		}
		return errors.Wrap(err, "insert work")
	}
	return nil
}

const workSelectColumns = `
tenant_id, work_id, a2a_task_id, context_id, requester_agent_id, executor_agent_id,
source_conversation_id, source_task_id, state, terminal_reason, idempotency_key,
trace_id, root_trace_id, span_id, parent_span_id, parent_work_id, parent_edge_type,
delegation_depth, retry_count, max_depth, max_children, max_fan_out, max_concurrency,
max_runtime_ms, max_retries, max_tokens, max_work_units, used_children, used_fan_out,
used_runtime_ms, used_tokens, used_work_units, created_at, updated_at, started_at,
completed_at, version
`

func scanWorkRow(scanner interface{ Scan(dest ...any) error }) (*WorkMessage, error) {
	var w WorkMessage
	err := scanner.Scan(
		&w.TenantID,
		&w.WorkID,
		&w.A2ATaskID,
		&w.ContextID,
		&w.RequesterAgentID,
		&w.ExecutorAgentID,
		&w.SourceConversationID,
		&w.SourceTaskID,
		&w.State,
		&w.TerminalReason,
		&w.IdempotencyKey,
		&w.TraceID,
		&w.RootTraceID,
		&w.SpanID,
		&w.ParentSpanID,
		&w.ParentWorkID,
		&w.ParentEdgeType,
		&w.DelegationDepth,
		&w.RetryCount,
		&w.MaxDepth,
		&w.MaxChildren,
		&w.MaxFanOut,
		&w.MaxConcurrency,
		&w.MaxRuntimeMs,
		&w.MaxRetries,
		&w.MaxTokens,
		&w.MaxWorkUnits,
		&w.UsedChildren,
		&w.UsedFanOut,
		&w.UsedRuntimeMs,
		&w.UsedTokens,
		&w.UsedWorkUnits,
		&w.CreatedAt,
		&w.UpdatedAt,
		&w.StartedAt,
		&w.CompletedAt,
		&w.Version,
	)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// GetWork retrieves a work record by primary key (tenant_id, work_id).
func (s *Store) GetWork(ctx context.Context, tenantID, workID string) (*WorkMessage, error) {
	if tenantID == "" {
		tenantID = "default"
	}
	query := fmt.Sprintf("SELECT %s FROM a2a888_work WHERE tenant_id = $1 AND work_id = $2;", workSelectColumns)
	row := s.dbConnManager.GetDB().QueryRowContext(ctx, query, tenantID, workID)
	w, err := scanWorkRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrWorkNotFound
		}
		return nil, errors.Wrap(err, "get work")
	}
	return w, nil
}

// GetWorkByA2ATaskID retrieves a work record by (tenant_id, a2a_task_id).
func (s *Store) GetWorkByA2ATaskID(ctx context.Context, tenantID, a2aTaskID string) (*WorkMessage, error) {
	if tenantID == "" {
		tenantID = "default"
	}
	query := fmt.Sprintf("SELECT %s FROM a2a888_work WHERE tenant_id = $1 AND a2a_task_id = $2;", workSelectColumns)
	row := s.dbConnManager.GetDB().QueryRowContext(ctx, query, tenantID, a2aTaskID)
	w, err := scanWorkRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrWorkNotFound
		}
		return nil, errors.Wrap(err, "get work by a2a task id")
	}
	return w, nil
}

// GetWorkByIdempotencyKey retrieves a work record by (tenant_id, requester_agent_id, idempotency_key).
func (s *Store) GetWorkByIdempotencyKey(ctx context.Context, tenantID, requesterAgentID, idempotencyKey string) (*WorkMessage, error) {
	if tenantID == "" {
		tenantID = "default"
	}
	query := fmt.Sprintf("SELECT %s FROM a2a888_work WHERE tenant_id = $1 AND requester_agent_id = $2 AND idempotency_key = $3;", workSelectColumns)
	row := s.dbConnManager.GetDB().QueryRowContext(ctx, query, tenantID, requesterAgentID, idempotencyKey)
	w, err := scanWorkRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrWorkNotFound
		}
		return nil, errors.Wrap(err, "get work by idempotency key")
	}
	return w, nil
}

const updateWorkStateSQL = `
UPDATE a2a888_work
SET state = $3,
    terminal_reason = $4,
    completed_at = CASE WHEN $3 IN ('COMPLETED', 'FAILED', 'CANCELED', 'REJECTED') AND completed_at IS NULL THEN now() ELSE completed_at END,
    started_at = CASE WHEN $3 = 'WORKING' AND started_at IS NULL THEN now() ELSE started_at END,
    updated_at = now(),
    version = version + 1
WHERE tenant_id = $1 AND work_id = $2 AND version = $5
RETURNING version;
`

// UpdateWorkState performs an optimistic-locking transition of a work record's state.
func (s *Store) UpdateWorkState(ctx context.Context, tenantID, workID string, expectedVersion uint64, newState string, terminalReason string) (uint64, error) {
	if tenantID == "" {
		tenantID = "default"
	}
	var newVersion uint64
	err := s.dbConnManager.GetDB().QueryRowContext(
		ctx,
		updateWorkStateSQL,
		tenantID,
		workID,
		newState,
		terminalReason,
		expectedVersion,
	).Scan(&newVersion)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrWorkVersionMismatch
		}
		return 0, errors.Wrap(err, "update work state")
	}
	return newVersion, nil
}

// ListWork returns a list of work items matching filter criteria and total count.
func (s *Store) ListWork(ctx context.Context, filter ListWorkFilter) ([]*WorkMessage, int, error) {
	if filter.TenantID == "" {
		filter.TenantID = "default"
	}
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}

	whereClauses := []string{"tenant_id = $1"}
	args := []any{filter.TenantID}
	idx := 2

	if filter.ContextID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("context_id = $%d", idx))
		args = append(args, filter.ContextID)
		idx++
	}
	if filter.RequesterAgentID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("requester_agent_id = $%d", idx))
		args = append(args, filter.RequesterAgentID)
		idx++
	}
	if filter.ExecutorAgentID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("executor_agent_id = $%d", idx))
		args = append(args, filter.ExecutorAgentID)
		idx++
	}
	if filter.State != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("state = $%d", idx))
		args = append(args, filter.State)
		idx++
	}

	whereSQL := strings.Join(whereClauses, " AND ")

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM a2a888_work WHERE %s;", whereSQL)
	var totalCount int
	if err := s.dbConnManager.GetDB().QueryRowContext(ctx, countQuery, args...).Scan(&totalCount); err != nil {
		return nil, 0, errors.Wrap(err, "count work")
	}

	listQuery := fmt.Sprintf(
		"SELECT %s FROM a2a888_work WHERE %s ORDER BY created_at DESC, work_id DESC LIMIT $%d OFFSET $%d;",
		workSelectColumns,
		whereSQL,
		idx,
		idx+1,
	)
	args = append(args, filter.Limit, filter.Offset)

	rows, err := s.dbConnManager.GetDB().QueryContext(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, errors.Wrap(err, "list work query")
	}
	defer rows.Close()

	var result []*WorkMessage
	for rows.Next() {
		w, err := scanWorkRow(rows)
		if err != nil {
			return nil, 0, errors.Wrap(err, "scan work row")
		}
		result = append(result, w)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, errors.Wrap(err, "iterate work rows")
	}
	return result, totalCount, nil
}

// ListPendingWorkForRecovery finds all work records that are in SUBMITTED or WORKING state across tenants.
func (s *Store) ListPendingWorkForRecovery(ctx context.Context) ([]*WorkMessage, error) {
	query := fmt.Sprintf(
		"SELECT %s FROM a2a888_work WHERE state IN ('SUBMITTED', 'WORKING') ORDER BY updated_at ASC;",
		workSelectColumns,
	)
	rows, err := s.dbConnManager.GetDB().QueryContext(ctx, query)
	if err != nil {
		return nil, errors.Wrap(err, "list pending work for recovery")
	}
	defer rows.Close()

	var result []*WorkMessage
	for rows.Next() {
		w, err := scanWorkRow(rows)
		if err != nil {
			return nil, errors.Wrap(err, "scan pending work row")
		}
		result = append(result, w)
	}
	return result, rows.Err()
}

const insertWorkArtifactSQL = `
INSERT INTO a2a888_work_artifact (
	tenant_id, work_id, artifact_id, name, description, media_type,
	external_uri, file_id, digest, size_bytes, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now(), now())
ON CONFLICT (tenant_id, work_id, artifact_id) DO UPDATE
SET name = EXCLUDED.name,
    description = EXCLUDED.description,
    media_type = EXCLUDED.media_type,
    external_uri = EXCLUDED.external_uri,
    file_id = EXCLUDED.file_id,
    digest = EXCLUDED.digest,
    size_bytes = EXCLUDED.size_bytes,
    updated_at = now();
`

// CreateWorkArtifact creates or updates an artifact reference for a work record.
func (s *Store) CreateWorkArtifact(ctx context.Context, artifact *WorkArtifactMessage) error {
	if artifact == nil {
		return errors.New("artifact cannot be nil")
	}
	if artifact.TenantID == "" {
		artifact.TenantID = "default"
	}
	_, err := s.dbConnManager.GetDB().ExecContext(
		ctx,
		insertWorkArtifactSQL,
		artifact.TenantID,
		artifact.WorkID,
		artifact.ArtifactID,
		artifact.Name,
		artifact.Description,
		artifact.MediaType,
		artifact.ExternalURI,
		artifact.FileID,
		artifact.Digest,
		artifact.SizeBytes,
	)
	if err != nil {
		return errors.Wrap(err, "insert work artifact")
	}
	return nil
}

const listWorkArtifactsSQL = `
SELECT tenant_id, work_id, artifact_id, name, description, media_type,
       external_uri, file_id, digest, size_bytes, created_at, updated_at
FROM a2a888_work_artifact
WHERE tenant_id = $1 AND work_id = $2
ORDER BY created_at ASC;
`

// ListWorkArtifacts retrieves all artifacts associated with a work record.
func (s *Store) ListWorkArtifacts(ctx context.Context, tenantID, workID string) ([]*WorkArtifactMessage, error) {
	if tenantID == "" {
		tenantID = "default"
	}
	rows, err := s.dbConnManager.GetDB().QueryContext(ctx, listWorkArtifactsSQL, tenantID, workID)
	if err != nil {
		return nil, errors.Wrap(err, "list work artifacts")
	}
	defer rows.Close()

	var result []*WorkArtifactMessage
	for rows.Next() {
		var a WorkArtifactMessage
		if err := rows.Scan(
			&a.TenantID,
			&a.WorkID,
			&a.ArtifactID,
			&a.Name,
			&a.Description,
			&a.MediaType,
			&a.ExternalURI,
			&a.FileID,
			&a.Digest,
			&a.SizeBytes,
			&a.CreatedAt,
			&a.UpdatedAt,
		); err != nil {
			return nil, errors.Wrap(err, "scan work artifact")
		}
		result = append(result, &a)
	}
	return result, rows.Err()
}

const insertWorkEventSQL = `
INSERT INTO a2a888_work_event (
	tenant_id, event_id, work_id, sequence, trace_id, root_trace_id, span_id,
	parent_span_id, event_type, provider_id, session_id, policy_decision,
	retry_count, terminal_reason, metadata, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, now());
`

// AppendWorkEvent writes an event to the durable event log.
func (s *Store) AppendWorkEvent(ctx context.Context, event *WorkEventMessage) error {
	if event == nil {
		return errors.New("event cannot be nil")
	}
	if event.TenantID == "" {
		event.TenantID = "default"
	}
	metaJSON, err := json.Marshal(event.Metadata)
	if err != nil {
		metaJSON = []byte("{}")
	}

	_, err = s.dbConnManager.GetDB().ExecContext(
		ctx,
		insertWorkEventSQL,
		event.TenantID,
		event.EventID,
		event.WorkID,
		event.Sequence,
		event.TraceID,
		event.RootTraceID,
		event.SpanID,
		event.ParentSpanID,
		event.EventType,
		event.ProviderID,
		event.SessionID,
		event.PolicyDecision,
		event.RetryCount,
		event.TerminalReason,
		metaJSON,
	)
	if err != nil {
		return errors.Wrap(err, "insert work event")
	}
	return nil
}

const listWorkEventsSQL = `
SELECT tenant_id, event_id, work_id, sequence, trace_id, root_trace_id, span_id,
       parent_span_id, event_type, provider_id, session_id, policy_decision,
       retry_count, terminal_reason, metadata, created_at
FROM a2a888_work_event
WHERE tenant_id = $1 AND work_id = $2 AND sequence > $3
ORDER BY sequence ASC
LIMIT $4;
`

// ListWorkEvents retrieves ordered events for a work record with sequence > afterSequence.
func (s *Store) ListWorkEvents(ctx context.Context, tenantID, workID string, afterSequence uint64, limit int) ([]*WorkEventMessage, error) {
	if tenantID == "" {
		tenantID = "default"
	}
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.dbConnManager.GetDB().QueryContext(ctx, listWorkEventsSQL, tenantID, workID, afterSequence, limit)
	if err != nil {
		return nil, errors.Wrap(err, "list work events")
	}
	defer rows.Close()

	var result []*WorkEventMessage
	for rows.Next() {
		var e WorkEventMessage
		var metaJSON []byte
		if err := rows.Scan(
			&e.TenantID,
			&e.EventID,
			&e.WorkID,
			&e.Sequence,
			&e.TraceID,
			&e.RootTraceID,
			&e.SpanID,
			&e.ParentSpanID,
			&e.EventType,
			&e.ProviderID,
			&e.SessionID,
			&e.PolicyDecision,
			&e.RetryCount,
			&e.TerminalReason,
			&metaJSON,
			&e.CreatedAt,
		); err != nil {
			return nil, errors.Wrap(err, "scan work event")
		}
		if len(metaJSON) > 0 {
			_ = json.Unmarshal(metaJSON, &e.Metadata)
		}
		result = append(result, &e)
	}
	return result, rows.Err()
}

const getLatestWorkEventSequenceSQL = `
SELECT COALESCE(MAX(sequence), 0)
FROM a2a888_work_event
WHERE tenant_id = $1 AND work_id = $2;
`

// GetLatestWorkEventSequence returns the highest sequence number written for a work record (or 0 if none).
func (s *Store) GetLatestWorkEventSequence(ctx context.Context, tenantID, workID string) (uint64, error) {
	if tenantID == "" {
		tenantID = "default"
	}
	var seq uint64
	err := s.dbConnManager.GetDB().QueryRowContext(ctx, getLatestWorkEventSequenceSQL, tenantID, workID).Scan(&seq)
	if err != nil {
		return 0, errors.Wrap(err, "get latest work event sequence")
	}
	return seq, nil
}
