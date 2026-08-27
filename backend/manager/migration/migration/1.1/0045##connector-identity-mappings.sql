-- Version: 1.1.45
-- === Explicit tenant-scoped external identity and conversation mappings ===

CREATE TABLE IF NOT EXISTS a2a888_connector_identity_map (
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    installation_id TEXT NOT NULL,
    external_identity_type TEXT NOT NULL,
    external_identity_id TEXT NOT NULL,
    principal_id TEXT NOT NULL,
    linked_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, installation_id, external_identity_type, external_identity_id),
    CONSTRAINT a2a888_connector_identity_map_identity_check CHECK (organization_id <> '' AND installation_id <> '' AND external_identity_type <> '' AND external_identity_id <> '' AND principal_id <> '')
);

CREATE TABLE IF NOT EXISTS a2a888_connector_conversation_map (
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    installation_id TEXT NOT NULL,
    external_conversation_id TEXT NOT NULL,
    conversation_name TEXT NOT NULL,
    mapped_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, installation_id, external_conversation_id),
    CONSTRAINT a2a888_connector_conversation_map_identity_check CHECK (organization_id <> '' AND installation_id <> '' AND external_conversation_id <> '' AND conversation_name <> '')
);
