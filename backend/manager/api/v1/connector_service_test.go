package v1

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	a2a888 "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
)

func TestConnectorInstallValidatesBoundedCredentialBeforePersistence(t *testing.T) {
	service := NewConnectorService(nil, nil)
	_, err := service.InstallConnector(context.Background(), connect.NewRequest(&a2a888.InstallConnectorRequest{
		Installation: &a2a888.ConnectorInstallation{OrganizationId: "org-a", InstallationId: "install-a", Kind: "line"},
	}))
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	require.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}

func TestConnectorUninstallValidatesTenantIdentity(t *testing.T) {
	service := NewConnectorService(nil, nil)
	_, err := service.UninstallConnector(context.Background(), connect.NewRequest(&a2a888.UninstallConnectorRequest{OrganizationId: "org-a"}))
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	require.Equal(t, connect.CodeInvalidArgument, connectErr.Code())
}
