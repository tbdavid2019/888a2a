package v1

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	a2a888 "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
)

func TestUsageServiceRequiresAuthentication(t *testing.T) {
	service := NewUsageService(nil)
	_, err := service.GetUsageSummary(context.Background(), connect.NewRequest(&a2a888.GetUsageSummaryRequest{}))
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	require.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func TestUsageVisibilityIsLimitedToOwnerAndBillingAdmin(t *testing.T) {
	require.True(t, canViewOrganizationUsage(a2a888.OrganizationRole_ORGANIZATION_ROLE_OWNER))
	require.True(t, canViewOrganizationUsage(a2a888.OrganizationRole_ORGANIZATION_ROLE_BILLING_ADMIN))
	require.False(t, canViewOrganizationUsage(a2a888.OrganizationRole_ORGANIZATION_ROLE_ADMIN))
	require.False(t, canViewOrganizationUsage(a2a888.OrganizationRole_ORGANIZATION_ROLE_MEMBER))
}
