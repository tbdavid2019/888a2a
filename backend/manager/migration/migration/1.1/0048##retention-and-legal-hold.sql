-- Version: 1.1.48
-- === Tenant-scoped retention policy and legal holds ===

CREATE TABLE IF NOT EXISTS a2a888_retention_hold (
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    reason TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, resource_type, resource_id),
    CONSTRAINT a2a888_retention_hold_identity_check CHECK (organization_id <> '' AND resource_type <> '' AND resource_id <> '' AND reason <> '')
);

CREATE TABLE IF NOT EXISTS a2a888_retention_outcome (
    id BIGSERIAL PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    action TEXT NOT NULL,
    detail TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT a2a888_retention_outcome_action_check CHECK (action IN ('REDACTED', 'SKIPPED_LEGAL_HOLD', 'FAILED'))
);
CREATE INDEX IF NOT EXISTS idx_a2a888_retention_outcome_tenant ON a2a888_retention_outcome(organization_id, created_at DESC);
