package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pkg/errors"

	a2a888 "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
	models "github.com/tbdavid2019/888a2a/backend/generated-go/store"
)

// TenantPrincipal describes a principal after membership and organization
// lifecycle checks have been applied. It is deliberately separate from
// UserMessage and AgentMessage: a human identity may have several independent
// tenant principals, while an Agent is a distinct executor identity.
func (s *OrganizationStore) ResolveTenantPrincipal(ctx context.Context, organizationID string, principalID int) (*a2a888.TenantPrincipal, error) {
	if organizationID == "" || principalID == 0 {
		return nil, ErrMembershipNotFound
	}
	var (
		principalType, name, handle, membershipState, role string
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT p.type, p.name, p.handle, m.state, m.role
		FROM principal p
		JOIN organization_memberships m ON m.principal_id = p.id
		JOIN organizations o ON o.id = m.organization_id
		WHERE p.id = $1 AND m.organization_id = $2 AND o.state <> 'CLOSED'
	`, principalID, organizationID).Scan(&principalType, &name, &handle, &membershipState, &role)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrMembershipNotFound
	}
	if err != nil {
		return nil, errors.Wrap(err, "resolve tenant principal")
	}

	typeValue := a2a888.TenantPrincipalType_TENANT_PRINCIPAL_TYPE_END_USER
	if parsed, ok := models.PrincipalType_value[principalType]; ok {
		switch models.PrincipalType(parsed) {
		case models.PrincipalType_SERVICE_ACCOUNT:
			typeValue = a2a888.TenantPrincipalType_TENANT_PRINCIPAL_TYPE_SERVICE_ACCOUNT
		case models.PrincipalType_SYSTEM_BOT:
			typeValue = a2a888.TenantPrincipalType_TENANT_PRINCIPAL_TYPE_SYSTEM_BOT
		}
	}
	suspended := membershipState != "ACTIVE"
	return &a2a888.TenantPrincipal{
		PrincipalId:    formatPrincipalID(principalID),
		OrganizationId: organizationID,
		PrincipalType:  typeValue,
		DisplayName:    name,
		Handle:         handle,
		EffectiveRoles: []string{role},
		IsSuspended:    suspended,
	}, nil
}

// ResolveTenantAgent resolves an Agent executor without conflating it with a
// human/service-account row in principal. Agent identities are tenant-bound by
// their own resource row and inherit suspension from the owning organization.
func (s *OrganizationStore) ResolveTenantAgent(ctx context.Context, organizationID, resourceID string) (*a2a888.TenantPrincipal, error) {
	if organizationID == "" || resourceID == "" {
		return nil, ErrMembershipNotFound
	}
	var agentID, name string
	var workspaceID sql.NullString
	var deleted, enabled bool
	err := s.db.QueryRowContext(ctx, `
		SELECT resource_id, name, workspace_id, deleted, enabled
		FROM agent
		WHERE organization_id = $1 AND resource_id = $2
	`, organizationID, resourceID).Scan(&agentID, &name, &workspaceID, &deleted, &enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrMembershipNotFound
	}
	if err != nil {
		return nil, errors.Wrap(err, "resolve tenant agent")
	}
	resolvedWorkspace := ""
	if workspaceID.Valid {
		resolvedWorkspace = workspaceID.String
	}
	return &a2a888.TenantPrincipal{
		PrincipalId:        agentID,
		OrganizationId:     organizationID,
		PrincipalType:      a2a888.TenantPrincipalType_TENANT_PRINCIPAL_TYPE_AGENT,
		DisplayName:        name,
		Handle:             agentID,
		IsSuspended:        deleted || !enabled,
		CurrentWorkspaceId: resolvedWorkspace,
	}, nil
}

func formatPrincipalID(id int) string {
	return fmt.Sprintf("%d", id)
}
