# Changelog

All notable changes to 888a2a are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project uses [Semantic Versioning](https://semver.org/) for releases.

## [2026-08-26]

### Added

- Added multi-tenant Organization, Workspace, OrganizationMembership, and TenantPrincipal resource contracts in Protobuf (`proto/v1/a2a888/organization.proto`).
- Added database migration `0028##organization-tenancy.sql` creating `organizations`, `workspaces`, `organization_memberships` tables and seeding default organization boundaries across existing principals, agents, machines, and conversations.
- Added Go `OrganizationStore` (`backend/manager/store/organization.go`) providing transactional organization CRUD, slug lookup, workspace management, membership queries, and comprehensive unit tests.
- Added tenant-aware IAM permission evaluator (`CheckTenantPermission`, `CheckOrganizationActive`) in `backend/manager/component/iam/manager.go` enforcing lifecycle states and role boundaries.
- Added frontend `OrganizationSlice` (`frontend/src/stores/organization.ts`) with active organization selection, membership state tracking, and unit tests.

### Changed

- Synchronized OpenSpec main specifications (`a2a-agent-network`, `agent-network-safety`, `agent-runtime-foundation`, `product-identity-migration`) and archived completed change `build-888a2a-agent-network-foundation`.

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
