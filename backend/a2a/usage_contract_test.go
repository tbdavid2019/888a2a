package a2a

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	a2a888 "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
)

func TestUsageContractsRoundTripWithoutBillingProviderFields(t *testing.T) {
	event := &a2a888.UsageEvent{
		OrganizationId: "org-a", WorkspaceId: "workspace-a", PrincipalId: "user-a", AgentId: "agent-a",
		Feature: "agent.turn", Quantity: 3, Unit: "count", OccurredAt: timestamppb.Now(),
		SourceReference: "command-a", IdempotencyKey: "source-event-a",
	}
	payload, err := protojson.Marshal(event)
	require.NoError(t, err)
	var decoded a2a888.UsageEvent
	require.NoError(t, protojson.Unmarshal(payload, &decoded))
	require.Equal(t, event.OrganizationId, decoded.OrganizationId)
	require.Equal(t, event.IdempotencyKey, decoded.IdempotencyKey)
	require.EqualValues(t, event.Quantity, decoded.Quantity)
	require.NotContains(t, string(payload), "stripe")
	require.NotContains(t, string(payload), "paddle")
}
