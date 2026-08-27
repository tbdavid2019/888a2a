package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/pkg/errors"
)

func (s *Store) AddRetentionHold(ctx context.Context, organizationID, resourceType, resourceID, reason string) error {
	if organizationID == "" || resourceType == "" || resourceID == "" || reason == "" {
		return errors.New("retention hold requires tenant, resource, and reason")
	}
	_, err := s.GetDB().ExecContext(ctx, `
		INSERT INTO a2a888_retention_hold (organization_id,resource_type,resource_id,reason)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (organization_id,resource_type,resource_id) DO UPDATE SET reason=EXCLUDED.reason
	`, organizationID, resourceType, resourceID, reason)
	return errors.Wrap(err, "add retention hold")
}

// RedactExpiredConnectorEvents irreversibly removes raw connector payloads
// past cutoff while retaining the delivery identity and a tenant-scoped
// retention outcome. Legal-held events are preserved and audited as skipped.
func (s *Store) RedactExpiredConnectorEvents(ctx context.Context, organizationID string, cutoff time.Time, limit int) (int, error) {
	if s == nil || s.GetDB() == nil {
		return 0, errors.New("retention database is required")
	}
	if organizationID == "" || cutoff.IsZero() || limit <= 0 {
		return 0, errors.New("retention tenant, cutoff, and positive limit are required")
	}
	tx, err := s.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return 0, errors.Wrap(err, "begin retention run")
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `
		SELECT installation_id, external_event_id
		FROM a2a888_connector_inbox
		WHERE organization_id=$1 AND received_at < $2 AND raw_payload <> '{}'::jsonb
		ORDER BY received_at LIMIT $3 FOR UPDATE SKIP LOCKED
	`, organizationID, cutoff.UTC(), limit)
	if err != nil {
		return 0, errors.Wrap(err, "select retention candidates")
	}
	type candidate struct {
		installationID string
		eventID        string
	}
	var candidates []candidate
	for rows.Next() {
		var installationID, eventID string
		if err := rows.Scan(&installationID, &eventID); err != nil {
			return 0, errors.Wrap(err, "scan retention candidate")
		}
		candidates = append(candidates, candidate{installationID: installationID, eventID: eventID})
	}
	if err := rows.Err(); err != nil {
		return 0, errors.Wrap(err, "iterate retention candidates")
	}
	if err := rows.Close(); err != nil {
		return 0, errors.Wrap(err, "close retention candidates")
	}
	count := 0
	for _, candidate := range candidates {
		installationID, eventID := candidate.installationID, candidate.eventID
		resourceID := installationID + ":" + eventID
		var held bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM a2a888_retention_hold WHERE organization_id=$1 AND resource_type='connector_event' AND resource_id=$2)`, organizationID, resourceID).Scan(&held); err != nil {
			return 0, errors.Wrap(err, "check retention hold")
		}
		action := "REDACTED"
		detail := "connector raw payload redacted after retention cutoff"
		if held {
			action = "SKIPPED_LEGAL_HOLD"
			detail = "connector raw payload preserved by legal hold"
		} else if _, err := tx.ExecContext(ctx, `UPDATE a2a888_connector_inbox SET raw_payload='{}'::jsonb, updated_at=now() WHERE organization_id=$1 AND installation_id=$2 AND external_event_id=$3`, organizationID, installationID, eventID); err != nil {
			return 0, errors.Wrap(err, "redact connector payload")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO a2a888_retention_outcome (organization_id,resource_type,resource_id,action,detail) VALUES ($1,'connector_event',$2,$3,$4)`, organizationID, resourceID, action, detail); err != nil {
			return 0, errors.Wrap(err, "record retention outcome")
		}
		count++
	}
	if err := tx.Commit(); err != nil {
		return 0, errors.Wrap(err, "commit retention run")
	}
	return count, nil
}

func (s *Store) HasRetentionHold(ctx context.Context, organizationID, resourceType, resourceID string) (bool, error) {
	if s == nil || s.GetDB() == nil {
		return false, errors.New("retention database is required")
	}
	var held bool
	err := s.GetDB().QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM a2a888_retention_hold WHERE organization_id=$1 AND resource_type=$2 AND resource_id=$3)`, organizationID, resourceType, resourceID).Scan(&held)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return held, errors.Wrap(err, "check retention hold")
}
