-- Version: 1.1.40
-- === Organization-scoped approval policies and immutable requests ===

CREATE TABLE IF NOT EXISTS a2a888_approval_policy (
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, name),
    CONSTRAINT a2a888_approval_policy_name_check CHECK (name <> '')
);

CREATE TABLE IF NOT EXISTS a2a888_approval_policy_version (
    organization_id TEXT NOT NULL,
    policy_name TEXT NOT NULL,
    version TEXT NOT NULL,
    workspace_id TEXT,
    resource_pattern TEXT NOT NULL DEFAULT '',
    agent_id TEXT NOT NULL DEFAULT '',
    skill TEXT NOT NULL DEFAULT '',
    action_type TEXT NOT NULL DEFAULT '',
    destination_pattern TEXT NOT NULL DEFAULT '',
    requester_class TEXT NOT NULL DEFAULT '',
    risk_level SMALLINT NOT NULL DEFAULT 0,
    approver_principal_ids TEXT[] NOT NULL DEFAULT '{}',
    approver_group_ids TEXT[] NOT NULL DEFAULT '{}',
    approver_roles TEXT[] NOT NULL DEFAULT '{}',
    required_approvals INTEGER NOT NULL,
    timeout_seconds INTEGER NOT NULL,
    on_timeout TEXT NOT NULL DEFAULT 'DENY',
    escalation_principal_ids TEXT[] NOT NULL DEFAULT '{}',
    escalation_group_ids TEXT[] NOT NULL DEFAULT '{}',
    escalation_roles TEXT[] NOT NULL DEFAULT '{}',
    prohibit_requester_approval BOOLEAN NOT NULL DEFAULT FALSE,
    prohibit_agent_owner_sole_approval BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, policy_name, version),
    CONSTRAINT a2a888_approval_policy_version_policy_fk
        FOREIGN KEY (organization_id, policy_name)
        REFERENCES a2a888_approval_policy(organization_id, name) ON DELETE CASCADE,
    CONSTRAINT a2a888_approval_policy_version_workspace_fk
        FOREIGN KEY (organization_id, workspace_id)
        REFERENCES workspaces(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT a2a888_approval_policy_version_required_check CHECK (required_approvals > 0),
    CONSTRAINT a2a888_approval_policy_version_timeout_check CHECK (timeout_seconds > 0),
    CONSTRAINT a2a888_approval_policy_version_risk_check CHECK (risk_level BETWEEN 0 AND 4),
    CONSTRAINT a2a888_approval_policy_version_timeout_action_check CHECK (on_timeout IN ('DENY', 'ESCALATE'))
);

CREATE TABLE IF NOT EXISTS a2a888_approval_request (
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    workspace_id TEXT,
    policy_name TEXT NOT NULL,
    policy_version TEXT NOT NULL,
    requester_principal_id TEXT NOT NULL,
    executing_agent_id TEXT NOT NULL DEFAULT '',
    executing_agent_owner_id TEXT NOT NULL DEFAULT '',
    action_json JSONB NOT NULL,
    intent_hash TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'PENDING',
    required_approvals INTEGER NOT NULL,
    approval_count INTEGER NOT NULL DEFAULT 0,
    execution_nonce TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    terminal_reason TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (organization_id, name),
    CONSTRAINT a2a888_approval_request_policy_fk
        FOREIGN KEY (organization_id, policy_name, policy_version)
        REFERENCES a2a888_approval_policy_version(organization_id, policy_name, version) ON DELETE RESTRICT,
    CONSTRAINT a2a888_approval_request_workspace_fk
        FOREIGN KEY (organization_id, workspace_id)
        REFERENCES workspaces(organization_id, id) ON DELETE RESTRICT,
    CONSTRAINT a2a888_approval_request_identity_check CHECK (name <> '' AND requester_principal_id <> '' AND intent_hash <> '' AND execution_nonce <> ''),
    CONSTRAINT a2a888_approval_request_state_check CHECK (state IN ('PENDING', 'APPROVED', 'DENIED', 'EXPIRED', 'CANCELLED', 'SUPERSEDED', 'EXECUTED')),
    CONSTRAINT a2a888_approval_request_count_check CHECK (approval_count >= 0 AND approval_count <= required_approvals),
    CONSTRAINT a2a888_approval_request_expiry_check CHECK (expires_at > created_at)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_a2a888_approval_request_nonce
    ON a2a888_approval_request (organization_id, execution_nonce);
CREATE INDEX IF NOT EXISTS idx_a2a888_approval_request_pending
    ON a2a888_approval_request (organization_id, state, expires_at);

CREATE TABLE IF NOT EXISTS a2a888_approval_decision (
    organization_id TEXT NOT NULL,
    name TEXT NOT NULL,
    request_name TEXT NOT NULL,
    approver_principal_id TEXT NOT NULL,
    approver_role TEXT NOT NULL DEFAULT '',
    outcome TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    intent_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, name),
    CONSTRAINT a2a888_approval_decision_request_fk
        FOREIGN KEY (organization_id, request_name)
        REFERENCES a2a888_approval_request(organization_id, name) ON DELETE RESTRICT,
    CONSTRAINT a2a888_approval_decision_outcome_check CHECK (outcome IN ('APPROVE', 'DENY')),
    CONSTRAINT a2a888_approval_decision_identity_check CHECK (name <> '' AND approver_principal_id <> '' AND intent_hash <> '')
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_a2a888_approval_decision_approver
    ON a2a888_approval_decision (organization_id, request_name, approver_principal_id);
CREATE INDEX IF NOT EXISTS idx_a2a888_approval_decision_request
    ON a2a888_approval_decision (organization_id, request_name, created_at);

-- Requests and decisions are append-only records. Lifecycle updates may alter
-- state metadata, but the action binding and every decision must remain fixed.
CREATE OR REPLACE FUNCTION a2a888_reject_approval_request_intent_update()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.organization_id <> OLD.organization_id OR NEW.name <> OLD.name
       OR NEW.policy_name <> OLD.policy_name OR NEW.policy_version <> OLD.policy_version
       OR NEW.requester_principal_id <> OLD.requester_principal_id
       OR NEW.executing_agent_id <> OLD.executing_agent_id
       OR NEW.executing_agent_owner_id <> OLD.executing_agent_owner_id
       OR NEW.action_json <> OLD.action_json OR NEW.intent_hash <> OLD.intent_hash
       OR NEW.execution_nonce <> OLD.execution_nonce OR NEW.expires_at <> OLD.expires_at THEN
        RAISE EXCEPTION 'approval request intent is immutable';
    END IF;
    RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS a2a888_approval_request_intent_immutable ON a2a888_approval_request;
CREATE TRIGGER a2a888_approval_request_intent_immutable
    BEFORE UPDATE ON a2a888_approval_request
    FOR EACH ROW EXECUTE FUNCTION a2a888_reject_approval_request_intent_update();

CREATE OR REPLACE FUNCTION a2a888_reject_approval_decision_update()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'approval decisions are immutable';
END $$;
DROP TRIGGER IF EXISTS a2a888_approval_decision_immutable ON a2a888_approval_decision;
CREATE TRIGGER a2a888_approval_decision_immutable
    BEFORE UPDATE ON a2a888_approval_decision
    FOR EACH ROW EXECUTE FUNCTION a2a888_reject_approval_decision_update();
