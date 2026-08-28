## MODIFIED Requirements

### Requirement: Compatibility levels are evidence-based
The system SHALL publish detected, protocol-ready, functionally verified and full-loop verified status by Provider version, platform and selected transport. A bridge-required or Pull-only Provider SHALL remain distinguishable from a detected-only Provider and SHALL not be treated as automatically executable.

#### Scenario: Provider only passes detection
- **WHEN** the binary exists but initialize or a required runtime probe fails
- **THEN** the UI marks the Provider detected but unavailable for automatic Agent execution

#### Scenario: Provider bridge is not configured
- **WHEN** the Provider binary passes detection but its configured A2A/ACP/Gateway/CLI bridge has no verified configuration
- **THEN** the runtime reports BRIDGE_REQUIRED or PULL_ONLY with evidence describing the missing bridge, and preserves explicit Pull access
