-- Version: 1.1.33
-- === Shared tenant-scoped nonce replay protection ===

CREATE TABLE IF NOT EXISTS a2a888_nonce_replay (
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    agent_resource_id TEXT NOT NULL,
    nonce TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, agent_resource_id, nonce),
    CONSTRAINT a2a888_nonce_replay_identity_check CHECK (organization_id <> '' AND agent_resource_id <> '' AND nonce <> ''),
    CONSTRAINT a2a888_nonce_replay_expiry_check CHECK (expires_at > created_at)
);

CREATE INDEX IF NOT EXISTS idx_a2a888_nonce_replay_expiry
    ON a2a888_nonce_replay (expires_at);
