## 1. Product Identity and Baseline

- [x] 1.1 Inventory Agent Network files that will be changed and define their 888a2a binary, CLI, API, environment and documentation names; verify the mapping is reviewed and uses `A2A888_` for new environment variables.
- [x] 1.2 Add a changed-file naming gate for Agent Network work; verify an unapproved legacy identifier fixture fails while migration/attribution allowlist fixtures pass.
- [x] 1.3 Capture baseline Provider, executor, Machine multi-Agent, session, chattool and dispatcher tests; verify the targeted Go packages and production build pass before behavior changes.

## 2. Contracts and Deterministic Harnesses

- [x] 2.1 Add the official A2A Go SDK at a pinned version behind a dedicated package boundary; verify the project builds and reports the supported A2A protocol version.
- [x] 2.2 Define Provider manifest, prepared-runtime, compatibility-level and runtime-status contracts; verify `buf format`, `buf lint`, generation and manifest validation fixtures pass.
- [ ] 2.3 Define durable Machine assignment event/ack/replay contracts; verify malformed sequence, wrong Machine and duplicate acknowledgement cases are rejected or idempotent.
- [ ] 2.4 Define A2A work, context, parent edge, artifact reference, budget and trace persistence contracts; verify fresh migration and upgrade migration tests pass.
- [ ] 2.5 Build deterministic fake ACP v1, ACP v2 and A2A peer harnesses; verify ordinary CI can simulate twelve Agents without external credentials.

## 3. Provider Manifest and npm Runtime

- [ ] 3.1 Implement Provider manifest validation for runtime kind, protocol, platform, exact version, binary, integrity, capabilities and permission profile; verify floating npm versions fail unit tests.
- [ ] 3.2 Convert OpenCode, Claude Code and Codex definitions to manifest-backed registry entries; verify detection and model-probe regression tests remain green.
- [ ] 3.3 Represent embedded Pi and custom ACP v1/v2 through the same runtime-selection contract; verify current Pi and custom-provider tests pass.
- [ ] 3.4 Implement Machine npm cache keys and staging directories by package/version/integrity/platform/architecture; verify equivalent requests share only immutable prepared data.
- [ ] 3.5 Implement atomic npm package preparation and local-bin resolution; verify interrupted install never publishes READY and the previous verified version remains launchable.
- [ ] 3.6 Implement integrity failure quarantine and explicit retry/remove operations; verify a modified package cannot launch and the audit event contains no secret data.
- [ ] 3.7 Move Claude Code from floating turn-time `npx` launch to a pinned prepared local binary; verify offline launch and real ACP opt-in integration tests.
- [ ] 3.8 Add Provider READY/BROKEN/QUARANTINED/UPDATE_AVAILABLE status and compatibility evidence to Manager API/UI; verify detected-only Providers cannot be selected for automatic execution.
- [ ] 3.9 Add Provider package/version to session launch fingerprint; verify compatible restarts resume and incompatible upgrades cold-start only affected Agents.

## 4. Multi-Agent Machine Reliability and Isolation

- [ ] 4.1 Add durable per-Machine assignment storage with monotonic sequence and idempotency identity; verify create/config/remove events persist transactionally.
- [ ] 4.2 Make Machine report the last acknowledged assignment sequence during connect; verify Manager replays every missing event in order.
- [ ] 4.3 Implement idempotent Machine apply and acknowledgement for create/config/remove; verify replay does not create duplicate or zombie runners.
- [ ] 4.4 Add full-roster reconciliation after assignment replay; verify missed deletes and stale configs converge to Manager state.
- [ ] 4.5 Strengthen per-Agent workspace, session, env, credential, limits and process ownership on a shared Machine; verify twelve fake Agents cannot read or mutate peer state.
- [ ] 4.6 Add Machine/Agent runtime capacity and availability reporting to Agent Directory inputs; verify an offline or saturated Agent is not advertised as ready for new work.
- [ ] 4.7 Run multi-Agent runner concurrency, cancellation and process-tree cleanup tests; verify one Agent timeout or crash does not terminate unrelated Agent runners.

## 5. A2A Gateway, Directory and Durable Work

- [ ] 5.1 Implement tenant-ready A2A 1.0 HTTP+JSON routing and protocol version negotiation using the official SDK; verify an external SDK fixture can connect without private wire extensions.
- [ ] 5.2 Build Agent Card projection from Agent profile, skills, Provider capabilities and runtime availability; verify disabled/private skills are omitted.
- [ ] 5.3 Implement authenticated Agent Directory list/get and skill filtering; verify callers only see accessible peers and readiness state.
- [ ] 5.4 Implement idempotent A2A send-message acceptance and durable work creation before acknowledgement; verify a lost response retry returns the existing task.
- [ ] 5.5 Implement get-task and tenant-ready list-tasks with cursor pagination; verify one Agent cannot enumerate inaccessible peer work.
- [ ] 5.6 Implement send-streaming-message and subscribe-to-task from persisted status/message/artifact events; verify reconnect resumes from durable state.
- [ ] 5.7 Implement cancel-task with authorization and terminal-state idempotency; verify repeated cancellation does not corrupt completed work.
- [ ] 5.8 Link A2A work/context/artifacts to existing Conversation/Task/thread resources through additive projections; verify human-readable history and A2A state remain consistent.
- [ ] 5.9 Add Manager restart recovery for accepted/running work; verify tasks resume or enter an explicit recoverable state without duplicate execution.

## 6. Agent-Side Peer Collaboration

- [ ] 6.1 Add 888a2a Agent tools for peer list/get and skill discovery; verify output includes Agent Card capabilities and verified readiness.
- [ ] 6.2 Add Agent tool for idempotent A2A task send with optional parent/context/budget; verify target wake-up occurs without sender polling the target process.
- [ ] 6.3 Add Agent tools for task get/list/subscribe/cancel and result/artifact reply; verify a delegated review task returns to the originating context.
- [ ] 6.4 Update Agent prompt and re-anchor instructions to use A2A tasks for work delegation while retaining Channel/DM for collaboration context; verify prompt tests reject direct process assumptions.
- [ ] 6.5 Project A2A delegation/status/result summaries into source task threads; verify humans can inspect peer, state, trace and artifacts without seeing hidden reasoning or secrets.

## 7. Bounded Orchestration and Focused Safety

- [ ] 7.1 Add parent/child work edges and cycle detection before commit; verify direct and indirect delegation cycles are rejected.
- [ ] 7.2 Implement maximum depth, child count and fan-out limits; verify excess delegation returns a durable policy-limit event.
- [ ] 7.3 Implement per-coordinator concurrency, retry, runtime and token/work-unit budgets; verify scheduling cannot exceed the remaining parent budget.
- [ ] 7.4 Implement parallel fan-out and explicit join with success, partial-failure and timeout policy; verify deterministic aggregation of ten peer results.
- [ ] 7.5 Propagate root cancellation to queued descendants and running runtimes; verify new children are blocked and every descendant reaches an observable terminal state.
- [ ] 7.6 Replace unconditional ACP permission selection with focused runtime policy; verify unclassified shell/write/network/secret/side-effecting MCP requests are denied by default.
- [ ] 7.7 Preserve configurable workspace-read access with canonical path confinement; verify symlink escape and cross-Agent path tests remain denied on Linux and macOS.
- [ ] 7.8 Add Agent Network audit/trace events for discovery, delegation, Provider/session, permission, budget, retry, cancellation and terminal outcome; verify credentials and hidden reasoning are excluded.

## 8. Twelve-Agent Acceptance Gate

- [ ] 8.1 Create a reproducible two-Machine/twelve-Agent test topology with one coordinator, ten specialists and one reviewer/aggregator; verify every Agent publishes an Agent Card and reaches READY.
- [ ] 8.2 Run deterministic fake-Provider fan-out/join acceptance; verify ten peer tasks return ordered terminal results with no duplicates or lost accepted work.
- [ ] 8.3 Restart Manager during active work and disconnect/reconnect one Machine; verify durable assignments, work, streams and session recovery satisfy the gate.
- [ ] 8.4 Retry a lost A2A send response and cancel one descendant; verify idempotency and cancellation propagation produce complete traces.
- [ ] 8.5 Attempt cross-Agent workspace, credential and unauthorized peer access; verify every probe is denied and audited.
- [ ] 8.6 Run opt-in Codex, Claude Code and OpenCode acceptance on two real Machines when available; verify Provider-specific compatibility report and session resume results.
- [ ] 8.7 Run required formatting, repeated golangci-lint, targeted/full Go tests, ACP opt-in tests, Proto checks, frontend checks and production build; verify all configured quality gates pass.
- [ ] 8.8 Publish the Agent Network operator guide, Provider manifests, security defaults, troubleshooting and acceptance report; verify a clean operator can bring twelve Agents online using only documented steps.
