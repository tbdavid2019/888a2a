package v1

import (
	"context"

	"connectrpc.com/connect"
	"github.com/pkg/errors"

	"github.com/tbdavid2019/888a2a/backend/common"
	a2a888 "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
	"github.com/tbdavid2019/888a2a/backend/generated-go/a2a888/a2a888connect"
	"github.com/tbdavid2019/888a2a/backend/manager/store"
)

// ConnectorService exposes tenant-scoped installation health to organization
// administrators without returning credentials or raw webhook payloads.
type ConnectorService struct {
	a2a888connect.UnimplementedConnectorServiceHandler
	store *store.Store
}

func NewConnectorService(s *store.Store) *ConnectorService { return &ConnectorService{store: s} }

func (s *ConnectorService) ListConnectorInstallations(ctx context.Context, req *connect.Request[a2a888.ListConnectorInstallationsRequest]) (*connect.Response[a2a888.ListConnectorInstallationsResponse], error) {
	user, ok := GetUserFromContext(ctx)
	if !ok || user == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	organizationID := req.Msg.OrganizationId
	if organizationID == "" {
		organizationID, ok = common.GetOrganizationIDFromContext(ctx)
		if !ok || organizationID == "" {
			organizationID = user.DefaultOrganizationID
		}
	}
	membership, err := s.store.GetMembership(ctx, organizationID, user.ID)
	if err != nil || membership.State != a2a888.MembershipState_MEMBERSHIP_STATE_ACTIVE || !canManageOrganization(membership.Role) {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("connector administration denied"))
	}
	installations, err := s.store.ListConnectorInstallations(ctx, organizationID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Wrap(err, "list connector installations"))
	}
	return connect.NewResponse(&a2a888.ListConnectorInstallationsResponse{Installations: installations}), nil
}
