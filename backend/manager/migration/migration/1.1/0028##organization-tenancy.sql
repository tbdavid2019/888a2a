-- Version: 1.1.28
-- === 888a2a Organization and Tenant Foundation ===
-- Introduces organizations, workspaces, organization_memberships, and binds
-- agents, machines, conversations, and principals to tenant boundaries.

-- Organizations
CREATE TABLE IF NOT EXISTS organizations (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'ACTIVE',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT organizations_state_check CHECK (state IN ('ACTIVE', 'SUSPENDED', 'CLOSED'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_organizations_slug ON organizations(slug);

-- Workspaces
CREATE TABLE IF NOT EXISTS workspaces (
    id TEXT PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    slug TEXT NOT NULL,
    is_default BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, slug)
);

CREATE INDEX IF NOT EXISTS idx_workspaces_organization ON workspaces(organization_id);

-- Organization Memberships
CREATE TABLE IF NOT EXISTS organization_memberships (
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    principal_id INT NOT NULL REFERENCES principal(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'MEMBER',
    state TEXT NOT NULL DEFAULT 'ACTIVE',
    workspace_ids TEXT[] NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, principal_id),
    CONSTRAINT org_memberships_role_check CHECK (role IN ('OWNER', 'ADMIN', 'MEMBER', 'GUEST')),
    CONSTRAINT org_memberships_state_check CHECK (state IN ('ACTIVE', 'SUSPENDED', 'INVITED'))
);

CREATE INDEX IF NOT EXISTS idx_org_memberships_principal ON organization_memberships(principal_id);

-- 1. Seed default organization and workspace
INSERT INTO organizations (id, name, slug, state)
VALUES ('default', 'Default Organization', 'default', 'ACTIVE')
ON CONFLICT (id) DO NOTHING;

INSERT INTO workspaces (id, organization_id, name, slug, is_default)
VALUES ('default', 'default', 'Default Workspace', 'default', true)
ON CONFLICT (id) DO NOTHING;

-- 2. Seed existing principals into default organization as OWNERs
INSERT INTO organization_memberships (organization_id, principal_id, role, state, workspace_ids)
SELECT 'default', id, 'OWNER', 'ACTIVE', ARRAY['default']
FROM principal
ON CONFLICT (organization_id, principal_id) DO NOTHING;

-- 3. Add organization_id / workspace_id columns to collaboration entities
ALTER TABLE principal ADD COLUMN IF NOT EXISTS default_organization_id TEXT DEFAULT 'default' REFERENCES organizations(id);

ALTER TABLE agent ADD COLUMN IF NOT EXISTS organization_id TEXT NOT NULL DEFAULT 'default' REFERENCES organizations(id);
ALTER TABLE agent ADD COLUMN IF NOT EXISTS workspace_id TEXT DEFAULT 'default' REFERENCES workspaces(id);
CREATE INDEX IF NOT EXISTS idx_agent_organization ON agent(organization_id);
CREATE INDEX IF NOT EXISTS idx_agent_workspace ON agent(workspace_id);

ALTER TABLE machine ADD COLUMN IF NOT EXISTS organization_id TEXT NOT NULL DEFAULT 'default' REFERENCES organizations(id);
CREATE INDEX IF NOT EXISTS idx_machine_organization ON machine(organization_id);

ALTER TABLE conversation ADD COLUMN IF NOT EXISTS organization_id TEXT NOT NULL DEFAULT 'default' REFERENCES organizations(id);
ALTER TABLE conversation ADD COLUMN IF NOT EXISTS workspace_id TEXT DEFAULT 'default' REFERENCES workspaces(id);
CREATE INDEX IF NOT EXISTS idx_conversation_organization ON conversation(organization_id);
CREATE INDEX IF NOT EXISTS idx_conversation_workspace ON conversation(workspace_id);

ALTER TABLE mcp_server ADD COLUMN IF NOT EXISTS organization_id TEXT NOT NULL DEFAULT 'default' REFERENCES organizations(id);
CREATE INDEX IF NOT EXISTS idx_mcp_server_organization ON mcp_server(organization_id);

ALTER TABLE file ADD COLUMN IF NOT EXISTS organization_id TEXT NOT NULL DEFAULT 'default' REFERENCES organizations(id);
CREATE INDEX IF NOT EXISTS idx_file_organization ON file(organization_id);

ALTER TABLE task ADD COLUMN IF NOT EXISTS organization_id TEXT NOT NULL DEFAULT 'default' REFERENCES organizations(id);
CREATE INDEX IF NOT EXISTS idx_task_organization ON task(organization_id);

ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS organization_id TEXT NOT NULL DEFAULT 'default' REFERENCES organizations(id);
CREATE INDEX IF NOT EXISTS idx_audit_log_organization ON audit_log(organization_id);

ALTER TABLE api_provider ADD COLUMN IF NOT EXISTS organization_id TEXT NOT NULL DEFAULT 'default' REFERENCES organizations(id);
CREATE INDEX IF NOT EXISTS idx_api_provider_organization ON api_provider(organization_id);

ALTER TABLE user_group ADD COLUMN IF NOT EXISTS organization_id TEXT NOT NULL DEFAULT 'default' REFERENCES organizations(id);
CREATE INDEX IF NOT EXISTS idx_user_group_organization ON user_group(organization_id);

ALTER TABLE reminder ADD COLUMN IF NOT EXISTS organization_id TEXT NOT NULL DEFAULT 'default' REFERENCES organizations(id);
CREATE INDEX IF NOT EXISTS idx_reminder_organization ON reminder(organization_id);
