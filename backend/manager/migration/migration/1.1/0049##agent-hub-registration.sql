-- Hub enrollment, registered peer identity, and lease metadata.
-- Token columns contain only one-way hashes; native runtime credentials and
-- provider session IDs are deliberately absent.
CREATE TABLE IF NOT EXISTS a2a888_hub (
    hub_id TEXT PRIMARY KEY,
    mode TEXT NOT NULL DEFAULT 'closed',
    bootstrap_token_hash TEXT NOT NULL DEFAULT '',
    registration_enabled BOOLEAN NOT NULL DEFAULT false,
    public_confirmed BOOLEAN NOT NULL DEFAULT false,
    registration_ttl_seconds INTEGER NOT NULL DEFAULT 86400,
    peer_lease_seconds INTEGER NOT NULL DEFAULT 90,
    max_registered_agents INTEGER NOT NULL DEFAULT 100,
    max_tasks_per_minute INTEGER NOT NULL DEFAULT 60,
    max_concurrent_tasks INTEGER NOT NULL DEFAULT 4,
    max_payload_bytes BIGINT NOT NULL DEFAULT 1048576,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT a2a888_hub_identity_check CHECK (hub_id <> ''),
    CONSTRAINT a2a888_hub_mode_check CHECK (mode IN ('closed', 'open', 'public')),
    CONSTRAINT a2a888_hub_public_confirmation_check CHECK (mode <> 'public' OR public_confirmed),
    CONSTRAINT a2a888_hub_limits_check CHECK (registration_ttl_seconds > 0 AND peer_lease_seconds > 0 AND max_registered_agents > 0 AND max_tasks_per_minute > 0 AND max_concurrent_tasks > 0 AND max_payload_bytes BETWEEN 1 AND 1048576)
);

CREATE TABLE IF NOT EXISTS a2a888_hub_agent (
    hub_id TEXT NOT NULL REFERENCES a2a888_hub(hub_id) ON DELETE CASCADE,
    agent_id TEXT NOT NULL,
    registration_key_hash TEXT NOT NULL,
    agent_token_hash TEXT NOT NULL,
    display_name TEXT NOT NULL,
    provider_family TEXT NOT NULL,
    transport_id TEXT NOT NULL,
    capabilities JSONB NOT NULL DEFAULT '[]'::jsonb,
    agent_card_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    state TEXT NOT NULL DEFAULT 'PENDING',
    automatic_execution BOOLEAN NOT NULL DEFAULT false,
    last_seen_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ NOT NULL,
    lease_expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    revoke_reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (hub_id, agent_id),
    CONSTRAINT a2a888_hub_agent_identity_check CHECK (agent_id <> '' AND registration_key_hash <> '' AND agent_token_hash <> '' AND display_name <> '' AND provider_family <> '' AND transport_id <> ''),
    CONSTRAINT a2a888_hub_agent_state_check CHECK (state IN ('PENDING', 'ONLINE', 'OFFLINE', 'REVOKED', 'EXPIRED')),
    CONSTRAINT a2a888_hub_agent_expiry_check CHECK (expires_at > created_at),
    CONSTRAINT a2a888_hub_agent_lease_check CHECK (lease_expires_at > created_at),
    CONSTRAINT a2a888_hub_agent_revocation_check CHECK ((state = 'REVOKED') = (revoked_at IS NOT NULL))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_a2a888_hub_agent_registration ON a2a888_hub_agent(hub_id, registration_key_hash);
CREATE UNIQUE INDEX IF NOT EXISTS uq_a2a888_hub_agent_token ON a2a888_hub_agent(hub_id, agent_token_hash);
CREATE INDEX IF NOT EXISTS idx_a2a888_hub_agent_lease ON a2a888_hub_agent(hub_id, state, expires_at);

CREATE TABLE IF NOT EXISTS a2a888_hub_inbox (
    sequence BIGSERIAL PRIMARY KEY,
    hub_id TEXT NOT NULL,
    target_agent_id TEXT NOT NULL,
    requester_agent_id TEXT NOT NULL,
    task_id TEXT NOT NULL,
    context_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    message TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'PENDING',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    acknowledged_at TIMESTAMPTZ,
    CONSTRAINT a2a888_hub_inbox_agent_fk FOREIGN KEY (hub_id, target_agent_id)
        REFERENCES a2a888_hub_agent(hub_id, agent_id) ON DELETE CASCADE,
    CONSTRAINT a2a888_hub_inbox_identity_check CHECK (hub_id <> '' AND target_agent_id <> '' AND requester_agent_id <> '' AND task_id <> '' AND context_id <> '' AND idempotency_key <> '' AND message <> ''),
    CONSTRAINT a2a888_hub_inbox_state_check CHECK (state IN ('PENDING', 'ACKNOWLEDGED', 'CANCELED')),
    CONSTRAINT a2a888_hub_inbox_ack_check CHECK ((state = 'PENDING' AND acknowledged_at IS NULL) OR (state <> 'PENDING' AND acknowledged_at IS NOT NULL))
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_a2a888_hub_inbox_idempotency ON a2a888_hub_inbox(hub_id, target_agent_id, requester_agent_id, idempotency_key);
CREATE INDEX IF NOT EXISTS idx_a2a888_hub_inbox_poll ON a2a888_hub_inbox(hub_id, target_agent_id, state, sequence);
