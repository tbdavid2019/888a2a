## Context

See `proposal.md` for motivation and scope. The repository already has the hardest reusable pieces for a first Agent Network: one Machine can host multiple Agent runners, each Agent has a durable workspace and ACP session state, Manager/Machine and per-Agent bidi streams exist, PostgreSQL stores conversations/tasks/cursors, and Provider adapters exist for Codex, Claude Code and OpenCode.

The missing path is a coherent network contract. Current Agent-to-Agent behavior uses internal DM/Channel tools, Provider discovery is built-in code, Claude Code launches a floating npm wrapper, assignment pushes can be missed, ACP permissions can be automatically allowed, and there is no standards-compliant A2A endpoint or bounded task graph. Full SaaS tenancy, mature human IM and external connectors are deliberately deferred until this network proves value with the user's existing Agents.

## Goals / Non-Goals

**Goals:**

- Bring at least twelve 888a2a Agents online across two Machines and three Provider types.
- Make npm／`npx` Provider installation reproducible, pinned, cached and separate from turn execution.
- Use A2A 1.0 for Agent discovery and work exchange.
- Preserve current human-readable conversation/task context while adding durable A2A work records.
- Recover Agent roster, tasks and sessions after Manager or Machine restart.
- Prevent runaway delegation and unapproved high-risk runtime operations.
- Produce an automated E2E gate before broader SaaS work begins.

**Non-Goals:**

- Full Organization/multi-tenant migration or billing.
- WuKongIM or another distributed human IM engine.
- Slack, Teams, LINE, WhatsApp or Web Widget connectors.
- A full visual workflow builder or marketplace.
- Unlimited autonomous swarms.
- Full quorum/escalation Approval Center; focused safety defaults are included.

## Decisions

### 1. Focused Agent Network architecture

```text
888a2a Agent / Operator
          │
          ▼
    A2A 1.0 Gateway
 Agent Card / Send / Stream
 Get / List / Cancel / Subscribe
          │
          ▼
 Agent Directory + Work Coordinator
├── Peer skills and availability
├── Durable A2A work/context/artifacts
├── Parent/child task graph
├── Budget / depth / fan-out / cancel
└── Conversation/task projection
          │
          ▼
 Durable Assignment + Agent Streams
          │
      ┌───┴───────────────┐
      ▼                   ▼
  Machine A           Machine B
  ├── Agent 1         ├── Agent 7
  ├── Agent 2         ├── Agent 8
  ├── ...             ├── ...
  └── Agent 6         └── Agent 12
      │                   │
      └───────┬───────────┘
              ▼
       Runtime Gateway
 Provider manifest / pinned npm cache
 ACP v1 / ACP v2 / MCP / embedded Pi
```

This design uses current PostgreSQL conversations/tasks as internal durable communication and projection. It does not attempt the mature human IM migration in this focused change. A future MessagePlane adapter can replace the transport without changing A2A work identities.

### 2. A2A 1.0 is the Agent work boundary

Use the official A2A Go SDK. The first supported interface is A2A 1.0 HTTP+JSON over HTTPS, including streaming/subscription support exposed by the SDK. Each enabled Agent has a tenant-ready URL/interface identity and Agent Card. Internal clients use the same operations as future external clients.

The Agent Directory stores or derives:

- Agent identity and display name.
- Skills, descriptions and input/output media types.
- Verified Provider/runtime availability.
- A2A interfaces and security requirements.
- Focused access policy and current capacity.

Agent Card metadata is descriptive; it never grants authorization by itself.

Alternative considered: extend internal DM commands as the public Agent protocol. Rejected because it would preserve private semantics and omit A2A task/artifact/stream interoperability.

### 3. Durable work model maps to current collaboration

Add an A2A-compatible work record rather than replacing current messages/tasks immediately. Each work record includes:

```text
work_id / a2a_task_id / context_id
requester_agent / executor_agent
source_conversation / source_task / parent_work
state / created / updated / terminal reason
idempotency key / trace correlation
budget / depth / retry counters
artifact references
```

A source Agent sends work through a local 888a2a tool. Manager authenticates the source Agent from Machine/Agent context, authorizes the peer, commits the work and wake intent, then returns the A2A task. The target Agent consumes the task in its normal drain/execution lifecycle. Status, messages and artifacts are persisted before they are streamed to subscribers.

Existing Channel/DM/thread history receives concise system/task projections so humans can inspect delegation. A2A remains authoritative for the work lifecycle.

### 4. Provider manifests replace launch logic as the extension point

Provider manifest owns:

- Stable provider ID and display name.
- Runtime kind: embedded, system, npm or custom.
- ACP v1/v2 protocol.
- Supported OS/architecture.
- Executable or npm package, pinned version, binary and integrity.
- Model discovery, session resume, streaming, steering, MCP and tool-trace capabilities.
- Permission profile and compatibility evidence.

Existing Provider-specific event mapping remains in adapters. Adding a Provider does not change public Proto solely to add a provider ID.

### 5. npm preparation is a Machine lifecycle operation

Machine runtime preparation follows:

```text
UNAVAILABLE → INSTALLING → VERIFYING → READY
                       ├── BROKEN
                       └── QUARANTINED
READY → UPDATE_AVAILABLE → INSTALLING
```

Cache key includes package, exact version, integrity, platform and architecture. Installation is written to a staging directory and atomically published only after verification. Turn execution resolves the prepared local binary and performs no download or update. The last verified version stays available for rollback.

Default lifecycle scripts are denied unless the audited manifest explicitly requires and permits them. Package cache is immutable/shared; per-Agent workspace, session, env and credentials remain isolated.

### 6. Durable assignment closes the current multi-Agent reliability gap

Persist Machine assignment events with monotonically increasing per-Machine sequence, event type, Agent/config reference and idempotency identity. Machine reports its last acknowledged sequence when connecting. Manager replays pending events, Machine applies them idempotently, acknowledges progress and then performs full roster reconciliation.

Create, config update and remove use the same path. A missed live push cannot leave a permanent stale or zombie runner.

### 7. Session continuity stays per Agent

Keep the current per-Agent workspace and session fingerprint model. Add Provider package/version compatibility to the launch fingerprint. Compatible Machine restart attempts resume; incompatible runtime changes deliberately cold-start. Session persistence is best-effort, while A2A work and collaboration context are durable and recoverable.

### 8. Focused security replaces unconditional permission approval

Before broad external access, runtime permission uses a small enforceable policy:

- Read inside the Agent workspace: configurable allow.
- Filesystem write, shell, external network, secret and side-effecting MCP: default deny unless explicitly allowed for that Agent/profile.
- Paths remain canonicalized and root-confined.
- Subprocess cancellation terminates descendants.
- Secrets are never copied into A2A messages, Agent Cards, audit payloads or peer context.

Full user/group/quorum approval comes later. The focused change can surface `AUTH_REQUIRED` or deny; it cannot silently allow high-risk work based only on model judgment.

### 9. Minimal orchestrator is bounded, not autonomous-by-default

The first task graph supports parent/child, parallel fan-out and join. It rejects cycles and enforces configured limits for depth, children, concurrent work, retries, runtime and token/work budget. Cancellation of a root blocks new children, cancels queued work and propagates to running runtimes.

No Agent may create unrestricted descendants merely because it received a task. Each child is authorized against target Agent accessibility and remaining parent budget.

### 10. Twelve-Agent acceptance gate

The E2E fixture provisions:

- Two Machine processes.
- At least twelve Agents.
- Codex, Claude Code and OpenCode where installed, with deterministic fake Provider substitutes in ordinary CI.
- One coordinator, multiple specialists and a reviewer/aggregator.

The gate tests:

1. Every Agent registers and publishes an Agent Card.
2. Coordinator discovers peers by skill.
3. Coordinator fans out bounded tasks to at least ten peers.
4. Peers stream state and return text/artifact results.
5. Aggregator joins results.
6. Manager restarts during the run.
7. One Machine disconnects and reconnects.
8. Duplicate send is retried.
9. One descendant is cancelled.
10. Cross-Agent workspace/credential probes are denied.

Acceptance requires no lost accepted work, no duplicate task execution, deterministic terminal states and complete trace correlation.

### 11. Delivery stages

#### Stage A: Contracts and fake harness

- Product naming gate for changed files.
- Provider manifest and npm runtime interfaces.
- A2A server/client fake interoperability harness.
- Work/task and assignment schemas.

#### Stage B: Runtime and Machine reliability

- Atomic npm preparation and Provider migration.
- Per-Agent isolation and session fingerprints.
- Durable assignment/ack/replay/reconciliation.

#### Stage C: A2A network

- Agent Cards/Directory.
- A2A operations and work store.
- Agent-side peer tools and conversation projection.

#### Stage D: Bounded orchestration and safety

- Parent/child graph, join, limits and cancel.
- Default-deny runtime policy.
- Audit and trace.

#### Stage E: Twelve-Agent gate

- Deterministic CI fixture.
- Opt-in real Provider run.
- Restart/reconnect/dedup/isolation report.

Only after Stage E passes does the master roadmap proceed to full Organization, IM and Connector work.

## Risks / Trade-offs

- [Existing data model is not full A2A] → Add durable A2A work records and projections; defer destructive task replacement.
- [Official SDK/API versions can change] → Pin SDK, expose supported protocol version and run interoperability fixtures.
- [npm supply-chain execution] → Pin version/integrity, deny hidden install/update, quarantine failure and isolate runtime.
- [Twelve real Agents are costly and nondeterministic in CI] → Use deterministic fake Providers for required CI and opt-in real Provider acceptance.
- [Single-workspace transport remains temporarily] → Keep interfaces tenant-ready and block external multi-tenant access until Organization migration.
- [Minimal safety may reduce Agent capability] → Prefer explicit deny over unsafe automation; add Organization Approval in the next governance change.
- [Task fan-out can overload Machines] → Apply coordinator/tenant-ready concurrency and budget limits before scheduling.
- [Manager/Machine streams are process-affine] → Durable assignment/work is authoritative; live streams are delivery paths with reconnect/replay.

## Migration Plan

1. Add new 888a2a Agent Network contracts and naming gates without deleting current conversations/tasks or Provider code.
2. Add Provider manifests and compatibility projection while existing Provider launch remains behind a feature flag.
3. Introduce atomic npm preparation; migrate Claude Code from floating turn launch to prepared pinned local binary.
4. Add durable Machine assignment and run live push plus ack/replay in observe/compare mode.
5. Add A2A work tables, Agent Cards and read-only Directory before enabling task submission.
6. Enable A2A send/stream/list/get/cancel for selected Agents and project work into existing conversation/task history.
7. Enable bounded parent/child work and focused runtime policy.
8. Run the twelve-Agent fake gate, then the opt-in real Provider gate.
9. Make manifest runtime, durable assignment and A2A work the default only after parity and recovery tests pass.

Rollback is feature-flagged. Current internal DM/Channel communication remains available during migration. Prepared Provider versions retain the previous verified runtime. Disabling A2A stops new tasks but preserves all work records for read/recovery. Durable assignment can fall back to full roster reconciliation without deleting Agent workspaces.

## Open Questions

- Exact pinned npm versions and integrity values are selected from the compatibility spike and recorded in manifests.
- Real-provider acceptance runs on which two physical Machines is an operational choice and does not change the contracts.
