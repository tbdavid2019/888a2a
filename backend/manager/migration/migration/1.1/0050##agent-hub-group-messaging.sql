-- Agent Hub Group Messaging, membership state machine, invitations, and fanout delivery tracking.
CREATE TABLE IF NOT EXISTS a2a888_hub_group (
    group_id TEXT PRIMARY KEY,
    hub_id TEXT NOT NULL REFERENCES a2a888_hub(hub_id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'ACTIVE',
    owner_agent_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    archived_at TIMESTAMPTZ,
    CONSTRAINT a2a888_hub_group_owner_fk FOREIGN KEY (hub_id, owner_agent_id)
        REFERENCES a2a888_hub_agent(hub_id, agent_id) ON DELETE CASCADE,
    CONSTRAINT a2a888_hub_group_name_check CHECK (name <> '' AND length(name) <= 128),
    CONSTRAINT a2a888_hub_group_state_check CHECK (state IN ('ACTIVE', 'ARCHIVED')),
    CONSTRAINT a2a888_hub_group_archive_check CHECK ((state = 'ACTIVE' AND archived_at IS NULL) OR (state = 'ARCHIVED' AND archived_at IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS idx_a2a888_hub_group_owner ON a2a888_hub_group(hub_id, owner_agent_id, state);

CREATE TABLE IF NOT EXISTS a2a888_hub_group_member (
    hub_id TEXT NOT NULL,
    group_id TEXT NOT NULL REFERENCES a2a888_hub_group(group_id) ON DELETE CASCADE,
    agent_id TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'MEMBER',
    state TEXT NOT NULL DEFAULT 'ACTIVE',
    joined_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    left_at TIMESTAMPTZ,
    removed_at TIMESTAMPTZ,
    PRIMARY KEY (hub_id, group_id, agent_id),
    CONSTRAINT a2a888_hub_group_member_agent_fk FOREIGN KEY (hub_id, agent_id)
        REFERENCES a2a888_hub_agent(hub_id, agent_id) ON DELETE CASCADE,
    CONSTRAINT a2a888_hub_group_member_role_check CHECK (role IN ('OWNER', 'ADMIN', 'MEMBER')),
    CONSTRAINT a2a888_hub_group_member_state_check CHECK (state IN ('ACTIVE', 'LEFT', 'REMOVED'))
);
CREATE INDEX IF NOT EXISTS idx_a2a888_hub_group_member_agent ON a2a888_hub_group_member(hub_id, agent_id, state);

CREATE TABLE IF NOT EXISTS a2a888_hub_group_invitation (
    id BIGSERIAL PRIMARY KEY,
    hub_id TEXT NOT NULL,
    group_id TEXT NOT NULL REFERENCES a2a888_hub_group(group_id) ON DELETE CASCADE,
    inviter_agent_id TEXT NOT NULL,
    invitee_agent_id TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'PENDING',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    responded_at TIMESTAMPTZ,
    CONSTRAINT a2a888_hub_group_invitation_inviter_fk FOREIGN KEY (hub_id, inviter_agent_id)
        REFERENCES a2a888_hub_agent(hub_id, agent_id) ON DELETE CASCADE,
    CONSTRAINT a2a888_hub_group_invitation_invitee_fk FOREIGN KEY (hub_id, invitee_agent_id)
        REFERENCES a2a888_hub_agent(hub_id, agent_id) ON DELETE CASCADE,
    CONSTRAINT a2a888_hub_group_invitation_state_check CHECK (state IN ('PENDING', 'ACCEPTED', 'DECLINED', 'EXPIRED', 'REVOKED')),
    CONSTRAINT a2a888_hub_group_invitation_expiry_check CHECK (expires_at > created_at)
);
CREATE INDEX IF NOT EXISTS idx_a2a888_hub_group_invitation_invitee ON a2a888_hub_group_invitation(hub_id, invitee_agent_id, state);

CREATE TABLE IF NOT EXISTS a2a888_hub_group_message (
    id BIGSERIAL PRIMARY KEY,
    hub_id TEXT NOT NULL,
    group_id TEXT NOT NULL REFERENCES a2a888_hub_group(group_id) ON DELETE CASCADE,
    sender_agent_id TEXT NOT NULL,
    context_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    message TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT a2a888_hub_group_message_sender_fk FOREIGN KEY (hub_id, sender_agent_id)
        REFERENCES a2a888_hub_agent(hub_id, agent_id) ON DELETE CASCADE,
    CONSTRAINT a2a888_hub_group_message_identity_check CHECK (hub_id <> '' AND group_id <> '' AND sender_agent_id <> '' AND context_id <> '' AND idempotency_key <> '' AND message <> '')
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_a2a888_hub_group_message_idempotency ON a2a888_hub_group_message(hub_id, group_id, sender_agent_id, idempotency_key);
CREATE INDEX IF NOT EXISTS idx_a2a888_hub_group_message_history ON a2a888_hub_group_message(group_id, id);

CREATE TABLE IF NOT EXISTS a2a888_hub_group_delivery (
    sequence BIGINT PRIMARY KEY REFERENCES a2a888_hub_inbox(sequence) ON DELETE CASCADE,
    hub_id TEXT NOT NULL,
    group_id TEXT NOT NULL REFERENCES a2a888_hub_group(group_id) ON DELETE CASCADE,
    group_message_id BIGINT NOT NULL REFERENCES a2a888_hub_group_message(id) ON DELETE CASCADE,
    target_agent_id TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'PENDING',
    CONSTRAINT a2a888_hub_group_delivery_agent_fk FOREIGN KEY (hub_id, target_agent_id)
        REFERENCES a2a888_hub_agent(hub_id, agent_id) ON DELETE CASCADE,
    CONSTRAINT a2a888_hub_group_delivery_state_check CHECK (state IN ('PENDING', 'ACKNOWLEDGED', 'CANCELED'))
);
