package v1

import (
	"context"

	"connectrpc.com/connect"
	"github.com/pkg/errors"

	"github.com/tbdavid2019/888a2a/backend/common"
	a2a888 "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
	"github.com/tbdavid2019/888a2a/backend/generated-go/a2a888/a2a888connect"
	"github.com/tbdavid2019/888a2a/backend/manager/component/connectorvault"
	"github.com/tbdavid2019/888a2a/backend/manager/store"
)

// ConnectorService exposes tenant-scoped installation health to organization
// administrators without returning credentials or raw webhook payloads.
type ConnectorService struct {
	a2a888connect.UnimplementedConnectorServiceHandler
	store *store.Store
	vault *connectorvault.Vault
}

func NewConnectorService(s *store.Store, vault *connectorvault.Vault) *ConnectorService {
	return &ConnectorService{store: s, vault: vault}
}

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

func (s *ConnectorService) InstallConnector(ctx context.Context, req *connect.Request[a2a888.InstallConnectorRequest]) (*connect.Response[a2a888.ConnectorInstallation], error) {
	installation := req.Msg.GetInstallation()
	if installation == nil || installation.OrganizationId == "" || installation.InstallationId == "" || installation.Kind == "" || len(req.Msg.Credential) == 0 || len(req.Msg.Credential) > 16<<10 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("connector installation and bounded credential are required"))
	}
	if _, err := requireOrganizationAdmin(ctx, s.store, installation.OrganizationId); err != nil {
		return nil, err
	}
	if s.vault == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("connector credential vault is not configured"))
	}
	if installation.Health == a2a888.ConnectorHealth_CONNECTOR_HEALTH_UNSPECIFIED {
		installation.Health = a2a888.ConnectorHealth_CONNECTOR_HEALTH_HEALTHY
	}
	if err := s.vault.Put(ctx, installation.OrganizationId, installation.InstallationId, req.Msg.Credential); err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Wrap(err, "store connector credential"))
	}
	if err := s.store.UpsertConnectorInstallation(ctx, installation); err != nil {
		_ = s.vault.Revoke(ctx, installation.OrganizationId, installation.InstallationId)
		return nil, connect.NewError(connect.CodeInternal, errors.Wrap(err, "store connector installation"))
	}
	return connect.NewResponse(installation), nil
}

func (s *ConnectorService) UninstallConnector(ctx context.Context, req *connect.Request[a2a888.UninstallConnectorRequest]) (*connect.Response[a2a888.UninstallConnectorResponse], error) {
	if req.Msg.OrganizationId == "" || req.Msg.InstallationId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("organization_id and installation_id are required"))
	}
	if _, err := requireOrganizationAdmin(ctx, s.store, req.Msg.OrganizationId); err != nil {
		return nil, err
	}
	if s.vault != nil {
		if err := s.vault.Revoke(ctx, req.Msg.OrganizationId, req.Msg.InstallationId); err != nil {
			return nil, connect.NewError(connect.CodeInternal, errors.Wrap(err, "revoke connector credential"))
		}
	}
	if err := s.store.DeleteConnectorInstallation(ctx, req.Msg.OrganizationId, req.Msg.InstallationId); err != nil {
		return nil, connect.NewError(connect.CodeInternal, errors.Wrap(err, "delete connector installation"))
	}
	return connect.NewResponse(&a2a888.UninstallConnectorResponse{}), nil
}
