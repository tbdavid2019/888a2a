# product-identity-migration Specification

## Purpose
確保 Agent Network 新增的所有產品、Agent、CLI、runtime、環境與文件介面從第一天只使用 888a2a／888a2a Agent identity，並為完整 rename change 建立阻止回歸的邊界。

## Requirements

### Requirement: Agent Network surfaces use 888a2a identity
The system SHALL use `888a2a` and `888a2a Agent` on every new Agent Network API, CLI, runtime status, binary, installer, log and document surface.

#### Scenario: Operator starts the Agent Network
- **WHEN** an operator installs, configures and starts the focused Agent Network release
- **THEN** every newly introduced visible surface uses 888a2a terminology

### Requirement: New environment variables are shell-safe
The system SHALL use the `A2A888_` prefix for every new environment variable introduced by this change.

#### Scenario: Operator configures npm runtime
- **WHEN** an operator configures Provider runtime behavior through environment variables
- **THEN** all documented names begin with `A2A888_` and work in supported shells

### Requirement: New code rejects legacy identifiers
The system SHALL include a validation gate that rejects legacy product identifiers introduced in files added or materially changed by this focused change, excluding approved migration and attribution records.

#### Scenario: Changed runtime file introduces legacy branding
- **WHEN** validation finds an unapproved legacy identifier in a changed Agent Network file
- **THEN** the validation fails before the change can be released
