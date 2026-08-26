-- Version: 1.1.30
-- === 888a2a Durable Connector Inbox ===

CREATE TABLE IF NOT EXISTS a2a888_connector_inbox (
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    installation_id TEXT NOT NULL,
    external_event_id TEXT NOT NULL,
    external_event_type TEXT NOT NULL,
    external_conversation TEXT NOT NULL DEFAULT '',
    raw_payload JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'RECEIVED',
    attempts INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, installation_id, external_event_id),
    CONSTRAINT a2a888_connector_inbox_status_check CHECK (status IN ('RECEIVED', 'PROCESSED', 'FAILED')),
    CONSTRAINT a2a888_connector_inbox_identity_check CHECK (organization_id <> '' AND installation_id <> '' AND external_event_id <> ''),
    CONSTRAINT a2a888_connector_inbox_type_check CHECK (external_event_type <> ''),
    CONSTRAINT a2a888_connector_inbox_attempts_check CHECK (attempts >= 0)
);

CREATE INDEX IF NOT EXISTS idx_a2a888_connector_inbox_pending
    ON a2a888_connector_inbox (organization_id, status, received_at)
    WHERE status IN ('RECEIVED', 'FAILED');
