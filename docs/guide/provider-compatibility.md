# Provider compatibility and evidence

This document is the evidence boundary for local Agent providers. A catalog
entry means that 888a2a knows the provider family and its possible transport.
It does not mean that the provider is installed, authenticated, or safe for
automatic execution.

## Evidence levels

| Evidence | Meaning | Automatic A2A execution |
| --- | --- | --- |
| `PULL_ONLY` | Work can be explicitly pulled by an operator or local runtime. | No |
| `DETECTED_ONLY` | A binary or provider identity was detected. | No |
| `PROTOCOL_READY` | The protocol handshake succeeded. | No, until a full-loop gate passes |
| `FULL_LOOP_VERIFIED` | Send, output, cleanup, and tenant boundaries were verified for the selected transport. | Yes, for that transport only |
| `BRIDGE_REQUIRED` | An explicit bridge is needed before push execution. | No |
| `UNAVAILABLE` | The runtime or bridge is missing, broken, or quarantined. | No |

## Current evidence

| Provider | Transport | Current evidence | Reproduce with |
| --- | --- | --- | --- |
| Codex | ACP v2 | Local full-loop gate passed on 2026-08-28 with installed `codex app-server` and local `CODEX_HOME`. | `A2A888_RUN_CODEX_ACP_TESTS=1 CODEX_HOME=/path/to/codex-home go test ./backend/agent/bridge -run TestCodexACPBridgeRealGateIsOptIn -count=1` |
| OpenClaw | Private Gateway `/v1/responses` | Deterministic authenticated bridge tests pass. Live evidence is pending a running local Gateway and operator-approved token. | `A2A888_RUN_OPENCLAW_BRIDGE_TESTS=1 A2A888_OPENCLAW_GATEWAY_URL=http://127.0.0.1:18789 A2A888_OPENCLAW_GATEWAY_TOKEN=... go test ./backend/a2a -run TestOpenClawBridgeLiveGateIsOptIn -count=1` |
| agy / Antigravity | Bounded CLI | Local smoke gate passed on 2026-08-28 with agy 1.1.22; stream-json final response parsing and bounded execution were verified. | `A2A888_RUN_AGY_BRIDGE_TESTS=1 go test ./backend/a2a -run TestAgyCommandBridgeRealSmokeIsOptIn -count=1` |
| Claude Code, OpenCode, Pi | Existing ACP / prepared runtime | Existing runtime-specific gates remain the source of truth. A2A push requires an explicit selected bridge. | See provider and executor package tests |
| Hermes, Goose, Cline, Cursor, Aider, Qwen Code, Kiro, GitHub Copilot, ZeroClaw, Grok, Reasonix, WorkBuddy, DeepSeek Harness, Qwen Office, DuMate, TraeWork, OpenHands | Provider-specific bridge or Pull | Cataloged as `BRIDGE_REQUIRED`, `PENDING_VERIFICATION`, or `PULL_ONLY`; no automatic A2A claim is made. | Add a provider-specific deterministic and opt-in live gate before enabling push |

## Recording a new result

Every provider result must record the provider family, transport ID, provider
version, operating system, architecture, gate name, date, and outcome. Never
record access tokens, native session IDs, private executable paths, or complete
environment dumps. A failed or skipped live gate keeps the provider
non-automatic.

The VOKO provider matrix informed the separation between catalog, transport,
session, fallback, and evidence. This repository uses that public architectural
idea only; it does not copy VOKO source code, credentials, branding, or
provider-specific private behavior.
