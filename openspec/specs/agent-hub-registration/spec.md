# agent-hub-registration Specification

## Purpose
讓外部 Agent 以 Hub URL 與註冊憑證加入 888a2a，取得 Hub 配發的穩定 Agent ID 與可撤銷 token，並以明確的生命週期保持連線狀態。

## Requirements

### Requirement: Hub modes have explicit registration policy
The Hub SHALL support `closed`, `open`, and `public` modes. When no mode is configured, `public` SHALL be the default with bounded limits and no automatic runtime execution. `closed` SHALL require existing tenant IAM, `open` SHALL require a bootstrap registration token, and an explicitly configured `public` mode SHALL require operator confirmation.

#### Scenario: Open mode rejects an invalid bootstrap token
- **WHEN** an Agent registers with a missing, expired, or invalid bootstrap token
- **THEN** the Hub rejects registration without creating an Agent identity or connection

#### Scenario: Public mode requires explicit operator enablement
- **WHEN** the Hub starts without public mode enabled
- **THEN** an unauthenticated registration request is rejected even if the route is reachable

#### Scenario: Missing mode uses bounded public defaults
- **WHEN** the operator omits `A2A888_HUB_MODE`
- **THEN** the Hub starts in public mode with bounded registration, payload, lease, rate, and concurrency limits and without automatic runtime execution

### Requirement: Registration assigns a Hub-scoped Agent identity
The Hub SHALL validate the declared Agent name, provider family, transport, capabilities, and Agent Card before assigning a unique Hub-scoped `agent_id`. The Hub SHALL return a separate per-Agent credential and SHALL NOT accept a caller-selected identity as authoritative.

#### Scenario: Agent joins an open Hub
- **WHEN** a valid Agent presents a bootstrap token and a valid declaration
- **THEN** the Hub returns a generated `agent_id`, a per-Agent token, the Hub identity, and the peer connection endpoint

#### Scenario: Duplicate declaration is retried
- **WHEN** the same registration idempotency key is submitted again with the same declaration
- **THEN** the Hub returns the original identity without issuing a second active Agent identity

### Requirement: Registered Agent credentials are revocable and scoped
The Hub SHALL store only a hash or verifier of each per-Agent token, bind it to one Hub and Agent identity, support expiration and revocation, and reject it after revocation without exposing token material in logs or peer metadata.

#### Scenario: Operator revokes a public Agent
- **WHEN** an operator revokes a registered Agent token
- **THEN** subsequent connection, heartbeat, directory, and task requests using that token are rejected

### Requirement: Agent connection state is leased
The Hub SHALL support authenticated connect, heartbeat, disconnect, and lease expiry. An Agent whose lease expires SHALL be marked offline and SHALL not be selected for push delivery until it reconnects and passes its bridge readiness checks.

#### Scenario: Agent stops heartbeating
- **WHEN** a registered Agent misses the configured lease deadline
- **THEN** the Hub marks the Agent offline and makes no new automatic task assignment to it
