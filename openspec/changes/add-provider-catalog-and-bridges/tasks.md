## 1. Catalog foundation

- [x] 1.1 Define canonical ProviderFamily, ProviderTransport, readiness status, capability, and evidence types; verify aliases normalize to one stable identity in unit tests.
- [x] 1.2 Add catalog entries for OpenClaw, Hermes, Codex, agy/Antigravity, Claude Code, OpenCode, Goose, Cline, Cursor, Gemini, Aider, Pi, Qwen Code, Kiro, GitHub Copilot, ZeroClaw, Grok, Reasonix, WorkBuddy, DeepSeek Harness, Qwen Office, DuMate, TraeWork, and OpenHands; verify every entry has a transport, safety state, and install hint.
- [x] 1.3 Add catalog sanitization and status projection; verify secrets, session IDs, private paths, and unverified readiness claims never reach API/UI output.

## 2. Bridge contract

- [x] 2.1 Define the tenant-bound AgentBridge interface for preflight, start, invoke, stream, cancel, health, and stop; verify missing identity, deadline, output-size, and cleanup failures are fail-closed.
- [x] 2.2 Implement deterministic fake ACP, Gateway, and CLI bridges; verify delivered, rejected, not-delivered, outcome-unknown, idempotency, cancellation, and late-event behavior.
- [x] 2.3 Add bridge instance/session binding records without persisting provider secrets or native session IDs in Manager; verify restart and stale-binding behavior.

## 3. First real runtimes

- [x] 3.1 Adapt the existing Codex ACP v2 thread executor as an explicit A2A bridge; verify tenant/context/turn propagation and run the opt-in real Codex gate when local `codex app-server` and `CODEX_HOME` are available.
- [x] 3.2 Implement an authenticated loopback OpenClaw Gateway bridge using only documented Gateway/ACP surfaces; verify endpoint allowlisting, auth failure, bounded request/response, and opt-in live health/turn tests.
- [x] 3.3 Implement a bounded agy/Antigravity CLI/MCP bridge; verify argv construction, workspace confinement, timeout, output parsing, and opt-in local smoke test without exposing credentials.
- [x] 3.4 Register Pull-only or bridge-required fallbacks for the remaining catalog providers; verify detected binaries cannot be selected for automatic execution without transport evidence.

## 4. A2A integration

- [x] 4.1 Connect A2A task execution to the selected AgentBridge while preserving durable task state, correlation, idempotency, and terminal event ordering; verify fake bridge A2A send/stream/get/cancel/subscribe flows.
- [x] 4.2 Project bridge readiness and supported capabilities into public and authenticated Agent Cards; verify non-ready transports are not advertised as executable.
- [x] 4.3 Add external A2A client interoperability gates for Agent Card, send, stream, get, list, cancel, and subscribe; verify cross-tenant credentials cannot enumerate or execute another tenant's work.

## 5. Operator UI and evidence

- [x] 5.1 Add provider catalog cards and filters to the Machine/Agent UI; verify ready, detected-only, bridge-required, pull-only, unavailable, and pending states have distinct copy and controls.
- [ ] 5.2 Add install/configure/repair/rollback actions only for supported transports; verify the UI never offers an automatic action for a non-ready provider.
- [x] 5.3 Add provider compatibility documentation and real-runtime evidence templates; verify every claimed provider/version/platform result links to a reproducible gate.
- [ ] 5.4 Run the complete backend/frontend/proto/naming/build checks, update the dated CHANGELOG, and push a green GitHub Actions run before marking this change complete.
