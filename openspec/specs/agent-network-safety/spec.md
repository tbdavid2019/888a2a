# agent-network-safety Specification

## Purpose
在完整 SaaS Approval 與計費系統完成前，先為十幾個互相委派的 888a2a Agent 建立不可繞過的工具權限、委派限制、取消、冪等、稽核與資源邊界。

## Requirements

### Requirement: High-risk runtime permission defaults to deny
The system SHALL remove unconditional ACP permission approval and SHALL deny unclassified high-risk filesystem write, shell, network, secret and external side-effect requests unless an explicit focused policy allows them.

#### Scenario: Peer Agent requests an unapproved shell action
- **WHEN** delegated work reaches a shell permission request without an allow policy
- **THEN** the runtime denies the request and records the requesting task, peer and action summary

### Requirement: Delegation graph is bounded
The system SHALL enforce maximum delegation depth, child count, fan-out, concurrency, runtime, retry and token or work-unit budget.

#### Scenario: Coordinator exceeds fan-out
- **WHEN** an Agent attempts to create more children than the configured limit
- **THEN** the additional delegation is rejected and the parent receives a policy-limit event

### Requirement: Cyclic delegation is rejected
The system SHALL detect direct and indirect task dependency cycles before scheduling a child task.

#### Scenario: Agent delegates back to an ancestor task
- **WHEN** a proposed child would create a cycle in the active task graph
- **THEN** the system rejects the edge and retains the existing graph unchanged

### Requirement: Cancellation propagates
The system SHALL propagate an authorized root cancellation to queued and running descendants and connected runtimes.

#### Scenario: Operator cancels a fan-out task
- **WHEN** an authorized operator cancels the root task
- **THEN** new children are blocked, queued children are cancelled and running children receive runtime cancellation with observable terminal state

### Requirement: Task submission and consumption are idempotent
The system SHALL use stable request and delivery identities so retry or replay cannot create duplicate Agent tasks or repeated external effects.

#### Scenario: A2A send response is lost
- **WHEN** the sender retries the same A2A request after losing the response
- **THEN** the receiver returns the existing task and does not schedule another execution

### Requirement: Agent Network actions are auditable
The system SHALL record Agent discovery, delegation, task state, Provider/session, permission decision, tool summary, cancellation, budget and terminal outcome without logging credentials or hidden reasoning.

#### Scenario: Operator traces a failed delegation
- **WHEN** an operator opens a failed task trace
- **THEN** the trace identifies requester, executor, parent, Provider, policy decisions, retries and final failure cause
