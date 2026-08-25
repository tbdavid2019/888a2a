-- Version: 1.1.27
-- === 888a2a Machine Assignment persistence ===
-- These additive tables persist durable per-Machine assignment events with
-- monotonic sequences, idempotency keys, and acknowledgement state.

CREATE TABLE IF NOT EXISTS a2a888_machine_assignment_event (
    machine_resource_id TEXT NOT NULL,
    sequence BIGINT NOT NULL,
    event_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    agent_resource_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    config_revision TEXT NOT NULL DEFAULT '',
    config_payload_reference TEXT NOT NULL DEFAULT '',
    config_payload_digest TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (machine_resource_id, sequence),
    CONSTRAINT a2a888_machine_assignment_event_machine_check CHECK (machine_resource_id <> ''),
    CONSTRAINT a2a888_machine_assignment_event_sequence_check CHECK (sequence > 0),
    CONSTRAINT a2a888_machine_assignment_event_id_check CHECK (event_id <> ''),
    CONSTRAINT a2a888_machine_assignment_event_idempotency_check CHECK (idempotency_key <> ''),
    CONSTRAINT a2a888_machine_assignment_event_agent_check CHECK (agent_resource_id <> ''),
    CONSTRAINT a2a888_machine_assignment_event_type_check CHECK (event_type IN ('CREATE', 'CONFIG_UPDATE', 'REMOVE'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_a2a888_machine_assignment_event_id
    ON a2a888_machine_assignment_event (machine_resource_id, event_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_a2a888_machine_assignment_event_idempotency
    ON a2a888_machine_assignment_event (machine_resource_id, idempotency_key);
CREATE INDEX IF NOT EXISTS idx_a2a888_machine_assignment_event_agent
    ON a2a888_machine_assignment_event (machine_resource_id, agent_resource_id);

CREATE TABLE IF NOT EXISTS a2a888_machine_assignment_state (
    machine_resource_id TEXT PRIMARY KEY,
    high_watermark BIGINT NOT NULL DEFAULT 0,
    last_ack_sequence BIGINT NOT NULL DEFAULT 0,
    last_ack_event_id TEXT NOT NULL DEFAULT '',
    last_ack_idempotency_key TEXT NOT NULL DEFAULT '',
    full_roster_revision TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT a2a888_machine_assignment_state_machine_check CHECK (machine_resource_id <> ''),
    CONSTRAINT a2a888_machine_assignment_state_hw_check CHECK (high_watermark >= 0),
    CONSTRAINT a2a888_machine_assignment_state_ack_check CHECK (last_ack_sequence >= 0)
);
