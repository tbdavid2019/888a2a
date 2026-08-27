package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	a2a888 "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
)

func TestConnectorDeliveryRequiresInstallationPartition(t *testing.T) {
	var services *Store
	if err := services.EnqueueConnectorDelivery(context.Background(), "", "install", "conversation", "event", map[string]string{"text": "x"}, time.Time{}); err == nil {
		t.Fatal("connector delivery without tenant was accepted")
	}
}

func TestConnectorDeliveryOutboxAndDivergenceAreTenantScoped(t *testing.T) {
	services, _ := requireCommandEventIntegrationStore(t)
	ctx := context.Background()
	require.NoError(t, services.EnqueueConnectorDelivery(ctx, "default", "install-a", "conversation-a", "event-a", map[string]string{"text": "hello"}, time.Now().UTC()))
	claimed, err := services.ClaimOutboxEvents(ctx, "connector-worker", 10)
	require.NoError(t, err)
	var found bool
	for _, event := range claimed {
		if event.AggregateID == "install-a" && event.EventType == "CONNECTOR_DELIVERY" {
			found = true
			require.NoError(t, services.AckOutboxEvent(ctx, "connector-worker", event.EventID))
		}
	}
	require.True(t, found)
	require.NoError(t, services.RecordConnectorDivergence(ctx, "default", "install-a", "external:1", "internal:1", "event-a", "destination capability not supported"))
	var count int
	require.NoError(t, services.GetDB().QueryRowContext(ctx, `SELECT count(*) FROM a2a888_connector_divergence WHERE organization_id='default' AND installation_id='install-a'`).Scan(&count))
	require.Equal(t, 1, count)
}

func TestConnectorInstallationStatusProjectsDeliveryCounters(t *testing.T) {
	services, _ := requireCommandEventIntegrationStore(t)
	ctx := context.Background()
	require.NoError(t, services.UpsertConnectorInstallation(ctx, &a2a888.ConnectorInstallation{
		OrganizationId: "default", InstallationId: "line-status", Kind: "line", Enabled: true,
		Capabilities: []string{"text", "replies"}, Health: a2a888.ConnectorHealth_CONNECTOR_HEALTH_HEALTHY,
	}))
	require.NoError(t, services.EnqueueConnectorDelivery(ctx, "default", "line-status", "conversation-status", "status-event", map[string]string{"text": "queued"}, time.Now().UTC()))
	installations, err := services.ListConnectorInstallations(ctx, "default")
	require.NoError(t, err)
	require.Len(t, installations, 1)
	require.EqualValues(t, 1, installations[0].PendingDeliveries)
	require.Equal(t, []string{"text", "replies"}, installations[0].Capabilities)
}
