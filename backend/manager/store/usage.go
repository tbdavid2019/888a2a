package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/timestamppb"

	a2a888 "github.com/tbdavid2019/888a2a/backend/generated-go/a2a888"
)

// RecordUsageEvent appends one immutable, tenant-scoped usage event. A
// repeated source idempotency key is accepted only when the event payload is
// identical; a conflicting replay is rejected.
func (s *Store) RecordUsageEvent(ctx context.Context, event *a2a888.UsageEvent) error {
	if s == nil || s.GetDB() == nil {
		return errors.New("usage store database is required")
	}
	if event == nil || event.OrganizationId == "" || event.Feature == "" || event.Unit == "" || event.IdempotencyKey == "" || event.Quantity < 0 || event.OccurredAt == nil || !event.OccurredAt.IsValid() {
		return errors.New("usage event organization, feature, unit, quantity, occurred_at, and idempotency_key are required")
	}
	if event.Name == "" {
		sum := sha256.Sum256([]byte(event.OrganizationId + "\x00" + event.IdempotencyKey))
		event.Name = "organizations/" + event.OrganizationId + "/usageEvents/" + hex.EncodeToString(sum[:])
	}
	_, err := s.GetDB().ExecContext(ctx, `
		INSERT INTO a2a888_usage_event
		(organization_id,name,workspace_id,principal_id,agent_id,feature,quantity,unit,occurred_at,source_reference,idempotency_key)
		VALUES ($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (organization_id,idempotency_key) DO NOTHING
	`, event.OrganizationId, event.Name, event.WorkspaceId, event.PrincipalId, event.AgentId, event.Feature, event.Quantity, event.Unit, event.OccurredAt.AsTime(), event.SourceReference, event.IdempotencyKey)
	if err != nil {
		return errors.Wrap(err, "record usage event")
	}
	var quantity int64
	var feature, unit, principalID, agentID, workspaceID, sourceReference string
	err = s.GetDB().QueryRowContext(ctx, `
		SELECT feature, unit, quantity, principal_id, agent_id, COALESCE(workspace_id,''), source_reference
		FROM a2a888_usage_event WHERE organization_id = $1 AND idempotency_key = $2
	`, event.OrganizationId, event.IdempotencyKey).Scan(&feature, &unit, &quantity, &principalID, &agentID, &workspaceID, &sourceReference)
	if err != nil {
		return errors.Wrap(err, "verify usage event idempotency")
	}
	if feature != event.Feature || unit != event.Unit || quantity != event.Quantity || principalID != event.PrincipalId || agentID != event.AgentId || workspaceID != event.WorkspaceId || sourceReference != event.SourceReference {
		return errors.New("usage event idempotency key conflicts with an existing event")
	}
	return nil
}

// RecomputeUsageAggregate rebuilds a reporting row exclusively from the
// immutable event ledger. It never mutates source events.
func (s *Store) RecomputeUsageAggregate(ctx context.Context, organizationID, feature, unit string, start, end time.Time) (*a2a888.UsageAggregate, error) {
	if s == nil || s.GetDB() == nil {
		return nil, errors.New("usage store database is required")
	}
	if organizationID == "" || feature == "" || unit == "" || !end.After(start) {
		return nil, errors.New("usage aggregate organization, feature, unit, and period are required")
	}
	var quantity int64
	if err := s.GetDB().QueryRowContext(ctx, `
		SELECT COALESCE(SUM(quantity), 0) FROM a2a888_usage_event
		WHERE organization_id = $1 AND feature = $2 AND unit = $3
		  AND occurred_at >= $4 AND occurred_at < $5
	`, organizationID, feature, unit, start.UTC(), end.UTC()).Scan(&quantity); err != nil {
		return nil, errors.Wrap(err, "sum usage events")
	}
	_, err := s.GetDB().ExecContext(ctx, `
		INSERT INTO a2a888_usage_aggregate
		(organization_id,feature,unit,period_start,period_end,quantity,recomputed_at)
		VALUES ($1,$2,$3,$4,$5,$6,now())
		ON CONFLICT (organization_id,feature,unit,period_start,period_end)
		DO UPDATE SET quantity = EXCLUDED.quantity, recomputed_at = EXCLUDED.recomputed_at
	`, organizationID, feature, unit, start.UTC(), end.UTC(), quantity)
	if err != nil {
		return nil, errors.Wrap(err, "store usage aggregate")
	}
	return &a2a888.UsageAggregate{
		Name:           fmt.Sprintf("organizations/%s/usageAggregates/%s/%s", organizationID, feature, start.UTC().Format("20060102")),
		OrganizationId: organizationID,
		Feature:        feature,
		Unit:           unit,
		PeriodStart:    timestamppb.New(start.UTC()),
		PeriodEnd:      timestamppb.New(end.UTC()),
		Quantity:       quantity,
		RecomputedAt:   timestamppb.Now(),
	}, nil
}

// GetUsageAggregate reads only the requested tenant and period.
func (s *Store) GetUsageAggregate(ctx context.Context, organizationID, feature, unit string, start, end time.Time) (*a2a888.UsageAggregate, error) {
	if s == nil || s.GetDB() == nil {
		return nil, errors.New("usage store database is required")
	}
	var quantity int64
	var recomputedAt time.Time
	err := s.GetDB().QueryRowContext(ctx, `
		SELECT quantity, recomputed_at FROM a2a888_usage_aggregate
		WHERE organization_id = $1 AND feature = $2 AND unit = $3 AND period_start = $4 AND period_end = $5
	`, organizationID, feature, unit, start.UTC(), end.UTC()).Scan(&quantity, &recomputedAt)
	if err != nil {
		return nil, errors.Wrap(err, "get usage aggregate")
	}
	return &a2a888.UsageAggregate{
		Name:           fmt.Sprintf("organizations/%s/usageAggregates/%s/%s", organizationID, feature, start.UTC().Format("20060102")),
		OrganizationId: organizationID,
		Feature:        feature,
		Unit:           unit,
		PeriodStart:    timestamppb.New(start.UTC()),
		PeriodEnd:      timestamppb.New(end.UTC()),
		Quantity:       quantity,
		RecomputedAt:   timestamppb.New(recomputedAt),
	}, nil
}

// UpsertSubscription stores the provider-neutral subscription state that
// governs chargeable operations for an Organization.
func (s *Store) UpsertSubscription(ctx context.Context, subscription *a2a888.Subscription) error {
	if s == nil || s.GetDB() == nil {
		return errors.New("usage store database is required")
	}
	if subscription == nil || subscription.OrganizationId == "" || subscription.Name == "" || subscription.EffectiveFrom == nil || !subscription.EffectiveFrom.IsValid() {
		return errors.New("subscription organization, name, and effective_from are required")
	}
	if subscription.State == a2a888.SubscriptionState_SUBSCRIPTION_STATE_UNSPECIFIED {
		return errors.New("subscription state is required")
	}
	var effectiveUntil any
	if subscription.EffectiveUntil != nil && subscription.EffectiveUntil.IsValid() {
		effectiveUntil = subscription.EffectiveUntil.AsTime()
	}
	_, err := s.GetDB().ExecContext(ctx, `
		INSERT INTO a2a888_subscription (organization_id,name,state,effective_from,effective_until,grace_policy)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (organization_id,name) DO UPDATE SET state=EXCLUDED.state,
		 effective_from=EXCLUDED.effective_from, effective_until=EXCLUDED.effective_until,
		 grace_policy=EXCLUDED.grace_policy, updated_at=now()
	`, subscription.OrganizationId, subscription.Name, subscriptionStateDB(subscription.State), subscription.EffectiveFrom.AsTime(), effectiveUntil, subscription.GracePolicy)
	if err != nil {
		return errors.Wrap(err, "upsert subscription")
	}
	return nil
}

// UpsertEntitlement stores an Organization feature entitlement independently
// of any payment provider or plan name.
func (s *Store) UpsertEntitlement(ctx context.Context, entitlement *a2a888.Entitlement) error {
	if s == nil || s.GetDB() == nil {
		return errors.New("usage store database is required")
	}
	if entitlement == nil || entitlement.OrganizationId == "" || entitlement.Feature == "" || entitlement.Unit == "" || entitlement.Limit < 0 || entitlement.EffectiveFrom == nil || !entitlement.EffectiveFrom.IsValid() {
		return errors.New("entitlement organization, feature, unit, limit, and effective_from are required")
	}
	if entitlement.OverageDecision == a2a888.UsageDecision_USAGE_DECISION_UNSPECIFIED {
		return errors.New("entitlement overage_decision is required")
	}
	var effectiveUntil any
	if entitlement.EffectiveUntil != nil && entitlement.EffectiveUntil.IsValid() {
		effectiveUntil = entitlement.EffectiveUntil.AsTime()
	}
	_, err := s.GetDB().ExecContext(ctx, `
		INSERT INTO a2a888_entitlement (organization_id,feature,enabled,quota_limit,unit,period,overage_decision,effective_from,effective_until)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (organization_id,feature) DO UPDATE SET enabled=EXCLUDED.enabled,
		 quota_limit=EXCLUDED.quota_limit, unit=EXCLUDED.unit, period=EXCLUDED.period,
		 overage_decision=EXCLUDED.overage_decision, effective_from=EXCLUDED.effective_from,
		 effective_until=EXCLUDED.effective_until
	`, entitlement.OrganizationId, entitlement.Feature, entitlement.Enabled, entitlement.Limit, entitlement.Unit, entitlement.Period, usageDecisionDB(entitlement.OverageDecision), entitlement.EffectiveFrom.AsTime(), effectiveUntil)
	if err != nil {
		return errors.Wrap(err, "upsert entitlement")
	}
	return nil
}

type QuotaEvaluation struct {
	Decision a2a888.UsageDecision
	Reason   string
	Consumed int64
	Limit    int64
}

// EvaluateQuota applies subscription, entitlement, and period usage rules and
// records the decision. A missing subscription or entitlement fails closed.
func (s *Store) EvaluateQuota(ctx context.Context, organizationID, feature, unit string, quantity int64, start, end, now time.Time) (QuotaEvaluation, error) {
	if s == nil || s.GetDB() == nil {
		return QuotaEvaluation{Decision: a2a888.UsageDecision_USAGE_DECISION_DENY}, errors.New("usage store database is required")
	}
	if organizationID == "" || feature == "" || unit == "" || quantity < 0 || !end.After(start) {
		return QuotaEvaluation{Decision: a2a888.UsageDecision_USAGE_DECISION_DENY}, errors.New("quota organization, feature, unit, quantity, and period are required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	decision := QuotaEvaluation{Decision: a2a888.UsageDecision_USAGE_DECISION_DENY}
	var subscriptionState string
	err := s.GetDB().QueryRowContext(ctx, `
		SELECT state FROM a2a888_subscription
		WHERE organization_id = $1 AND effective_from <= $2 AND (effective_until IS NULL OR effective_until > $2)
		ORDER BY effective_from DESC LIMIT 1
	`, organizationID, now.UTC()).Scan(&subscriptionState)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return decision, errors.Wrap(err, "read subscription state")
	}
	if err == sql.ErrNoRows || subscriptionState == "READ_ONLY" || subscriptionState == "SUSPENDED" || subscriptionState == "CANCELLED" {
		decision.Reason = "Organization subscription does not permit chargeable operations"
		return decision, s.recordQuotaDecision(ctx, organizationID, feature, unit, quantity, decision, start, end)
	}
	var enabled bool
	var limit int64
	var configuredUnit, overage string
	err = s.GetDB().QueryRowContext(ctx, `
		SELECT enabled, quota_limit, unit, overage_decision FROM a2a888_entitlement
		WHERE organization_id = $1 AND feature = $2 AND effective_from <= $3
		  AND (effective_until IS NULL OR effective_until > $3)
	`, organizationID, feature, now.UTC()).Scan(&enabled, &limit, &configuredUnit, &overage)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return decision, errors.Wrap(err, "read entitlement")
	}
	if err == sql.ErrNoRows || !enabled || configuredUnit != unit {
		decision.Reason = "feature entitlement is unavailable"
		return decision, s.recordQuotaDecision(ctx, organizationID, feature, unit, quantity, decision, start, end)
	}
	if err := s.GetDB().QueryRowContext(ctx, `
		SELECT COALESCE(SUM(quantity), 0) FROM a2a888_usage_event
		WHERE organization_id = $1 AND feature = $2 AND unit = $3 AND occurred_at >= $4 AND occurred_at < $5
	`, organizationID, feature, unit, start.UTC(), end.UTC()).Scan(&decision.Consumed); err != nil {
		return decision, errors.Wrap(err, "read quota consumption")
	}
	decision.Limit = limit
	if limit == 0 || decision.Consumed <= limit-quantity {
		decision.Decision = a2a888.UsageDecision_USAGE_DECISION_ALLOW
		decision.Reason = "entitlement and quota available"
	} else {
		decision.Decision = parseUsageDecision(overage)
		decision.Reason = "Organization quota exceeded"
		if decision.Decision == a2a888.UsageDecision_USAGE_DECISION_UNSPECIFIED {
			decision.Decision = a2a888.UsageDecision_USAGE_DECISION_DENY
		}
	}
	return decision, s.recordQuotaDecision(ctx, organizationID, feature, unit, quantity, decision, start, end)
}

func (s *Store) recordQuotaDecision(ctx context.Context, organizationID, feature, unit string, quantity int64, decision QuotaEvaluation, start, end time.Time) error {
	_, err := s.GetDB().ExecContext(ctx, `
		INSERT INTO a2a888_quota_decision (organization_id,feature,unit,requested_quantity,consumed_quantity,quota_limit,decision,reason,period_start,period_end)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`, organizationID, feature, unit, quantity, decision.Consumed, decision.Limit, usageDecisionDB(decision.Decision), decision.Reason, start.UTC(), end.UTC())
	return errors.Wrap(err, "record quota decision")
}

func subscriptionStateDB(value a2a888.SubscriptionState) string {
	return strings.TrimPrefix(value.String(), "SUBSCRIPTION_STATE_")
}

func usageDecisionDB(value a2a888.UsageDecision) string {
	return strings.TrimPrefix(value.String(), "USAGE_DECISION_")
}

func parseUsageDecision(value string) a2a888.UsageDecision {
	return a2a888.UsageDecision(a2a888.UsageDecision_value["USAGE_DECISION_"+value])
}

// GetCurrentSubscription returns the effective provider-neutral subscription
// for an Organization at now.
func (s *Store) GetCurrentSubscription(ctx context.Context, organizationID string, now time.Time) (*a2a888.Subscription, error) {
	if s == nil || s.GetDB() == nil {
		return nil, errors.New("usage store database is required")
	}
	var subscription a2a888.Subscription
	var state string
	var effectiveFrom, effectiveUntil, createdAt, updatedAt time.Time
	var until sql.NullTime
	err := s.GetDB().QueryRowContext(ctx, `
		SELECT name, state, effective_from, effective_until, grace_policy, created_at, updated_at
		FROM a2a888_subscription
		WHERE organization_id = $1 AND effective_from <= $2 AND (effective_until IS NULL OR effective_until > $2)
		ORDER BY effective_from DESC LIMIT 1
	`, organizationID, now.UTC()).Scan(&subscription.Name, &state, &effectiveFrom, &until, &subscription.GracePolicy, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "get current subscription")
	}
	if until.Valid {
		effectiveUntil = until.Time
		subscription.EffectiveUntil = timestamppb.New(effectiveUntil)
	}
	subscription.OrganizationId = organizationID
	subscription.State = parseSubscriptionState(state)
	subscription.EffectiveFrom = timestamppb.New(effectiveFrom)
	subscription.CreatedAt = timestamppb.New(createdAt)
	subscription.UpdatedAt = timestamppb.New(updatedAt)
	return &subscription, nil
}

// ListEntitlements returns effective entitlements for one Organization.
func (s *Store) ListEntitlements(ctx context.Context, organizationID string, now time.Time) ([]*a2a888.Entitlement, error) {
	rows, err := s.GetDB().QueryContext(ctx, `
		SELECT name, feature, enabled, quota_limit, unit, period, overage_decision, effective_from, effective_until
		FROM a2a888_entitlement WHERE organization_id = $1 AND effective_from <= $2
		  AND (effective_until IS NULL OR effective_until > $2) ORDER BY feature
	`, organizationID, now.UTC())
	if err != nil {
		return nil, errors.Wrap(err, "list entitlements")
	}
	defer rows.Close()
	var result []*a2a888.Entitlement
	for rows.Next() {
		var item a2a888.Entitlement
		var decision string
		var effectiveFrom time.Time
		var effectiveUntil sql.NullTime
		if err := rows.Scan(&item.Name, &item.Feature, &item.Enabled, &item.Limit, &item.Unit, &item.Period, &decision, &effectiveFrom, &effectiveUntil); err != nil {
			return nil, errors.Wrap(err, "scan entitlement")
		}
		item.OrganizationId = organizationID
		item.OverageDecision = parseUsageDecision(decision)
		item.EffectiveFrom = timestamppb.New(effectiveFrom)
		if effectiveUntil.Valid {
			item.EffectiveUntil = timestamppb.New(effectiveUntil.Time)
		}
		result = append(result, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, "iterate entitlements")
	}
	return result, nil
}

// ListUsageAggregates returns only tenant-scoped aggregates inside a bounded
// requested period.
func (s *Store) ListUsageAggregates(ctx context.Context, organizationID string, start, end time.Time) ([]*a2a888.UsageAggregate, error) {
	rows, err := s.GetDB().QueryContext(ctx, `
		SELECT feature, unit, period_start, period_end, quantity, recomputed_at
		FROM a2a888_usage_aggregate
		WHERE organization_id = $1 AND period_start >= $2 AND period_end <= $3
		ORDER BY period_start, feature, unit
	`, organizationID, start.UTC(), end.UTC())
	if err != nil {
		return nil, errors.Wrap(err, "list usage aggregates")
	}
	defer rows.Close()
	var result []*a2a888.UsageAggregate
	for rows.Next() {
		var item a2a888.UsageAggregate
		var periodStart, periodEnd, recomputedAt time.Time
		if err := rows.Scan(&item.Feature, &item.Unit, &periodStart, &periodEnd, &item.Quantity, &recomputedAt); err != nil {
			return nil, errors.Wrap(err, "scan usage aggregate")
		}
		item.OrganizationId = organizationID
		item.Name = fmt.Sprintf("organizations/%s/usageAggregates/%s/%s", organizationID, item.Feature, periodStart.Format("20060102"))
		item.PeriodStart = timestamppb.New(periodStart)
		item.PeriodEnd = timestamppb.New(periodEnd)
		item.RecomputedAt = timestamppb.New(recomputedAt)
		result = append(result, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, "iterate usage aggregates")
	}
	return result, nil
}

func parseSubscriptionState(value string) a2a888.SubscriptionState {
	return a2a888.SubscriptionState(a2a888.SubscriptionState_value["SUBSCRIPTION_STATE_"+value])
}
