-- Version: 1.1.46
-- === Explicit connector bridge divergence records ===

CREATE TABLE IF NOT EXISTS a2a888_connector_divergence (
    id BIGSERIAL PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    installation_id TEXT NOT NULL,
    source_ref TEXT NOT NULL,
    destination_ref TEXT NOT NULL,
    external_event_id TEXT NOT NULL,
    reason TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT a2a888_connector_divergence_identity_check CHECK (organization_id <> '' AND installation_id <> '' AND source_ref <> '' AND destination_ref <> '' AND external_event_id <> '' AND reason <> '')
);
CREATE INDEX IF NOT EXISTS idx_a2a888_connector_divergence_tenant ON a2a888_connector_divergence(organization_id, created_at DESC);
