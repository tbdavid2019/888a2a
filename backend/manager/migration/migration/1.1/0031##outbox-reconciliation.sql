-- Version: 1.1.31
-- === 888a2a Outbox Reconciliation Records ===

CREATE TABLE IF NOT EXISTS a2a888_outbox_reconciliation (
    id BIGSERIAL PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    event_id TEXT NOT NULL REFERENCES a2a888_outbox_event(event_id) ON DELETE CASCADE,
    actor_id TEXT NOT NULL,
    action TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT a2a888_outbox_reconciliation_action_check CHECK (action IN ('REPLAY', 'RECONCILE')),
    CONSTRAINT a2a888_outbox_reconciliation_identity_check CHECK (organization_id <> '' AND event_id <> '' AND actor_id <> '')
);

CREATE INDEX IF NOT EXISTS idx_a2a888_outbox_reconciliation_tenant
    ON a2a888_outbox_reconciliation (organization_id, created_at);
