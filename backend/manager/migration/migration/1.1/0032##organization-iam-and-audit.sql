-- Version: 1.1.32
-- === Organization IAM, workspace bindings, and delegated audit evidence ===
-- All changes are additive. Existing unscoped records belong to the default
-- organization and continue to resolve through the compatibility window.

ALTER TABLE policy ADD COLUMN IF NOT EXISTS organization_id TEXT NOT NULL DEFAULT 'default' REFERENCES organizations(id);
CREATE INDEX IF NOT EXISTS idx_policy_organization ON policy(organization_id);
DROP INDEX IF EXISTS idx_policy_unique_resource_type_resource_type;
CREATE UNIQUE INDEX IF NOT EXISTS idx_policy_tenant_resource_type ON policy(organization_id, resource_type, resource, type);

ALTER TABLE role ADD COLUMN IF NOT EXISTS organization_id TEXT NOT NULL DEFAULT 'default' REFERENCES organizations(id);
CREATE INDEX IF NOT EXISTS idx_role_organization ON role(organization_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_role_tenant_resource_id ON role(organization_id, resource_id);

-- Expand the role catalog without dropping membership data. The enum names in
-- Proto are prefixed, while the persisted compatibility values remain short.
ALTER TABLE organization_memberships DROP CONSTRAINT IF EXISTS org_memberships_role_check;
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'org_memberships_role_check') THEN
        ALTER TABLE organization_memberships ADD CONSTRAINT org_memberships_role_check
            CHECK (role IN ('OWNER', 'ADMIN', 'MEMBER', 'GUEST', 'BILLING_ADMIN', 'AGENT_ADMIN', 'APPROVER'));
    END IF;
END $$;

-- Composite references make it impossible to bind an object from another
-- organization even when a globally unique ID is guessed.
CREATE UNIQUE INDEX IF NOT EXISTS idx_workspaces_organization_id ON workspaces(organization_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_user_group_organization_id ON user_group(organization_id, id);

-- Existing conversation identities were globally unique. Include the tenant
-- so the same human pair or channel slug can exist independently per tenant.
DROP INDEX IF EXISTS idx_conversation_dm_unique;
CREATE UNIQUE INDEX IF NOT EXISTS idx_conversation_dm_unique
    ON conversation(organization_id, agent_id, created_by) WHERE type = 1;
DROP INDEX IF EXISTS idx_conversation_agent_dm_unique;
CREATE UNIQUE INDEX IF NOT EXISTS idx_conversation_agent_dm_unique
    ON conversation(organization_id, agent_dm_a, agent_dm_b) WHERE type = 3;
DROP INDEX IF EXISTS idx_conversation_user_dm_unique;
CREATE UNIQUE INDEX IF NOT EXISTS idx_conversation_user_dm_unique
    ON conversation(organization_id, user_dm_a, user_dm_b) WHERE type = 4;
DROP INDEX IF EXISTS idx_conversation_channel_title_unique;
CREATE UNIQUE INDEX IF NOT EXISTS idx_conversation_channel_title_unique
    ON conversation(organization_id, title) WHERE type = 2;

-- Explicit join rows make workspace grants enforceable by foreign keys. The
-- legacy workspace_ids array remains in organization_memberships for API
-- compatibility and is mirrored by the store during updates.
CREATE TABLE IF NOT EXISTS organization_membership_workspaces (
    organization_id TEXT NOT NULL,
    principal_id INT NOT NULL,
    workspace_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, principal_id, workspace_id),
    FOREIGN KEY (organization_id, principal_id) REFERENCES organization_memberships(organization_id, principal_id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id, workspace_id) REFERENCES workspaces(organization_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_org_membership_workspaces_workspace ON organization_membership_workspaces(organization_id, workspace_id);

CREATE TABLE IF NOT EXISTS organization_group_bindings (
    id BIGSERIAL PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    group_id TEXT NOT NULL REFERENCES user_group(id) ON DELETE CASCADE,
    workspace_id TEXT REFERENCES workspaces(id) ON DELETE CASCADE,
    role TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT organization_group_binding_role_check CHECK (role <> ''),
    CONSTRAINT organization_group_binding_group_fk FOREIGN KEY (organization_id, group_id) REFERENCES user_group(organization_id, id) ON DELETE CASCADE,
    CONSTRAINT organization_group_binding_workspace_fk FOREIGN KEY (organization_id, workspace_id) REFERENCES workspaces(organization_id, id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_org_group_bindings_unique
    ON organization_group_bindings(organization_id, group_id, COALESCE(workspace_id, ''), role);
CREATE INDEX IF NOT EXISTS idx_org_group_bindings_lookup ON organization_group_bindings(organization_id, group_id, workspace_id);

ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS organization_id TEXT NOT NULL DEFAULT 'default' REFERENCES organizations(id);
ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS requester_id TEXT NOT NULL DEFAULT '';
ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS executor_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_audit_log_tenant_time ON audit_log(organization_id, created_at);

-- Existing rows are default-tenant rows. Populate the explicit membership
-- join table for the default workspace without changing any user grants.
INSERT INTO organization_membership_workspaces (organization_id, principal_id, workspace_id)
SELECT m.organization_id, m.principal_id, w.id
FROM organization_memberships m
JOIN workspaces w ON w.organization_id = m.organization_id AND w.id = ANY(m.workspace_ids)
ON CONFLICT DO NOTHING;

-- Complete the collaboration projection backfill. These tables are derived
-- from a conversation or agent, so existing rows inherit that owner's tenant.
ALTER TABLE command ADD COLUMN IF NOT EXISTS organization_id TEXT NOT NULL DEFAULT 'default' REFERENCES organizations(id);
ALTER TABLE chat_message ADD COLUMN IF NOT EXISTS organization_id TEXT NOT NULL DEFAULT 'default' REFERENCES organizations(id);
ALTER TABLE chat_message ADD COLUMN IF NOT EXISTS workspace_id TEXT REFERENCES workspaces(id);
ALTER TABLE conversation_member_meta ADD COLUMN IF NOT EXISTS organization_id TEXT NOT NULL DEFAULT 'default' REFERENCES organizations(id);
ALTER TABLE command_conversation ADD COLUMN IF NOT EXISTS organization_id TEXT NOT NULL DEFAULT 'default' REFERENCES organizations(id);
ALTER TABLE command_token_usage ADD COLUMN IF NOT EXISTS organization_id TEXT NOT NULL DEFAULT 'default' REFERENCES organizations(id);
ALTER TABLE agent_channel_cursor ADD COLUMN IF NOT EXISTS organization_id TEXT NOT NULL DEFAULT 'default' REFERENCES organizations(id);
ALTER TABLE user_channel_cursor ADD COLUMN IF NOT EXISTS organization_id TEXT NOT NULL DEFAULT 'default' REFERENCES organizations(id);
ALTER TABLE thread_participant ADD COLUMN IF NOT EXISTS organization_id TEXT NOT NULL DEFAULT 'default' REFERENCES organizations(id);
ALTER TABLE message_reaction ADD COLUMN IF NOT EXISTS organization_id TEXT NOT NULL DEFAULT 'default' REFERENCES organizations(id);

UPDATE command c SET organization_id = conv.organization_id
FROM conversation conv WHERE conv.id = c.conversation_id;
UPDATE chat_message cm SET organization_id = conv.organization_id, workspace_id = conv.workspace_id
FROM conversation conv WHERE conv.id = cm.conversation_id;
UPDATE conversation_member_meta cm SET organization_id = conv.organization_id
FROM conversation conv WHERE conv.id = cm.conversation_id;
UPDATE command_conversation cc SET organization_id = conv.organization_id
FROM conversation conv WHERE conv.id = cc.conversation_id;
UPDATE command_token_usage cu SET organization_id = a.organization_id
FROM agent a WHERE a.id = cu.agent_id;
UPDATE agent_channel_cursor ac SET organization_id = conv.organization_id
FROM conversation conv WHERE conv.id = ac.conversation_id;
UPDATE user_channel_cursor uc SET organization_id = conv.organization_id
FROM conversation conv WHERE conv.id = uc.conversation_id;
UPDATE thread_participant tp SET organization_id = conv.organization_id
FROM chat_message cm JOIN conversation conv ON conv.id = cm.conversation_id
WHERE cm.id = tp.thread_root_message_id;
UPDATE message_reaction mr SET organization_id = cm.organization_id
FROM chat_message cm WHERE cm.id = mr.message_id;

CREATE INDEX IF NOT EXISTS idx_command_organization ON command(organization_id);
CREATE INDEX IF NOT EXISTS idx_chat_message_organization ON chat_message(organization_id, conversation_id);
CREATE INDEX IF NOT EXISTS idx_conversation_member_meta_organization ON conversation_member_meta(organization_id, conversation_id);
CREATE INDEX IF NOT EXISTS idx_command_conversation_organization ON command_conversation(organization_id, conversation_id);
CREATE INDEX IF NOT EXISTS idx_command_token_usage_organization ON command_token_usage(organization_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_channel_cursor_organization ON agent_channel_cursor(organization_id, conversation_id);
CREATE INDEX IF NOT EXISTS idx_user_channel_cursor_organization ON user_channel_cursor(organization_id, conversation_id);
CREATE INDEX IF NOT EXISTS idx_thread_participant_organization ON thread_participant(organization_id, thread_root_message_id);
CREATE INDEX IF NOT EXISTS idx_message_reaction_organization ON message_reaction(organization_id, message_id);
