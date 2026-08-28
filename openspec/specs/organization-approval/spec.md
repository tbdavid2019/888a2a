# organization-approval Specification

## Purpose
讓每個 Organization 依自身團隊、資源、技能、風險與職責分工建立 Approval Policy，並對人類、Bot、A2A 與 runtime action 統一執行。

## Requirements

### Requirement: Approval policies are organization-scoped
The system SHALL allow each Organization to define approval policies scoped by workspace, resource, Agent, skill, action type, requester class, destination and risk level.

#### Scenario: Finance policy matches a payment action
- **WHEN** a Finance Agent requests a payment action that matches an Organization policy
- **THEN** the system creates an approval request using that policy's approvers, quorum, expiry and escalation rules

### Requirement: Approvers can be users, groups or roles
The system SHALL resolve eligible approvers from explicit users, Organization groups and roles at decision time while excluding suspended or conflicted members.

#### Scenario: Group membership changes during approval
- **WHEN** a member is removed from the selected approver group before deciding
- **THEN** that member can no longer approve the pending request

### Requirement: Approval requests bind immutable action intent
The system SHALL bind each approval request to the requester, executing Agent, Organization, resource, action, normalized parameters, destination, risk summary, task, command, expiry and nonce.

#### Scenario: Tool parameters change after approval
- **WHEN** an Agent changes any bound material parameter after approval
- **THEN** the old decision is invalid and the changed action requires a new policy evaluation

### Requirement: Policies support quorum and separation of duties
The system SHALL support configurable quorum and SHALL allow policies to prevent the requester or executing Agent owner from being the sole approver.

#### Scenario: Two-person approval is required
- **WHEN** a policy requires two distinct approvers
- **THEN** the action remains blocked until two eligible principals approve or a terminal deny or expiry occurs

### Requirement: Approval has explicit lifecycle
The system SHALL model pending, approved, denied, expired, cancelled, superseded and executed states and SHALL record every transition with actor and timestamp.

#### Scenario: Approval expires
- **WHEN** a pending request reaches its expiry without quorum
- **THEN** the request becomes expired and the protected action cannot execute using that request

### Requirement: Escalation is policy-driven
The system SHALL support timed escalation to another user, group or role without silently converting a missing decision into approval.

#### Scenario: Primary approvers do not respond
- **WHEN** the configured escalation delay elapses
- **THEN** the request adds or transfers eligibility according to policy and records the escalation event

### Requirement: A2A authorization uses approval safely
The system SHALL translate applicable pending approvals to A2A authorization-required state and SHALL keep credentials out of ordinary messages and audit payloads.

#### Scenario: Approval completes an A2A authorization request
- **WHEN** valid quorum is reached for an A2A task
- **THEN** the task can resume using a narrowly scoped authorization result bound to the requesting Agent and action

### Requirement: Approval administration is auditable
The system SHALL restrict policy creation and modification to authorized Organization roles and SHALL preserve versioned policy and decision history.

#### Scenario: Admin changes a policy
- **WHEN** an Organization admin changes approvers or risk rules
- **THEN** the system records the old and new policy versions and applies the configured versioning rule to pending requests
