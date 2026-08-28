## Purpose

將產品、Agent、程式介面、執行檔、設定、資料路徑、映像、文件與營運識別完整統一為 888a2a，並安全承接既有安裝所需的資料遷移與授權歸屬。

## ADDED Requirements

### Requirement: Product identity is 888a2a
The system SHALL use `888a2a` as the product name and `888a2a Agent` as the Agent role name on every new public, administrative and operational surface.

#### Scenario: User opens any product surface
- **WHEN** a user opens the web UI, CLI help, installer, generated documentation or runtime status
- **THEN** the visible product and Agent names use only 888a2a terminology

### Requirement: Technical identifiers have defined 888a2a targets
The system SHALL define and use 888a2a identifiers for module paths, Proto namespaces, resources, binaries, CLI commands, environment variables, config keys, data paths, sockets, service names, images, release assets, cookies, metrics and permissions.

#### Scenario: Clean installation completes
- **WHEN** an operator performs a clean installation
- **THEN** every created file, directory, service, image reference, environment example and emitted metric uses the approved 888a2a identifier mapping

### Requirement: Environment variables use a shell-safe prefix
The system SHALL use `A2A888_` as the environment-variable prefix because shell variable names cannot begin with a digit.

#### Scenario: Operator configures the platform
- **WHEN** an operator follows current deployment documentation
- **THEN** every environment variable is valid in supported shells and begins with `A2A888_`

### Requirement: Existing state can migrate safely
The system SHALL provide a one-time importer or bounded compatibility reader for existing configuration, credentials, sessions and workspaces before removing old locations or keys.

#### Scenario: Existing Machine upgrades
- **WHEN** a Machine with existing credentials, Agent sessions and workspace data starts the 888a2a release
- **THEN** the state is imported or read safely, preserved atomically and subsequently written only to 888a2a locations

### Requirement: Compatibility does not leak into new contracts
The system SHALL confine legacy identifier support to documented migration code and SHALL NOT emit legacy names in new APIs, events, logs, UI or generated artifacts.

#### Scenario: Migrated client performs normal work
- **WHEN** a migrated client creates resources and executes an Agent task
- **THEN** all new resource names, events and operational output use 888a2a identifiers

### Requirement: License and attribution survive rebranding
The system SHALL preserve all copyright, license, NOTICE and source-attribution obligations while changing product branding.

#### Scenario: 888a2a distribution is produced
- **WHEN** a release artifact or source distribution is published
- **THEN** it contains the required license and attribution records without presenting the former product brand as the current product name

### Requirement: Rename completion has a zero-residual gate
The system SHALL fail the rename completion check when an unapproved legacy product identifier remains outside the migration allowlist or required attribution records.

#### Scenario: Legacy identifier remains in runtime code
- **WHEN** repository validation finds a legacy product identifier in non-migration runtime code
- **THEN** the validation fails and the release cannot be marked rename-complete
