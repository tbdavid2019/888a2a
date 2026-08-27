-- Version: 1.1.47
-- === Tenant-scoped connector installation status ===

CREATE TABLE IF NOT EXISTS a2a888_connector_installation (
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    installation_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    capabilities TEXT[] NOT NULL DEFAULT '{}',
    health TEXT NOT NULL DEFAULT 'HEALTHY',
    last_error TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, installation_id),
    CONSTRAINT a2a888_connector_installation_identity_check CHECK (organization_id <> '' AND installation_id <> '' AND kind <> ''),
    CONSTRAINT a2a888_connector_installation_health_check CHECK (health IN ('HEALTHY', 'DEGRADED', 'FAILED', 'DISABLED'))
);
