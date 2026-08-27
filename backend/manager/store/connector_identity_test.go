package store

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExternalIdentityMappingRejectsDisplayNameOnlyIdentity(t *testing.T) {
	err := (ExternalIdentityMapping{OrganizationID: "org-a", InstallationID: "install-a", PrincipalID: "user-a"}).validate()
	require.Error(t, err)
}

func TestExternalIdentityMappingRequiresTenantAndInstallation(t *testing.T) {
	err := (ExternalIdentityMapping{OrganizationID: "org-a", InstallationID: "install-a", IdentityType: "user", ExternalIdentityID: "external-a", PrincipalID: "user-a"}).validate()
	require.NoError(t, err)
}
