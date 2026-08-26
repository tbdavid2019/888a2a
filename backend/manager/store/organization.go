package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lib/pq"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/timestamppb"

	a2a888 "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
)

var (
	// ErrOrganizationNotFound is returned when an organization does not exist.
	ErrOrganizationNotFound = errors.New("organization not found")
	// ErrWorkspaceNotFound is returned when a workspace does not exist.
	ErrWorkspaceNotFound = errors.New("workspace not found")
	// ErrMembershipNotFound is returned when a principal membership does not exist.
	ErrMembershipNotFound = errors.New("organization membership not found")
)

// OrganizationStore encapsulates database operations for organizations, workspaces, and memberships.
type OrganizationStore struct {
	db *sql.DB
}

// NewOrganizationStore creates a new OrganizationStore.
func NewOrganizationStore(db *sql.DB) *OrganizationStore {
	return &OrganizationStore{db: db}
}

// CreateOrganization inserts a new organization.
func (s *OrganizationStore) CreateOrganization(ctx context.Context, org *a2a888.Organization) (*a2a888.Organization, error) {
	if org == nil || org.Id == "" || org.Slug == "" {
		return nil, errors.New("organization id and slug are required")
	}

	stateStr := org.State.String()
	if org.State == a2a888.OrganizationState_ORGANIZATION_STATE_UNSPECIFIED {
		stateStr = a2a888.OrganizationState_ORGANIZATION_STATE_ACTIVE.String()
	}

	metaBytes, err := json.Marshal(org.Metadata)
	if err != nil {
		metaBytes = []byte("{}")
	}

	now := time.Now().UTC()
	query := `
		INSERT INTO organizations (id, name, slug, state, metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, name, slug, state, metadata, created_at, updated_at
	`

	var (
		resID, resName, resSlug, resState string
		resMetaBytes                      []byte
		resCreatedAt, resUpdatedAt        time.Time
	)

	err = s.db.QueryRowContext(ctx, query, org.Id, org.Name, org.Slug, stateStr, metaBytes, now, now).Scan(
		&resID, &resName, &resSlug, &resState, &resMetaBytes, &resCreatedAt, &resUpdatedAt,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to insert organization")
	}

	var meta map[string]string
	_ = json.Unmarshal(resMetaBytes, &meta)

	return &a2a888.Organization{
		Id:        resID,
		Name:      resName,
		Slug:      resSlug,
		State:     parseOrgState(resState),
		Metadata:  meta,
		CreatedAt: timestamppb.New(resCreatedAt),
		UpdatedAt: timestamppb.New(resUpdatedAt),
	}, nil
}

// GetOrganization retrieves an organization by its ID.
func (s *OrganizationStore) GetOrganization(ctx context.Context, id string) (*a2a888.Organization, error) {
	query := `
		SELECT id, name, slug, state, metadata, created_at, updated_at
		FROM organizations
		WHERE id = $1
	`

	var (
		resID, resName, resSlug, resState string
		resMetaBytes                      []byte
		resCreatedAt, resUpdatedAt        time.Time
	)

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&resID, &resName, &resSlug, &resState, &resMetaBytes, &resCreatedAt, &resUpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrOrganizationNotFound
		}
		return nil, errors.Wrap(err, "failed to get organization")
	}

	var meta map[string]string
	_ = json.Unmarshal(resMetaBytes, &meta)

	return &a2a888.Organization{
		Id:        resID,
		Name:      resName,
		Slug:      resSlug,
		State:     parseOrgState(resState),
		Metadata:  meta,
		CreatedAt: timestamppb.New(resCreatedAt),
		UpdatedAt: timestamppb.New(resUpdatedAt),
	}, nil
}

// GetOrganizationBySlug retrieves an organization by slug.
func (s *OrganizationStore) GetOrganizationBySlug(ctx context.Context, slug string) (*a2a888.Organization, error) {
	query := `
		SELECT id, name, slug, state, metadata, created_at, updated_at
		FROM organizations
		WHERE slug = $1
	`

	var (
		resID, resName, resSlug, resState string
		resMetaBytes                      []byte
		resCreatedAt, resUpdatedAt        time.Time
	)

	err := s.db.QueryRowContext(ctx, query, slug).Scan(
		&resID, &resName, &resSlug, &resState, &resMetaBytes, &resCreatedAt, &resUpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrOrganizationNotFound
		}
		return nil, errors.Wrap(err, "failed to get organization by slug")
	}

	var meta map[string]string
	_ = json.Unmarshal(resMetaBytes, &meta)

	return &a2a888.Organization{
		Id:        resID,
		Name:      resName,
		Slug:      resSlug,
		State:     parseOrgState(resState),
		Metadata:  meta,
		CreatedAt: timestamppb.New(resCreatedAt),
		UpdatedAt: timestamppb.New(resUpdatedAt),
	}, nil
}

// UpdateOrganizationState updates the operational state of an organization.
func (s *OrganizationStore) UpdateOrganizationState(ctx context.Context, id string, state a2a888.OrganizationState) error {
	query := `
		UPDATE organizations
		SET state = $1, updated_at = now()
		WHERE id = $2
	`
	res, err := s.db.ExecContext(ctx, query, state.String(), id)
	if err != nil {
		return errors.Wrap(err, "failed to update organization state")
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "failed to get affected rows")
	}
	if rows == 0 {
		return ErrOrganizationNotFound
	}
	return nil
}

// CreateWorkspace creates a new workspace within an organization.
func (s *OrganizationStore) CreateWorkspace(ctx context.Context, ws *a2a888.Workspace) (*a2a888.Workspace, error) {
	if ws == nil || ws.Id == "" || ws.OrganizationId == "" || ws.Slug == "" {
		return nil, errors.New("workspace id, organization id, and slug are required")
	}

	now := time.Now().UTC()
	query := `
		INSERT INTO workspaces (id, organization_id, name, slug, is_default, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, organization_id, name, slug, is_default, created_at, updated_at
	`

	var (
		resID, resOrgID, resName, resSlug string
		resIsDefault                      bool
		resCreatedAt, resUpdatedAt        time.Time
	)

	err := s.db.QueryRowContext(ctx, query, ws.Id, ws.OrganizationId, ws.Name, ws.Slug, ws.IsDefault, now, now).Scan(
		&resID, &resOrgID, &resName, &resSlug, &resIsDefault, &resCreatedAt, &resUpdatedAt,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create workspace")
	}

	return &a2a888.Workspace{
		Id:             resID,
		OrganizationId: resOrgID,
		Name:           resName,
		Slug:           resSlug,
		IsDefault:      resIsDefault,
		CreatedAt:      timestamppb.New(resCreatedAt),
		UpdatedAt:      timestamppb.New(resUpdatedAt),
	}, nil
}

// ListWorkspaces lists all workspaces for an organization.
func (s *OrganizationStore) ListWorkspaces(ctx context.Context, orgID string) ([]*a2a888.Workspace, error) {
	query := `
		SELECT id, organization_id, name, slug, is_default, created_at, updated_at
		FROM workspaces
		WHERE organization_id = $1
		ORDER BY is_default DESC, name ASC
	`
	rows, err := s.db.QueryContext(ctx, query, orgID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to query workspaces")
	}
	defer rows.Close()

	var workspaces []*a2a888.Workspace
	for rows.Next() {
		var (
			wsID, resOrgID, name, slug string
			isDefault                  bool
			createdAt, updatedAt       time.Time
		)
		if err := rows.Scan(&wsID, &resOrgID, &name, &slug, &isDefault, &createdAt, &updatedAt); err != nil {
			return nil, errors.Wrap(err, "failed to scan workspace")
		}
		workspaces = append(workspaces, &a2a888.Workspace{
			Id:             wsID,
			OrganizationId: resOrgID,
			Name:           name,
			Slug:           slug,
			IsDefault:      isDefault,
			CreatedAt:      timestamppb.New(createdAt),
			UpdatedAt:      timestamppb.New(updatedAt),
		})
	}
	return workspaces, rows.Err()
}

// AddMembership adds a principal to an organization.
func (s *OrganizationStore) AddMembership(ctx context.Context, m *a2a888.OrganizationMembership) (*a2a888.OrganizationMembership, error) {
	if m == nil || m.OrganizationId == "" || m.PrincipalId == "" {
		return nil, errors.New("organization id and principal id are required")
	}

	roleStr := m.Role.String()
	if m.Role == a2a888.OrganizationRole_ORGANIZATION_ROLE_UNSPECIFIED {
		roleStr = a2a888.OrganizationRole_ORGANIZATION_ROLE_MEMBER.String()
	}

	stateStr := m.State.String()
	if m.State == a2a888.MembershipState_MEMBERSHIP_STATE_UNSPECIFIED {
		stateStr = a2a888.MembershipState_MEMBERSHIP_STATE_ACTIVE.String()
	}

	now := time.Now().UTC()
	query := `
		INSERT INTO organization_memberships (organization_id, principal_id, role, state, workspace_ids, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING organization_id, principal_id, role, state, workspace_ids, created_at, updated_at
	`

	var (
		resOrgID, resRole, resState string
		resPrincipalID              int
		resWorkspaceIDs             []string
		resCreatedAt, resUpdatedAt  time.Time
	)

	err := s.db.QueryRowContext(ctx, query, m.OrganizationId, m.PrincipalId, roleStr, stateStr, pq.Array(m.WorkspaceIds), now, now).Scan(
		&resOrgID, &resPrincipalID, &resRole, &resState, pq.Array(&resWorkspaceIDs), &resCreatedAt, &resUpdatedAt,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to insert organization membership")
	}

	return &a2a888.OrganizationMembership{
		OrganizationId: resOrgID,
		PrincipalId:    fmt.Sprintf("%d", resPrincipalID),
		Role:           parseOrgRole(resRole),
		State:          parseMembershipState(resState),
		WorkspaceIds:   resWorkspaceIDs,
		CreatedAt:      timestamppb.New(resCreatedAt),
		UpdatedAt:      timestamppb.New(resUpdatedAt),
	}, nil
}

// GetMembership retrieves a principal's membership in an organization.
func (s *OrganizationStore) GetMembership(ctx context.Context, orgID string, principalID int) (*a2a888.OrganizationMembership, error) {
	query := `
		SELECT organization_id, principal_id, role, state, workspace_ids, created_at, updated_at
		FROM organization_memberships
		WHERE organization_id = $1 AND principal_id = $2
	`

	var (
		resOrgID, resRole, resState string
		resPrincipalID              int
		resWorkspaceIDs             []string
		resCreatedAt, resUpdatedAt  time.Time
	)

	err := s.db.QueryRowContext(ctx, query, orgID, principalID).Scan(
		&resOrgID, &resPrincipalID, &resRole, &resState, pq.Array(&resWorkspaceIDs), &resCreatedAt, &resUpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrMembershipNotFound
		}
		return nil, errors.Wrap(err, "failed to get membership")
	}

	return &a2a888.OrganizationMembership{
		OrganizationId: resOrgID,
		PrincipalId:    fmt.Sprintf("%d", resPrincipalID),
		Role:           parseOrgRole(resRole),
		State:          parseMembershipState(resState),
		WorkspaceIds:   resWorkspaceIDs,
		CreatedAt:      timestamppb.New(resCreatedAt),
		UpdatedAt:      timestamppb.New(resUpdatedAt),
	}, nil
}

// UpdateMembership updates role, state, or workspace bindings for a member.
func (s *OrganizationStore) UpdateMembership(ctx context.Context, m *a2a888.OrganizationMembership) error {
	if m == nil || m.OrganizationId == "" || m.PrincipalId == "" {
		return errors.New("organization id and principal id are required")
	}

	query := `
		UPDATE organization_memberships
		SET role = $1, state = $2, workspace_ids = $3, updated_at = now()
		WHERE organization_id = $4 AND principal_id = $5
	`
	res, err := s.db.ExecContext(ctx, query, m.Role.String(), m.State.String(), pq.Array(m.WorkspaceIds), m.OrganizationId, m.PrincipalId)
	if err != nil {
		return errors.Wrap(err, "failed to update organization membership")
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "failed to get affected rows")
	}
	if rows == 0 {
		return ErrMembershipNotFound
	}
	return nil
}

// SetDefaultOrganizationForPrincipal sets the active default organization for a principal.
func (s *OrganizationStore) SetDefaultOrganizationForPrincipal(ctx context.Context, principalID int, orgID string) error {
	query := `
		UPDATE principal
		SET default_organization_id = $1
		WHERE id = $2
		  AND EXISTS (
			SELECT 1
			FROM organization_memberships m
			JOIN organizations o ON o.id = m.organization_id
			WHERE m.organization_id = $1
			  AND m.principal_id = $2
			  AND m.state = 'ACTIVE'
			  AND o.state <> 'CLOSED'
		  )
	`
	result, err := s.db.ExecContext(ctx, query, orgID, principalID)
	if err != nil {
		return errors.Wrap(err, "failed to set default organization for principal")
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "failed to get affected principal rows")
	}
	if rows == 0 {
		return ErrMembershipNotFound
	}
	return nil
}

// ListOrganizationsForPrincipal retrieves all active organizations a principal belongs to.
func (s *OrganizationStore) ListOrganizationsForPrincipal(ctx context.Context, principalID int) ([]*a2a888.Organization, error) {
	query := `
		SELECT o.id, o.name, o.slug, o.state, o.metadata, o.created_at, o.updated_at
		FROM organizations o
		INNER JOIN organization_memberships m ON o.id = m.organization_id
		WHERE m.principal_id = $1 AND m.state = 'ACTIVE' AND o.state <> 'CLOSED'
		ORDER BY o.name ASC
	`
	rows, err := s.db.QueryContext(ctx, query, principalID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to query organizations for principal")
	}
	defer rows.Close()

	var orgs []*a2a888.Organization
	for rows.Next() {
		var (
			resID, resName, resSlug, resState string
			resMetaBytes                      []byte
			resCreatedAt, resUpdatedAt        time.Time
		)
		if err := rows.Scan(&resID, &resName, &resSlug, &resState, &resMetaBytes, &resCreatedAt, &resUpdatedAt); err != nil {
			return nil, errors.Wrap(err, "failed to scan organization")
		}
		var meta map[string]string
		_ = json.Unmarshal(resMetaBytes, &meta)

		orgs = append(orgs, &a2a888.Organization{
			Id:        resID,
			Name:      resName,
			Slug:      resSlug,
			State:     parseOrgState(resState),
			Metadata:  meta,
			CreatedAt: timestamppb.New(resCreatedAt),
			UpdatedAt: timestamppb.New(resUpdatedAt),
		})
	}
	return orgs, rows.Err()
}

// ListMemberships retrieves all memberships belonging to an organization.
func (s *OrganizationStore) ListMemberships(ctx context.Context, orgID string) ([]*a2a888.OrganizationMembership, error) {
	if orgID == "" {
		return nil, errors.New("organization id is required")
	}

	query := `
		SELECT organization_id, principal_id, role, state, workspace_ids, created_at, updated_at
		FROM organization_memberships
		WHERE organization_id = $1
		ORDER BY created_at ASC
	`
	rows, err := s.db.QueryContext(ctx, query, orgID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to query memberships")
	}
	defer rows.Close()

	var memberships []*a2a888.OrganizationMembership
	for rows.Next() {
		var (
			resOrgID, resRole, resState string
			resPrincipalID              int
			resWorkspaceIDs             []string
			resCreatedAt, resUpdatedAt  time.Time
		)
		if err := rows.Scan(&resOrgID, &resPrincipalID, &resRole, &resState, pq.Array(&resWorkspaceIDs), &resCreatedAt, &resUpdatedAt); err != nil {
			return nil, errors.Wrap(err, "failed to scan membership")
		}
		memberships = append(memberships, &a2a888.OrganizationMembership{
			OrganizationId: resOrgID,
			PrincipalId:    fmt.Sprintf("%d", resPrincipalID),
			Role:           parseOrgRole(resRole),
			State:          parseMembershipState(resState),
			WorkspaceIds:   resWorkspaceIDs,
			CreatedAt:      timestamppb.New(resCreatedAt),
			UpdatedAt:      timestamppb.New(resUpdatedAt),
		})
	}
	return memberships, rows.Err()
}

func parseOrgState(s string) a2a888.OrganizationState {
	switch s {
	case "ACTIVE", "ORGANIZATION_STATE_ACTIVE":
		return a2a888.OrganizationState_ORGANIZATION_STATE_ACTIVE
	case "SUSPENDED", "ORGANIZATION_STATE_SUSPENDED":
		return a2a888.OrganizationState_ORGANIZATION_STATE_SUSPENDED
	case "CLOSED", "ORGANIZATION_STATE_CLOSED":
		return a2a888.OrganizationState_ORGANIZATION_STATE_CLOSED
	default:
		return a2a888.OrganizationState_ORGANIZATION_STATE_ACTIVE
	}
}

func parseOrgRole(s string) a2a888.OrganizationRole {
	switch s {
	case "OWNER", "ORGANIZATION_ROLE_OWNER":
		return a2a888.OrganizationRole_ORGANIZATION_ROLE_OWNER
	case "ADMIN", "ORGANIZATION_ROLE_ADMIN":
		return a2a888.OrganizationRole_ORGANIZATION_ROLE_ADMIN
	case "MEMBER", "ORGANIZATION_ROLE_MEMBER":
		return a2a888.OrganizationRole_ORGANIZATION_ROLE_MEMBER
	case "GUEST", "ORGANIZATION_ROLE_GUEST":
		return a2a888.OrganizationRole_ORGANIZATION_ROLE_GUEST
	default:
		return a2a888.OrganizationRole_ORGANIZATION_ROLE_MEMBER
	}
}

func parseMembershipState(s string) a2a888.MembershipState {
	switch s {
	case "ACTIVE", "MEMBERSHIP_STATE_ACTIVE":
		return a2a888.MembershipState_MEMBERSHIP_STATE_ACTIVE
	case "SUSPENDED", "MEMBERSHIP_STATE_SUSPENDED":
		return a2a888.MembershipState_MEMBERSHIP_STATE_SUSPENDED
	case "INVITED", "MEMBERSHIP_STATE_INVITED":
		return a2a888.MembershipState_MEMBERSHIP_STATE_INVITED
	default:
		return a2a888.MembershipState_MEMBERSHIP_STATE_ACTIVE
	}
}

// TenantCacheKey scopes a cache key by organization ID to prevent cross-tenant cache collisions.
func TenantCacheKey(orgID string, resourceType string, key string) string {
	if orgID == "" {
		orgID = "default"
	}
	return fmt.Sprintf("org:%s:%s:%s", orgID, resourceType, key)
}

// TenantProjectionKey scopes a local projection key by organization ID.
func TenantProjectionKey(orgID string, projectionName string, id string) string {
	if orgID == "" {
		orgID = "default"
	}
	return fmt.Sprintf("org:%s:proj:%s:%s", orgID, projectionName, id)
}
