-- Version: 1.1.37
-- === MessagePlane reconciliation and quarantine records ===

CREATE TABLE IF NOT EXISTS a2a888_message_reconciliation (
    id BIGSERIAL PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    conversation_id TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    action TEXT NOT NULL,
    detail TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT a2a888_message_reconciliation_identity_check CHECK (organization_id <> '' AND conversation_id <> '' AND resource_type <> '' AND resource_id <> ''),
    CONSTRAINT a2a888_message_reconciliation_action_check CHECK (action IN ('REPAIRED', 'QUARANTINED'))
);

CREATE INDEX IF NOT EXISTS idx_a2a888_message_reconciliation_tenant
    ON a2a888_message_reconciliation (organization_id, conversation_id, created_at DESC);
