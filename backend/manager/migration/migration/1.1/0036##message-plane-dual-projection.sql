-- Version: 1.1.36
-- === MessagePlane dual projection and consumer cursor ===

CREATE TABLE IF NOT EXISTS a2a888_message_projection (
    organization_id TEXT NOT NULL,
    message_id UUID NOT NULL,
    conversation_id TEXT NOT NULL,
    client_msg_no TEXT NOT NULL,
    message_seq BIGINT NOT NULL,
    sender_id TEXT NOT NULL,
    content TEXT NOT NULL DEFAULT '',
    attachments JSONB NOT NULL DEFAULT '[]',
    mentions JSONB NOT NULL DEFAULT '[]',
    thread_root_id TEXT,
    reactions JSONB NOT NULL DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, message_id),
    CONSTRAINT a2a888_message_projection_message_fk
        FOREIGN KEY (organization_id, message_id)
        REFERENCES a2a888_message(organization_id, message_id) ON DELETE CASCADE,
    CONSTRAINT a2a888_message_projection_identity_check
        CHECK (organization_id <> '' AND conversation_id <> '' AND client_msg_no <> '' AND sender_id <> ''),
    CONSTRAINT a2a888_message_projection_sequence_check CHECK (message_seq > 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_a2a888_message_projection_sequence
    ON a2a888_message_projection (organization_id, conversation_id, message_seq);
CREATE INDEX IF NOT EXISTS idx_a2a888_message_projection_history
    ON a2a888_message_projection (organization_id, conversation_id, message_seq);

-- Existing MessagePlane rows must enter the projection atomically during the
-- upgrade so replay parity does not depend on a future write.
INSERT INTO a2a888_message_projection (
    organization_id, message_id, conversation_id, client_msg_no, message_seq,
    sender_id, content, attachments, mentions, thread_root_id, reactions
)
SELECT organization_id, message_id, conversation_id, client_msg_no, message_seq,
       sender_id, COALESCE(payload->>'content', ''), COALESCE(payload->'attachments', '[]'::jsonb),
       COALESCE(payload->'mentions', '[]'::jsonb), NULLIF(payload->>'thread_root_id', ''),
       COALESCE(payload->'reactions', '[]'::jsonb)
FROM a2a888_message
ON CONFLICT (organization_id, message_id) DO NOTHING;

CREATE TABLE IF NOT EXISTS a2a888_message_projection_cursor (
    organization_id TEXT NOT NULL,
    conversation_id TEXT NOT NULL,
    consumer_type TEXT NOT NULL,
    consumer_id TEXT NOT NULL,
    message_seq BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, conversation_id, consumer_type, consumer_id),
    CONSTRAINT a2a888_message_projection_cursor_identity_check
        CHECK (organization_id <> '' AND conversation_id <> '' AND consumer_type <> '' AND consumer_id <> ''),
    CONSTRAINT a2a888_message_projection_cursor_sequence_check CHECK (message_seq >= 0)
);

CREATE INDEX IF NOT EXISTS idx_a2a888_message_projection_cursor_consumer
    ON a2a888_message_projection_cursor (organization_id, consumer_type, consumer_id);
