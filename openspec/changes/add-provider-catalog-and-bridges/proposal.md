## Why

888a2a currently has only three built-in ACP providers and its A2A gateway does
not have a consistent way to represent other local Agent runtimes. This makes
installed tools such as OpenClaw, Hermes, agy, Goose, Cline, Cursor, and Aider
look unsupported or encourages unsafe ad-hoc command handling. We need a
provider catalog and explicit bridges so detection, execution, session
continuity, and A2A exposure are separate, testable claims.

## What Changes

- Add a canonical provider catalog containing the supported local Agent
  families shown in the provider matrix, including OpenClaw, Hermes, Codex,
  agy/Antigravity, Claude Code, OpenCode, Goose, Cline, Cursor, Gemini, Aider,
  Pi, Qwen Code, Kiro, GitHub Copilot, ZeroClaw, Grok, Reasonix, WorkBuddy,
  DeepSeek Harness, Qwen Office, DuMate, TraeWork, and OpenHands.
- Represent each provider's transport, installation command, readiness level,
  session capability, and safety state without claiming automatic execution
  when only detection or Pull is available.
- Add explicit bridge contracts for A2A↔ACP, A2A↔OpenClaw Gateway, and
  A2A↔CLI/MCP runtimes, with bounded process lifetime, tenant identity, and
  fail-closed error semantics.
- Add a provider catalog/status surface to the Machine UI so operators can
  distinguish ready, detected-only, bridge-required, unavailable, and pending
  providers.
- Add deterministic fake bridges and opt-in real-runtime gates for Codex,
  OpenClaw, and agy; third-party credentials remain local and are never
  committed or exposed through the Manager.

## Capabilities

### New Capabilities

- `provider-catalog`: Canonical provider families, aliases, transport metadata,
  readiness evidence, and operator-visible status.
- `agent-runtime-bridges`: Explicit bridges between A2A and ACP, Gateway, CLI,
  or MCP runtimes with bounded lifecycle and security contracts.

### Modified Capabilities

- `openspec/specs/a2a-agent-network`: A2A Agent Cards and tasks may be backed by
  an explicitly configured local runtime bridge, while unsupported runtimes
  remain Pull-only instead of being presented as ready.
- `openspec/specs/agent-runtime-foundation`: Provider registry and runtime
  preparation consume catalog identities and expose readiness evidence.

## Impact

- Affected Go packages: `backend/agent/provider`, `backend/agent/runtime`,
  `backend/a2a`, and Machine/Manager APIs.
- Affected frontend: provider selection, Machine provider status, and operator
  diagnostics.
- Adds no mandatory third-party runtime dependency; external CLIs and Gateway
  credentials are optional host capabilities.
- Requires new provider catalog and bridge contract tests, plus opt-in tests
  that run only when the corresponding local runtime and credentials exist.
