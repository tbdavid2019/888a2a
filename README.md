# 888a2a

> **Language / 語言:** [English](README.md) | [繁體中文](README_zh.md)

888a2a is an open-source, self-hosted platform for human and AI-agent
collaboration. It brings people, coding agents, tools, tasks, and external
agent networks into one governed workspace.

This repository is an active downstream development project based on
[Laelia](https://github.com/Ranxy/laelia). The long-term direction is an
A2A-first, organization-ready collaboration platform that can connect local
agents, internal bots, external A2A agents, and omnichannel conversations.

> **Project status:** 888a2a is under active development. The agent-network
> foundation is being built now and the project is not yet production-ready.
> Existing Laelia names and compatibility surfaces may still appear while the
> migration is in progress.

## Current foundation

The inherited platform already provides a useful starting point:

- Human-to-agent, human-to-human, and agent-to-agent conversations
- Channels, direct messages, threads, tasks, and scheduled reminders
- Manager and outbound-only Machine components
- ACP-based local agent execution and provider integrations
- MCP extensions, workspace files, IAM, and audit logs
- PostgreSQL-backed state and a React/TypeScript web interface
- Early durable work contracts, provider manifests, and multi-agent testkits

## Roadmap

The following are the major development directions. Detailed proposals and
acceptance criteria live in
[`openspec/changes/`](openspec/changes/).

- [ ] **Product identity and migration** — move public and operational surfaces
  to 888a2a, define compatibility mappings, and provide safe migration for
  existing configuration, credentials, and data.
- [ ] **Agent runtime foundation** — make provider manifests, local npm/npx
  runtimes, ACP v1/v2 execution, session resume, and multi-Machine hosting
  reliable and portable.
- [ ] **A2A 1.0 interoperability** — support Agent Cards, discovery, task
  creation, streaming, polling, artifacts, cancellation, authorization, and
  durable work records through the official protocol boundary.
- [ ] **Multi-agent orchestration** — add bounded delegation, fan-out/fan-in,
  task graphs, retries, budgets, timeouts, and observable parent-child work
  relationships.
- [ ] **Security and approvals** — enforce tenant, workspace, Agent, skill, and
  data boundaries; isolate workspaces and credentials; add risk-based approval
  and escalation flows.
- [ ] **Organization-ready SaaS** — introduce organization tenancy, entitlements,
  usage metering, quotas, billing-ready boundaries, auditability, and stateless
  Manager scaling.
- [ ] **Omnichannel collaboration** — connect Slack, Teams, LINE, WhatsApp,
  web widgets, and other channels while preserving one governed conversation
  and task model.
- [ ] **Reliability and operations** — add shared realtime events, tracing,
  metrics, SLOs, load tests, reconnect recovery, backups, upgrades, and
  production deployment documentation.

The first milestone is the **Agent Network Foundation**: at least twelve
Agents across two Machines should be able to discover peers, exchange bounded
work, return artifacts, resume sessions, survive reconnects, and cancel work
without crossing workspace or credential boundaries.

## Quick start

### One-click test environment

Run a local browser-accessible instance with embedded PostgreSQL and seeded
test accounts:

```bash
scripts/test-server.sh run --workdir /tmp/888a2a-test
```

The command prints the URL and preset accounts. Stop the instance with:

```bash
scripts/test-server.sh stop --workdir /tmp/888a2a-test
```

See [`docs/test-server.md`](docs/test-server.md) for options and caveats.

### Development

```bash
# Backend
go run ./backend/manager/bin/server/main.go --port 8181 --debug

# Frontend
pnpm --dir frontend dev

# Build
go build -ldflags "-w -s" -p=16 -o ./build/laelia ./backend/manager/bin/server/main.go
```

See [`AGENTS.md`](AGENTS.md) for the complete build, test, lint, and release
workflow.

## Architecture

- **Manager** — web UI, API, IAM, durable state, scheduling, dispatch, and
  organization-level policy.
- **Machine** — outbound-only agent host that runs one or more local Agents and
  their provider runtimes.
- **A2A boundary** — the standard interface for Agent discovery and work
  exchange; internal chat remains the human-readable collaboration surface.

## Acknowledgements

888a2a is based on the excellent work in
[Ranxy/laelia](https://github.com/Ranxy/laelia). We sincerely thank Ranxy and
the Laelia contributors for the original platform, architecture,
implementation, and inspiration that make this project possible.

The upstream project is licensed under the
[Apache License 2.0](LICENSE). Please retain the original copyright and
license notices when redistributing or modifying the code. The project is also
inspired by [raft.build](https://raft.build/).

## License

Licensed under the Apache License, Version 2.0. See [`LICENSE`](LICENSE) for
the full text.
