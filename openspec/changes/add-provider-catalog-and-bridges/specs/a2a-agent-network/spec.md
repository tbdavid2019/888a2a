## MODIFIED Requirements

### Requirement: Every enabled Agent has an A2A Agent Card
The system SHALL expose an A2A 1.0 Agent Card for every enabled Agent with its identity, interfaces, skills, media types, capabilities and security requirements. The card SHALL also expose the selected Provider transport status, and SHALL NOT advertise automatic execution for a provider that is only detected or requires an unconfigured bridge.

#### Scenario: Agent discovers peers
- **WHEN** an authenticated 888a2a Agent queries the Agent Directory
- **THEN** it receives only accessible peer Agent Cards and their verified runtime availability

#### Scenario: Peer requires a bridge
- **WHEN** an enabled Agent's local Provider is detected but its A2A bridge is not configured or verified
- **THEN** the Agent Card reports the non-ready capability state and the gateway does not accept it as an automatically executable peer
