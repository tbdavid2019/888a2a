## MODIFIED Requirements

### Requirement: Every enabled Agent has an A2A Agent Card
The system SHALL expose an A2A 1.0 Agent Card for every enabled Agent with its identity, interfaces, skills, media types, capabilities and security requirements. A Hub-registered Agent MAY be represented by a server-assigned peer ID and the card SHALL report only verified runtime availability.

#### Scenario: Agent discovers peers
- **WHEN** an authenticated 888a2a Agent queries the Agent Directory
- **THEN** it receives only accessible peer Agent Cards and their verified runtime availability

#### Scenario: Open Hub peer is discovered
- **WHEN** a registered Agent queries its Hub directory for a peer ID
- **THEN** it receives the peer's server-assigned identity and safe card data without receiving private host or credential details

### Requirement: A2A core operations are supported
The system SHALL support send message, send streaming message, get task, list tasks, cancel task and subscribe semantics required by the selected A2A 1.0 interface. Hub routing SHALL resolve a target by server-assigned peer ID.

#### Scenario: Agent delegates a long-running task
- **WHEN** Agent A sends a task to Agent B and subscribes to updates
- **THEN** Agent A receives ordered durable status, message and artifact updates through completion or terminal failure

### Requirement: A2A work is durable and traceable
The system SHALL persist work identity, context, requester, executor, source conversation/task, state, artifacts, parent, idempotency and trace correlation before acknowledging accepted work.

#### Scenario: Manager restarts after accepting work
- **WHEN** the Manager restarts after acknowledging an A2A task but before execution finishes
- **THEN** the task remains queryable and resumes or reaches an explicit recoverable state without duplicate execution

### Requirement: Agents can invoke peers through local tools
The system SHALL provide Agent-side tools for peer discovery, task send, status read/subscribe, result reply and cancellation without requiring direct process access or polling another Agent runtime. Tools SHALL accept a Hub peer ID as the target identity.

#### Scenario: Coding Agent requests review
- **WHEN** a Coding Agent discovers a Review Agent and delegates a review task
- **THEN** the Review Agent is woken, performs the work and returns its result to the originating task context

### Requirement: A2A tasks link to current collaboration context
The system SHALL map A2A work to existing internal Conversation/Task resources through additive references so current human-readable history remains available.

#### Scenario: Human inspects Agent delegation
- **WHEN** a human opens the source task thread
- **THEN** the UI or API can show the delegated A2A task, peer Agent, state and returned artifacts

### Requirement: Network gate proves twelve-Agent interoperability
The system SHALL pass an automated acceptance scenario using at least twelve Agents across two Machines and at least three Provider types.

#### Scenario: Twelve-Agent network completes bounded work
- **WHEN** one coordinator delegates bounded parallel work across accessible peers while Manager and one Machine reconnect during the run
- **THEN** all accepted tasks complete or report explicit terminal states with no lost messages, duplicate task execution or cross-Agent data access
