-- Per-Organization native collaboration rollout with a durable rollback path.
CREATE TABLE IF NOT EXISTS a2a888_collaboration_rollout (
    organization_id TEXT PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    mode TEXT NOT NULL DEFAULT 'LEGACY',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT a2a888_collaboration_rollout_mode_check CHECK (mode IN ('LEGACY', 'DUAL', 'MESSAGE_PLANE'))
);
