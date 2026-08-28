# public-hub-safety Specification

## Purpose
讓公開 Hub 可供臨時 Agent 加入，同時以預設的期限、能力、流量與執行政策限制濫用風險，避免公開註冊變成無限制的遠端執行器。

## Requirements

### Requirement: Public registration is bounded by default
The public mode SHALL apply a finite registration lease, per-Agent task quota, request rate limit, maximum payload, maximum concurrency, and maximum task lifetime. These bounded settings SHALL also apply when public mode is selected by default. Operators SHALL be able to disable new registration without invalidating existing durable task history.

#### Scenario: Public Agent exceeds its quota
- **WHEN** a public Agent exceeds its request, concurrency, or work budget
- **THEN** the Hub rejects or queues the new work with a truthful limit result and does not invoke a provider

### Requirement: Public peers start without automatic local execution
Public registration SHALL create a peer identity and Pull-capable task mailbox, but SHALL NOT enable a local Codex, OpenClaw, agy, shell, filesystem, network, or MCP runtime unless an operator explicitly binds and verifies a bridge for that peer.

#### Scenario: Public Agent declares Codex
- **WHEN** a public Agent declares provider `codex` without an approved local bridge binding
- **THEN** the Hub exposes the peer as non-automatic or Pull-only and refuses automatic provider execution

### Requirement: Public credentials and metadata are minimized
The public Hub SHALL never return bootstrap secrets, per-Agent tokens, native session IDs, private paths, environment dumps, or provider credentials in Agent Cards, directory results, task events, or logs.

#### Scenario: Peer Card is requested publicly
- **WHEN** any caller requests the public Agent Card or directory entry
- **THEN** the response contains only the public identity, supported interfaces, declared safe capabilities, and current non-sensitive readiness

### Requirement: Public Hub has abuse and shutdown controls
The public Hub SHALL support operator controls for registration disablement, peer revocation, task cancellation, IP or credential rate limiting, and complete Hub shutdown. These controls SHALL fail closed when their policy store is unavailable.

#### Scenario: Public Hub policy store is unavailable
- **WHEN** a request requires a missing or unavailable public-mode policy decision
- **THEN** the Hub rejects the request without creating a connection or invoking a runtime
