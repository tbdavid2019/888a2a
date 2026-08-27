package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/pkg/errors"
)

type ExternalIdentityMapping struct {
	OrganizationID     string
	InstallationID     string
	IdentityType       string
	ExternalIdentityID string
	PrincipalID        string
	LinkedAt           time.Time
}

func (s *Store) LinkExternalIdentity(ctx context.Context, mapping ExternalIdentityMapping) error {
	if err := mapping.validate(); err != nil {
		return err
	}
	_, err := s.GetDB().ExecContext(ctx, `
		INSERT INTO a2a888_connector_identity_map (organization_id,installation_id,external_identity_type,external_identity_id,principal_id)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (organization_id,installation_id,external_identity_type,external_identity_id)
		DO UPDATE SET principal_id=EXCLUDED.principal_id, linked_at=now()
	`, mapping.OrganizationID, mapping.InstallationID, mapping.IdentityType, mapping.ExternalIdentityID, mapping.PrincipalID)
	return errors.Wrap(err, "link external identity")
}

func (s *Store) ResolveExternalIdentity(ctx context.Context, organizationID, installationID, identityType, externalIdentityID string) (ExternalIdentityMapping, error) {
	if organizationID == "" || installationID == "" || identityType == "" || externalIdentityID == "" {
		return ExternalIdentityMapping{}, errors.New("external identity lookup is incomplete")
	}
	var mapping ExternalIdentityMapping
	err := s.GetDB().QueryRowContext(ctx, `
		SELECT organization_id,installation_id,external_identity_type,external_identity_id,principal_id,linked_at
		FROM a2a888_connector_identity_map
		WHERE organization_id=$1 AND installation_id=$2 AND external_identity_type=$3 AND external_identity_id=$4
	`, organizationID, installationID, identityType, externalIdentityID).Scan(&mapping.OrganizationID, &mapping.InstallationID, &mapping.IdentityType, &mapping.ExternalIdentityID, &mapping.PrincipalID, &mapping.LinkedAt)
	if err != nil {
		return ExternalIdentityMapping{}, errors.Wrap(err, "resolve external identity")
	}
	return mapping, nil
}

func (s *Store) UnlinkExternalIdentity(ctx context.Context, organizationID, installationID, identityType, externalIdentityID string) error {
	if organizationID == "" || installationID == "" || identityType == "" || externalIdentityID == "" {
		return errors.New("external identity unlink is incomplete")
	}
	result, err := s.GetDB().ExecContext(ctx, `DELETE FROM a2a888_connector_identity_map WHERE organization_id=$1 AND installation_id=$2 AND external_identity_type=$3 AND external_identity_id=$4`, organizationID, installationID, identityType, externalIdentityID)
	if err != nil {
		return errors.Wrap(err, "unlink external identity")
	}
	count, err := result.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "check external identity unlink")
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (m ExternalIdentityMapping) validate() error {
	if m.OrganizationID == "" || m.InstallationID == "" || m.IdentityType == "" || m.ExternalIdentityID == "" || m.PrincipalID == "" {
		return errors.New("external identity mapping must use explicit non-empty identifiers")
	}
	return nil
}
