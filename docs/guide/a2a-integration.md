# A2A integration

888a2a exposes an authenticated A2A 1.0 HTTP+JSON gateway on the Manager:

```text
GET  /.well-known/agent-card.json
GET  /a2a/v1/{organization}/agents/{agent}/agent-card.json
POST /a2a/v1/{organization}/agents/{agent}/tasks
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
  OpenResponses `/v1/responses`, and `/tools/invoke`. It does not become an
  A2A peer merely by being installed. Use an explicit bridge or an OpenClaw
  plugin that translates A2A HTTP+JSON to its Gateway API.
- Antigravity (`agy`) provides CLI print/stream and MCP surfaces, not a native
  A2A HTTP server. Use an explicit bridge that owns the A2A Agent Card and
  forwards requests to `agy` with a bounded process and workspace policy.

Do not expose OpenClaw `/tools/invoke` or an ACP bridge directly to the public
internet. Keep bridge credentials in the local secret store and pass only
tenant-scoped A2A credentials to the Manager.
