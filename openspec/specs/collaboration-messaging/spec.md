# collaboration-messaging Specification

## Purpose
提供人與人、人與 Agent、多人與多 Agent 共用的可靠協作訊息能力，涵蓋 Channel、DM、Thread、附件、排序、同步、編輯、撤回與跨裝置一致性。

## Requirements

### Requirement: Conversations support mixed principal membership
The system SHALL support direct messages and multi-member Channels containing human principals, agent principals and service accounts subject to Organization IAM.

#### Scenario: Mixed team collaborates in a channel
- **WHEN** authorized humans and Agents are added to a Channel
- **THEN** every member can receive the messages and events permitted by its role and connector capabilities

### Requirement: Messages have durable per-conversation order
The system SHALL assign every persisted conversation event a monotonic per-conversation sequence and SHALL use that sequence as the authoritative display and replay order.

#### Scenario: Concurrent messages arrive
- **WHEN** multiple members send messages concurrently to the same conversation
- **THEN** all clients converge on the same sequence order after synchronization

### Requirement: Message submission is idempotent
The system SHALL accept a client-generated idempotency identifier and SHALL return the original result when the same sender retries the same submission.

#### Scenario: Client retries after timeout
- **WHEN** a client retries a previously accepted message using the same idempotency identifier
- **THEN** the system returns the existing message identity and does not append a duplicate message

### Requirement: Message changes are append-only events
The system SHALL represent edits, recalls, redactions, reactions and moderation actions as ordered events linked to the original message, while projecting the current visible state according to policy.

#### Scenario: Sender recalls a message
- **WHEN** an authorized sender recalls a message within the Organization policy window
- **THEN** members receive a recall event, the visible content is replaced, and audit or legal-hold retention follows Organization policy

### Requirement: Offline and multi-device synchronization is resumable
The system SHALL allow each user device and Agent consumer to resume from a durable cursor without losing or duplicating visible events.

#### Scenario: Device reconnects after being offline
- **WHEN** a device reconnects with its last acknowledged cursor
- **THEN** the system returns every authorized conversation event after that cursor in deterministic order

### Requirement: Collaboration features are capability-aware
The system SHALL expose support for threads, reactions, edits, recalls, read state, typing, presence, attachments and rich content per native client or external connector.

#### Scenario: Destination lacks edit support
- **WHEN** a user edits a message bridged to a connector that cannot edit messages
- **THEN** the system applies the configured fallback and records that the external destination did not receive an equivalent edit

### Requirement: Message retention is organization-controlled
The system SHALL enforce Organization retention, deletion, export and legal-hold policies without allowing normal recall to bypass compliance retention.

#### Scenario: Legal hold covers recalled content
- **WHEN** a message under legal hold is recalled by its sender
- **THEN** normal members no longer see the content while authorized compliance access retains the immutable record

### Requirement: Agent execution controls are explicit conversation events
The system SHALL represent start, steer, cancel and completion controls for Agent work as authorized structured events linked to the relevant message and task.

#### Scenario: Human stops an Agent response
- **WHEN** an authorized human stops an in-progress Agent execution
- **THEN** the runtime receives cancellation and the conversation records the resulting terminal state
