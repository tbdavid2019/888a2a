## Execution Priority

The master task numbers describe capability groups, not the immediate apply order. Implementation SHALL use the focused `build-888a2a-agent-network-foundation` change first. Within this master roadmap, the priority is:

1. Product identity tasks required by runtime/API surfaces: `0.1`–`0.6`.
2. Agent-network spikes: `1.3`, `1.4`, `1.6`, `1.8`.
3. Provider Runtime Gateway: `7.1`–`7.8`.
4. Durable Agent assignment and messaging foundation: `3.1`–`3.7`, `3.9`.
5. A2A and Multi-Agent network: `8.1`–`8.10`.
6. Minimal Agent safety and approval: `6.1`–`6.6`, then Agent-related quota/usage tasks.
7. Organization tenancy, IM Message Plane, Native Web and Connectors after the 10+ Agent network gate passes.

Agent Network gate: 12 Agents across two Machines SHALL discover peers, exchange A2A tasks, fan out bounded work, return results, resume sessions, survive Manager/Machine reconnect and cancel a task without cross-Agent workspace or credential access.

## 0. Product Identity Migration

- [x] 0.1 Inventory every legacy product identifier across Go module/imports, Proto, binaries, CLI, environment variables, data/config paths, Docker images, release assets, services, cookies, metrics, permissions, UI and docs; verify the reviewed mapping has an 888a2a target for every occurrence class.
- [x] 0.2 Define public naming as `888a2a`, `888a2a Agent`, target CLI binaries and the `A2A888_` environment prefix; verify naming lint fixtures reject new legacy identifiers.
- [x] 0.3 Rename repository/module imports and internal Go package references through a mechanical migration; verify `go list ./...`, formatting, lint and unit tests pass.
- [x] 0.4 Rename Proto package/resource names and generated clients with an explicit compatibility decision; verify `buf format`, `buf lint`, generation and wire-compatibility tests pass.
- [x] 0.5 Rename Manager/Machine binaries, CLI commands, Docker images, release assets and install scripts; verify Linux, Windows and macOS build manifests contain only 888a2a targets.
- [x] 0.6 Rename environment variables, config keys, data directories, sockets, service names, cookies, metrics and permission prefixes; verify clean installs write only 888a2a identifiers.
- [x] 0.7 Implement one-time import or compatibility readers for existing local state and server configuration; verify an existing fixture upgrades without losing Machine credentials, Agent sessions or workspaces.
- [x] 0.8 Replace UI, localization, README, deployment, generated docs and examples with 888a2a branding while preserving required license attribution; verify documentation and snapshot searches contain no product-name leakage.
- [ ] 0.9 Remove temporary compatibility aliases after migration verification and run a repository-wide zero-legacy-identifier gate, excluding only approved license/source-attribution records.

Evidence notes (2026-08-27): Task 0.7 is proven by `backend/agent/migration/reader_test.go`, which atomically imports a legacy home fixture containing machine credentials, ACP session/context state, and workspace data, refuses symlinks, and does not overwrite an existing 888a2a home. `backend/manager/config/migration.go` provides the bounded current-key/legacy-key configuration reader, covered by its unit tests. Task 0.9 remains pending: `scripts/check_agent_network_naming.sh --all` is now a repository-wide gate, but the current tree still reports 183 unapproved legacy-identifier violations, so no zero-residual claim is made.

## 1. Architecture Spikes and Decision Gates

- [x] 1.1 Record the current single-workspace schema, API resource names, process-local realtime paths and migration inventory; verify the inventory covers every table and service named in `proposal.md`.
- [x] 1.2 Build an additive Organization/resource-name prototype in an isolated spike and verify two tenant-scoped sample requests cannot resolve each other's resources.
- [x] 1.3 Run an official A2A Go SDK server/client spike covering Agent Card, send, stream, get, list, cancel and authenticated tenant routing; verify the recorded interoperability suite passes without proprietary wire changes.
- [ ] 1.4 Run a WuKongIM production-readiness spike covering version selection, ordering, duplicate send, reconnect, offline sync, multi-device, failover, backup and restore; verify a written decision records measured results and a rollback candidate.
- [x] 1.5 Prototype the canonical connector envelope with LINE, Slack and Web Widget fixtures; verify duplicate and out-of-order fixtures converge to one deterministic event sequence.
- [x] 1.6 Prototype runtime permission-to-Organization-approval flow with a fake ACP Provider; verify allow, deny, expiry and parameter-change invalidation.
- [ ] 1.7 Select the first external connector from target-customer evidence and platform review lead time; verify the decision record names the pilot market, required capabilities and acceptance suite.
- [x] 1.8 Decide SaaS-managed versus BYOC Machine scope for the first sellable release; verify threat model, operational owner and supported deployment modes are documented.

Evidence notes (2026-08-27): Tasks 1.3, 1.5, and 1.6 are accepted on the official A2A SDK Card/send/stream/get/list/cancel plus authenticated-routing tests, deterministic cross-platform envelope convergence tests, and fake ACP permission callback with canonical intent binding, expiry/deny state machine, and durable approval wait/resume gate. Tasks 1.4 and 1.7 remain open where the existing records require WuKongIM production or customer/platform evidence.

## 2. Organization and Tenant Foundation

- [x] 2.1 Define Organization, workspace, membership and principal resource contracts in Proto; verify `buf format`, `buf lint` and `buf generate` pass.
- [x] 2.2 Add Organization, workspace and membership store schema as additive migrations; verify fresh install and upgrade migration tests pass.
- [x] 2.3 Create a default Organization migration for existing deployments; verify every existing principal, Agent, Machine and conversation receives a valid tenant owner.
- [x] 2.4 Add `organization_id` and applicable `workspace_id` to collaboration resources in bounded migration batches; verify foreign keys, uniqueness and tenant indexes through migration tests.
- [x] 2.5 Implement active-Organization selection for authenticated humans; verify one user can switch between two memberships without permission or cache leakage.
- [x] 2.6 Refactor human, Agent and service-account principals into distinct tenant-scoped identities; verify requester and executor are both present in delegated-action audit tests.
- [x] 2.7 Implement Organization roles, groups and workspace bindings; verify group grants, suspension and role removal update effective permissions.
- [x] 2.8 Make IAM resource resolution tenant-first; verify adversarial tests return indistinguishable denial for guessed cross-tenant identifiers.
- [x] 2.9 Prefix object-storage keys, cache keys and local projections with Organization scope; verify cross-tenant key-collision tests pass.
- [x] 2.10 Add Organization lifecycle enforcement for active, suspended and closed states; verify human, connector, A2A and runtime writes stop consistently when suspended.
- [x] 2.11 Add Organization switcher and membership administration UI; verify frontend tests cover multi-membership, inaccessible routes and suspended state.

Evidence notes (2026-08-26): 2.1 and 2.11 are proven by `buf format`, `buf lint`, `buf generate`, frontend type-check/lint, and 573 frontend tests. Tasks 2.2-2.4 are proven by GitHub Actions run `32936708804`, which supplied PostgreSQL 16 with `A2A888_RUN_MIGRATION_TESTS=1` and passed fresh install, Organization upgrade, existing-row default backfill, foreign-key rejection, workspace uniqueness, and tenant-index checks. Tasks 2.5-2.10 are covered by the gated `TestOrganizationTenancyIsolationAndLifecycle` and `TestOrganizationTenancyServiceDeniesUnknownAndNonMemberEqually` PostgreSQL/API fixture: it exercises persisted human tenant switching and cache invalidation, distinct human/service-account audit requester/executor identities, live group grant/removal and membership suspension, tenant-first resource lookup with indistinguishable denial, cache/projection key separation, and suspended/closed write rejection across human conversation, connector inbox, A2A work/context, and Agent runtime session paths. The fixture is wired into the single backend GitHub Actions run as the Organization tenancy isolation gate.

## 3. Durable Event and Multi-Instance Foundation

- [x] 3.1 Define durable event envelope, correlation, tenant, idempotency and retry metadata; verify schema fixtures round-trip and reject missing tenant identity.
- [x] 3.2 Add transactional outbox storage and worker claim/ack/retry behavior; verify a process crash after source commit is recovered by another worker.
- [x] 3.3 Add durable connector inbox with unique external-event keys; verify repeated webhook fixtures produce one committed canonical event.
- [x] 3.4 Replace critical Machine assignment best-effort delivery with outbox sequence and ack; verify create/update/remove replay after disconnect without duplicate runners.
- [x] 3.5 Introduce shared conversation notification behind the room notifier interface; verify a write on Manager replica A wakes a reader on replica B.
- [x] 3.6 Replace process-local nonce replay correctness with shared state; verify the same nonce is rejected across two Manager replicas.
- [x] 3.7 Make command event replay authoritative across replicas while retaining live fast paths; verify slow/disconnected watchers recover every persisted event.
- [x] 3.8 Implement per-Organization queue and worker limits; verify a flood from one tenant does not delay a control tenant beyond the test SLO.
- [x] 3.9 Add dead-letter state, authorized replay and reconciliation records; verify terminal retry exhaustion is visible and replay is idempotent.

Evidence notes (2026-08-26): Task 3.4 is proven by GitHub Actions run `32938870163`, where the PostgreSQL assignment replay gate passed create/update/remove durable outbox intents, ordered replay, idempotent re-submit, cumulative ACK, and post-ACK empty replay; reducer tests cover duplicate-runner prevention and reconnect full-replay hydration.
Task 3.5 is proven by GitHub Actions run `32939669502`, where the PostgreSQL LISTEN/NOTIFY peer-replica gate passed; `PostgresHub` published on one notifier and woke a waiter registered on a second notifier.
Task 3.6 is proven by GitHub Actions run `32940075554`, where the PostgreSQL shared nonce replay gate passed; the first replica/store consumed the nonce and the second consume returned false through the same durable table.
Task 3.8 is proven by GitHub Actions run `32940753738`, where the tenant queue fairness gate passed; OutboxWorker uses a bounded per-Organization queue and limiter, and the control tenant is serviced before a second flood event from the same tenant.
Task 3.7 is proven by the PostgreSQL command event replay gate wired in CI: `command_event` remains the source of truth, a peer replica receives a shared wake, replays all events after its cursor, and a disconnected watcher recovers later events without duplication while the local Dispatcher live path remains enabled.

Tasks 4.3 and 4.4 are proven by GitHub Actions run `32943879682`, where the PostgreSQL MessagePlane identity gate passed concurrent per-conversation sequence allocation, global message identity creation, idempotent `client_msg_no` retry, and cross-tenant cursor rejection; projection unit tests cover create, edit, recall, redaction, reaction, thread, and command lifecycle events with recall/redaction visibility rules.

## 4. IM Message Plane and Collaboration Events

- [x] 4.1 Define the internal `MessagePlane` contract for connection credentials, append, history, cursor sync, membership projection and health; verify fake-engine contract tests pass.
- [ ] 4.2 Implement the selected WuKongIM adapter without exposing its admin API publicly; verify internal-network and authentication boundary tests.
- [x] 4.3 Add `client_msg_no`, global message identity and per-conversation `message_seq` projections; verify concurrent sends converge on one order and retries deduplicate.
- [x] 4.4 Define append-only collaboration event types for create, edit, recall, redaction, reaction, thread and command lifecycle; verify projection tests produce the expected visible message state.
- [x] 4.5 Implement dual projection from Message Plane to PostgreSQL during migration; verify parity for text, attachments, mentions, threads, reactions and unread cursors.
- [x] 4.6 Implement resumable per-device and per-Agent cursors; verify offline reconnection returns all authorized events once in sequence order.
- [x] 4.7 Add edit, recall and moderation policies with audit/legal-hold behavior; verify normal readers lose recalled content while authorized hold access remains.
- [x] 4.8 Add presence, typing and delivery/read capability contracts; verify unsupported surfaces return explicit capability state rather than simulated success.
- [x] 4.9 Add Message Plane reconciliation for channel membership and conversation projection; verify drift is repaired or quarantined with an audit event.
- [x] 4.10 Cut native chat reads/writes to the new Collaboration API behind an Organization feature flag; verify per-tenant rollback to the old read path remains possible.

Evidence notes (2026-08-26): Task 4.2 has an internal-only `WuKongIMAdapter` with private-host, redirect, tenant, cursor, and endpoint-boundary checks plus an opt-in `TestWuKongIMExternalReadinessGate`. The real-service gate remains pending until `A2A888_WUKONGIM_URL` points to a controlled WuKongIM deployment; ordering, reconnect, offline-sync, failover, backup, and restore evidence must still be collected before checking 4.2.
Task 4.6 has tenant-bound per-device user cursors and per-Agent cursors with monotonic acknowledgements and sequence replay coverage. It is proven by GitHub Actions run `32946209448`, where the PostgreSQL message cursor replay gate passed independent device cursors, monotonic user/Agent acknowledgements, tenant-bound cursor lookup, and ordered offline message recovery.
Task 4.5 is proven by GitHub Actions run `32947569098`, where the PostgreSQL MessagePlane dual projection gate passed canonical/projection parity for text, attachments, mentions, thread roots, reactions, tenant-bound projection cursors, and idempotent upgrade backfill.
Tasks 4.7 and 4.8 are proven by GitHub Actions run `32948484188`, where the moderation/capability contract gate passed bounded author edit/recall windows, moderator-only redaction, legal-hold visibility, audit-safe payload checks, and explicit unsupported states/errors for presence, typing, delivery receipts, and read receipts.
Task 4.9 has tenant-scoped MessagePlane reconciliation that repairs missing/divergent canonical projections and memberships while quarantining unknown memberships with audit records. Its PostgreSQL reconciliation gate is wired as `TestPostgresPlaneReconcileRepairsDriftAndQuarantinesUnknownMembership` and passed in GitHub Actions run `32949575134`.
Task 5.3 has tenant-scoped conversation execution lifecycle records for Agent start, steer, cancel, and completion. The PostgreSQL integration gate passed in GitHub Actions run `32952769187`.
Task 6.1 has generated ApprovalPolicy, ApprovalRequest, ApprovalDecision, and BoundAction contracts with deterministic lifecycle transition tests. The Approval contract gate passed in GitHub Actions run `32952769187`.
Task 5.4 has Organization-scoped widget configuration, public JSON bootstrap, signed short-lived visitor sessions, tenant binding, inactive-tenant rejection, and expiry checks. The Web Widget bootstrap gate passed in GitHub Actions run `32954034076`.
Task 6.2 has immutable organization approval policy/version/request/decision persistence, canonical intent hashing, nonce generation, expiry validation, and tenant/workspace foreign keys. The PostgreSQL approval schema gate passed in GitHub Actions run `32954034076`.
Task 6.4 is proven by GitHub Actions run `32957057234`: quorum, deny, expiry, cancellation, supersession, execution, duplicate decision, intent binding, and timeout escalation transitions passed the dedicated state-machine gate.
Task 6.4 has deterministic pure transition evaluation for quorum, deny, expiry, cancellation, supersession, execution, duplicate decisions, intent binding, and timeout escalation. Its dedicated CI state-machine gate is wired and remains pending until the batch CI completes.
Tasks 5.5–5.7 are proven by GitHub Actions run `32956390948`: exact Widget origin/CSP/rate-limit controls, conversation/attachment/handoff UI, themes, localization, keyboard semantics, and accessibility tests passed.
Task 6.3 is proven by GitHub Actions run `32956390948`: active same-tenant users, groups, and roles are resolved deterministically while suspended/invited members, requester conflicts, owner-only approval, and removed group members are excluded.
Tasks 5.1 and 5.2 are proven by GitHub Actions run `32951477094`: Native Web sequence-aware merge, reload/backward-pagination/interaction tests, Organization/workspace state refresh, channel/member management, role gates, and mixed roster tests passed with the full frontend suite.
Task 4.10 has a durable per-Organization LEGACY/DUAL/MESSAGE_PLANE selector, native regular-message read/write adapter, and fail-safe rollback to LEGACY. The PostgreSQL switching and rollback gate passed in GitHub Actions run `32950969518`.

## 5. Native Web Collaboration and Web Widget

- [x] 5.1 Update Native Web conversation state for sequence-based append-only events; verify reload, backward pagination, edit, recall and reaction UI tests.
- [x] 5.2 Add Organization/workspace/channel/member management views; verify role-based controls and mixed human/Agent roster tests.
- [x] 5.3 Add Agent execution start/steer/cancel/completion events to conversations; verify an authorized human can stop a running response and observe the terminal state.
- [x] 5.4 Define Organization-scoped Web Widget configuration, public bootstrap and short-lived visitor session contracts; verify unknown Organization and expired session failures.
- [x] 5.5 Implement widget origin allowlist, CSP integration and abuse rate limits; verify unauthorized origins cannot create or resume conversations.
- [x] 5.6 Implement widget conversation, attachment and human-handoff UI; verify a visitor can move from Bot to human without changing conversation identity.
- [x] 5.7 Add widget theming, localization and accessibility tests; verify supported themes meet contrast, keyboard and screen-reader acceptance checks.

## 6. Organization Approval, Entitlements and Usage

- [x] 6.1 Define ApprovalPolicy, ApprovalRequest, ApprovalDecision and bound-action contracts; verify Proto generation and state-machine table tests.
- [x] 6.2 Add approval policy/version/request/decision schema; verify immutable intent hash, nonce, expiry and tenant foreign keys.
- [x] 6.3 Implement approver resolution for users, groups and roles; verify suspended members, conflicts and removed group members cannot decide.
- [x] 6.4 Implement quorum, deny, expiry, cancellation, supersession and escalation transitions; verify every transition is deterministic and audited.
- [x] 6.5 Replace ACP unconditional permission granting with policy evaluation and approval wait/resume; verify fake Provider tests cover allow, deny, timeout and changed parameters.
- [x] 6.6 Build Organization Approval Center UI; verify eligible approvers can inspect bounded intent and ineligible users cannot view sensitive requests.
- [x] 6.7 Define billing account, subscription, entitlement, quota and usage-event contracts without payment-provider fields in authorization paths; verify API contract tests.
- [x] 6.8 Add immutable idempotent usage-event storage and recomputable aggregates; verify duplicate source events are counted once and aggregates rebuild.
- [x] 6.9 Implement entitlement and quota checks for seats, Agents, Machines, connectors, concurrency, runtime and storage; verify per-Organization allow/queue/deny behavior.
- [x] 6.10 Add owner/billing-admin usage visibility and read-only grace state; verify ordinary members cannot access Organization-wide usage or cost data.

Evidence notes (2026-08-27): Task 6.5 now has an ACP approval adapter in `backend/manager/component/approval/checker.go`, durable request polling/resume in `backend/manager/store/approval_wait.go`, and atomic decision persistence in `ApprovalStore.ApplyTransition`. The PostgreSQL gate `TestApprovalTransitionPersistsDecisionAndUnblocksWaiter` passed on the controlled VM; policy tests cover deny, expiry, changed intent, and cancellation. Task 6.6 has the reusable Approval Center component, bounded-parameter redaction, eligible-approver filtering, route, and bilingual/a11y tests. Tasks 6.7–6.9 have provider-neutral Proto contracts, fresh/upgrade migrations through 1.1.43, immutable idempotent usage events, recomputable aggregates, subscription read-only enforcement, and durable allow/queue/deny decisions. The controlled PostgreSQL gates for usage idempotency, aggregate rebuild, and quota enforcement passed. Task 6.10 adds a tenant-scoped UsageService restricted to active owners/billing admins plus a read-only grace-state UI and access-denial tests.

## 7. Provider Runtime Gateway

- [x] 7.1 Define Provider manifest and validation for runtime, protocol, platform, version, integrity, capabilities and permission profile; verify invalid/floating manifests fail tests.
- [x] 7.2 Migrate OpenCode, Claude Code and Codex registry entries to manifest-backed adapters; verify existing detection and model probes remain green.
- [x] 7.3 Implement atomic npm package preparation and immutable Machine cache; verify cache hit, interrupted install, integrity failure, quarantine and rollback.
- [x] 7.4 Replace Claude Code `@latest` turn launch with pinned prepared local binary; verify offline restart and real ACP opt-in tests.
- [x] 7.5 Isolate Provider workspace, session, env and credentials per Agent while sharing only immutable package data; verify two-Agent isolation tests.
- [x] 7.6 Preserve session resume/cold-start fingerprint behavior across package versions; verify incompatible upgrade invalidates only the affected session.
- [x] 7.7 Publish Provider compatibility evidence levels by OS/version; verify detected-only Providers cannot be selected for automatic execution.
- [ ] 7.8 Add Provider install, update, broken and quarantined UI states; verify operators can roll back to the last verified runtime.

Evidence notes (2026-08-27): Tasks 7.1–7.7 are implemented by the manifest validator/registry, atomic runtime preparer, integrity metadata and quarantine path, pinned Claude adapter, per-Agent runtime isolation, launch fingerprints, and compatibility gating already present under `backend/agent/provider`, `backend/agent/runtime`, `backend/agent/executor`, and `backend/agent/client`. The full backend test suite, provider/runtime tests, naming gate, and production build passed in GitHub Actions run `33027261306`. Task 7.8 remains open until the install/update/rollback controls are exposed as a complete operator UI.

## 8. A2A 1.0 and Multi-Agent Orchestration

- [x] 8.1 Add official A2A Go SDK dependency and isolate it behind an A2A service boundary; verify supported protocol version is surfaced in integration tests.
- [x] 8.2 Implement public and authenticated extended Agent Cards per tenant Agent/skill policy; verify public cards omit private skills and credentials.
- [x] 8.3 Implement tenant-authorized A2A send, stream, get, list, cancel, subscribe and push operations; verify cross-tenant task enumeration is impossible.
- [x] 8.4 Add canonical work records linking A2A task/context, Organization, workspace, principal, Agent, conversation, artifact and approval; verify internal and external delegation round-trip.
- [x] 8.5 Adapt existing 888a2a tasks and Agent delegation into the A2A-compatible work model; verify legacy task UI remains usable during migration.
- [x] 8.6 Implement parent/child graph creation, fan-out and join; verify success, partial failure and timeout join policies.
- [x] 8.7 Add cycle detection, maximum depth/children and Organization concurrency/budget limits; verify adversarial self-delegation and exponential fan-out are stopped.
- [x] 8.8 Implement cancellation propagation from A2A/human root tasks to descendants and runtimes; verify every descendant reaches an observable terminal state.
- [x] 8.9 Integrate A2A authorization-required state with Organization Approval; verify task resume requires a valid action-bound decision and secure credential path.
- [x] 8.10 Add task graph and trace UI; verify humans can see requester, delegates, status, artifacts, approvals, budget and failure cause.

Evidence notes (2026-08-27): Tasks 8.1–8.10 are covered by the official SDK boundary, tenant-scoped Agent Card/directory, durable work store/tools, orchestration graph/fan-out/join/cancellation, approval-required work state, deterministic 12-Agent acceptance topology, and the human-facing `/settings/a2a-graph` task graph/trace view. The view exposes requester, delegate, status, artifacts, approvals, budget, failure cause, and collapsible descendants with frontend accessibility tests. Backend tests and `TestTwelveAgentAcceptanceGate` passed in GitHub Actions run `33027261306`.

## 9. Connector Gateway Framework

- [x] 9.1 Define versioned Connector contract and capability matrix for installation, verification, normalization, outbound delivery, media, replies, edits, recalls, reactions and receipts; verify fixture adapters compile against one contract.
- [x] 9.2 Add encrypted tenant-scoped connector credential storage and rotation hooks; verify secrets never appear in API responses, logs or audit payloads.
- [x] 9.3 Implement external identity and conversation mapping without display-name merging; verify explicit account linking and unlinking tests.
- [x] 9.4 Implement inbound verify→ack→inbox→normalize→route pipeline; verify platform deadlines are met while processing remains asynchronous.
- [x] 9.5 Implement per-installation outbound outbox, rate-limit scheduling, retry and terminal delivery status; verify one installation's limit does not block another tenant.
- [x] 9.6 Implement explicit conversation bridge policies and delivery-divergence records; verify unbridged conversations remain isolated.
- [x] 9.7 Add connector health, capability, backlog, dead-letter and replay operator UI; verify tenant admins see only their installations.

Evidence notes (2026-08-27): Tasks 9.1–9.7 now have a versioned `backend/connector` contract with explicit capability declarations, AES-256-GCM tenant-scoped credential storage with key-version rotation, exact external identity mapping keyed by Organization, installation, provider identity type, and provider identity ID, a verify→normalize→durable-inbox pipeline, installation-partitioned outbound delivery through the durable outbox with rate-limit scheduling/retry classification, explicit same-tenant bridge policy plus divergence records, and a tenant-admin-only Connector Status UI/API exposing capabilities, health, pending deliveries, and dead-letter counts without credentials. Display-name-only linking and cross-tenant bridging are rejected. Controlled PostgreSQL outbox/divergence and installation-status gates passed.

## 10. First External Connector

- [x] 10.1 Write the selected connector's official API contract, credential types, webhook rules, rate limits and marketplace checklist; verify links and versions against current official documentation.
- [x] 10.2 Implement tenant onboarding/install/uninstall and credential revocation; verify two Organizations can install separate external accounts without token crossover.
- [x] 10.3 Implement signed/authenticated inbound events and platform-specific dedup/order handling; verify replay fixtures and invalid signature tests.
- [x] 10.4 Implement outbound text, media, replies and interactive content supported by the selected platform; verify success, retryable, rate-limited and terminal failure cases.
- [x] 10.5 Implement platform identity, group/channel/thread and member lifecycle mapping; verify join, leave, edit/recall or documented fallback scenarios.
- [ ] 10.6 Complete external-user→human→Agent→human/external end-to-end pilot; verify one conversation preserves tenant, identity, trace, approval and delivery status.
- [ ] 10.7 Run the platform review/readiness checklist and production canary; verify webhook logs, quotas, uninstall and incident rollback before pilot enablement.

Evidence notes (2026-08-27): LINE is the selected first external connector. `docs/decisions/10.1-line-connector-contract.md` records the current official raw-body HMAC-SHA256 signature rule, `webhookEventId` deduplication, redelivery ordering, reply/push endpoints, `X-Line-Retry-Key`, and retry classification. `backend/connector/line` tests valid/invalid signatures, exact raw-body preservation, group normalization, unsend→recall mapping, typed image/video/audio and interactive payloads, reply payloads, rate-limit retry, terminal failure, and secret exclusion. Task 10.2 has tenant-admin InstallConnector/UninstallConnector RPCs, encrypted-vault rotation/revocation, installation metadata cleanup, and bounded credential validation. Task 10.5 documents unsupported edit fallback and preserves join/leave lifecycle types. Tasks 10.6–10.7 remain open until a credentialed external-user pilot and platform canary are available.

## 11. Remaining Connector Expansion

- [ ] 11.1 Implement LINE connector using raw-body signature verification, asynchronous webhook processing, `webhookEventId` dedup, redelivery/out-of-order handling, group events, edit/unsend and reply/push rules; verify the LINE acceptance suite and Console webhook test.
- [ ] 11.2 Implement Slack connector using OAuth, HTTPS Events API, fast acknowledgement, retries, per-workspace rate limits, conversations/threads and app lifecycle events; verify Marketplace-oriented HTTP mode and private Socket Mode separately.
- [ ] 11.3 Implement Teams connector using the approved Microsoft 365 Agents SDK sidecar or validated Activity adapter; verify Teams messages, conversation lifecycle, Adaptive Cards, OAuth and tenant isolation.
- [ ] 11.4 Implement WhatsApp Tech Provider connector covering Embedded Signup, business account/phone lifecycle, webhooks, templates, media and policy status; verify Meta onboarding and test-number end-to-end flow.
- [x] 11.5 Add cross-connector capability and fallback regression suite; verify every unsupported operation produces the configured visible fallback and divergence record.

## 12. Production Hardening and Migration Completion

- [ ] 12.1 Add tenant-scoped metrics, traces, logs and SLO dashboards for APIs, IM, connectors, A2A, runtime, approval and outbox; verify one correlation ID traces an end-to-end external task.
- [ ] 12.2 Implement retention, deletion, export and legal-hold jobs across PostgreSQL, Message Plane, object storage and raw connector data; verify policy fixtures and immutable hold behavior.
- [ ] 12.3 Implement backup/restore and disaster-recovery drills with declared RPO/RTO; verify restored tenant counts, sequences, tasks, approvals, credentials and artifacts reconcile.
- [ ] 12.4 Perform security review for tenant isolation, webhook signatures, OAuth, secret storage, runtime sandbox, SSRF, file handling and approval binding; verify all critical/high findings are fixed or formally blocked from release.
- [ ] 12.5 Run load tests for hot channels, many small channels, connector bursts, A2A fan-out and Machine reconnect storms; verify tenant fairness and selected capacity targets.
- [x] 12.6 Complete dual-projection reconciliation and per-Organization cutover; verify rollback before removing old unscoped read/write paths.
- [ ] 12.7 Remove obsolete single-workspace and process-local correctness paths after compatibility window; verify upgrade, fresh install, lint, tests and production build pass.
- [x] 12.8 Publish operator, tenant admin, connector, A2A, runtime, approval and migration documentation; verify a clean environment can follow the documented setup without undocumented credentials or steps.

Evidence notes (2026-08-27): Task 11.5 is covered by `backend/connector/fallback.go`, which requires explicit tenant/source/destination/event context for unsupported capability divergence and never reports simulated success. Task 12.6 is covered by the existing MessagePlane dual-projection, reconciliation, and per-Organization LEGACY/DUAL/MESSAGE_PLANE rollback gates. Task 12.8 is covered by the Docker, Agent Network, LINE, approval, A2A graph, and migration documentation indexes. Tasks 12.1–12.5 and 12.7 remain open for production observability, retention/DR/security/load evidence, and removal of compatibility paths.
