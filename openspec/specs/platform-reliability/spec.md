# platform-reliability Specification

## Purpose
建立大型多租戶平台所需的可靠事件處理、水平擴展、背壓、稽核、監控、保留、對帳與災難復原行為，避免關鍵工作只存在單一進程記憶體。

## Requirements

### Requirement: Critical delivery uses durable inbox and outbox
The system SHALL commit critical outbound intent with its source transaction and SHALL persist inbound delivery identity before asynchronous processing.

#### Scenario: Process crashes after source commit
- **WHEN** a service crashes after committing a message or assignment but before external delivery
- **THEN** another worker can discover and deliver the pending outbox record

### Requirement: Event consumers are idempotent
The system SHALL require every durable consumer to use a stable event identity and tolerate at-least-once delivery without duplicating externally observable effects.

#### Scenario: Consumer receives an event twice
- **WHEN** the event bus redelivers an already completed event
- **THEN** the consumer returns the recorded result or no-op without duplicating messages, tasks, approvals or usage

### Requirement: Realtime works across service instances
The system SHALL allow clients, Machines and Agents connected to different service replicas to receive authorized events and command state without relying on process-local registries as the sole signal.

#### Scenario: Message is written on another replica
- **WHEN** replica A commits a conversation event and the reader is long-polling or streaming through replica B
- **THEN** replica B is notified and returns the new durable event

### Requirement: Backpressure preserves tenant fairness
The system SHALL bound queues and concurrency per Organization, connector, Agent and destination and SHALL isolate an overloaded tenant from unrelated tenants.

#### Scenario: One connector installation floods events
- **WHEN** one Organization exceeds its inbound connector processing allowance
- **THEN** its events are throttled or queued without exhausting workers reserved for other Organizations

### Requirement: Failures are observable and replayable
The system SHALL expose pending, retrying, dead-lettered and reconciled states for connector events, Machine assignments, Agent tasks, approvals and usage processing.

#### Scenario: Event reaches retry limit
- **WHEN** a retryable event exceeds its configured attempts or age
- **THEN** the system moves it to a tenant-scoped dead-letter state, alerts operators and allows authorized replay

### Requirement: Audit records are tenant-scoped and tamper-evident
The system SHALL record security, policy, connector, A2A, runtime, approval and administrative events with actor, tenant, target, correlation and outcome while excluding secrets.

#### Scenario: Auditor traces an external request
- **WHEN** an authorized auditor opens an A2A or connector event
- **THEN** the system can trace the inbound identity, normalized event, policy decisions, task graph, runtime actions and outbound deliveries within that Organization

### Requirement: Retention and deletion are enforceable
The system SHALL apply Organization retention and deletion schedules across messages, attachments, tasks, audit, connector raw payloads and runtime artifacts while honoring legal holds.

#### Scenario: Retention job processes expired connector payload
- **WHEN** a raw connector payload exceeds its retention period and has no legal hold
- **THEN** the system deletes or irreversibly redacts it and records the retention outcome

### Requirement: Reconciliation repairs external drift
The system SHALL support periodic reconciliation between Organization state and IM, connector, Machine and runtime projections.

#### Scenario: Connector membership drift is detected
- **WHEN** a reconciliation finds a platform mapping inconsistent with current Organization policy
- **THEN** the system repairs or quarantines the mapping and records the discrepancy

### Requirement: Disaster recovery has measurable objectives
The system SHALL define and test recovery point and recovery time objectives for tenant metadata, messages, tasks, approvals, credentials, object storage and runtime state.

#### Scenario: Primary region is restored from backup
- **WHEN** operators perform a scheduled disaster-recovery exercise
- **THEN** the restored environment meets the declared objectives and produces a verifiable reconciliation report
