## Purpose

提供可在一台或多台 Machine 上安全承載十幾個 888a2a Agent 的 Runtime Gateway，統一 Provider manifest、pinned npm／npx、session、workspace 與可靠 assignment。

## ADDED Requirements

### Requirement: Providers have validated manifests
The system SHALL require every selectable Provider to declare runtime type, protocol, platforms, executable or package, pinned version, integrity, capabilities, session behavior and permission profile.

#### Scenario: Provider manifest is incomplete
- **WHEN** a manifest omits a required version, protocol, binary or integrity field for a production npm Provider
- **THEN** the Provider is rejected before installation or selection

### Requirement: npm runtime is prepared atomically
The system SHALL install pinned npm packages into an immutable Machine cache, verify integrity and publish READY only after the complete package and local binary are available.

#### Scenario: npm installation is interrupted
- **WHEN** a package download or preparation process terminates before completion
- **THEN** the partial installation is not selected and the last verified version remains available

### Requirement: Agent turns never resolve floating packages
The system SHALL launch Agent turns from a prepared local binary and SHALL NOT perform implicit network installation, auto-update or `latest` resolution during execution.

#### Scenario: Registry is unavailable during a turn
- **WHEN** a verified cached Provider is selected while the npm registry is unreachable
- **THEN** the Agent turn starts using the cached local binary

### Requirement: Multiple Agents are isolated on one Machine
The system SHALL isolate workspace, session, environment, credentials, limits and process lifecycle for each Agent while allowing immutable Provider package data to be shared.

#### Scenario: Twelve Agents share a Machine
- **WHEN** twelve configured Agents run across supported Providers on the same Machine
- **THEN** each Agent can execute and resume without reading or mutating another Agent's workspace, session or credentials

### Requirement: Machine assignments are durable
The system SHALL sequence, persist, acknowledge, retry and replay Agent create, config-update and remove assignments.

#### Scenario: Assignment send races with disconnect
- **WHEN** a Machine disconnects before acknowledging an assignment
- **THEN** the assignment remains pending and is replayed idempotently after reconnect

### Requirement: Provider sessions resume when compatible
The system SHALL persist Provider session identity and a launch fingerprint and SHALL report resume, cold start and fallback outcomes.

#### Scenario: Machine process restarts
- **WHEN** an Agent runs again after the Machine process restarts with compatible Provider configuration
- **THEN** the runtime attempts to resume the persisted session and preserves Agent conversation continuity

### Requirement: Compatibility levels are evidence-based
The system SHALL publish detected, protocol-ready, functionally verified and full-loop verified status by Provider version and platform.

#### Scenario: Provider only passes detection
- **WHEN** the binary exists but initialize or a required runtime probe fails
- **THEN** the UI marks the Provider detected but unavailable for automatic Agent execution
