-- Version: 1.1.26
-- === 888a2a A2A-compatible work persistence ===
-- These additive tables persist the 888a2a work model independently from the
-- wire protocol. IDs are tenant-scoped because organization isolation is part
-- of the durable key shape even before the full organization model ships.

CREATE TABLE IF NOT EXISTS a2a888_work_context (
    tenant_id TEXT NOT NULL DEFAULT 'default',
    context_id TEXT NOT NULL,
    root_work_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    version BIGINT NOT NULL DEFAULT 1,
    PRIMARY KEY (tenant_id, context_id),
    CONSTRAINT a2a888_work_context_tenant_check CHECK (tenant_id <> ''),
    CONSTRAINT a2a888_work_context_id_check CHECK (context_id <> '')
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_a2a888_work_context_root
    ON a2a888_work_context (tenant_id, root_work_id)
    WHERE root_work_id IS NOT NULL AND root_work_id <> '';

CREATE TABLE IF NOT EXISTS a2a888_work (
    tenant_id TEXT NOT NULL DEFAULT 'default',
    work_id TEXT NOT NULL,
    a2a_task_id TEXT NOT NULL,
    context_id TEXT NOT NULL,
    requester_agent_id TEXT NOT NULL,
    executor_agent_id TEXT NOT NULL,
    source_conversation_id UUID REFERENCES conversation(id) ON DELETE SET NULL,
    source_task_id UUID REFERENCES task(message_id) ON DELETE SET NULL,
    state TEXT NOT NULL DEFAULT 'SUBMITTED',
    terminal_reason TEXT NOT NULL DEFAULT '',
    idempotency_key TEXT NOT NULL,
    trace_id TEXT NOT NULL DEFAULT '',
    root_trace_id TEXT NOT NULL DEFAULT '',
    span_id TEXT NOT NULL DEFAULT '',
    parent_span_id TEXT NOT NULL DEFAULT '',
    parent_work_id TEXT,
    parent_edge_type TEXT NOT NULL DEFAULT 'delegated',
    delegation_depth INTEGER NOT NULL DEFAULT 0,
    retry_count INTEGER NOT NULL DEFAULT 0,
    max_depth INTEGER NOT NULL DEFAULT 0,
    max_children INTEGER NOT NULL DEFAULT 0,
    max_fan_out INTEGER NOT NULL DEFAULT 0,
    max_concurrency INTEGER NOT NULL DEFAULT 0,
    max_runtime_ms BIGINT NOT NULL DEFAULT 0,
    max_retries INTEGER NOT NULL DEFAULT 0,
    max_tokens BIGINT NOT NULL DEFAULT 0,
    max_work_units BIGINT NOT NULL DEFAULT 0,
    used_children INTEGER NOT NULL DEFAULT 0,
    used_fan_out INTEGER NOT NULL DEFAULT 0,
    used_runtime_ms BIGINT NOT NULL DEFAULT 0,
    used_tokens BIGINT NOT NULL DEFAULT 0,
    used_work_units BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    version BIGINT NOT NULL DEFAULT 1,
    PRIMARY KEY (tenant_id, work_id),
    CONSTRAINT a2a888_work_tenant_check CHECK (tenant_id <> ''),
    CONSTRAINT a2a888_work_id_check CHECK (work_id <> ''),
    CONSTRAINT a2a888_work_a2a_task_id_check CHECK (a2a_task_id <> ''),
    CONSTRAINT a2a888_work_context_id_check CHECK (context_id <> ''),
    CONSTRAINT a2a888_work_requester_check CHECK (requester_agent_id <> ''),
    CONSTRAINT a2a888_work_executor_check CHECK (executor_agent_id <> ''),
    CONSTRAINT a2a888_work_idempotency_check CHECK (idempotency_key <> ''),
    CONSTRAINT a2a888_work_state_check CHECK (state IN (
        'AUTH_REQUIRED', 'SUBMITTED', 'WORKING', 'INPUT_REQUIRED',
        'COMPLETED', 'FAILED', 'CANCELED', 'REJECTED'
    )),
    CONSTRAINT a2a888_work_parent_check CHECK (
        parent_work_id IS NULL OR parent_work_id <> work_id
    ),
    CONSTRAINT a2a888_work_nonnegative_check CHECK (
        delegation_depth >= 0 AND retry_count >= 0 AND
        max_depth >= 0 AND max_children >= 0 AND max_fan_out >= 0 AND
        max_concurrency >= 0 AND max_runtime_ms >= 0 AND max_retries >= 0 AND
        max_tokens >= 0 AND max_work_units >= 0 AND used_children >= 0 AND
        used_fan_out >= 0 AND used_runtime_ms >= 0 AND used_tokens >= 0 AND
        used_work_units >= 0
    ),
    FOREIGN KEY (tenant_id, context_id)
        REFERENCES a2a888_work_context(tenant_id, context_id),
    FOREIGN KEY (tenant_id, parent_work_id)
        REFERENCES a2a888_work(tenant_id, work_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_a2a888_work_idempotency
    ON a2a888_work (tenant_id, requester_agent_id, idempotency_key);
CREATE UNIQUE INDEX IF NOT EXISTS uq_a2a888_work_a2a_task_id
    ON a2a888_work (tenant_id, a2a_task_id);
CREATE INDEX IF NOT EXISTS idx_a2a888_work_context_state
    ON a2a888_work (tenant_id, context_id, state, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_a2a888_work_executor_state
    ON a2a888_work (tenant_id, executor_agent_id, state, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_a2a888_work_requester_created
    ON a2a888_work (tenant_id, requester_agent_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_a2a888_work_parent
    ON a2a888_work (tenant_id, parent_work_id)
    WHERE parent_work_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS a2a888_work_artifact (
    tenant_id TEXT NOT NULL DEFAULT 'default',
    work_id TEXT NOT NULL,
    artifact_id TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    media_type TEXT NOT NULL DEFAULT '',
    external_uri TEXT NOT NULL DEFAULT '',
    file_id UUID REFERENCES file(id) ON DELETE SET NULL,
    digest TEXT NOT NULL DEFAULT '',
    size_bytes BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, work_id, artifact_id),
    CONSTRAINT a2a888_work_artifact_tenant_check CHECK (tenant_id <> ''),
    CONSTRAINT a2a888_work_artifact_id_check CHECK (artifact_id <> ''),
    CONSTRAINT a2a888_work_artifact_size_check CHECK (size_bytes >= 0),
    FOREIGN KEY (tenant_id, work_id)
        REFERENCES a2a888_work(tenant_id, work_id)
        ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_a2a888_work_artifact_work
    ON a2a888_work_artifact (tenant_id, work_id, created_at);
CREATE INDEX IF NOT EXISTS idx_a2a888_work_artifact_file
    ON a2a888_work_artifact (file_id)
    WHERE file_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS a2a888_work_event (
    tenant_id TEXT NOT NULL DEFAULT 'default',
    event_id TEXT NOT NULL,
    work_id TEXT NOT NULL,
    sequence BIGINT NOT NULL,
    trace_id TEXT NOT NULL DEFAULT '',
    root_trace_id TEXT NOT NULL DEFAULT '',
    span_id TEXT NOT NULL DEFAULT '',
    parent_span_id TEXT NOT NULL DEFAULT '',
    event_type TEXT NOT NULL,
    provider_id TEXT NOT NULL DEFAULT '',
    session_id TEXT NOT NULL DEFAULT '',
    policy_decision TEXT NOT NULL DEFAULT '',
    retry_count INTEGER NOT NULL DEFAULT 0,
    terminal_reason TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, event_id),
    CONSTRAINT a2a888_work_event_tenant_check CHECK (tenant_id <> ''),
    CONSTRAINT a2a888_work_event_id_check CHECK (event_id <> ''),
    CONSTRAINT a2a888_work_event_sequence_check CHECK (sequence > 0),
    CONSTRAINT a2a888_work_event_type_check CHECK (event_type <> ''),
    CONSTRAINT a2a888_work_event_retry_check CHECK (retry_count >= 0),
    FOREIGN KEY (tenant_id, work_id)
        REFERENCES a2a888_work(tenant_id, work_id)
        ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_a2a888_work_event_work_sequence
    ON a2a888_work_event (tenant_id, work_id, sequence);
CREATE UNIQUE INDEX IF NOT EXISTS uq_a2a888_work_event_work_sequence
    ON a2a888_work_event (tenant_id, work_id, sequence);
CREATE INDEX IF NOT EXISTS idx_a2a888_work_event_trace
    ON a2a888_work_event (tenant_id, trace_id, created_at)
    WHERE trace_id <> '';
