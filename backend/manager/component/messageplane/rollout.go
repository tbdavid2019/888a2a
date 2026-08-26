package messageplane

import (
	"context"
	"database/sql"
	"strings"

	"github.com/pkg/errors"
)

// PathMode selects the collaboration read/write path for one Organization.
// Legacy is the safe default. Dual keeps the legacy path authoritative while
// populating MessagePlane, and MessagePlane enables the new path. Keeping the
// selector in PostgreSQL makes rollback effective across Manager replicas.
type PathMode string

const (
	PathModeLegacy       PathMode = "LEGACY"
	PathModeDual         PathMode = "DUAL"
	PathModeMessagePlane PathMode = "MESSAGE_PLANE"
)

// PathSelector stores the per-Organization collaboration rollout flag.
type PathSelector struct {
	db *sql.DB
}

func NewPathSelector(db *sql.DB) (*PathSelector, error) {
	if db == nil {
		return nil, errors.New("collaboration path database is required")
	}
	return &PathSelector{db: db}, nil
}

// Mode returns LEGACY when an Organization has no explicit rollout row. A
// missing or malformed row is an error so callers can fail closed to LEGACY.
func (s *PathSelector) Mode(ctx context.Context, organizationID string) (PathMode, error) {
	if s == nil || s.db == nil {
		return PathModeLegacy, nil
	}
	organizationID = strings.TrimSpace(organizationID)
	if organizationID == "" {
		return PathModeLegacy, errors.New("organization_id is required")
	}
	var mode string
	err := s.db.QueryRowContext(ctx, `
		SELECT mode FROM a2a888_collaboration_rollout WHERE organization_id = $1
	`, organizationID).Scan(&mode)
	if errors.Is(err, sql.ErrNoRows) {
		return PathModeLegacy, nil
	}
	if err != nil {
		return PathModeLegacy, errors.Wrap(err, "read collaboration rollout")
	}
	selected := PathMode(strings.ToUpper(strings.TrimSpace(mode)))
	switch selected {
	case PathModeLegacy, PathModeDual, PathModeMessagePlane:
		return selected, nil
	default:
		return PathModeLegacy, errors.Errorf("invalid collaboration rollout mode %q", mode)
	}
}

// SetMode changes the Organization rollout atomically. It is intentionally a
// small internal boundary; an authenticated admin API can call it later after
// the rollout policy is exposed to operators.
func (s *PathSelector) SetMode(ctx context.Context, organizationID string, mode PathMode) error {
	if s == nil || s.db == nil {
		return errors.New("collaboration path database is required")
	}
	organizationID = strings.TrimSpace(organizationID)
	if organizationID == "" {
		return errors.New("organization_id is required")
	}
	if mode != PathModeLegacy && mode != PathModeDual && mode != PathModeMessagePlane {
		return errors.Errorf("invalid collaboration rollout mode %q", mode)
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO a2a888_collaboration_rollout (organization_id, mode)
		VALUES ($1, $2)
		ON CONFLICT (organization_id) DO UPDATE SET mode = EXCLUDED.mode, updated_at = now()
	`, organizationID, mode)
	return errors.Wrap(err, "set collaboration rollout")
}
