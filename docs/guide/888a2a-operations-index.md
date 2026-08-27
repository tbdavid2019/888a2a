# 888a2a operations index

Date: 2026-08-27

Use these runbooks as the operational entry points for the current platform:

- [Docker deployment](docker-deployment-guide.md): manager, machine, and
  PostgreSQL startup on servers without Go or Node.js.
- [Agent Network operations](agent-network-operator-guide.md): Machine,
  Provider runtime, A2A, and recovery operations.
- [LINE connector contract](../decisions/10.1-line-connector-contract.md):
  webhook verification, deduplication, replies, media, and retry behavior.
- [A2A task graph](../../frontend/src/pages/dashboard/a2a-graph.tsx): the UI
  view for requester, delegates, state, artifacts, approvals, budget, and
  failure cause.

All tenant operations require an explicit Organization scope. Credentials are
stored in the encrypted connector vault and must not be copied into logs,
events, or ordinary task messages.
