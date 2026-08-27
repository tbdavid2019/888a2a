package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	a2a888 "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
)

func TestRecordUsageEventRejectsIncompleteIdentity(t *testing.T) {
	var services *Store
	event := &a2a888.UsageEvent{Feature: "agent.turn", Unit: "count", IdempotencyKey: "event-1", OccurredAt: timestamppb.Now()}
	require.Error(t, services.RecordUsageEvent(context.Background(), event))
}

func TestUsageEventLedgerIsIdempotentAndAggregatesCanRebuild(t *testing.T) {
	services, _ := requireCommandEventIntegrationStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	event := &a2a888.UsageEvent{
		OrganizationId: "default", Feature: "agent.turn", Unit: "count", Quantity: 2,
		OccurredAt: timestamppb.New(now), SourceReference: "command-1", IdempotencyKey: "usage-1",
	}
	require.NoError(t, services.RecordUsageEvent(ctx, event))
	require.NotEmpty(t, event.Name)
	require.NoError(t, services.RecordUsageEvent(ctx, event))

	conflict := &a2a888.UsageEvent{
		OrganizationId: event.OrganizationId, Name: event.Name, Feature: event.Feature, Unit: event.Unit,
		Quantity: 3, OccurredAt: event.OccurredAt, SourceReference: event.SourceReference,
		IdempotencyKey: event.IdempotencyKey,
	}
	require.Error(t, services.RecordUsageEvent(ctx, conflict))

	aggregate, err := services.RecomputeUsageAggregate(ctx, "default", "agent.turn", "count", now.Add(-time.Minute), now.Add(time.Minute))
	require.NoError(t, err)
	require.EqualValues(t, 2, aggregate.Quantity)
	aggregate, err = services.RecomputeUsageAggregate(ctx, "default", "agent.turn", "count", now.Add(-time.Minute), now.Add(time.Minute))
	require.NoError(t, err)
	require.EqualValues(t, 2, aggregate.Quantity)

	_, err = services.GetDB().ExecContext(ctx, `UPDATE a2a888_usage_event SET quantity = 9 WHERE organization_id = 'default' AND idempotency_key = 'usage-1'`)
	require.Error(t, err)
}

func TestEvaluateQuotaEnforcesEntitlementAndReadOnlyState(t *testing.T) {
	services, _ := requireCommandEventIntegrationStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	start, end := now.Add(-time.Minute), now.Add(time.Minute)
	require.NoError(t, services.UpsertSubscription(ctx, &a2a888.Subscription{
		Name: "organizations/default/subscriptions/primary", OrganizationId: "default",
		State: a2a888.SubscriptionState_SUBSCRIPTION_STATE_ACTIVE, EffectiveFrom: timestamppb.New(now.Add(-time.Hour)),
	}))
	require.NoError(t, services.UpsertEntitlement(ctx, &a2a888.Entitlement{
		Name: "organizations/default/entitlements/agent-turn", OrganizationId: "default", Feature: "agent.turn",
		Enabled: true, Limit: 3, Unit: "count", Period: "minute",
		OverageDecision: a2a888.UsageDecision_USAGE_DECISION_DENY, EffectiveFrom: timestamppb.New(now.Add(-time.Hour)),
	}))
	require.NoError(t, services.RecordUsageEvent(ctx, &a2a888.UsageEvent{
		OrganizationId: "default", Feature: "agent.turn", Unit: "count", Quantity: 2,
		OccurredAt: timestamppb.New(now), IdempotencyKey: "quota-used-1",
	}))
	result, err := services.EvaluateQuota(ctx, "default", "agent.turn", "count", 1, start, end, now)
	require.NoError(t, err)
	require.Equal(t, a2a888.UsageDecision_USAGE_DECISION_ALLOW, result.Decision)
	result, err = services.EvaluateQuota(ctx, "default", "agent.turn", "count", 2, start, end, now)
	require.NoError(t, err)
	require.Equal(t, a2a888.UsageDecision_USAGE_DECISION_DENY, result.Decision)

	require.NoError(t, services.UpsertSubscription(ctx, &a2a888.Subscription{
		Name: "organizations/default/subscriptions/primary", OrganizationId: "default",
		State: a2a888.SubscriptionState_SUBSCRIPTION_STATE_READ_ONLY, EffectiveFrom: timestamppb.New(now.Add(-time.Hour)),
	}))
	result, err = services.EvaluateQuota(ctx, "default", "agent.turn", "count", 1, start, end, now)
	require.NoError(t, err)
	require.Equal(t, a2a888.UsageDecision_USAGE_DECISION_DENY, result.Decision)
}
