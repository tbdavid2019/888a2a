-- Version: 1.1.35
-- === Tenant-bound per-device and per-Agent message cursors ===

ALTER TABLE agent_channel_cursor
    DROP CONSTRAINT IF EXISTS agent_channel_cursor_pkey;
ALTER TABLE agent_channel_cursor
    ADD CONSTRAINT agent_channel_cursor_pkey PRIMARY KEY (organization_id, agent_id, conversation_id);

ALTER TABLE user_channel_cursor
    ADD COLUMN IF NOT EXISTS device_id TEXT NOT NULL DEFAULT 'default';
ALTER TABLE user_channel_cursor
    DROP CONSTRAINT IF EXISTS user_channel_cursor_pkey;
ALTER TABLE user_channel_cursor
    ADD CONSTRAINT user_channel_cursor_pkey PRIMARY KEY (organization_id, principal_id, device_id, conversation_id);

CREATE INDEX IF NOT EXISTS idx_agent_channel_cursor_tenant_agent
    ON agent_channel_cursor (organization_id, agent_id, conversation_id);
CREATE INDEX IF NOT EXISTS idx_user_channel_cursor_tenant_device
    ON user_channel_cursor (organization_id, principal_id, device_id, conversation_id);
