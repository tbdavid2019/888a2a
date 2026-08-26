package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type AuditLogMessage struct {
	OrganizationID string
	RequesterID    string
	ExecutorID     string
	Method         string
	ActorType      string
	ActorID        string
	SourceIP       string
	Status         string
	Error          string
	// Resource is the target resource of the audited call, e.g. "agents/{rid}".
	Resource string
	// Payload is the structured change payload as JSON (e.g. IAM binding
	// deltas), stored in the jsonb payload column.
	Payload   string
	CreatedAt time.Time
}

// CreateAuditLogs inserts audit rows in multi-row statements, chunked so a
// batch never exceeds Postgres' 65,535 parameter limit (12 columns per row).
// The audit interceptor batches records in memory and flushes them here, so a
// steady stream of audited calls costs one round trip per chunk per flush
// window instead of one INSERT per call.
func (s *Store) CreateAuditLogs(ctx context.Context, logs []*AuditLogMessage) error {
	if len(logs) == 0 {
		return nil
	}
	const (
		columns   = 12
		chunkSize = 2000 // 24,000 parameters per statement, safely under the limit
	)
	for start := 0; start < len(logs); start += chunkSize {
		chunk := logs[start:min(start+chunkSize, len(logs))]
		var sb strings.Builder
		// strings.Builder never errors; discard the (int, error) results.
		write := func(s string) { _, _ = sb.WriteString(s) }
		write(`
			INSERT INTO audit_log (organization_id, requester_id, executor_id, method, actor_type, actor_id, source_ip, status, error, resource, payload, created_at)
			VALUES `)
		args := make([]any, 0, len(chunk)*columns)
		for i, l := range chunk {
			organizationID := l.OrganizationID
			if organizationID == "" {
				organizationID = "default"
			}
			if i > 0 {
				write(",")
			}
			base := i * columns
			write(fmt.Sprintf("($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
				base+1, base+2, base+3, base+4, base+5, base+6, base+7, base+8, base+9, base+10, base+11, base+12))
			args = append(args, organizationID, l.RequesterID, l.ExecutorID, l.Method, l.ActorType, l.ActorID, l.SourceIP, l.Status, l.Error, l.Resource, normalizeAuditPayload(l.Payload), l.CreatedAt)
		}
		if _, err := s.GetDB().ExecContext(ctx, sb.String(), args...); err != nil {
			return err
		}
	}
	return nil
}

// normalizeAuditPayload guarantees a value the jsonb payload column accepts.
// The interceptor emits "" when a call carries no structured change (e.g.
// ConnectMachine), and Postgres rejects ” as jsonb with SQLSTATE 22P02.
func normalizeAuditPayload(payload string) string {
	if !json.Valid([]byte(payload)) {
		return "{}"
	}
	return payload
}

// AuditLogRecord is a stored audit log row.
type AuditLogRecord struct {
	ID             int64
	OrganizationID string
	RequesterID    string
	ExecutorID     string
	Method         string
	ActorType      string
	ActorID        string
	SourceIP       string
	Status         string
	Error          string
	Resource       string
	Payload        string
	CreatedAt      time.Time
}

// FindAuditLogMessage filters ListAuditLogs. Nil fields are not filtered;
// ordering defaults to create_time DESC.
type FindAuditLogMessage struct {
	OrganizationID *string
	Method         *string
	ActorID        *string
	Resource       *string
	Status         *string
	Limit          *int
	Offset         *int
	OrderAsc       bool
}

// ListAuditLogs returns audit log rows matching the filter, ordered by
// create_time (then id for stable tie-breaking).
func (s *Store) ListAuditLogs(ctx context.Context, find *FindAuditLogMessage) ([]*AuditLogRecord, error) {
	if find == nil {
		find = &FindAuditLogMessage{}
	}
	where := []string{"organization_id = $1"}
	args := []any{tenantIDFromContext(ctx)}
	add := func(column string, v *string) {
		if v == nil {
			return
		}
		where = append(where, fmt.Sprintf("%s = $%d", column, len(args)+1))
		args = append(args, *v)
	}
	if find.OrganizationID != nil && *find.OrganizationID != tenantIDFromContext(ctx) {
		return nil, fmt.Errorf("audit log organization does not match request tenant")
	}
	add("method", find.Method)
	add("actor_id", find.ActorID)
	add("resource", find.Resource)
	add("status", find.Status)

	order := "DESC"
	if find.OrderAsc {
		order = "ASC"
	}
	query := `
		SELECT id, organization_id, requester_id, executor_id, method, actor_type, actor_id, source_ip, status, error, resource, payload, created_at
		FROM audit_log
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY created_at ` + order + `, id ` + order
	if find.Limit != nil {
		query += fmt.Sprintf(" LIMIT $%d", len(args)+1)
		args = append(args, *find.Limit)
	}
	if find.Offset != nil {
		query += fmt.Sprintf(" OFFSET $%d", len(args)+1)
		args = append(args, *find.Offset)
	}

	rows, err := s.GetDB().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []*AuditLogRecord
	for rows.Next() {
		r := &AuditLogRecord{}
		var payload []byte
		if err := rows.Scan(
			&r.ID, &r.OrganizationID, &r.RequesterID, &r.ExecutorID, &r.Method, &r.ActorType, &r.ActorID, &r.SourceIP, &r.Status,
			&r.Error, &r.Resource, &payload, &r.CreatedAt,
		); err != nil {
			return nil, err
		}
		r.Payload = string(payload)
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}
