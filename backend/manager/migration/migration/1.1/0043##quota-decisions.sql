-- Version: 1.1.43
-- === Durable entitlement and quota decisions ===

CREATE TABLE IF NOT EXISTS a2a888_quota_decision (
    id BIGSERIAL PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    feature TEXT NOT NULL,
    unit TEXT NOT NULL,
    requested_quantity BIGINT NOT NULL,
    consumed_quantity BIGINT NOT NULL,
    quota_limit BIGINT NOT NULL,
    decision TEXT NOT NULL,
    reason TEXT NOT NULL,
    period_start TIMESTAMPTZ NOT NULL,
    period_end TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT a2a888_quota_decision_feature_check CHECK (feature <> '' AND unit <> ''),
    CONSTRAINT a2a888_quota_decision_quantity_check CHECK (requested_quantity >= 0 AND consumed_quantity >= 0 AND quota_limit >= 0),
    CONSTRAINT a2a888_quota_decision_result_check CHECK (decision IN ('ALLOW', 'QUEUE', 'DENY')),
    CONSTRAINT a2a888_quota_decision_period_check CHECK (period_end > period_start)
);
CREATE INDEX IF NOT EXISTS idx_a2a888_quota_decision_tenant
    ON a2a888_quota_decision(organization_id, feature, created_at DESC);
