## Why

888a2a currently exposes tenant-scoped A2A routes, but an external Agent cannot join a Hub, receive a server-generated identity, and address another connected Agent by ID. This prevents the intended switch-like deployment model: one private Hub for personal Agents and one public Hub for temporary community Agents.

## What Changes

- Add Hub operating modes: `closed` (existing tenant IAM), `open` (bootstrap-token registration), and `public` (registration without a bootstrap token).
- Add Agent registration that validates a declared Agent Card, assigns a stable Hub-scoped `agent_id`, and returns a revocable per-Agent token.
- Add an authenticated connection, heartbeat, disconnect, and revoke lifecycle for registered Agents.
- Add a Hub directory and peer-ID task routing so callers address `target_agent_id` without knowing the target host or process.
- Keep public mode bounded by default with rate limits, quotas, capability restrictions, expiry, and no implicit local runtime execution.
- Add deterministic registration/routing tests and opt-in external interoperability tests.
- Document private and public Hub deployment, token handling, expiry, revocation, and abuse controls.

## Capabilities

### New Capabilities

- `agent-hub-registration`: Hub mode configuration, Agent enrollment, assigned identity, token lifecycle, heartbeat, and revocation.
- `agent-peer-routing`: Hub directory, peer-ID addressing, connection state, and durable task routing between registered Agents.
- `public-hub-safety`: Public registration defaults, quotas, expiry, capability policy, and fail-closed runtime execution.

### Modified Capabilities

- `openspec/specs/a2a-agent-network`: A2A callers may discover and address Hub-registered peers by server-assigned ID while preserving tenant and credential boundaries.
- `openspec/specs/agent-runtime-foundation`: A bridge may be selected by a Hub-registered Agent binding, but registration alone never enables a local provider runtime.

## Impact

- Adds Hub configuration, registration, token, connection, directory, and routing APIs.
- Adds PostgreSQL tables/migrations for registered Agent identities, hashed tokens, leases, and peer metadata.
- Updates A2A gateway authentication and task routing without changing existing closed-mode clients.
- Updates frontend Hub mode and peer management surfaces plus Traditional Chinese Taiwan copy.
- Adds no required provider dependency; Codex, OpenClaw, agy, and other runtimes remain explicit bridges.
