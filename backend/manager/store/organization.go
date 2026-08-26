package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/tbdavid2019/888a2a/backend/common"
	a2a888 "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
)

var (
	// ErrOrganizationNotFound is returned when an organization does not exist.
	ErrOrganizationNotFound = errors.New("organization not found")
	// ErrWorkspaceNotFound is returned when a workspace does not exist.
	ErrWorkspaceNotFound = errors.New("workspace not found")
	// ErrMembershipNotFound is returned when a principal membership does not exist.
	ErrMembershipNotFound   = errors.New("organization membership not found")
	ErrOrganizationInactive = errors.New("organization is not active")
)

func tenantIDFromContext(ctx context.Context) string {
	if orgID, ok := common.GetOrganizationIDFromContext(ctx); ok && orgID != "" {
		return orgID
	}
	return "default"
}

// OrganizationStore encapsulates database operations for organizations, workspaces, and memberships.
type OrganizationStore struct {
	db *sql.DB
}

// NewOrganizationStore creates a new OrganizationStore.
func NewOrganizationStore(db *sql.DB) *OrganizationStore {
	return &OrganizationStore{db: db}
}

// GetMembership exposes the tenant membership lookup through Store so auth
// interceptors can enforce an explicitly selected organization on the real
// production store. The dedicated OrganizationStore remains available for
// organization-specific operations.
func (s *Store) GetMembership(ctx context.Context, organizationID string, principalID int) (*a2a888.OrganizationMembership, error) {
	if s == nil || s.dbConnManager == nil {
		return nil, ErrMembershipNotFound
	}
	return NewOrganizationStore(s.GetDB()).GetMembership(ctx, organizationID, principalID)
}

// RequireOrganizationActive is the shared write guard for connectors, A2A,
// runtime, and other durable paths that do not pass through the IAM interceptor.
func (s *Store) RequireOrganizationActive(ctx context.Context, organizationID string) error {
	if organizationID == "" {
		return ErrOrganizationNotFound
	}
	var state string
	err := s.GetDB().QueryRowContext(ctx, `SELECT state FROM organizations WHERE id = $1`, organizationID).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrOrganizationNotFound
	}
	if err != nil {
		return errors.Wrap(err, "check organization state")
	}
	if !organizationStateAllowsWrites(state) {
		return ErrOrganizationInactive
	}
	return nil
}

// SetDefaultOrganizationForPrincipal persists a human's active organization
// and invalidates the unscoped user cache so the next authenticated request
// observes the new tenant selection.
func (s *Store) SetDefaultOrganizationForPrincipal(ctx context.Context, principalID int, organizationID string) error {
	if err := NewOrganizationStore(s.GetDB()).SetDefaultOrganizationForPrincipal(ctx, principalID, organizationID); err != nil {
		return err
	}
	if user, err := s.GetUserByID(ctx, principalID); err == nil && user != nil {
		s.invalidateUserCache(user.ID, user.Email)
	}
	return nil
}

func organizationStateAllowsWrites(state string) bool {
	return state == "ACTIVE"
}

// CreateOrganization inserts a new organization.
func (s *OrganizationStore) CreateOrganization(ctx context.Context, org *a2a888.Organization) (*a2a888.Organization, error) {
	if org == nil || org.Id == "" || org.Slug == "" {
		return nil, errors.New("organization id and slug are required")
	}

	stateStr := organizationStateDB(org.State)
	if org.State == a2a888.OrganizationState_ORGANIZATION_STATE_UNSPECIFIED {
		stateStr = "ACTIVE"
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
	if state == a2a888.OrganizationState_ORGANIZATION_STATE_UNSPECIFIED {
		return errors.New("organization state is required")
	}
	res, err := s.db.ExecContext(ctx, query, organizationStateDB(state), id)
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
	if err := s.requireOrganizationActive(ctx, ws.OrganizationId); err != nil {
		return nil, err
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

	principalID, err := strconv.Atoi(m.PrincipalId)
	if err != nil || principalID <= 0 {
		return nil, errors.New("principal id must be a positive integer")
	}
	if err := s.requireOrganizationActive(ctx, m.OrganizationId); err != nil {
		return nil, err
	}
	workspaceIDs := m.WorkspaceIds
	if workspaceIDs == nil {
		workspaceIDs = []string{}
	}
	if err := s.validateWorkspaceIDs(ctx, m.OrganizationId, workspaceIDs); err != nil {
		return nil, err
	}
	roleStr := organizationRoleDB(m.Role)
	stateStr := membershipStateDB(m.State)

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

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to begin membership transaction")
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(ctx, query, m.OrganizationId, principalID, roleStr, stateStr, pq.Array(workspaceIDs), now, now).Scan(
		&resOrgID, &resPrincipalID, &resRole, &resState, pq.Array(&resWorkspaceIDs), &resCreatedAt, &resUpdatedAt,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to insert organization membership")
	}
	if err := syncMembershipWorkspaces(ctx, tx, m.OrganizationId, principalID, workspaceIDs); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, errors.Wrap(err, "failed to commit membership transaction")
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

	principalID, err := strconv.Atoi(m.PrincipalId)
	if err != nil || principalID <= 0 {
		return errors.New("principal id must be a positive integer")
	}
	if err := s.requireOrganizationActive(ctx, m.OrganizationId); err != nil {
		return err
	}
	workspaceIDs := m.WorkspaceIds
	if workspaceIDs == nil {
		workspaceIDs = []string{}
	}
	if err := s.validateWorkspaceIDs(ctx, m.OrganizationId, workspaceIDs); err != nil {
		return err
	}
	query := `
		UPDATE organization_memberships
		SET role = $1, state = $2, workspace_ids = $3, updated_at = now()
		WHERE organization_id = $4 AND principal_id = $5
	`
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, "failed to begin membership transaction")
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, query, organizationRoleDB(m.Role), membershipStateDB(m.State), pq.Array(workspaceIDs), m.OrganizationId, principalID)
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
	if err := syncMembershipWorkspaces(ctx, tx, m.OrganizationId, principalID, workspaceIDs); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit membership transaction")
	}
	return nil
}

// RemoveMembership removes one tenant membership and its workspace grants.
func (s *OrganizationStore) RemoveMembership(ctx context.Context, organizationID string, principalID int) error {
	if organizationID == "" || principalID <= 0 {
		return errors.New("organization id and principal id are required")
	}
	if err := s.requireOrganizationActive(ctx, organizationID); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM organization_memberships WHERE organization_id = $1 AND principal_id = $2`, organizationID, principalID)
	if err != nil {
		return errors.Wrap(err, "remove organization membership")
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "check organization membership removal")
	}
	if rows == 0 {
		return ErrMembershipNotFound
	}
	return nil
}

func (s *OrganizationStore) requireOrganizationActive(ctx context.Context, organizationID string) error {
	if organizationID == "" {
		return ErrOrganizationNotFound
	}
	var state string
	err := s.db.QueryRowContext(ctx, `SELECT state FROM organizations WHERE id = $1`, organizationID).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrOrganizationNotFound
	}
	if err != nil {
		return errors.Wrap(err, "check organization state")
	}
	if !organizationStateAllowsWrites(state) {
		return ErrOrganizationInactive
	}
	return nil
}

func (s *OrganizationStore) validateWorkspaceIDs(ctx context.Context, organizationID string, workspaceIDs []string) error {
	if len(workspaceIDs) == 0 {
		return nil
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM workspaces WHERE organization_id = $1 AND id = ANY($2)`, organizationID, pq.Array(workspaceIDs)).Scan(&count); err != nil {
		return errors.Wrap(err, "validate organization workspaces")
	}
	if count != len(workspaceIDs) {
		return errors.New("membership workspace must belong to the organization")
	}
	return nil
}

func syncMembershipWorkspaces(ctx context.Context, tx *sql.Tx, organizationID string, principalID int, workspaceIDs []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM organization_membership_workspaces WHERE organization_id = $1 AND principal_id = $2`, organizationID, principalID); err != nil {
		return errors.Wrap(err, "clear membership workspaces")
	}
	for _, workspaceID := range workspaceIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO organization_membership_workspaces (organization_id, principal_id, workspace_id) VALUES ($1, $2, $3)`, organizationID, principalID, workspaceID); err != nil {
			return errors.Wrap(err, "add membership workspace")
		}
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

// ListGroupBindings returns organization-level role grants for groups. The
// organization predicate is mandatory even though group and workspace IDs are
// globally shaped, preventing guessed IDs from crossing tenant boundaries.
func (s *OrganizationStore) ListGroupBindings(ctx context.Context, orgID string) ([]*a2a888.OrganizationGroupBinding, error) {
	if orgID == "" {
		return nil, errors.New("organization id is required")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT organization_id, group_id, COALESCE(workspace_id, ''), role, created_at, updated_at
		FROM organization_group_bindings
		WHERE organization_id = $1
		ORDER BY created_at ASC, id ASC
	`, orgID)
	if err != nil {
		return nil, errors.Wrap(err, "list organization group bindings")
	}
	defer rows.Close()

	var bindings []*a2a888.OrganizationGroupBinding
	for rows.Next() {
		var binding a2a888.OrganizationGroupBinding
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&binding.OrganizationId, &binding.GroupId, &binding.WorkspaceId, &binding.Role, &createdAt, &updatedAt); err != nil {
			return nil, errors.Wrap(err, "scan organization group binding")
		}
		binding.CreatedAt = timestamppb.New(createdAt)
		binding.UpdatedAt = timestamppb.New(updatedAt)
		bindings = append(bindings, &binding)
	}
	return bindings, rows.Err()
}

// SetGroupBinding creates or replaces a tenant-scoped group role grant.
func (s *OrganizationStore) SetGroupBinding(ctx context.Context, binding *a2a888.OrganizationGroupBinding) (*a2a888.OrganizationGroupBinding, error) {
	if binding == nil || binding.OrganizationId == "" || binding.GroupId == "" || binding.Role == "" {
		return nil, errors.New("organization id, group id, and role are required")
	}
	if err := s.requireOrganizationActive(ctx, binding.OrganizationId); err != nil {
		return nil, err
	}
	var workspace any
	if binding.WorkspaceId != "" {
		workspace = binding.WorkspaceId
	}
	var result a2a888.OrganizationGroupBinding
	var createdAt, updatedAt time.Time
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO organization_group_bindings (organization_id, group_id, workspace_id, role)
		SELECT $1, $2, $3, $4
		WHERE EXISTS (SELECT 1 FROM user_group WHERE id = $2 AND organization_id = $1)
		  AND ($3::text IS NULL OR EXISTS (SELECT 1 FROM workspaces WHERE id = $3 AND organization_id = $1))
		ON CONFLICT (organization_id, group_id, COALESCE(workspace_id, ''), role)
		DO UPDATE SET updated_at = now()
		RETURNING organization_id, group_id, COALESCE(workspace_id, ''), role, created_at, updated_at
	`, binding.OrganizationId, binding.GroupId, workspace, binding.Role).Scan(
		&result.OrganizationId, &result.GroupId, &result.WorkspaceId, &result.Role, &createdAt, &updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("group or workspace does not belong to organization")
	}
	if err != nil {
		return nil, errors.Wrap(err, "set organization group binding")
	}
	result.CreatedAt = timestamppb.New(createdAt)
	result.UpdatedAt = timestamppb.New(updatedAt)
	return &result, nil
}

// RemoveGroupBinding removes exactly one organization-scoped group role grant.
func (s *OrganizationStore) RemoveGroupBinding(ctx context.Context, organizationID, groupID, workspaceID, role string) error {
	if organizationID == "" || groupID == "" || role == "" {
		return errors.New("organization id, group id, and role are required")
	}
	var workspace any
	if workspaceID != "" {
		workspace = workspaceID
	}
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM organization_group_bindings
		WHERE organization_id = $1 AND group_id = $2
		  AND COALESCE(workspace_id, '') = COALESCE($3::text, '') AND role = $4
	`, organizationID, groupID, workspace, role)
	if err != nil {
		return errors.Wrap(err, "remove organization group binding")
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "check organization group binding removal")
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
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

func organizationStateDB(state a2a888.OrganizationState) string {
	switch state {
	case a2a888.OrganizationState_ORGANIZATION_STATE_SUSPENDED:
		return "SUSPENDED"
	case a2a888.OrganizationState_ORGANIZATION_STATE_CLOSED:
		return "CLOSED"
	default:
		return "ACTIVE"
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
	case "BILLING_ADMIN", "ORGANIZATION_ROLE_BILLING_ADMIN":
		return a2a888.OrganizationRole_ORGANIZATION_ROLE_BILLING_ADMIN
	case "AGENT_ADMIN", "ORGANIZATION_ROLE_AGENT_ADMIN":
		return a2a888.OrganizationRole_ORGANIZATION_ROLE_AGENT_ADMIN
	case "APPROVER", "ORGANIZATION_ROLE_APPROVER":
		return a2a888.OrganizationRole_ORGANIZATION_ROLE_APPROVER
	default:
		return a2a888.OrganizationRole_ORGANIZATION_ROLE_MEMBER
	}
}

func organizationRoleDB(role a2a888.OrganizationRole) string {
	switch role {
	case a2a888.OrganizationRole_ORGANIZATION_ROLE_OWNER:
		return "OWNER"
	case a2a888.OrganizationRole_ORGANIZATION_ROLE_ADMIN:
		return "ADMIN"
	case a2a888.OrganizationRole_ORGANIZATION_ROLE_BILLING_ADMIN:
		return "BILLING_ADMIN"
	case a2a888.OrganizationRole_ORGANIZATION_ROLE_AGENT_ADMIN:
		return "AGENT_ADMIN"
	case a2a888.OrganizationRole_ORGANIZATION_ROLE_APPROVER:
		return "APPROVER"
	case a2a888.OrganizationRole_ORGANIZATION_ROLE_GUEST:
		return "GUEST"
	default:
		return "MEMBER"
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

func membershipStateDB(state a2a888.MembershipState) string {
	switch state {
	case a2a888.MembershipState_MEMBERSHIP_STATE_SUSPENDED:
		return "SUSPENDED"
	case a2a888.MembershipState_MEMBERSHIP_STATE_INVITED:
		return "INVITED"
	default:
		return "ACTIVE"
	}
}

// TenantCacheKey scopes a cache key by organization ID to prevent cross-tenant cache collisions.
func TenantCacheKey(orgID string, resourceType string, key string) string {
	if orgID == "" {
		orgID = "default"
	}
	return fmt.Sprintf("org:%s:%s:%s", sanitizeTenantKey(orgID), resourceType, key)
}

// TenantProjectionKey scopes a local projection key by organization ID.
func TenantProjectionKey(orgID string, projectionName string, id string) string {
	if orgID == "" {
		orgID = "default"
	}
	return fmt.Sprintf("org:%s:proj:%s:%s", sanitizeTenantKey(orgID), projectionName, id)
}

func sanitizeTenantKey(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "default"
	}
	var b strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 || b.String() == "." || b.String() == ".." {
		return "default"
	}
	return b.String()
}
