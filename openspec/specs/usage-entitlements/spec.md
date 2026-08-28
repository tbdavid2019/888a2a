# usage-entitlements Specification

## Purpose
預留組織收費、方案權益、配額與使用量計量邊界，使產品能先執行功能限制與成本治理，之後再接任意付款或合約系統。

## Requirements

### Requirement: Organizations have billing-independent entitlements
The system SHALL evaluate product access through Organization entitlements rather than direct checks against a payment provider or plan name.

#### Scenario: Feature entitlement is absent
- **WHEN** an Organization requests a feature without the required entitlement
- **THEN** the system rejects the request with a stable entitlement error and does not call an external billing provider

### Requirement: Quotas are enforceable by resource and period
The system SHALL support quotas for seats, Agents, Machines, connectors, A2A requests, concurrency, execution time, tokens, storage and outbound messages over configured periods.

#### Scenario: Organization reaches Agent concurrency quota
- **WHEN** starting another Agent task would exceed the Organization concurrency quota
- **THEN** the system queues or rejects the task according to the entitlement policy and records the quota decision

### Requirement: Usage events are immutable and idempotent
The system SHALL record usage events with Organization, workspace, principal, Agent, feature, quantity, unit, timestamp, source reference and idempotency key.

#### Scenario: Metering event is replayed
- **WHEN** a consumer processes the same source usage event again
- **THEN** the usage ledger contains only one chargeable event for that idempotency key

### Requirement: Usage aggregates are reproducible
The system SHALL derive reporting aggregates from immutable usage events and SHALL support recomputation after correcting aggregation logic.

#### Scenario: Aggregation rule changes
- **WHEN** an authorized operator recomputes a billing period using a corrected rule
- **THEN** the aggregate is rebuilt from source events without modifying the original events

### Requirement: Subscription lifecycle is provider-neutral
The system SHALL model billing account, subscription state, effective period, entitlement set and grace policy without depending on Stripe, Paddle or another provider-specific identifier in product authorization logic.

#### Scenario: Future billing provider updates subscription
- **WHEN** an authenticated billing adapter reports a subscription change
- **THEN** the system updates the Organization entitlement state through the stable internal contract

### Requirement: Usage visibility follows organization roles
The system SHALL expose current entitlement, quota consumption and usage summaries to authorized Organization owners and billing admins.

#### Scenario: Regular member requests billing usage
- **WHEN** a member without billing visibility requests Organization-wide usage
- **THEN** the system denies access without exposing aggregate cost or other members' usage details

### Requirement: Suspension and grace are explicit
The system SHALL apply configurable grace, read-only and suspension behavior when entitlement or subscription state becomes invalid.

#### Scenario: Organization enters read-only grace
- **WHEN** an Organization enters a configured read-only grace state
- **THEN** members can access permitted existing data while new chargeable executions and outbound deliveries are blocked
