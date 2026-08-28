## Purpose

提供可重用且明確受限的 A2A↔ACP、A2A↔OpenClaw Gateway 與 A2A↔CLI/MCP bridge，讓外部 Agent 工作交換能安全映射到本機 Provider，同時保留租戶、Session、結果與失敗邊界。

## ADDED Requirements

### Requirement: Bridges preserve tenant and task identity
Every bridge SHALL carry the authenticated organization, caller principal, A2A task/context identity, correlation ID and bridge instance identity through the entire request. A bridge SHALL reject missing or mismatched identity before invoking a local runtime.

#### Scenario: Bridge receives a task for another organization
- **WHEN** the request path, credential scope and local bridge binding identify different organizations
- **THEN** the bridge rejects the task without starting the Provider

### Requirement: Bridges have bounded and explicit lifecycle
Every bridge SHALL define start, health, invoke, cancel and stop behavior, enforce execution time and output limits, isolate workspace/environment, and release child processes, sockets and temporary files on completion or failure.

#### Scenario: Local runtime stops responding
- **WHEN** an ACP, Gateway or CLI invocation exceeds its configured deadline
- **THEN** the bridge cancels or terminates only its owned runtime, records a terminal timeout outcome and leaves no unowned process or active binding

### Requirement: Bridge outcomes are truthful and idempotent
A bridge SHALL classify delivery as delivered, rejected, not_delivered or outcome_unknown, preserve message and turn identity across retries, and SHALL NOT retry a possibly executed task through a different transport.

#### Scenario: Response is lost after Provider execution
- **WHEN** the bridge cannot determine whether the local runtime completed the task
- **THEN** it returns outcome_unknown and requires durable task reconciliation instead of invoking the Provider a second time

### Requirement: Unsupported runtimes remain Pull-capable
An Agent without a verified automatic bridge SHALL retain a tenant-scoped Pull path. The system SHALL label it as Pull-only and SHALL NOT simulate Push or A2A completion.

#### Scenario: agy has no configured bridge
- **WHEN** agy is installed but no approved CLI/MCP bridge is configured
- **THEN** the catalog exposes agy as PULL_ONLY or BRIDGE_REQUIRED and stores incoming work for explicit Pull consumption

### Requirement: Real-runtime gates are opt-in
Real Codex, OpenClaw and agy bridge tests SHALL require explicit environment flags and local credentials/configuration. Default CI SHALL use deterministic fake runtimes and SHALL never print or persist credentials.

#### Scenario: CI lacks OpenClaw credentials
- **WHEN** the OpenClaw integration flag is not set or the Gateway is unreachable
- **THEN** the real-runtime test is skipped with a clear reason and deterministic bridge tests still run
