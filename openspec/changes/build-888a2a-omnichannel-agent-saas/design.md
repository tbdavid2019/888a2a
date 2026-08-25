## Context

See `proposal.md` for motivation and scope. 888a2a currently combines a Go Manager, PostgreSQL, React frontend and outbound-only Machine processes. It already supports humans and Agents in Channels/DMs, threads, tasks, reminders, IAM, audit, ACP v1/v2, MCP and local provider execution.

The current implementation assumes one workspace and one Manager process for several realtime/security paths. Chat ordering uses a PostgreSQL conversation version; frontend updates use long polling; room notifications, command watchers and nonce replay state include process-local components; activity generation and some assignment delivery are best-effort. These constraints are acceptable for a self-hosted collaboration product but do not meet the new multi-tenant SaaS specs.

External channel semantics are not interchangeable. Slack, Teams, LINE, WhatsApp and Web Widget differ in authentication, event ordering, retries, replies, edits, recalls, interactive components, attachment lifetime, marketplace approval and rate limits. A2A 1.0 standardizes Agent work exchange but does not replace human IM, Organization IAM, Machine lifecycle, runtime control or tool access.

The earlier `docs/plan/888a2a-provider-gateway-fork-sdd.md` remains useful for the downstream/upstream and runtime-provider analysis, but its optional late A2A/multi-tenant sequencing is superseded by this design.

## Goals / Non-Goals

**Goals:**

- Turn 888a2a into an Organization-first, commercially operable Human＋Agent SaaS.
- Preserve 888a2a's useful collaboration, Machine, ACP, MCP and provider foundations.
- Adopt A2A 1.0 as the standard Agent work protocol.
- Use a mature IM message plane rather than expanding process-local chat delivery into a new distributed IM implementation.
- Support first-class human, Agent, service-account and external identities.
- Add a connector platform that can grow from Web Widget and one external channel to Slack, Teams, LINE and WhatsApp.
- Make Organization approval, entitlements, quotas, usage and audit foundational instead of later bolt-ons.
- Deliver the platform in independently sellable vertical slices.

**Non-Goals:**

- Implement every connector or Provider in the first release.
- Replace A2A, ACP, MCP or vendor protocols with a proprietary equivalent.
- Build payment collection in this change; only provider-neutral billing boundaries are included.
- Provide full cross-platform message equivalence where a destination does not support the source operation.
- Promise multi-region active-active, E2EE federation, voice/video or a public marketplace in the first sellable release.
- Expose IM-engine administrative APIs, Provider processes or tenant secrets to public clients.

## Decisions

### 0. Product identity is 888a2a

All new product, role and user-facing names use `888a2a` and `888a2a Agent`. The migration includes repository/module identity, Proto namespace, binary names, CLI commands, environment-variable prefix, data/config directories, Docker images, release artifacts, service names, cookies, metrics, permission strings, UI copy and documentation.

Because shell environment-variable names cannot start with a digit, the target environment prefix is `A2A888_`. Existing persisted data is imported through a one-time migration or compatibility reader, then written only to 888a2a locations. License and source attribution remain intact even after product branding changes.

Alternative considered: retain the old internal namespace while changing only UI copy. Rejected because the user explicitly requires a complete 888a2a product identity and continued internal leakage would complicate packages, operations, documentation and customer support.

### 1. Recommended overall architecture

The following is the target platform shape and the primary explanation diagram for stakeholders:

```text
  Slack   Teams   LINE   WhatsApp   Web Widget   Native Web
    │       │       │        │          │            │
    └───────┴───────┴────────┴──────────┴────────────┘
                            │
                   Connector Gateway
             驗簽、去重、限流、格式轉換、重試
                            │
                            ▼
                   Collaboration Plane
          Organization / Workspace / Conversation
           Human / Agent / Group / IAM / Approval
                            │
               ┌────────────┴────────────┐
               ▼                         ▼
          IM Message Plane          Agent Work Plane
            WuKongIM                   A2A 1.0
       排序、離線、多裝置         Task、Artifact、Streaming
               │                         │
               │                  Multi-Agent Orchestrator
               │                  Fan-out / Join / Budget
               │                         │
               └────────────┬────────────┘
                            ▼
                     Runtime Gateway
                 ACP v1/v2 / npx / MCP
            Codex / Claude / OpenCode / Pi
```

Rationale:

- Human IM, Agent work, runtime control and tools have different protocols and scaling behavior.
- Organization policy and conversation routing sit above both message and Agent work planes.
- Connectors are ingress/egress adapters, not independent truth stores.
- A2A provides interoperability; the Orchestrator provides bounded many-Agent coordination that A2A does not prescribe.

Alternative considered: keep all behavior inside the current CommandService and PostgreSQL chat tables. Rejected because it would require building ordering, multi-device, offline synchronization, realtime clustering, presence, redaction and hot-channel distribution in the application layer.

### 2. Organization-first resource hierarchy

The canonical hierarchy is:

```text
Organization
├── BillingAccount / Entitlements / Quotas
├── Memberships / Groups / Roles
├── Workspaces / Projects
│   ├── Conversations / Tasks / Files
│   ├── Agents / Skills
│   └── Connector bindings
├── Machines / Runtime installations
├── Approval policies / Requests
└── Audit / Usage events
```

Every persisted resource and durable event carries `organization_id`; workspace-bound records also carry `workspace_id`. Public resource names become organization/workspace scoped. Authorization resolves tenant scope before querying a resource so a not-found distinction cannot leak cross-tenant existence.

One human account may have multiple Organization memberships. Human, Agent and service-account principals remain different types and credentials. Acting on behalf of another principal records both requester and executor.

Alternative considered: one deployed Manager per customer. Retained as a private/self-hosted deployment option, but rejected as the SaaS data model because it prevents shared control-plane operations, cross-organization A2A policy and efficient commercial tenancy.

### 3. Collaboration Plane owns business semantics

The Collaboration Plane is the product control plane for:

- Organization/workspace/channel membership and IAM.
- Conversation and external-channel mappings.
- Human/Agent identity and presence projection.
- Thread, task, approval, retention and moderation policy.
- Connector capability and delivery status.
- Search, audit, activity and compliance projections.

It does not assign final message order or maintain every client connection. Message commands are authorized here, then delivered through a transactional outbox to the IM Message Plane. Incoming IM/connector events return through durable consumers and update projections.

### 4. Mature IM engine behind an adapter

WuKongIM is the reference candidate for the first IM Message Plane because it is Go/Apache-2.0 and provides per-channel ordering, idempotency identifiers, persistence, offline synchronization, multi-device sessions, presence and distributed channel runtime. It remains behind an internal `MessagePlane` contract so product APIs and projections do not depend on its payload or storage layout.

Production adoption requires a spike covering:

- Stable version selection; WuKongIM v3 is currently beta.
- Per-channel order and duplicate behavior under failover.
- Browser SDK and reconnect behavior.
- Tenant channel namespace and subscriber reconciliation.
- Recall/edit/system-event semantics.
- Backup, restore, upgrade and observability.
- Performance and resource limits under representative hot channels.

The WuKongIM HTTP administration API stays on a private network. Public clients call 888a2a business APIs for mutations and receive short-lived IM connection credentials only after tenant authorization.

Alternative considered: Matrix. It provides a mature standard and federation but brings a larger room/state/E2EE model and operational surface than the first product slice requires. Revisit if open federation or E2EE becomes a primary requirement.

### 5. Append-only collaboration event model

The message plane assigns:

```text
client_msg_no   client retry/idempotency identity
message_id      globally unique server identity
message_seq     monotonic order within a conversation/channel
```

Application behavior is expressed as events:

```text
MESSAGE_CREATED
MESSAGE_EDITED
MESSAGE_RECALLED
MESSAGE_REDACTED
REACTION_ADDED
REACTION_REMOVED
THREAD_LINKED
COMMAND_STARTED
COMMAND_STEERED
COMMAND_CANCELLED
COMMAND_COMPLETED
```

Visible state is a projection. Recall hides content from normal readers; retention/legal-hold policy decides whether protected audit storage preserves it. Connector adapters map unsupported operations to declared fallbacks and record delivery divergence.

Alternative considered: update/delete message rows in place. Rejected because multi-device convergence, retries, connector replay, audit and legal hold require an ordered mutation history.

### 6. Connector Gateway with canonical envelope and vendor extensions

Each connector installation is tenant-scoped and has its own credentials, webhook endpoint, rate-limit state and durable queues. The gateway flow is:

```text
Webhook / Socket / Platform Event
          │
          ▼
Verify signature and installation
          │
          ▼
Persist inbox identity + encrypted raw reference
          │
          ▼
Normalize common envelope + vendor extension
          │
          ▼
Resolve tenant / identity / conversation / bridge
          │
          ▼
Policy and routing
          │
          ├── Human inbox
          ├── Agent task
          └── Outbound connector outbox
```

Common envelope fields include Organization, installation, external event/conversation/sender IDs, event time, delivery time, canonical event type, content parts, attachments, reply/thread relation, idempotency identity and capability metadata. Validated vendor-specific data remains under a namespaced extension.

Connector behavior is capability-based. Threads, edits, recalls, reactions, read receipts, buttons, templates and media do not receive fake equivalence. Cross-channel bridging requires explicit Organization policy.

Platform strategy:

- Web Widget first because its identity, UX and lifecycle are controlled by the product.
- First external connector selected by target market: LINE for Taiwan/Japan customer communication, Slack for software teams, Teams for Microsoft enterprise, WhatsApp for international customer engagement.
- Slack production distribution uses HTTPS Events API; Socket Mode remains for private deployments because Socket Mode apps cannot currently be listed in the public Slack Marketplace.
- Teams may use a Node sidecar based on the official Microsoft 365 Agents SDK while the Go core owns canonical contracts.
- LINE processing verifies the raw-body signature, acknowledges quickly, deduplicates `webhookEventId` and tolerates redelivery/out-of-order events.
- WhatsApp includes Tech Provider onboarding, business account/phone lifecycle, webhooks and template/policy status, not only message send/receive.

### 7. A2A 1.0 is the Agent work contract

The platform uses the official A2A Go SDK for public and internal Agent task exchange. Each published Agent has a public Agent Card and optionally an authenticated extended card. The gateway supports the standard task lifecycle, message/artifact parts, streaming, polling, subscription, push configuration and tenant routing.

Internal collaboration messages are not automatically converted to A2A. An explicit task creation, assignment, mention-driven delegation or routing rule creates an A2A-compatible work item linked to the source conversation/thread.

The canonical work record includes Organization/workspace, requester, executing Agent, A2A task/context IDs, parent task, source conversation, state, artifacts, approval reference, budget and trace correlation.

Alternative considered: preserve 888a2a Agent DM as the Agent work protocol. Rejected for public interoperability and long-running task semantics; DMs remain a human-readable collaboration surface.

### 8. Multi-Agent Orchestrator sits above A2A

A2A is pairwise interoperability. The Orchestrator adds:

- Parent/child task graph.
- Parallel fan-out and explicit join.
- Cycle detection.
- Max depth and child count.
- Per-Organization concurrency, token/cost and time budgets.
- Retry, timeout and partial-failure policy.
- Cancellation propagation.
- Human approval checkpoints.
- Trace graph and dead-letter handling.

The Orchestrator never grants capabilities by delegation alone. Each child task is authorized for its target Agent, skill, data and destination.

### 9. Runtime Gateway remains on Machine

Machine remains outbound-only and hosts multiple Agent runners. Runtime Gateway separates:

- Provider manifest and discovery.
- Package preparation and immutable cache.
- ACP v1/v2 execution and session resume.
- MCP server projection.
- Per-Agent workspace/env/secret isolation.
- Tool event mapping and policy requests.
- Compatibility evidence.

Production npm Providers require pinned version and integrity. Installation is separate from turn execution; a turn launches a prepared local binary and cannot silently resolve `latest`. Package cache may be shared, but workspace/session/secrets cannot.

### 10. Organization Approval is a policy engine

Approval policy selects approvers by user, group or role and matches workspace/resource, Agent/skill, action type, requester, destination and risk. Requests bind a normalized immutable action snapshot and support quorum, expiry, escalation, deny, cancellation and supersession.

A2A tasks use `AUTH_REQUIRED` while waiting. Runtime permission requests use the same policy engine. Credentials remain out-of-band and scoped to the requesting Agent/action. Approval does not inherit from a global platform admin unless an Organization explicitly configures that role.

### 11. Entitlements and usage are provider-neutral

Product authorization calls an Entitlement service, not Stripe or plan-name conditionals. Immutable usage events feed aggregates and quotas. Initial meters cover human seats, Agents, Machines, connectors, A2A work, concurrency, runtime minutes/tokens, outbound messages, storage and external calls.

Payment collection is a later adapter. This allows manual enterprise contracts, Stripe, Paddle or another system to update the same subscription/entitlement boundary.

### 12. Durable event processing replaces critical best-effort paths

PostgreSQL transactional outbox is the first delivery mechanism because it fits the existing stack and keeps source mutation plus intent atomic. Consumers are idempotent and tenant-fair. A shared event broker can replace or supplement polling after measured throughput requires it.

Critical flows include connector inbox/outbox, message projection, Machine assignment, A2A work, approval, usage and audit. Process-local hubs remain optimization caches only. Multi-instance correctness comes from durable state and shared notification.

### 13. Service and storage topology

Initial production topology:

```text
API Gateway / WAF
├── Human and Organization API
├── Connector webhook endpoints
├── A2A endpoints and Agent Cards
└── Web Widget endpoints
        │
        ▼
Stateless Manager replicas
├── Collaboration Service
├── Connector workers
├── A2A Gateway / Orchestrator
├── Approval / Entitlement / Usage
└── Machine connection gateway
        │
        ├── PostgreSQL
        ├── Object storage
        ├── Secret manager
        ├── Message Plane
        └── Shared notification / event infrastructure
```

Machine streams need a shared connection directory or connection-gateway ownership so commands can reach a Machine connected to another replica. No business operation relies on sticky sessions for correctness.

### 14. Delivery phases

#### Phase 0: Agent-network architecture and safety spikes

- Complete the product-identity mapping required for new runtime/API surfaces.
- Run the official A2A Go SDK server/client interoperability spike.
- Prototype Provider manifest, pinned npm／`npx` preparation and local launch.
- Prove one Machine can host multiple isolated Agent runtimes concurrently.
- Prove durable Machine assignment and Agent message recovery across reconnect.
- Prove default-deny runtime permission, cancel and basic delegation budget.

Exit: A2A, npm runtime, multi-Agent hosting and safety decisions are backed by test artifacts; no irreversible SaaS migration is required.

#### Phase 1: Provider Runtime Gateway foundation

- Provider manifest and compatibility levels.
- Atomic pinned npm package cache with integrity, quarantine and rollback.
- Manifest-backed Codex, Claude Code, OpenCode and embedded Pi fallback.
- Per-Agent workspace, session, environment and credential isolation.
- Durable Machine assignment/config/remove delivery.

Exit: at least 12 configured 888a2a Agents across two Machines can stay online, launch the expected Provider and resume isolated sessions.

#### Phase 2: A2A Agent Network MVP

- A2A 1.0 Agent Cards, discovery, send, stream, get, list and cancel.
- Internal Agent directory and skill/capability lookup.
- A2A-compatible durable work records linked to existing conversations/tasks.
- Agent-to-Agent delegation tools and reply wake-up.
- Minimal parent/child fan-out, join, cycle/depth/fan-out/concurrency limits and trace.

Exit: 10+ Agents discover each other, exchange tasks, delegate parallel work, return artifacts/results and recover after Manager/Machine restart without message loss or duplicate task execution.

#### Phase 3: Agent safety and operational governance

- Runtime policy and Organization Approval for high-risk tools.
- A2A `AUTH_REQUIRED` integration.
- Token/time/concurrency budget and cancellation propagation.
- Audit, usage events, dead-letter and replay for Agent work.

Exit: an operator can constrain, cancel, approve and trace every multi-Agent task before public access is enabled.

#### Phase 4: Organization SaaS foundation

- Organization/workspace schema and migration.
- Multi-membership, roles, groups and tenant-aware IAM.
- Tenant credential boundaries, audit and entitlement skeleton.
- Durable inbox/outbox and multi-instance notification completion.

Exit: two Organizations share one deployment with verified isolation while the Agent Network remains functional.

#### Phase 5: Native collaboration and IM slice

- Collaboration Service with selected Message Plane integration.
- Native Web and Web Widget.
- Mixed human/Agent Channel, DM, Thread and Task.
- Full Organization Approval center and usage visibility.

Exit: an Organization can onboard a human team and 888a2a Agents, embed the widget, complete an approved task and inspect audit/usage.

#### Phase 6: Connector and enterprise expansion

- Connector framework and first target-market connector.
- Remaining Slack/Teams/LINE/WhatsApp adapters as independent deliveries.
- HA, backup/restore, retention/legal hold, quota enforcement and operator tooling.
- Marketplace reviews and customer-specific compliance work.

Exit: production SLOs and connector-specific acceptance suites are met.

## Risks / Trade-offs

- [Scope spans several products] → Ship one vertical slice at a time; no connector or Provider enters the core without an adapter contract.
- [WuKongIM v3 is beta] → Gate selection on a production spike; keep MessagePlane abstraction and a rollback candidate.
- [Two durable stores can drift] → Define explicit ownership, transactional outbox, idempotent projection and periodic reconciliation.
- [Connector features differ] → Maintain capability matrices and vendor extensions; expose delivery divergence instead of hiding it.
- [Connector marketplace approval is outside engineering control] → Start review early and keep Web Widget/native product independently sellable.
- [Multi-Agent loops can consume unbounded resources] → Enforce graph cycle detection, depth/fan-out, quotas, budgets and cancellation.
- [A2A task model differs from current tasks] → Introduce a compatible work model and migrate through adapters rather than overloading old statuses silently.
- [Tenant retrofit can leak data] → Make Organization mandatory at schema/API/cache/object-store/event boundaries and add adversarial isolation tests.
- [Approval becomes operationally complex] → Start with explicit policy templates, immutable requests and small state machine; add advanced escalation after core correctness.
- [Official SDKs use different languages] → Permit isolated connector sidecars behind versioned contracts while keeping the Go core authoritative.
- [Downstream diverges from upstream] → Upstream small neutral fixes; keep product-specific tenancy/connectors/A2A in the downstream roadmap.

## Migration Plan

1. Define the 888a2a identifier mapping and add compatibility readers/importers for existing config, data and credentials without emitting new legacy identifiers.
2. Rename build, runtime, API, UI, image and documentation surfaces to 888a2a and verify migration before deleting compatibility readers.
3. Add Organization resources and create one default Organization for every existing deployment.
4. Backfill existing principals, agents, machines, conversations, tasks, settings, IAM, files and audit records with the default Organization.
5. Add tenant-aware APIs and resource names behind a compatibility layer; reject new unscoped writes after backfill validation.
6. Introduce durable inbox/outbox and shared notification while retaining existing in-process fast paths as non-authoritative optimizations.
7. Add the MessagePlane adapter and dual-project new messages to old and new read models; compare order, unread, threads, reactions and attachments.
8. Cut native clients to the new Collaboration API per Organization feature flag; keep rollback to the old read path until reconciliation is clean.
9. Add A2A work records and adapt existing task/Agent delegation into the compatible model; publish Agent Cards only for opted-in Agents.
10. Roll out Approval and entitlement enforcement in observe-only mode, then deny/queue mode after policy coverage is verified.
11. Enable Web Widget, then one external connector for pilot Organizations; isolate connector credentials and queues per tenant.
12. Remove old unscoped APIs, process-local correctness assumptions and obsolete task/message fields only after the compatibility window and export verification.

Rollback is phase-scoped. Schema changes are additive until cutover. Connector and A2A exposure can be disabled per Organization. Message dual-projection remains until old/new parity is demonstrated. Runtime Provider upgrades retain the last verified package. No rollback deletes tenant data or audit history.

## Open Questions

- Which WuKongIM stable/beta version, deployment topology and browser SDK pass the production spike?
- Which external connector is first for the target market: LINE, Slack, Teams or WhatsApp?
- Which workloads run on SaaS-managed Machines versus customer-owned BYOC Machines?
- What are the default retention, legal-hold and raw connector payload periods?
- Which initial usage meters are customer-visible versus internal cost telemetry?
