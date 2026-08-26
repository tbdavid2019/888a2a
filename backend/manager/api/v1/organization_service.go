package v1

import (
	"context"

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
