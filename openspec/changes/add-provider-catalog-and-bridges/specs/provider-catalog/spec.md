## Purpose

建立一份可驗證的 Provider family 與 transport catalog，讓 888a2a 能清楚區分已準備、僅偵測、需要 bridge、僅 Pull 與尚待驗證的本機 Agent，而不把名稱出現在 UI 誤當成可執行能力。

## ADDED Requirements

### Requirement: Provider families have one canonical catalog identity
The system SHALL represent each supported Agent family with one canonical identifier, display name, aliases, supported transports, session model, installation hint and safety status. Aliases SHALL resolve to the canonical identifier and SHALL NOT become separate runtime identities.

#### Scenario: Operator selects an aliased provider
- **WHEN** an installed provider is discovered through an alias such as a legacy CLI name
- **THEN** the catalog returns one canonical family identity and preserves the alias only as detection metadata

### Requirement: Catalog status reflects verified capability
The system SHALL expose separate states for READY, DETECTED_ONLY, BRIDGE_REQUIRED, PULL_ONLY, UNAVAILABLE and PENDING_VERIFICATION. A provider SHALL be selectable for automatic execution only when its selected transport has current platform and version evidence.

#### Scenario: Provider binary exists without a safe adapter
- **WHEN** a catalog command is found but no transport bridge has passed its contract checks
- **THEN** the UI shows DETECTED_ONLY or BRIDGE_REQUIRED and automatic execution remains disabled

### Requirement: Catalog covers the supported provider matrix
The catalog SHALL include OpenClaw, Hermes, Claude Code, Codex, agy/Antigravity, DeepSeek Harness, WorkBuddy, Qwen Office, DuMate, TraeWork, Cline, ZeroClaw, Qwen Code, Kiro CLI, GitHub Copilot CLI, OpenHands, Aider, OpenCode, Goose, Gemini, Cursor, Grok, Pi and Reasonix, with each entry's actual transport and evidence state explicitly declared.

#### Scenario: Host has no optional provider installed
- **WHEN** the Machine reports its provider catalog
- **THEN** every catalog entry remains visible with a non-ready status or installation hint, and no absent provider is reported as READY

### Requirement: Catalog metadata is safe to expose
The catalog SHALL NOT include credentials, raw session identifiers, local secret paths or unverified claims. Installation and executable metadata SHALL be bounded and sanitized before being returned to Manager or UI callers.

#### Scenario: Provider metadata contains a secret-looking value
- **WHEN** a discovery result includes a token, key, cookie or private path
- **THEN** the catalog redacts or omits that value before persistence or display
