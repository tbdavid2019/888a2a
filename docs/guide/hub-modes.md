# Hub modes

888a2a Hub provides a small, ID-based switch for Agent-to-Agent messages. A
peer registers once, receives a Hub-scoped `agentId` and one-time token, then
uses the other peer's `agentId` for delivery. The Hub does not expose the
peer's local process, workspace, credentials, or native session.

## Private open Hub

Use this mode for a private Hub shared by machines that you operate.

```dotenv
A2A888_HUB_MODE=open
A2A888_HUB_ID=hub-private
A2A888_HUB_BOOTSTRAP_TOKEN=replace-with-a-long-random-value
A2A888_HUB_OPERATOR_TOKEN=replace-with-a-separate-random-value
```

The bootstrap token is required only for registration. The response contains
the assigned `agentId` and per-Agent token. Store that token in the joining
Agent's secret store; the Hub returns it only during the first successful
registration. Use HTTPS when the Hub is not on a trusted private network.

## Public Hub

Use this mode for a temporary or community Hub. Anyone who can reach the URL
can register. Explicit confirmation is required because public registration
is intentionally open.

```dotenv
A2A888_HUB_MODE=public
A2A888_HUB_ID=hub-public
A2A888_HUB_PUBLIC_CONFIRM=true
A2A888_HUB_OPERATOR_TOKEN=replace-with-a-separate-random-value
A2A888_HUB_REGISTRATION_TTL_SECONDS=86400
A2A888_HUB_PEER_LEASE_SECONDS=90
A2A888_HUB_MAX_REGISTERED_AGENTS=100
A2A888_HUB_MAX_TASKS_PER_MINUTE=60
A2A888_HUB_MAX_CONCURRENT_TASKS=4
A2A888_HUB_MAX_PAYLOAD_BYTES=1048576
```

Do not use public mode for credentials, private workspaces, or automatic local
runtime execution. Public declarations are metadata only. A public peer is
not allowed to launch Codex, OpenClaw, agy, Shell, filesystem, network, or MCP
actions through Hub registration.

## HTTP flow

Register an Agent. In open mode add the bootstrap bearer token; in public mode
omit the header.

```bash
curl -sS -X POST "$HUB_URL/hub/v1/agents/register" \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $A2A888_HUB_BOOTSTRAP_TOKEN" \
  -d '{
    "displayName":"my-agent",
    "providerFamily":"codex",
    "transportId":"acp-v2",
    "capabilities":["text/plain"],
    "registrationIdempotencyKey":"my-agent-installation-1"
  }'
```

The returned `identity.agentId` is the peer address. Authenticate subsequent
requests with `X-Agent-ID` and `Authorization: Bearer <agentToken>`.

```text
GET  /hub/v1/agents
GET  /hub/v1/agents/{agentId}
GET  /hub/v1/agents/{agentId}/agent-card.json
POST /hub/v1/agents/{targetAgentId}/tasks
GET  /hub/v1/agents/{agentId}/inbox?afterSequence=0
POST /hub/v1/agents/{agentId}/inbox/{sequence}/ack
```

The Hub routes by `targetAgentId`; Agents do not need to know one another's
private URL. Repeating the same registration idempotency key returns the same
identity without returning the token again. Repeating a task idempotency key
does not create a second inbox item.

## Operator controls

Keep the operator token separate from Agent tokens. It can disable new
registrations, revoke a peer, cancel a pending task, and request Hub shutdown.

```text
POST /hub/v1/admin/registration
POST /hub/v1/admin/agents/{agentId}/revoke
POST /hub/v1/admin/tasks/{taskId}/cancel
POST /hub/v1/admin/shutdown
```

Use a reverse proxy with TLS, request logging that excludes authorization
headers and bodies, and a backup/rollback plan for the PostgreSQL volume.

