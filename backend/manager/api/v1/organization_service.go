package v1

import (
	"context"
	"database/sql"
	"strconv"

	"connectrpc.com/connect"
	"github.com/pkg/errors"

	"github.com/tbdavid2019/888a2a/backend/common"
	a2a888 "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
	"github.com/tbdavid2019/888a2a/backend/generated-go/a2a888/a2a888connect"
	"github.com/tbdavid2019/888a2a/backend/manager/component/iam"
	"github.com/tbdavid2019/888a2a/backend/manager/store"
)

// OrganizationService implements a2a888connect.OrganizationServiceHandler.
type OrganizationService struct {
	a2a888connect.UnimplementedOrganizationServiceHandler
	store *store.Store
	iam   *iam.Manager
}

// NewOrganizationService creates a new OrganizationService.
func NewOrganizationService(s *store.Store, iamMgr *iam.Manager) *OrganizationService {
	return &OrganizationService{
		store: s,
		iam:   iamMgr,
	}
}

func (s *OrganizationService) ListOrganizations(ctx context.Context, _ *connect.Request[a2a888.ListOrganizationsRequest]) (*connect.Response[a2a888.ListOrganizationsResponse], error) {
	user, hasUser := GetUserFromContext(ctx)
	if !hasUser {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}

	orgStore := store.NewOrganizationStore(s.store.GetDB())
	orgs, err := orgStore.ListOrganizationsForPrincipal(ctx, user.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Wrap(err, "failed to list organizations"))
	}

	activeOrgID, _ := common.GetOrganizationIDFromContext(ctx)
	activeOrgID = activeOrganizationID(orgs, activeOrgID, user.DefaultOrganizationID)

	return connect.NewResponse(&a2a888.ListOrganizationsResponse{
		Organizations:        orgs,
		ActiveOrganizationId: activeOrgID,
	}), nil
}

func (s *OrganizationService) GetOrganization(ctx context.Context, req *connect.Request[a2a888.GetOrganizationRequest]) (*connect.Response[a2a888.GetOrganizationResponse], error) {
	user, hasUser := GetUserFromContext(ctx)
	if !hasUser {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}

	orgID := req.Msg.OrganizationId
	if orgID == "" {
		var ok bool
		orgID, ok = common.GetOrganizationIDFromContext(ctx)
		if !ok || orgID == "" {
			orgID = user.DefaultOrganizationID
			if orgID == "" {
				orgID = "default"
			}
		}
	}

	orgStore := store.NewOrganizationStore(s.store.GetDB())
	membership, err := orgStore.GetMembership(ctx, orgID, user.ID)
	if err != nil || membership.State != a2a888.MembershipState_MEMBERSHIP_STATE_ACTIVE {
		// Spec 2.8 Indistinguishable Denial: prevent enumeration of tenant IDs by unauthenticated/non-member callers
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("access to organization denied"))
	}

	org, err := orgStore.GetOrganization(ctx, orgID)
	if err != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("access to organization denied"))
	}

	return connect.NewResponse(&a2a888.GetOrganizationResponse{
		Organization:      org,
		CurrentMembership: membership,
	}), nil
}

func (s *OrganizationService) SwitchOrganization(ctx context.Context, req *connect.Request[a2a888.SwitchOrganizationRequest]) (*connect.Response[a2a888.SwitchOrganizationResponse], error) {
	user, hasUser := GetUserFromContext(ctx)
	if !hasUser {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}

	targetOrgID := req.Msg.OrganizationId
	if targetOrgID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("organization_id is required"))
	}

	orgStore := store.NewOrganizationStore(s.store.GetDB())
	membership, err := orgStore.GetMembership(ctx, targetOrgID, user.ID)
	if err != nil || membership.State != a2a888.MembershipState_MEMBERSHIP_STATE_ACTIVE {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("target organization membership is not active"))
	}

	org, err := orgStore.GetOrganization(ctx, targetOrgID)
	if err != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("access to target organization denied"))
	}

	if org.State == a2a888.OrganizationState_ORGANIZATION_STATE_CLOSED {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("organization is closed"))
	}

	if err := orgStore.SetDefaultOrganizationForPrincipal(ctx, user.ID, targetOrgID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Wrap(err, "failed to set active organization"))
	}

	return connect.NewResponse(&a2a888.SwitchOrganizationResponse{
		Organization: org,
		Membership:   membership,
	}), nil
}

func (s *OrganizationService) ListWorkspaces(ctx context.Context, req *connect.Request[a2a888.ListWorkspacesRequest]) (*connect.Response[a2a888.ListWorkspacesResponse], error) {
	user, hasUser := GetUserFromContext(ctx)
	if !hasUser {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}

	orgID := req.Msg.OrganizationId
	if orgID == "" {
		var ok bool
		orgID, ok = common.GetOrganizationIDFromContext(ctx)
		if !ok || orgID == "" {
			orgID = user.DefaultOrganizationID
			if orgID == "" {
				orgID = "default"
			}
		}
	}

	orgStore := store.NewOrganizationStore(s.store.GetDB())
	membership, err := orgStore.GetMembership(ctx, orgID, user.ID)
	if err != nil || membership.State != a2a888.MembershipState_MEMBERSHIP_STATE_ACTIVE {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("access to organization workspaces denied"))
	}

	workspaces, err := orgStore.ListWorkspaces(ctx, orgID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Wrap(err, "failed to list workspaces"))
	}

	return connect.NewResponse(&a2a888.ListWorkspacesResponse{
		Workspaces: workspaces,
	}), nil
}

func (s *OrganizationService) ListMemberships(ctx context.Context, req *connect.Request[a2a888.ListMembershipsRequest]) (*connect.Response[a2a888.ListMembershipsResponse], error) {
	user, hasUser := GetUserFromContext(ctx)
	if !hasUser {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}

	orgID := req.Msg.OrganizationId
	if orgID == "" {
		var ok bool
		orgID, ok = common.GetOrganizationIDFromContext(ctx)
		if !ok || orgID == "" {
			orgID = user.DefaultOrganizationID
			if orgID == "" {
				orgID = "default"
			}
		}
	}

	orgStore := store.NewOrganizationStore(s.store.GetDB())
	membership, err := orgStore.GetMembership(ctx, orgID, user.ID)
	if err != nil || membership.State != a2a888.MembershipState_MEMBERSHIP_STATE_ACTIVE {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("access to organization memberships denied"))
	}
	if !canManageOrganization(membership.Role) {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("organization membership administration denied"))
	}

	memberships, err := orgStore.ListMemberships(ctx, orgID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Wrap(err, "failed to list memberships"))
	}

	return connect.NewResponse(&a2a888.ListMembershipsResponse{
		Memberships: memberships,
	}), nil
}

func (s *OrganizationService) AddMembership(ctx context.Context, req *connect.Request[a2a888.AddMembershipRequest]) (*connect.Response[a2a888.OrganizationMembership], error) {
	_, err := requireOrganizationAdmin(ctx, s.store, req.Msg.GetMembership().GetOrganizationId())
	if err != nil {
		return nil, err
	}
	membership := req.Msg.GetMembership()
	if membership == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("membership is required"))
	}
	orgStore := store.NewOrganizationStore(s.store.GetDB())
	created, err := orgStore.AddMembership(ctx, membership)
	if err != nil {
		return nil, organizationStoreError(err)
	}
	return connect.NewResponse(created), nil
}

func (s *OrganizationService) UpdateMembership(ctx context.Context, req *connect.Request[a2a888.UpdateMembershipRequest]) (*connect.Response[a2a888.OrganizationMembership], error) {
	membership := req.Msg.GetMembership()
	if membership == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("membership is required"))
	}
	if _, err := requireOrganizationAdmin(ctx, s.store, membership.OrganizationId); err != nil {
		return nil, err
	}
	principalID, parseErr := strconv.Atoi(membership.PrincipalId)
	if parseErr != nil || principalID <= 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("principal id must be a positive integer"))
	}
	orgStore := store.NewOrganizationStore(s.store.GetDB())
	current, err := orgStore.GetMembership(ctx, membership.OrganizationId, principalID)
	if err != nil {
		return nil, organizationStoreError(err)
	}
	nextRole := membership.Role
	if nextRole == a2a888.OrganizationRole_ORGANIZATION_ROLE_UNSPECIFIED {
		nextRole = a2a888.OrganizationRole_ORGANIZATION_ROLE_MEMBER
	}
	nextState := membership.State
	if nextState == a2a888.MembershipState_MEMBERSHIP_STATE_UNSPECIFIED {
		nextState = a2a888.MembershipState_MEMBERSHIP_STATE_ACTIVE
	}
	if current.Role == a2a888.OrganizationRole_ORGANIZATION_ROLE_OWNER && (nextRole != a2a888.OrganizationRole_ORGANIZATION_ROLE_OWNER || nextState != a2a888.MembershipState_MEMBERSHIP_STATE_ACTIVE) {
		if err := ensureAnotherOwner(ctx, orgStore, membership.OrganizationId, principalID); err != nil {
			return nil, err
		}
	}
	if err := orgStore.UpdateMembership(ctx, membership); err != nil {
		return nil, organizationStoreError(err)
	}
	updated, err := orgStore.GetMembership(ctx, membership.OrganizationId, principalID)
	if err != nil {
		return nil, organizationStoreError(err)
	}
	return connect.NewResponse(updated), nil
}

func (s *OrganizationService) RemoveMembership(ctx context.Context, req *connect.Request[a2a888.RemoveMembershipRequest]) (*connect.Response[a2a888.RemoveMembershipResponse], error) {
	if req.Msg.OrganizationId == "" || req.Msg.PrincipalId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("organization_id and principal_id are required"))
	}
	if _, err := requireOrganizationAdmin(ctx, s.store, req.Msg.OrganizationId); err != nil {
		return nil, err
	}
	principalID, err := strconv.Atoi(req.Msg.PrincipalId)
	if err != nil || principalID <= 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("principal id must be a positive integer"))
	}
	orgStore := store.NewOrganizationStore(s.store.GetDB())
	membership, err := orgStore.GetMembership(ctx, req.Msg.OrganizationId, principalID)
	if err != nil {
		return nil, organizationStoreError(err)
	}
	if membership.Role == a2a888.OrganizationRole_ORGANIZATION_ROLE_OWNER {
		if err := ensureAnotherOwner(ctx, orgStore, req.Msg.OrganizationId, principalID); err != nil {
			return nil, err
		}
	}
	if err := orgStore.RemoveMembership(ctx, req.Msg.OrganizationId, principalID); err != nil {
		return nil, organizationStoreError(err)
	}
	return connect.NewResponse(&a2a888.RemoveMembershipResponse{}), nil
}

func (s *OrganizationService) ListGroupBindings(ctx context.Context, req *connect.Request[a2a888.ListGroupBindingsRequest]) (*connect.Response[a2a888.ListGroupBindingsResponse], error) {
	if _, err := requireOrganizationAdmin(ctx, s.store, req.Msg.OrganizationId); err != nil {
		return nil, err
	}
	bindings, err := store.NewOrganizationStore(s.store.GetDB()).ListGroupBindings(ctx, req.Msg.OrganizationId)
	if err != nil {
		return nil, organizationStoreError(err)
	}
	return connect.NewResponse(&a2a888.ListGroupBindingsResponse{Bindings: bindings}), nil
}

func (s *OrganizationService) SetGroupBinding(ctx context.Context, req *connect.Request[a2a888.SetGroupBindingRequest]) (*connect.Response[a2a888.OrganizationGroupBinding], error) {
	binding := req.Msg.GetBinding()
	if binding == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("group binding is required"))
	}
	if _, err := requireOrganizationAdmin(ctx, s.store, binding.OrganizationId); err != nil {
		return nil, err
	}
	result, err := store.NewOrganizationStore(s.store.GetDB()).SetGroupBinding(ctx, binding)
	if err != nil {
		return nil, organizationStoreError(err)
	}
	return connect.NewResponse(result), nil
}

func (s *OrganizationService) RemoveGroupBinding(ctx context.Context, req *connect.Request[a2a888.RemoveGroupBindingRequest]) (*connect.Response[a2a888.RemoveGroupBindingResponse], error) {
	if _, err := requireOrganizationAdmin(ctx, s.store, req.Msg.OrganizationId); err != nil {
		return nil, err
	}
	if err := store.NewOrganizationStore(s.store.GetDB()).RemoveGroupBinding(ctx, req.Msg.OrganizationId, req.Msg.GroupId, req.Msg.WorkspaceId, req.Msg.Role); err != nil {
		return nil, organizationStoreError(err)
	}
	return connect.NewResponse(&a2a888.RemoveGroupBindingResponse{}), nil
}

func requireOrganizationAdmin(ctx context.Context, stores *store.Store, organizationID string) (*a2a888.OrganizationMembership, error) {
	if organizationID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("organization_id is required"))
	}
	user, ok := GetUserFromContext(ctx)
	if !ok || user == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	membership, err := store.NewOrganizationStore(stores.GetDB()).GetMembership(ctx, organizationID, user.ID)
	if err != nil || membership.State != a2a888.MembershipState_MEMBERSHIP_STATE_ACTIVE || !canManageOrganization(membership.Role) {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("organization administration denied"))
	}
	return membership, nil
}

func ensureAnotherOwner(ctx context.Context, orgStore *store.OrganizationStore, organizationID string, principalID int) error {
	memberships, err := orgStore.ListMemberships(ctx, organizationID)
	if err != nil {
		return organizationStoreError(err)
	}
	for _, membership := range memberships {
		if membership.PrincipalId != strconv.Itoa(principalID) && membership.Role == a2a888.OrganizationRole_ORGANIZATION_ROLE_OWNER && membership.State == a2a888.MembershipState_MEMBERSHIP_STATE_ACTIVE {
			return nil
		}
	}
	return connect.NewError(connect.CodeFailedPrecondition, errors.New("organization must retain an active owner"))
}

func organizationStoreError(err error) error {
	switch {
	case errors.Is(err, store.ErrOrganizationInactive):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	case errors.Is(err, store.ErrOrganizationNotFound), errors.Is(err, store.ErrMembershipNotFound), errors.Is(err, sql.ErrNoRows):
		return connect.NewError(connect.CodeNotFound, errors.New("organization resource not found"))
	default:
		return connect.NewError(connect.CodeInternal, err)
	}
}

func canManageOrganization(role a2a888.OrganizationRole) bool {
	return role == a2a888.OrganizationRole_ORGANIZATION_ROLE_OWNER ||
		role == a2a888.OrganizationRole_ORGANIZATION_ROLE_ADMIN
}

func activeOrganizationID(orgs []*a2a888.Organization, candidate, fallback string) string {
	for _, org := range orgs {
		if org != nil && org.Id == candidate {
			return candidate
		}
	}
	for _, org := range orgs {
		if org != nil && org.Id == fallback {
			return fallback
		}
	}
	for _, org := range orgs {
		if org != nil {
			return org.Id
		}
	}
	return "default"
}
