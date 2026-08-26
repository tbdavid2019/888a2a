-- Version: 1.1.34
-- === Durable MessagePlane identity, ordering, and membership projection ===

CREATE TABLE IF NOT EXISTS a2a888_message_cursor (
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    conversation_id TEXT NOT NULL,
    next_message_seq BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (organization_id, conversation_id),
    CONSTRAINT a2a888_message_cursor_identity_check CHECK (organization_id <> '' AND conversation_id <> ''),
    CONSTRAINT a2a888_message_cursor_sequence_check CHECK (next_message_seq >= 0)
);

CREATE TABLE IF NOT EXISTS a2a888_message (
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    conversation_id TEXT NOT NULL,
    message_id UUID NOT NULL,
    client_msg_no TEXT NOT NULL,
    message_seq BIGINT NOT NULL,
    sender_id TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, message_id),
    CONSTRAINT a2a888_message_identity_check CHECK (organization_id <> '' AND conversation_id <> '' AND message_id IS NOT NULL AND client_msg_no <> '' AND sender_id <> ''),
    CONSTRAINT a2a888_message_sequence_check CHECK (message_seq > 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_a2a888_message_sequence
    ON a2a888_message (organization_id, conversation_id, message_seq);
CREATE UNIQUE INDEX IF NOT EXISTS uq_a2a888_message_client_retry
    ON a2a888_message (organization_id, conversation_id, sender_id, client_msg_no);
CREATE INDEX IF NOT EXISTS idx_a2a888_message_history
    ON a2a888_message (organization_id, conversation_id, message_seq);

CREATE TABLE IF NOT EXISTS a2a888_message_membership (
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    conversation_id TEXT NOT NULL,
    principal_id TEXT NOT NULL,
    role TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, conversation_id, principal_id),
    CONSTRAINT a2a888_message_membership_identity_check CHECK (organization_id <> '' AND conversation_id <> '' AND principal_id <> '' AND role <> '')
);

CREATE INDEX IF NOT EXISTS idx_a2a888_message_membership_conversation
    ON a2a888_message_membership (organization_id, conversation_id);
