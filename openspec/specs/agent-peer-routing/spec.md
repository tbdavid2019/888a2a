# agent-peer-routing Specification

## Purpose
讓已註冊的 Agent 透過 Hub 目錄以 `peer_id` 找到彼此，並以持久化、可追蹤且不依賴實際網路位置的方式傳送 A2A 工作。

## Requirements

### Requirement: Peer IDs resolve within one Hub
The Hub SHALL maintain a directory keyed by Hub identity and server-assigned `agent_id`. Directory results SHALL expose only declared capabilities, transport readiness, connection state, and non-sensitive Agent Card data.

#### Scenario: Agent looks up a peer by ID
- **WHEN** an authenticated Agent requests a peer ID within the same Hub
- **THEN** the Hub returns that peer's current card and readiness, or a not-found result without revealing another Hub's identity

### Requirement: Task delivery targets a peer ID
The Hub SHALL accept a task addressed to a `target_agent_id`, resolve the current connection, and forward the task without requiring the caller to know the target host, process, or provider CLI.

#### Scenario: Connected peer receives a task
- **WHEN** Agent A sends a valid task to Agent B's peer ID
- **THEN** the Hub persists the task, forwards it to Agent B, and returns ordered status and result events tied to the same task and context IDs

#### Scenario: Peer is offline
- **WHEN** Agent A sends a task to a known but offline peer
- **THEN** the Hub persists the task as pending or not-delivered and does not claim completion or silently send it through another provider

### Requirement: Peer routing is tenant and credential scoped
The Hub SHALL verify that the caller's credential is allowed to address the target peer and SHALL prevent peer enumeration or task access across organizations, Hub instances, or revoked identities.

#### Scenario: Caller uses another organization's peer ID
- **WHEN** a caller requests or sends work to a peer outside its authorized scope
- **THEN** the Hub returns an authorization or not-found result and performs no delivery

### Requirement: Routing preserves idempotency and reconciliation
The Hub SHALL preserve caller idempotency keys, task IDs, context IDs, correlation IDs, and terminal event ordering across reconnects. It SHALL classify ambiguous delivery as unknown and SHALL require reconciliation before retrying.

#### Scenario: Hub disconnects after forwarding
- **WHEN** the Hub loses the response after the peer may have accepted a task
- **THEN** the task remains queryable with an unknown delivery state and the Hub does not issue a second delivery through another route
