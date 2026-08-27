package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConnectorIdentityLinkResolveAndUnlink(t *testing.T) {
	services, _ := requireCommandEventIntegrationStore(t)
	ctx := context.Background()
	mapping := ExternalIdentityMapping{
		OrganizationID: "default", InstallationID: "connector-a", IdentityType: "user",
		ExternalIdentityID: "external-user-1", PrincipalID: "principal-1",
	}
	require.NoError(t, services.LinkExternalIdentity(ctx, mapping))
	resolved, err := services.ResolveExternalIdentity(ctx, mapping.OrganizationID, mapping.InstallationID, mapping.IdentityType, mapping.ExternalIdentityID)
	require.NoError(t, err)
	require.Equal(t, mapping.PrincipalID, resolved.PrincipalID)
	require.NoError(t, services.UnlinkExternalIdentity(ctx, mapping.OrganizationID, mapping.InstallationID, mapping.IdentityType, mapping.ExternalIdentityID))
	_, err = services.ResolveExternalIdentity(ctx, mapping.OrganizationID, mapping.InstallationID, mapping.IdentityType, mapping.ExternalIdentityID)
	require.Error(t, err)
}
