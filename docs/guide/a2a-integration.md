# A2A integration

888a2a exposes an authenticated A2A 1.0 HTTP+JSON gateway on the Manager:

```text
GET  /.well-known/agent-card.json
GET  /a2a/v1/{organization}/agents/{agent}/agent-card.json
POST /a2a/v1/{organization}/agents/{agent}/message:send
POST /a2a/v1/{organization}/agents/{agent}/message:stream
GET  /a2a/v1/{organization}/agents/{agent}/tasks/{task}
```

Agent Card requests are public. Task operations require an 888a2a access token
and the organization path must match the caller's active membership. The
gateway stores accepted tasks in PostgreSQL before returning the response.

## Client compatibility

- A2A clients that implement the official HTTP+JSON protocol can connect
  directly by loading the Agent Card.
- The local Codex CLI speaks ACP v2/app-server, not A2A HTTP+JSON. 888a2a can
  use Codex as a local Provider through its ACP runtime; making Codex an
  external A2A peer requires an explicit A2A↔ACP bridge.
- OpenClaw's documented local integration surfaces are its Gateway WebSocket,
  OpenResponses `/v1/responses`, and `/tools/invoke`. 888a2a now includes an
  explicit authenticated OpenClaw bridge that uses `/v1/responses`; it does
  not expose `/tools/invoke`. A live bridge check still requires a running
  local Gateway and its operator-approved credential.
- Antigravity (`agy`) provides CLI print/stream and MCP surfaces, not a native
  A2A HTTP server. Use an explicit bridge that owns the A2A Agent Card and
  forwards requests to `agy` with a bounded process and workspace policy.

Do not expose OpenClaw `/tools/invoke` or an ACP bridge directly to the public
internet. Keep bridge credentials in the local secret store and pass only
tenant-scoped A2A credentials to the Manager.

## Local provider bridges

The Gateway only executes a local provider when an explicit bridge is
configured. Without one, a task fails with a clear `no verified provider
bridge is configured` result; it is never completed by a placeholder executor.

The first bridges are:

- Codex CLI: set `A2A888_A2A_BRIDGE_PROVIDER=codex` and an absolute
  `A2A888_A2A_BRIDGE_WORKDIR`.
- agy / Antigravity CLI: set `A2A888_A2A_BRIDGE_PROVIDER=agy` and an absolute
  `A2A888_A2A_BRIDGE_WORKDIR`.
- OpenClaw Gateway: set `A2A888_A2A_BRIDGE_PROVIDER=openclaw`,
  `A2A888_OPENCLAW_GATEWAY_URL` to a private or loopback Gateway URL, and
  `A2A888_OPENCLAW_GATEWAY_TOKEN`. The optional
  `A2A888_OPENCLAW_AGENT_ID` selects the OpenClaw agent; otherwise the Gateway
  default agent is used.

`A2A888_A2A_BRIDGE_AGENT_ID` limits the bridge to one 888a2a Agent resource and
defaults to `default`. Provider credentials remain local process configuration;
they are not stored in the Manager database, Agent Card, task metadata, or
logs. Codex and agy currently use the bounded CLI bridge. The native Codex ACP
v2 session adapter remains a separate compatibility path until its full A2A
session mapping gate passes.
