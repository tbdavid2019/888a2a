## 1. Hub contracts and configuration

- [x] 1.1 Define Hub mode, registration declaration, assigned identity, token response, peer state, lease, and capability contracts; verify protobuf/JSON validation rejects unknown modes and oversized fields.
- [x] 1.2 Add closed/open/public configuration with closed as the default; verify startup configuration and environment parsing never enables public mode implicitly.
- [x] 1.3 Add registration, heartbeat, disconnect, revoke, directory, inbox, and peer-task route contracts; verify consistent 400/401/403/404/409/429 error bodies.

## 2. Identity and persistence

- [x] 2.1 Add PostgreSQL tables and migration for Hubs, registered Agents, token hashes, registration idempotency, leases, revocation, and peer declarations; verify fresh install and upgrade migration paths.
- [x] 2.2 Implement bootstrap-token and per-Agent-token issuance, hashing, expiration, rotation, and revocation; verify plaintext tokens never persist or appear in logs/API metadata.
- [x] 2.3 Implement registration idempotency and Hub-scoped random Agent IDs; verify retries return one identity and caller-selected IDs cannot overwrite existing peers.
- [x] 2.4 Implement lease heartbeats and stale-peer reconciliation; verify expired peers are offline and cannot receive automatic push work.

## 3. Hub directory and mailbox

- [x] 3.1 Implement authenticated Agent connect/disconnect/heartbeat lifecycle; verify open and public registration paths cannot bypass token or policy checks.
- [ ] 3.2 Implement peer directory lookup/list by server-assigned ID with safe Agent Card projection; verify private endpoints, paths, credentials, and native session IDs are absent.
- [ ] 3.3 Implement durable peer inbox polling with cursor, acknowledgment, timeout, and cancellation; verify reconnect replay is ordered and idempotent.
- [ ] 3.4 Implement `target_agent_id` task routing through the mailbox; verify offline, revoked, unknown, duplicate, and ambiguous-delivery outcomes are truthful.

## 4. Public Hub safety

- [ ] 4.1 Add public-mode rate limits, registration quotas, task budgets, concurrency caps, payload limits, lease TTL, and expiry cleanup; verify limit failures do not invoke a runtime.
- [ ] 4.2 Add public capability policy and bridge gate; verify a public peer declaration cannot launch Codex, OpenClaw, agy, shell, filesystem, network, or MCP without an approved verified bridge.
- [ ] 4.3 Add operator controls to disable registration, revoke peers, cancel peer work, and shut down the Hub; verify unavailable policy state fails closed.

## 5. A2A and operator surfaces

- [ ] 5.1 Project Hub-registered peers into A2A Agent Cards and preserve existing closed-mode routes; verify peer ID, Hub scope, readiness, and capability projection.
- [ ] 5.2 Add A2A client interoperability tests for registration, card, directory, send, stream, get, list, cancel, inbox replay, and cross-Hub/cross-tenant rejection.
- [ ] 5.3 Add private-Hub and public-Hub deployment documentation with HTTPS, token custody, rate limits, expiry, revocation, and rollback guidance; use Traditional Chinese Taiwan terminology.
- [ ] 5.4 Add Hub mode and peer management UI with explicit safety labels and no token display after enrollment; verify public mode cannot expose automatic runtime actions.
- [ ] 5.5 Run full backend/frontend/proto/naming/build checks, update the dated CHANGELOG, and push a green GitHub Actions run before marking this change complete.
