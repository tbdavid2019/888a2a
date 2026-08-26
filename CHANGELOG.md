# Changelog

All notable changes to 888a2a are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
This project records changes by calendar date and does not maintain release versions.

## [2026-08-26]

### Added

- Added a gated PostgreSQL assignment integration test covering create/update/remove outbox intents, ordered replay, idempotent re-submit, cumulative ACK, and post-ACK empty replay.
- Added a dedicated verbose GitHub Actions assignment replay gate so durable Machine delivery evidence is visible in CI logs.
- Added a PostgreSQL-backed shared room notifier with a peer-replica integration gate for cross-Manager conversation wakeups.
- Added tenant-scoped PostgreSQL nonce replay storage and a shared-consume integration gate for multi-Manager heartbeat replay protection.
- Recorded CI evidence for completed OpenSpec task 3.4, including PostgreSQL durable assignment replay and ACK verification.
- Added durable Machine assignment delivery over MachineChannel, including full-log replay after process restart, ordered delta replay after reconnect, idempotent reducer application, and cumulative ACK handling.
- Added MachineChannel durable assignment replay/ACK messages and Manager-side assignment outbox delivery with per-Organization queue and concurrency limits.
- Added tenant-scoped membership administration RPCs/UI, delegated requester/executor audit evidence, composite Organization IAM bindings, collaboration projection tenant columns, and cache/S3 isolation guards.
- Added a real PostgreSQL Organization tenancy migration test covering existing-row default backfill plus tenant foreign-key and workspace uniqueness enforcement.
- Recorded PostgreSQL CI evidence for Organization fresh install, upgrade, existing-row backfill, foreign-key rejection, and uniqueness enforcement; OpenSpec tasks 2.2–2.4 are now checked.
- Added Product Identity Inventory document (`docs/product_identity_inventory.md`) mapping all legacy identifiers to approved `888a2a` targets and shell-safe `A2A888_` environment variables.
- Published draft Section 1 Architecture Decision Records (`docs/decisions/1.1-single-workspace-inventory.md` through `docs/decisions/1.8-saas-vs-byoc-scope.md`). External WuKongIM, connector, approval, and tenant integration evidence remains pending.
- Added multi-tenant Organization, Workspace, OrganizationMembership, and TenantPrincipal resource contracts in Protobuf (`proto/v1/a2a888/organization.proto`).
- Added `OrganizationService` Connect RPC service and handler (`backend/manager/api/v1/organization_service.go`) supporting `ListOrganizations`, `GetOrganization` (with indistinguishable denial against tenant probing), `SwitchOrganization`, `ListWorkspaces`, and `ListMemberships`.
- Added request header tenant resolution (`X-Organization-ID`, `X-Tenant-ID`) in auth interceptor (`backend/manager/api/auth/auth.go`), injecting active tenant context to all downstream RPCs.
- Enforced strict Agent tenant isolation and active owner verification in IAM evaluator (`backend/manager/component/iam/manager.go`).
- Wired `TenantObjectKey` S3 prefix isolation to production upload handlers (`channel_file_service.go`, `user_avatar_service.go`, `agent_avatar_service.go`).
- Added database migration `0028##organization-tenancy.sql` and `LATEST.sql` creating `organizations`, `workspaces`, `organization_memberships` tables, and adding foreign key indexes and `organization_id` columns across `file`, `task`, `audit_log`, `api_provider`, `user_group`, and `reminder`.
- Added `TenantCacheKey` and `TenantProjectionKey` helpers in `store` ensuring multi-tenant isolation for in-memory caches and localized data projections.
- Added static contract tests for default Organization backfill, collaboration resource foreign keys/indexes, IAM lifecycle matrix evaluation, adversarial input rejection, and S3 object key prefix collision resistance. Real PostgreSQL and cross-tenant integration tests remain pending.
- Added Go `OrganizationStore` (`backend/manager/store/organization.go`) providing transactional organization CRUD, slug lookup, workspace management, membership queries, and comprehensive unit tests.
- Added tenant-aware IAM permission evaluator (`CheckTenantPermission`, `CheckOrganizationActive`, `EvaluateMembershipPermission`) in `backend/manager/component/iam/manager.go` enforcing lifecycle states and role boundaries (strictly rejecting `INVITED` and `SUSPENDED` memberships).
- Added frontend `OrgSwitcher` component mounted into `DesktopSidebar` and `MobileSidebar` navigation (`frontend/src/components/sidebar.tsx`) with active tenant switching, suspended banner, and cache clearing on tenant switch.
- Added frontend `OrganizationSlice` (`frontend/src/stores/organization.ts`) with active organization selection, membership state tracking, and unit tests.

### Changed

- Added a dedicated verbose GitHub Actions PostgreSQL migration gate so fresh install, Organization upgrade, existing-row backfill, foreign-key, and uniqueness evidence is visible in CI logs.
- Renamed binaries (`build/888a2a`, `888a2a-machine`), CLI commands, Docker image targets (`888a2a/manager`, `888a2a/machine`), and build scripts (`scripts/build_888a2a.sh`, `scripts/build_888a2a_manager_docker.sh`, `scripts/build_888a2a_machine_docker.sh`) while providing backwards-compatible invocation wrappers.
- Renamed environment variables (`A2A888_PG_URL`, `A2A888_ALLOWED_ORIGINS`, `A2A888_COOKIE_SAMESITE`, `A2A888_MANAGER_URL`, `A2A888_DAEMON_SOCKET`, `A2A888_SESSION_TOKEN`, `A2A888_AGENT`, `A2A888_COMMAND`, `A2A888_TEST_CACHE`), home directory (`~/.888a2a`), socket paths, cookies (`888a2a-access-token`), headers (`X-888a2a-Agent`), and UI storage keys (`888a2a-sidebar-collapsed`, `888a2a.language`, `888a2a_oauth_state_`) with dual-read fallback compatibility.
- Updated UI localization files (`frontend/src/locales/en-US.json`, `frontend/src/locales/zh-CN.json`), frontend command helpers (`machine-token.ts`, `agent-token.ts`, `oauth.ts`), documentation (`docs/deploy.md`, `docs/deploy_zh.md`, `docs/test-server.md`), developer guides (`AGENTS.md`), and testserver launcher (`tools/testserver/`, `scripts/test-server.sh`, `scripts/build_test_server.sh`) to `888a2a` branding and `A2A888_` environment variables while preserving required upstream license/copyright attributions.
- Established wire-compatibility strategy for Protobuf services preserving legacy service package namespaces for backwards compatibility while exposing new multi-agent/organization capabilities under `proto/v1/a2a888/` (`a2a888.v1`) and generating code to `backend/generated-go/a2a888`.
- Updated authentication and permission evaluators to recognize both `888a2a.*` and legacy tokens and permission prefixes seamlessly.
- Migrated Go module path and package references repository-wide from `github.com/Ranxy/laelia` to `github.com/tbdavid2019/888a2a` across `go.mod`, 299 Go source files, `.proto` files, build scripts, Dockerfiles, and linter settings.
- Synchronized OpenSpec main specifications (`a2a-agent-network`, `agent-network-safety`, `agent-runtime-foundation`, `product-identity-migration`) and archived completed change `build-888a2a-agent-network-foundation`.

### Fixed

- Fixed a Pi session lifecycle race where a prompt response arriving during process exit could re-mark a reaped session as warm; added regression coverage for cold restart state.
- Fixed production tenant-header authorization by exposing organization membership lookup on the real Store implementation, and removed legacy procedure literals from auth tests so the naming gate remains green.
- Added bounded fair tenant queueing and wired per-Organization admission limits into outbox workers.
- Made legacy machine state migration copy an existing legacy home into `.888a2a` atomically before startup continues.
- Updated the active Dockerfiles and pi build documentation to use `A2A888_*` variables and `888a2a` runtime targets while retaining explicit legacy aliases.
- Added the tenant-scoped durable event envelope and PostgreSQL outbox schema with idempotency, claim, acknowledgement, retry, and dead-letter states; real PostgreSQL worker recovery remains an integration gate.
- Added the tenant-scoped connector inbox schema and idempotent external-event recording API; real webhook adapter fixtures remain an integration gate.
- Added an idempotent outbox worker loop with claim, handler, acknowledgement, retry, and backoff behavior; multi-worker PostgreSQL crash recovery remains an integration gate.
- Added the internal MessagePlane contract and deterministic fake-engine tests for tenant-scoped credentials, append/history cursors, deduplication, membership projection, and health.
- Connected Machine assignment persistence to the durable outbox in the same database transaction; replay/ACK behavior remains covered by the existing reducer tests while real multi-replica recovery remains pending.
- Added authorized dead-letter replay with tenant-scoped outbox reconciliation records.
- Added active-Organization write guards to Connector inbox and durable A2A create/update paths so suspended or closed tenants cannot enqueue new work.
- Added frontend compatibility resolution for both `888a2a.*` and legacy permission namespaces during the identity migration.
- Added PostgreSQL integration gates for outbox lease reclaim, connector event deduplication, and dead-letter replay/reconciliation.
- Added a bounded per-Organization execution limiter with deterministic fairness tests.
- Wired the bounded tenant queue into OutboxWorker so claimed events are delivered in fair per-Organization order before acknowledgement.
- Loaded the persisted default Organization, Agent tenant, workspace, and Machine tenant fields into runtime models before authorization and tenant-aware object-key generation.
- Added resource-level Organization checks for Agent, Machine, Conversation, File, Command, and Reminder IAM targets.
- Restricted Organization membership listing to active owners and admins, and rejected unknown active-organization candidates.
- Revalidated Organization task progress so only evidence-backed work remains checked in OpenSpec.

### Removed

- Removed obsolete upstream design documents in `docs/plan/` in favor of OpenSpec specifications and `docs/guide/` operator documentation.

## [2026-08-25]

### Added

- Added durable per-Machine assignment storage and event logging in PostgreSQL (`machine_assignments` & `machine_assignment_events`) with strictly monotonic sequence numbers, idempotency keys, and transaction boundary integrity.
- Added monotonic assignment stream cursor persistence (`state.LastAckCursor`) and replay protocol (`ApplyAssignmentReplay`) with gap/regression validation in `MachineClient`.
- Added idempotent Machine assignment apply (`ApplyAssignmentEvent`) preventing duplicate runners and zombie processes.
- Added full-roster reconciliation (`ReconcileRoster`) converging stale configs and reaping untracked zombie runners to authoritative high watermark and revision.
- Added per-Agent workspace path confinement (`ConfinePathToAgentWorkspace`), strict ownership assertion (`AssertAgentOwnership`), and environment isolation (`BuildIsolatedEnvironment`) preventing cross-Agent data or credential leakage across concurrent agents.
- Added Machine capacity and live Agent availability reporting (`IsAgentReadyForWork`, `GetAgentAvailability`, `GetCapacityReport`), advertising readiness only when agents and hosting machines are healthy, connected, and under saturation limits.
- Added multi-agent runner concurrency, timeout, cancellation, and crash isolation verification ensuring peer runners continue unperturbed when an individual agent times out or crashes.
- Added strict manifest digest calculation (`ComputeManifestDigest`) and validation across NPM, System, Embedded, and Custom runtime providers.
- Added runtime preparation metadata tracking (`.runtime_meta.json`) recording identity digest, manifest digest, package SRI integrity, binary sha256, size, and preparation timestamp.
- Added disk binary tamper detection and automatic isolation to quarantine directory (`quarantine/<identity_digest>.<timestamp>`) with path traversal protection.
- Added retry capability for failed runtime preparations.
- Added `RuntimeStatus` (`READY`, `BROKEN`, `QUARANTINED`, `UPDATE_AVAILABLE`, `DETECTED`) and `CompatibilityLevel` evidence fields to `AgentProviderInfo` proto definitions and manager validation.
- Added session launch fingerprint validation binding provider version, manifest digest, package integrity, runtime cache identity, and binary sha256.
- Connected Machine runtime preparation to Agent launch so ACP sessions use the verified local executable and runtime identity metadata.
- Added npm lockfile version and SRI verification before a package can become READY.
- Added system executable version probing and local executable resolution for system, embedded, and custom runtime manifests.
- Added tenant-ready A2A 1.0 HTTP+JSON gateway with standard protocol version negotiation (`A2A-Version`) and official SDK compatibility (`github.com/a2aproject/a2a-go/v2`).
- Added Agent Card projection from agent metadata, provider capabilities, and operational readiness, omitting disabled and private skills.
- Added authenticated Agent Directory service with tenant isolation, skill filtering, and live readiness state reporting.
- Added PostgreSQL durable storage for A2A work contexts, work records, artifact references, and monotonic event logs (`a2a888_work`, `a2a888_work_context`, `a2a888_work_artifact`, `a2a888_work_event`).
- Added idempotent send-message acceptance, durable work creation before acknowledgement, and lost response retry safety.
- Added get-task and tenant-isolated list-tasks with cursor pagination and peer work isolation.
- Added event broker with durable event log replay from sequence markers and live subscription delivery across streaming clients.
- Added terminal-state idempotent task cancellation preventing corruption of completed work.
- Added additive projections linking A2A delegation, states, and artifacts to conversation and task message threads.
- Added Manager restart recovery for active work, safely resuming or transitioning interrupted tasks without duplicate execution.
- Added 888a2a Agent tools (`PeerList`, `PeerGet`) for peer and skill discovery with Agent Card capabilities and verified operational readiness (`READY`, `BUSY`, `OFFLINE`, `UNAVAILABLE`).
- Added idempotent A2A task sending tool (`TaskSend`) supporting context ID, parent delegation edges, budget constraints, trace correlation, and target wake-up without polling.
- Added audit-safe A2A delegation, status update, and terminal outcome projections for source task threads (`FormatThreadSummary`, `FormatDelegationSummary`, `FormatStatusUpdateSummary`, `FormatResultSummary`) exposing peer, state, trace, and artifacts while strictly excluding hidden reasoning and secrets.
- Added parent/child work edges and cycle detection (`CycleDetector`, `TaskGraph`) rejecting direct and indirect delegation cycles before commit.
- Added delegation depth, child count, fan-out, concurrency, retry, and token/work-unit budget enforcement (`ValidateDelegationLimits`, `ValidateFanOutLimit`, `ValidateConcurrencyLimit`, `ValidateBudgetAvailability`, `AllocateChildBudget`) returning durable policy-limit events.
- Added parallel fan-out coordinator (`ExecuteFanOut`) supporting `ALL_SUCCESS`, `PARTIAL_FAILURE`, `QUORUM`, and `FIRST_SUCCESS` join policies, concurrency bounds, timeout cancellation, and deterministic index-aligned aggregation of peer specialist results.
- Added root tree cancellation propagation (`CancelWorkTree`, `EnsureTerminalState`) terminating active subprocess runtimes, blocking new child delegations, and driving all descendants to observable terminal states.
- Added focused runtime safety policy (`RuntimePolicy`, `DefaultRuntimePolicy`, `EvaluateACPPermission`) replacing unconditional ACP permission approvals with default-deny rules for unapproved shell, write, network, secret, and side-effecting MCP operations.
- Added canonical path confinement (`ValidatePathConfinement`) for configurable workspace-read access, strictly denying directory traversal, symlink escapes, and cross-Agent workspace probes.
- Added Agent Network audit and trace recording (`TraceRecorder`, `SanitizeMetadata`) capturing discovery, delegation, runtime session, permission, budget, retry, cancellation, and terminal outcome events while scrubbing credentials and hidden reasoning.
- Added comprehensive 12-Agent acceptance test suite (`TestTwelveAgentAcceptanceGate`) verifying 2-Machine topology, 10-specialist fan-out/join, Manager restart recovery, Machine disconnect/reconnect cursor replay, lost response deduplication, cancellation propagation, and cross-Agent security isolation.
- Published the 888a2a Agent Network Operator Guide (`docs/guide/agent-network-operator-guide.md`) detailing architecture, environment variables, provider manifests, security defaults, step-by-step deployment, and troubleshooting runbooks.
- Added top-level multi-stage `Dockerfile`, `docker-compose.yml`, and `docker-compose.example.yml` for zero-Go server deployment.
- Published the Docker Deployment & Installation Guide (`docs/guide/docker-deployment-guide.md`) with one-command startup, volume persistence, and PostgreSQL maintenance runbooks.
- Added the 888a2a project README and Traditional Chinese README with the
  project direction, upstream attribution, roadmap, and major TODO areas.
- Added GitHub Actions CI for Agent Network naming checks, Go lint/tests/build,
  and frontend format/lint/type-check/tests/build.
- Added Node.js 24 as the frontend CI and release workflow runtime.
- Added SLSA generic provenance generation for release binaries.

### Changed

- Updated Agent prompt, communication guide, and re-anchor instructions to use A2A tasks for work delegation while retaining Channel/DM for conversational collaboration, and explicitly rejecting direct process control, shared memory, and busy-polling assumptions.
- Updated Claude Code provider manifest to pinned `@agentclientprotocol/claude-agent-acp@0.70.0` with verified SRI integrity sha512 hash and eliminated turn-time npx download fallback.
- Updated Manager API provider validation to prohibit quarantined, broken, and unverified detected-only providers from automatic execution.
- Updated Manager frontend UI to display runtime status and compatibility evidence badges and disable unusable providers from selection.
- Updated Dispatcher to check live machine connection, command in-flight saturation, and administrative status before routing work to agents.
- Established `tbdavid2019/888a2a` as the public repository while retaining
  `Ranxy/laelia` as the upstream source.
- Normalized existing frontend imports so the Biome CI check passes.

### Fixed

- Made the tampered Agent JWT test modify the payload reliably instead of
  changing a Base64URL signature character whose unused bits may decode to the
  same bytes.

## How to update this file

Add every meaningful change directly to the section for today's date (`## [YYYY-MM-DD]`) before committing.
Do not use `[Unreleased]`; all entries are organized strictly by date.
