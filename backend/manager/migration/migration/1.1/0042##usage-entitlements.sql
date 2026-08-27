-- Version: 1.1.42
-- === Provider-neutral entitlements and immutable usage ledger ===

CREATE TABLE IF NOT EXISTS a2a888_billing_account (
    organization_id TEXT PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    currency TEXT NOT NULL DEFAULT 'USD',
    grace_policy TEXT NOT NULL DEFAULT 'READ_ONLY',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT a2a888_billing_account_identity_check CHECK (organization_id <> '')
);

CREATE TABLE IF NOT EXISTS a2a888_subscription (
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'TRIAL',
    effective_from TIMESTAMPTZ NOT NULL,
    effective_until TIMESTAMPTZ,
    grace_policy TEXT NOT NULL DEFAULT 'READ_ONLY',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, name),
    CONSTRAINT a2a888_subscription_identity_check CHECK (name <> ''),
    CONSTRAINT a2a888_subscription_state_check CHECK (state IN ('TRIAL', 'ACTIVE', 'GRACE', 'READ_ONLY', 'SUSPENDED', 'CANCELLED')),
    CONSTRAINT a2a888_subscription_period_check CHECK (effective_until IS NULL OR effective_until > effective_from)
);

CREATE INDEX IF NOT EXISTS idx_a2a888_subscription_active
    ON a2a888_subscription(organization_id, state, effective_from, effective_until);

CREATE TABLE IF NOT EXISTS a2a888_entitlement (
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    feature TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    quota_limit BIGINT NOT NULL DEFAULT 0,
    unit TEXT NOT NULL DEFAULT '',
    period TEXT NOT NULL DEFAULT '',
    overage_decision TEXT NOT NULL DEFAULT 'DENY',
    effective_from TIMESTAMPTZ NOT NULL DEFAULT now(),
    effective_until TIMESTAMPTZ,
    PRIMARY KEY (organization_id, feature),
    CONSTRAINT a2a888_entitlement_identity_check CHECK (feature <> ''),
    CONSTRAINT a2a888_entitlement_limit_check CHECK (quota_limit >= 0),
    CONSTRAINT a2a888_entitlement_decision_check CHECK (overage_decision IN ('ALLOW', 'QUEUE', 'DENY')),
    CONSTRAINT a2a888_entitlement_period_check CHECK (effective_until IS NULL OR effective_until > effective_from)
);

CREATE TABLE IF NOT EXISTS a2a888_usage_event (
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    workspace_id TEXT,
    principal_id TEXT NOT NULL DEFAULT '',
    agent_id TEXT NOT NULL DEFAULT '',
    feature TEXT NOT NULL,
    quantity BIGINT NOT NULL,
    unit TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    source_reference TEXT NOT NULL DEFAULT '',
    idempotency_key TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, name),
    CONSTRAINT a2a888_usage_event_workspace_fk FOREIGN KEY (organization_id, workspace_id)
        REFERENCES workspaces(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT a2a888_usage_event_identity_check CHECK (name <> '' AND feature <> '' AND unit <> '' AND idempotency_key <> ''),
    CONSTRAINT a2a888_usage_event_quantity_check CHECK (quantity >= 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_a2a888_usage_event_idempotency
    ON a2a888_usage_event(organization_id, idempotency_key);
CREATE INDEX IF NOT EXISTS idx_a2a888_usage_event_period
    ON a2a888_usage_event(organization_id, feature, unit, occurred_at);

CREATE TABLE IF NOT EXISTS a2a888_usage_aggregate (
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    feature TEXT NOT NULL,
    unit TEXT NOT NULL,
    period_start TIMESTAMPTZ NOT NULL,
    period_end TIMESTAMPTZ NOT NULL,
    quantity BIGINT NOT NULL DEFAULT 0,
    recomputed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, feature, unit, period_start, period_end),
    CONSTRAINT a2a888_usage_aggregate_period_check CHECK (period_end > period_start),
    CONSTRAINT a2a888_usage_aggregate_quantity_check CHECK (quantity >= 0)
);

CREATE OR REPLACE FUNCTION a2a888_reject_usage_event_update()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'usage events are immutable';
END $$;
DROP TRIGGER IF EXISTS a2a888_usage_event_immutable ON a2a888_usage_event;
CREATE TRIGGER a2a888_usage_event_immutable
    BEFORE UPDATE OR DELETE ON a2a888_usage_event
    FOR EACH ROW EXECUTE FUNCTION a2a888_reject_usage_event_update();
