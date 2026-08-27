-- Version: 1.1.44
-- === Tenant-scoped encrypted connector credentials ===

CREATE TABLE IF NOT EXISTS a2a888_connector_credential (
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    installation_id TEXT NOT NULL,
    ciphertext BYTEA NOT NULL,
    key_version TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, installation_id),
    CONSTRAINT a2a888_connector_credential_identity_check CHECK (organization_id <> '' AND installation_id <> '' AND octet_length(ciphertext) > 0 AND key_version <> '')
);
