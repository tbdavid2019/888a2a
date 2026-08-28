# omnichannel-connectors Specification

## Purpose
讓每個 Organization 安全接入 Slack、Teams、LINE、WhatsApp、Web Widget 與後續通路，同時保留各平台的身分、對話及功能差異。

## Requirements

### Requirement: Connector installations are tenant-scoped
The system SHALL bind every connector installation, credential, external account and webhook endpoint to exactly one Organization.

#### Scenario: Organization installs a connector
- **WHEN** an Organization admin completes a supported connector authorization flow
- **THEN** the system stores the installation under that Organization and prevents other Organizations from using its credentials or external conversations

### Requirement: Inbound webhooks are authenticated and acknowledged safely
The system SHALL verify each platform's required signature or authentication before accepting an event and SHALL enqueue accepted events for asynchronous processing within the platform acknowledgement deadline.

#### Scenario: Invalid webhook signature arrives
- **WHEN** an inbound webhook fails the connector's signature verification
- **THEN** the system rejects it without enqueueing, routing or exposing its payload to Agent execution

### Requirement: Inbound events are deduplicated and reorderable
The system SHALL retain each platform event identifier and delivery metadata so retries are idempotent and out-of-order deliveries can be reconciled.

#### Scenario: Platform redelivers an event
- **WHEN** a connector receives an event identifier that was already committed
- **THEN** the system acknowledges the delivery without creating another canonical event

### Requirement: Connectors normalize without losing vendor semantics
The system SHALL map common identity, conversation, message, attachment and lifecycle fields into a canonical envelope while preserving validated connector-specific extensions and raw references.

#### Scenario: LINE unsend event is received
- **WHEN** LINE sends an authenticated unsend event
- **THEN** the connector emits a canonical recall event linked to the mapped message and retains only the raw metadata allowed by Organization policy

### Requirement: External identities map explicitly to platform principals
The system SHALL maintain tenant-scoped mappings between external users, external conversations and internal principals or visitor identities without merging identities solely by display name.

#### Scenario: Same person contacts through two platforms
- **WHEN** two connector identities have not completed an authorized account-linking flow
- **THEN** the system keeps them as separate external principals

### Requirement: Outbound delivery respects platform capabilities and limits
The system SHALL queue outbound messages per installation, enforce rate limits, retry retryable failures, stop on terminal failures and expose delivery status.

#### Scenario: Slack returns a rate-limit response
- **WHEN** Slack rejects an outbound request with a retry interval
- **THEN** the connector defers delivery until the allowed time without blocking deliveries for unrelated Organizations

### Requirement: Conversation bridging is explicit
The system SHALL require an authorized bridge policy before forwarding content between an external channel and an internal or different external conversation.

#### Scenario: No bridge exists
- **WHEN** a message arrives from a connected external conversation with no bridge policy
- **THEN** the message remains in its mapped inbox and is not copied to other channels

### Requirement: Web Widget enforces embedding policy
The system SHALL issue Organization-scoped widget configuration and SHALL enforce allowed origins, visitor session continuity, abuse controls and conversation routing.

#### Scenario: Widget runs on an unauthorized domain
- **WHEN** a browser initializes a widget from a domain outside the Organization allowlist
- **THEN** the system refuses to create or resume a visitor conversation

### Requirement: Connector capability differences are observable
The system SHALL publish a capability matrix for each connector installation covering supported message types, threads, edits, recalls, reactions, receipts and interactive elements.

#### Scenario: Operator inspects connector status
- **WHEN** an Organization admin views a connector installation
- **THEN** the UI and API show supported capabilities, degraded functions, current health and pending delivery backlog
