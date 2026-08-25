## Purpose

以 A2A 1.0 提供組織內與跨組織 Agent 的標準發現、工作委派、長任務、產物及授權流程，並在其上建立受限制的多 Agent 協調能力。

## ADDED Requirements

### Requirement: Agents expose A2A 1.0 interfaces
The system SHALL expose standards-compliant A2A 1.0 Agent Cards and supported interfaces for every Agent that an Organization authorizes for A2A access.

#### Scenario: Client discovers an Agent
- **WHEN** an authorized or public client retrieves an Agent Card
- **THEN** the card declares the Agent identity, supported interfaces, protocol version, skills, media types, capabilities and security requirements permitted for that client

### Requirement: A2A access is tenant-authorized
The system SHALL authenticate and authorize every A2A request against Organization, workspace, Agent, skill, task and data scopes before revealing or changing task state.

#### Scenario: Client lists tasks
- **WHEN** an A2A client lists tasks without specifying a context
- **THEN** the result contains only tasks visible to the authenticated Organization and granted scopes

### Requirement: A2A tasks preserve standard lifecycle and artifacts
The system SHALL support A2A Message, Task, TaskStatus, Part, Artifact, context, streaming, polling, subscription, cancellation and push notification semantics.

#### Scenario: Long-running Agent produces artifacts
- **WHEN** an Agent emits progress and a file artifact for an active A2A task
- **THEN** authorized clients receive ordered status and artifact events and can later retrieve the durable task state

### Requirement: Internal and external delegation share one work model
The system SHALL represent internal Agent delegation and external A2A requests through the same tenant-scoped work model while preserving their original principals and protocol references.

#### Scenario: Internal Agent delegates externally
- **WHEN** an internal Agent delegates a subtask to an external A2A Agent
- **THEN** the child task links to the parent, records both Organizations and keeps the external task and context identifiers

### Requirement: Multi-Agent orchestration is bounded
The system SHALL enforce Organization-defined maximum fan-out, delegation depth, concurrency, token or cost budget, execution time and retry limits for task graphs.

#### Scenario: Agent exceeds delegation depth
- **WHEN** an Agent attempts to create a child task beyond the allowed depth
- **THEN** the orchestrator rejects the delegation and records a policy-limited task event

### Requirement: Task graphs prevent cycles and support joins
The system SHALL reject cyclic task dependencies and SHALL support fan-out/fan-in joins with explicit partial-failure policy.

#### Scenario: Parent waits for parallel children
- **WHEN** a parent task fans out to multiple child tasks and reaches a join
- **THEN** the parent resumes only after the configured success, failure or timeout condition is satisfied

### Requirement: Cancellation propagates through task graphs
The system SHALL propagate authorized cancellation to active descendants and connected runtimes while retaining immutable terminal history.

#### Scenario: Human cancels root task
- **WHEN** an authorized human cancels a root multi-Agent task
- **THEN** cancellable descendants receive cancellation, new delegation is blocked and the trace reports every resulting terminal state

### Requirement: Authorization-required tasks integrate with Organization approval
The system SHALL map work requiring human authorization to the A2A authorization-required state and the Organization approval workflow without placing sensitive credentials in ordinary task messages.

#### Scenario: External Agent requests destructive access
- **WHEN** an A2A Agent reaches an action requiring Organization approval
- **THEN** the task enters authorization-required state and resumes only after a valid bound approval or credential is delivered through the approved secure channel
