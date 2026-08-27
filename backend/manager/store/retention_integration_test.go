package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRetentionRedactsRawConnectorPayloadAndHonorsLegalHold(t *testing.T) {
	services, _ := requireCommandEventIntegrationStore(t)
	ctx := context.Background()
	old := time.Now().UTC().Add(-48 * time.Hour)
	for _, eventID := range []string{"retention-redact", "retention-hold"} {
		_, err := services.GetDB().ExecContext(ctx, `INSERT INTO a2a888_connector_inbox (organization_id,installation_id,external_event_id,external_event_type,raw_payload,received_at) VALUES ('default','retention-install',$1,'message','{"secret":"payload"}'::jsonb,$2)`, eventID, old)
		require.NoError(t, err)
	}
	require.NoError(t, services.AddRetentionHold(ctx, "default", "connector_event", "retention-install:retention-hold", "legal investigation"))
	count, err := services.RedactExpiredConnectorEvents(ctx, "default", time.Now().UTC().Add(-time.Hour), 10)
	require.NoError(t, err)
	require.Equal(t, 2, count)
	var redacted, held string
	require.NoError(t, services.GetDB().QueryRowContext(ctx, `SELECT raw_payload::text FROM a2a888_connector_inbox WHERE organization_id='default' AND external_event_id='retention-redact'`).Scan(&redacted))
	require.NoError(t, services.GetDB().QueryRowContext(ctx, `SELECT raw_payload::text FROM a2a888_connector_inbox WHERE organization_id='default' AND external_event_id='retention-hold'`).Scan(&held))
	require.Equal(t, "{}", redacted)
	require.Equal(t, `{"secret": "payload"}`, held)
}
