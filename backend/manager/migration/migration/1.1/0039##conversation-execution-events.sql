-- Conversation-visible Agent execution lifecycle events.
CREATE TABLE IF NOT EXISTS a2a888_conversation_execution_event (
    id BIGSERIAL PRIMARY KEY,
    organization_id TEXT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    conversation_id UUID NOT NULL REFERENCES conversation(id) ON DELETE CASCADE,
    command_id UUID NOT NULL REFERENCES command(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT a2a888_conversation_execution_event_type_check CHECK (event_type IN ('COMMAND_STARTED', 'COMMAND_STEERED', 'COMMAND_CANCELLED', 'COMMAND_COMPLETED'))
);
CREATE INDEX IF NOT EXISTS idx_a2a888_conversation_execution_event_conversation
    ON a2a888_conversation_execution_event (organization_id, conversation_id, id);
CREATE INDEX IF NOT EXISTS idx_a2a888_conversation_execution_event_command
    ON a2a888_conversation_execution_event (organization_id, command_id, id);
