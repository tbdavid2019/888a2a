-- Version: 1.1.29
-- === 888a2a Durable Event Outbox ===

CREATE TABLE IF NOT EXISTS a2a888_outbox_event (
    event_id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    aggregate_type TEXT NOT NULL,
    aggregate_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    correlation_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING',
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 5,
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    worker_id TEXT NOT NULL DEFAULT '',
    claimed_at TIMESTAMPTZ,
    delivered_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT a2a888_outbox_event_status_check CHECK (status IN ('PENDING', 'CLAIMED', 'DELIVERED', 'DEAD_LETTER')),
    CONSTRAINT a2a888_outbox_event_attempts_check CHECK (attempts >= 0 AND max_attempts > 0),
    CONSTRAINT a2a888_outbox_event_identity_check CHECK (event_id <> '' AND organization_id <> ''),
    CONSTRAINT a2a888_outbox_event_type_check CHECK (aggregate_type <> '' AND aggregate_id <> '' AND event_type <> ''),
    CONSTRAINT a2a888_outbox_event_correlation_check CHECK (correlation_id <> '')
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_a2a888_outbox_event_idempotency
    ON a2a888_outbox_event (organization_id, idempotency_key)
    WHERE idempotency_key <> '';
CREATE INDEX IF NOT EXISTS idx_a2a888_outbox_event_claimable
    ON a2a888_outbox_event (status, available_at, created_at)
    WHERE status = 'PENDING';
CREATE INDEX IF NOT EXISTS idx_a2a888_outbox_event_tenant
    ON a2a888_outbox_event (organization_id, status, created_at);
