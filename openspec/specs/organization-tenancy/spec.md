# organization-tenancy Specification

## Purpose
建立可商業化 SaaS 的組織級租戶、成員、Agent、Workspace 與 IAM 邊界，確保多個組織共用平台時資料、權限、憑證及運算資源仍可隔離與治理。

## Requirements

### Requirement: Organization is the tenant boundary
The system SHALL treat an Organization as the mandatory tenant boundary for every human, agent, service account, workspace, conversation, task, machine, connector, approval, audit event, credential and usage event.

#### Scenario: Cross-organization resource access is denied
- **WHEN** a principal authenticated for Organization A requests a resource owned by Organization B
- **THEN** the system returns a permission-denied result without revealing whether the target resource exists

### Requirement: Users can belong to multiple organizations
The system SHALL allow one human identity to hold independent memberships, roles and status in multiple Organizations.

#### Scenario: User switches active organization
- **WHEN** a user selects another Organization in which the user has an active membership
- **THEN** subsequent resource lists, permissions, quotas and audit context are scoped to the selected Organization

### Requirement: Human and machine principals remain distinct
The system SHALL model human principals, agent principals and service accounts as distinct principal types with separate credentials and permission grants.

#### Scenario: Human acts through an agent
- **WHEN** a human asks an Agent to perform an action
- **THEN** the action records both the requesting human and the executing Agent without allowing either identity to impersonate the other

### Requirement: Organization roles and groups govern access
The system SHALL support Organization owners, admins, members, billing admins, agent admins and approvers, plus custom roles and groups whose grants can be scoped to workspaces and resources.

#### Scenario: Group receives workspace access
- **WHEN** an Organization admin grants a group access to a workspace
- **THEN** every active member of that group receives the effective permissions defined by the binding

### Requirement: Workspace and project resources are organization-scoped
The system SHALL place collaborative resources under an Organization and a Workspace or Project where applicable, while preserving stable resource identifiers.

#### Scenario: Workspace resource is created
- **WHEN** an authorized member creates a Channel, Agent or Task inside a workspace
- **THEN** the resource inherits the Organization and workspace scope and cannot later be moved across Organizations

### Requirement: Organization lifecycle is enforceable
The system SHALL support active, suspended and closed Organization states and SHALL enforce the state consistently across human APIs, connectors, A2A and Machine execution.

#### Scenario: Organization is suspended
- **WHEN** an Organization is suspended by an authorized platform operator
- **THEN** new messages, connector deliveries, A2A work and runtime executions are rejected while data remains available for authorized recovery and export
