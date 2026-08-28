## MODIFIED Requirements

### Requirement: Provider sessions resume when compatible
The system SHALL persist Provider session identity and a launch fingerprint and SHALL report resume, cold start and fallback outcomes. A Hub registration or peer ID SHALL NOT by itself authorize a local Provider session; execution SHALL require an explicit verified bridge binding for the registered Agent.

#### Scenario: Machine process restarts
- **WHEN** an Agent runs again after the Machine process restarts with compatible Provider configuration
- **THEN** the runtime attempts to resume the persisted session and preserves Agent conversation continuity

#### Scenario: Hub Agent registers without a bridge
- **WHEN** a Hub-registered Agent declares a local Provider but has no verified bridge binding
- **THEN** the runtime keeps the Agent Pull-capable or unavailable and does not launch the Provider automatically
