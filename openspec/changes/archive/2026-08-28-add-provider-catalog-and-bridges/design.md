## Context

The existing Go registry has concrete ACP adapters for OpenCode, Claude Code and
Codex. The A2A gateway is now reachable in production, but a local CLI or
Gateway installation is not itself an A2A peer. Provider discovery, automatic
delivery, session recovery and A2A exposure therefore need an explicit shared
model.

The design follows the public VOKO architecture pattern of Catalog → Runtime
Registry → Dispatcher → Delivery Executor, while keeping 888a2a's existing
tenant IAM, prepared-runtime integrity and durable A2A work store as the source
of truth. See `proposal.md` and the four capability specs for behavior.

## Goals / Non-Goals

**Goals:**

- Give every provider one canonical family identity and explicit transport IDs.
- Add safe readiness states and operator-visible evidence for the full provider
  matrix from the proposal.
- Reuse one bridge contract for ACP, OpenClaw Gateway and bounded CLI/MCP
  execution.
- Keep unsupported or unverified providers Pull-capable without fake success.
- Make Codex, OpenClaw and agy real-runtime gates opt-in and credential-safe.

**Non-Goals:**

- Claiming every provider in the catalog is automatically executable.
- Copying VOKO source code, private cloud services, branding or proprietary
  protocol behavior.
- Moving Provider-native session IDs into PostgreSQL, Agent Cards or logs.
- Enabling public OpenClaw `/tools/invoke` or arbitrary shell execution.

## Decisions

1. **Catalog is metadata, not an executor.** Catalog entries describe family,
   aliases, install hints, transports, capabilities and evidence. Runtime
   adapters remain responsible for actual spawn or Gateway calls.
2. **Transport IDs are canonical.** Examples are `codex-acp2`,
   `openclaw-gateway`, `openclaw-cli`, `hermes-http`, and `agy-cli`. The family
   ID is never used to imply that every transport is available.
3. **Readiness is fail-closed.** Detection alone yields DETECTED_ONLY. A
   verified executable plus protocol probe can yield READY only for that
   transport and platform. Missing or uncertain bridges retain Pull.
4. **Bridge boundary is typed.** A bridge receives a tenant-bound A2A task and
   returns a bounded event stream plus a truthful delivery outcome. It cannot
   choose a second Provider, write routing caches, or guess a native Session.
5. **First real bridges are prioritized.** Codex uses the existing ACP v2
   thread path; OpenClaw uses its authenticated local Gateway or documented
   ACP bridge; agy uses a bounded CLI/MCP bridge. The remaining entries begin
   as catalog/Pull-only until their commands and session semantics are proven.
6. **UI uses one status vocabulary.** The Machine profile and Agent selection
   surface consume the same catalog status and disable automatic selection for
   non-ready transports.

## Risks / Trade-offs

- [Many providers change CLI flags and session formats] → Keep entries
  Pull-only until a provider-specific probe and real-runtime gate pass.
- [A Gateway credential can grant broad operator access] → Keep Gateway
  bridges loopback/private, use a dedicated scoped bridge credential where the
  provider supports it, redact credentials, and reject public `/tools/invoke`.
- [A CLI may execute tools outside the requested workspace] → run with an
  isolated working directory, explicit environment, bounded timeout and the
  existing RuntimePolicy.
- [Provider catalog grows faster than test infrastructure] → deterministic fake
  bridges cover contract semantics; real gates are opt-in and recorded by
  provider/platform/version.

## Migration Plan

1. Add catalog types and entries without changing existing Provider IDs.
2. Add fake bridge contract tests and expose non-ready statuses in the UI.
3. Enable Codex ACP, OpenClaw Gateway and agy CLI/MCP bridges one at a time
   behind explicit configuration and opt-in real gates.
4. Persist only canonical family/transport identity and evidence; preserve
   existing custom-provider and Pull behavior during migration.
5. Roll back by disabling a transport or bridge; do not delete work, history or
   provider cache data.

## Open Questions

- The exact OpenClaw Gateway endpoint and authentication mode to use in a
  deployment must be selected from the operator's active local configuration;
  the bridge SHALL not infer or print credentials.
