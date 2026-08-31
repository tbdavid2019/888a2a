## MODIFIED Requirements

### Requirement: Organization is the tenant boundary

The system SHALL treat an Organization as the mandatory tenant boundary for
every human, agent, service account, workspace, conversation, task, machine,
connector, approval, audit event, credential, usage event, local filesystem
object and S3-compatible object.

#### Scenario: Cross-organization resource access is denied

- **WHEN** a principal authenticated for Organization A requests a resource
  owned by Organization B
- **THEN** the system returns a permission-denied result without revealing
  whether the target resource exists

#### Scenario: Local object keys are tenant-prefixed

- **WHEN** two organizations store the same raw object key in local storage
- **THEN** the resulting filesystem paths are distinct and each organization can
  access only its own path through the authenticated service
