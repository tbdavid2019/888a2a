package store

import (
	"context"
	"database/sql"

	"github.com/lib/pq"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/timestamppb"

	a2a888 "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
)

func (s *Store) UpsertConnectorInstallation(ctx context.Context, installation *a2a888.ConnectorInstallation) error {
	if s == nil || s.GetDB() == nil {
		return errors.New("connector installation database is required")
	}
	if installation == nil || installation.OrganizationId == "" || installation.InstallationId == "" || installation.Kind == "" {
		return errors.New("connector installation organization, installation_id, and kind are required")
	}
	health := connectorHealthDB(installation.Health)
	if health == "" {
		return errors.New("connector installation health is required")
	}
	_, err := s.GetDB().ExecContext(ctx, `
		INSERT INTO a2a888_connector_installation (organization_id,installation_id,kind,enabled,capabilities,health,last_error)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (organization_id,installation_id) DO UPDATE SET kind=EXCLUDED.kind, enabled=EXCLUDED.enabled,
		 capabilities=EXCLUDED.capabilities, health=EXCLUDED.health, last_error=EXCLUDED.last_error, updated_at=now()
	`, installation.OrganizationId, installation.InstallationId, installation.Kind, installation.Enabled, pq.Array(installation.Capabilities), health, installation.LastError)
	return errors.Wrap(err, "upsert connector installation")
}

func (s *Store) ListConnectorInstallations(ctx context.Context, organizationID string) ([]*a2a888.ConnectorInstallation, error) {
	if s == nil || s.GetDB() == nil {
		return nil, errors.New("connector installation database is required")
	}
	if organizationID == "" {
		return nil, errors.New("connector installation organization is required")
	}
	rows, err := s.GetDB().QueryContext(ctx, `
		SELECT i.installation_id, i.kind, i.enabled, i.capabilities, i.health, i.last_error, i.updated_at,
		       (SELECT count(*) FROM a2a888_outbox_event e WHERE e.organization_id=i.organization_id AND e.aggregate_id=i.installation_id AND e.event_type='CONNECTOR_DELIVERY' AND e.status IN ('PENDING','CLAIMED')),
		       (SELECT count(*) FROM a2a888_outbox_event e WHERE e.organization_id=i.organization_id AND e.aggregate_id=i.installation_id AND e.event_type='CONNECTOR_DELIVERY' AND e.status='DEAD_LETTER')
		FROM a2a888_connector_installation i WHERE i.organization_id=$1 ORDER BY i.installation_id
	`, organizationID)
	if err != nil {
		return nil, errors.Wrap(err, "list connector installations")
	}
	defer rows.Close()
	var result []*a2a888.ConnectorInstallation
	for rows.Next() {
		var item a2a888.ConnectorInstallation
		var capabilities []string
		var health string
		var updatedAt sql.NullTime
		if err := rows.Scan(&item.InstallationId, &item.Kind, &item.Enabled, pq.Array(&capabilities), &health, &item.LastError, &updatedAt, &item.PendingDeliveries, &item.DeadLetterDeliveries); err != nil {
			return nil, errors.Wrap(err, "scan connector installation")
		}
		item.OrganizationId = organizationID
		item.Name = "organizations/" + organizationID + "/connectorInstallations/" + item.InstallationId
		item.Capabilities = capabilities
		item.Health = parseConnectorHealth(health)
		if updatedAt.Valid {
			item.UpdatedAt = timestamppb.New(updatedAt.Time)
		}
		result = append(result, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, "iterate connector installations")
	}
	return result, nil
}

// DeleteConnectorInstallation is the idempotent metadata half of uninstall.
// Credential revocation is performed by connectorvault.Revoke first.
func (s *Store) DeleteConnectorInstallation(ctx context.Context, organizationID, installationID string) error {
	if organizationID == "" || installationID == "" {
		return errors.New("connector uninstall organization and installation_id are required")
	}
	_, err := s.GetDB().ExecContext(ctx, `DELETE FROM a2a888_connector_installation WHERE organization_id=$1 AND installation_id=$2`, organizationID, installationID)
	return errors.Wrap(err, "delete connector installation")
}

func connectorHealthDB(value a2a888.ConnectorHealth) string {
	switch value {
	case a2a888.ConnectorHealth_CONNECTOR_HEALTH_HEALTHY:
		return "HEALTHY"
	case a2a888.ConnectorHealth_CONNECTOR_HEALTH_DEGRADED:
		return "DEGRADED"
	case a2a888.ConnectorHealth_CONNECTOR_HEALTH_FAILED:
		return "FAILED"
	case a2a888.ConnectorHealth_CONNECTOR_HEALTH_DISABLED:
		return "DISABLED"
	default:
		return ""
	}
}

func parseConnectorHealth(value string) a2a888.ConnectorHealth {
	return a2a888.ConnectorHealth(a2a888.ConnectorHealth_value["CONNECTOR_HEALTH_"+value])
}
