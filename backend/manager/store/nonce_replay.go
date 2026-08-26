package store

import (
	"context"
	"time"

	"github.com/pkg/errors"
)

// ConsumeNonce atomically records a tenant-scoped heartbeat nonce. It returns
// false for a nonce already consumed by this or another Manager replica.
func (s *Store) ConsumeNonce(ctx context.Context, agentResourceID, nonce string, expiresAt time.Time) (bool, error) {
	if s == nil || s.dbConnManager == nil || agentResourceID == "" || nonce == "" {
		return false, errors.New("agent resource ID and nonce are required")
	}
	organizationID := tenantIDFromContext(ctx)
	result, err := s.GetDB().ExecContext(ctx, `
		INSERT INTO a2a888_nonce_replay (organization_id, agent_resource_id, nonce, expires_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (organization_id, agent_resource_id, nonce) DO NOTHING
	`, organizationID, agentResourceID, nonce, expiresAt)
	if err != nil {
		return false, errors.Wrap(err, "consume nonce")
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, errors.Wrap(err, "check nonce consumption")
	}
	return rows == 1, nil
}
