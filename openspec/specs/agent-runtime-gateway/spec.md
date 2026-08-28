# agent-runtime-gateway Specification

## Purpose
建立可擴充且受治理的本機 Agent 執行層，以 ACP、固定版本 npm／npx、MCP 與 embedded runtime 安全接入多種 Coding Agent。

## Requirements

### Requirement: Providers declare a stable capability manifest
The system SHALL require each built-in or installed Provider to declare its stable identifier, runtime type, protocol, platform support, executable or package, version, capabilities, permission profile and verification status.

#### Scenario: Provider is discovered
- **WHEN** a Machine probes an installed Provider
- **THEN** the Manager receives the detected version and only the capabilities that the probe verified

### Requirement: npm Providers are immutable and verified
The system SHALL require production npm Providers to use a pinned package version and verified integrity and SHALL prohibit implicit `latest` resolution during Agent execution.

#### Scenario: Package integrity differs
- **WHEN** a downloaded package does not match the manifest integrity
- **THEN** the Machine quarantines the package and refuses to launch it

### Requirement: Provider installation is separate from turn execution
The system SHALL prepare and cache Provider runtimes before turns and SHALL launch turns from an already prepared local binary without hidden downloads or updates.

#### Scenario: Cached Provider starts offline
- **WHEN** a verified Provider is ready and the package registry is unreachable
- **THEN** the Machine can start the Provider using the cached installation

### Requirement: Runtime state is isolated per Agent
The system SHALL isolate each Agent's workspace, session state, allowed environment, credentials and execution limits even when multiple Agents share a Machine or Provider package cache.

#### Scenario: Two Agents share a Machine
- **WHEN** two Agents use the same Provider version concurrently
- **THEN** they may share immutable package files but cannot read or mutate each other's workspace, session or secret values

### Requirement: Sessions resume when supported
The system SHALL persist protocol session identifiers and fingerprints and SHALL resume compatible sessions while clearly reporting cold starts and resume fallback.

#### Scenario: Provider lost the previous session
- **WHEN** session resume returns a terminal not-found or incompatible result
- **THEN** the runtime starts a fresh session, records the fallback and continues from durable collaboration context

### Requirement: Runtime actions are observable and governed
The system SHALL emit structured lifecycle, output, tool, token, context, permission, cancellation and terminal events and SHALL apply runtime policy before granting tool permission.

#### Scenario: Provider requests a write tool
- **WHEN** a Provider requests filesystem write permission
- **THEN** the request is evaluated by Organization policy and is allowed, denied or routed to approval before execution

### Requirement: Provider compatibility is evidence-based
The system SHALL distinguish detected, protocol-initialized, functionally verified and full-loop verified Provider status for each supported platform and version.

#### Scenario: Provider only passes detection
- **WHEN** a Provider binary is present but its protocol or model probe fails
- **THEN** the compatibility report marks it detected but unavailable for automatic execution

### Requirement: Runtime failure does not lose work
The system SHALL keep pending messages and tasks in durable platform storage when a Machine or Provider is unavailable and SHALL resume eligible work after recovery.

#### Scenario: Machine disconnects during queued work
- **WHEN** a task targets an offline Machine
- **THEN** the task remains queued or transitions to an explicit waiting state without being acknowledged as executed
