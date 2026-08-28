## Context

See `proposal.md` for the motivation. The existing A2A gateway has durable task storage and tenant IAM, but it assumes that every caller is already an 888a2a principal. The new Hub layer must add enrollment and peer leases without weakening the existing closed-mode authorization path.

## Goals / Non-Goals

**Goals:**

- Let a local or remote Agent join a selected Hub with one bootstrap exchange.
- Give every registered Agent a Hub-scoped ID and independently revocable token.
- Route work through a durable Hub mailbox addressed by peer ID.
- Make `open` useful for a private two-machine Hub and make `public` bounded by safe defaults.
- Preserve A2A 1.0 compatibility for Agent Cards and task operations.

**Non-Goals:**

- Anonymous unlimited remote shell, ACP, MCP, or provider execution.
- Global IDs that are valid across unrelated Hubs.
- Persisting provider secrets, native runtime session IDs, or complete environment dumps.
- Replacing existing organization IAM or forcing all current closed-mode users through registration.

## Decisions

1. **Three explicit modes.** Add `closed`, `open`, and `public` to Hub configuration. `closed` remains the default. `open` requires a bootstrap token. `public` requires an explicit operator flag and has a shorter default lease, stricter quotas, and a restricted capability profile.

2. **Outbound registration plus polling mailbox.** An Agent sends `POST /hub/v1/agents:register` to the Hub, then uses its returned token for heartbeat and `GET /hub/v1/agents/{agent_id}/inbox`. This works behind NAT and does not require the Hub to connect to an Agent's private address. Server-sent events may be added later; durable polling is the correctness path.

3. **Separate enrollment and runtime credentials.** The bootstrap token is accepted only by registration. The Hub generates a random per-Agent token, stores only its hash, and returns it once over an allowed transport. Task requests use the per-Agent token. A registration token is never reused as a peer credential.

4. **Hub-scoped identity.** Store a random `hub_id` in Hub configuration and generate a random `agent_id` for each enrollment. Peer references include the Hub context. A reconnect with the same registration idempotency key returns the existing identity; a new key creates a new identity.

5. **Durable mailbox routing.** `target_agent_id` resolves to a registered peer record, and task delivery is appended to the existing durable A2A work/event tables before the caller receives an accepted response. Offline peers keep work pending; ambiguous delivery is `OUTCOME_UNKNOWN`, with no cross-transport retry.

6. **Capability and bridge gates.** A declaration describes capabilities, but it does not grant them. Public peers are Pull-only until an operator verifies a bridge. The bridge registry remains the only path to local Codex, OpenClaw, agy, shell, filesystem, network, or MCP execution.

7. **Policy enforcement at the Hub edge.** Registration, heartbeat, inbox, directory, and task routes use one authentication and policy layer. It applies IP and token rate limits, payload limits, lease expiry, per-peer quotas, tenant or Hub scope checks, and fail-closed behavior when policy state cannot be read.

8. **API compatibility.** Keep existing `/a2a/v1/{organization}/agents/{agent}` routes unchanged for closed-mode clients. Add Hub registration and peer-ID routes under `/hub/v1/`; the Hub can project a registered peer into an A2A Agent Card without exposing its private transport endpoint.

## Risks / Trade-offs

- [Public registration attracts spam and resource exhaustion] → Require explicit enablement, short leases, IP/token limits, quotas, bounded payloads, and operator revocation.
- [A bearer token can be copied] → Return it once, store only a hash, support rotation/revocation, use HTTPS outside loopback, and never place it in URLs or logs.
- [Polling increases latency] → Prefer correctness and NAT compatibility first; add SSE/WebSocket acceleration only over the same durable cursor and mailbox.
- [A peer declares unsafe capabilities] → Treat declarations as metadata only; execution still requires a verified bridge and runtime policy.
- [Two Hubs use the same peer label] → IDs are random and Hub-scoped; never use display names as routing keys.

## Migration Plan

1. Add configuration with `closed` as the default and no behavior change for current A2A or machine clients.
2. Add schema and registration APIs, then enable `open` only through explicit operator configuration.
3. Add directory and mailbox routing with deterministic fake peers before enabling public mode.
4. Add the public-mode safety gate and operator documentation; deploy public mode only behind HTTPS and a rate-limited ingress.
5. Roll back by disabling registration. Preserve registered identities and durable work for inspection; revoke active peer tokens when required.
