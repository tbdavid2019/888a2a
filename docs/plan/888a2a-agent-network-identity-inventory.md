# 888a2a Agent Network identity inventory

Status: reviewed inventory for `build-888a2a-agent-network-foundation` task 1.1.
This document defines names for Agent Network work only. It is not a
whole-repository rename plan and it does not change existing files by itself.

## Naming authority

| Surface | Approved Agent Network name | Rule |
| --- | --- | --- |
| Product | `888a2a` | Use on new user-facing and operational surfaces. |
| Agent role | `888a2a Agent` | Use for an Agent managed by the platform. |
| Manager binary | `888a2a` | Native release output is `build/888a2a`. |
| Machine binary and CLI | `888a2a-machine` | Subcommands retain their meaning: `setup`, `run`, `daemon`, `message`, `task`, `thread`, and `stop`. |
| Go module/import root | `github.com/tbdavid2019/888a2a` | Target for new Agent Network package boundaries; the current module remains a migration input until the master identity change. |
| New Proto package | `a2a888.v1` / `a2a888.store` | Proto identifiers cannot start with a digit. Existing `laelia.*` packages are migration inputs, not names for new contracts. |
| A2A protocol | `A2A 1.0` | Public Agent work boundary: Agent Card, discovery, send, stream, get, list, subscribe, and cancel. |
| Environment prefix | `A2A888_` | Every new product-scoped environment variable must use this shell-safe prefix. |
| Documentation title | `888a2a Agent Network` | Legacy product names may appear only in an explicit migration/attribution mapping. |

## File and surface mapping

The paths below are the Agent Network change surface. A directory entry means
that all files in that package are in scope when the corresponding capability
is implemented; tests in the same package follow the same naming decision.

| Current file or surface | Agent Network responsibility | Target binary / CLI / API / environment / documentation name |
| --- | --- | --- |
| `go.mod`, `backend/**` import paths | Go module and package identity | New boundaries use `github.com/tbdavid2019/888a2a/...`; current imports are compatibility inputs until the master migration. |
| `backend/manager/bin/server/main.go`, `backend/manager/bin/server/cmd/**`, `scripts/build_laelia.sh` | Manager process and native build | Binary `888a2a`; help and logs say `888a2a Manager` or `888a2a`, never introduce a legacy product name. |
| `backend/agent/bin/agent/main.go`, `backend/agent/cmd/**` | Machine process and Agent-facing CLI | Binary/command `888a2a-machine`; command examples use `888a2a-machine <subcommand>`. |
| `backend/agent/supervisor/**`, `backend/agent/state/**`, `backend/agent/home/**` | Machine supervision and local state | Runtime is `888a2a Machine`; new state/config examples use `A2A888_HOME` and `~/.888a2a`. Existing state is read only through migration compatibility. |
| `backend/agent/client/**` | Machine-to-Manager and per-Agent data plane | Logs and status use `888a2a Machine` and `888a2a Agent`; assignment/replay contracts use `a2a888` API types. |
| `backend/agent/provider/**` | Provider registry, detection, manifests and probes | Product surface is `888a2a Agent Runtime`; provider IDs remain provider-owned (`codex`, `claude-code`, `opencode`). Probe client name is `888a2a-machine-probe`. |
| `backend/agent/executor/**`, `backend/agent/acp2/**` | ACP v1/v2 execution, session and process lifecycle | Runtime status and audit values use `888a2a Agent`; protocol names remain `ACP v1` and `ACP v2`. |
| `backend/agent/chattools/**` | Agent-side collaboration and future A2A tools | New tools are described as `888a2a Agent` tools; new delegation uses A2A task operations, while existing channel/DM commands remain compatibility surfaces. |
| `backend/manager/component/dispatcher/**` | Assignment delivery, command dispatch, replay and ACK | Control-plane name is `888a2a Agent Network`; new durable event types use `a2a888` contract names and stable idempotency identities. |
| `backend/manager/component/iam/**`, `backend/manager/component/state/**` | Authorization and Manager lifecycle | New Agent Network policy/status output uses `888a2a`; no new legacy permission or log identifiers. |
| `backend/manager/api/v1/**` | Manager RPC handlers for Agent, Machine, Provider, task and conversation projections | Existing internal ConnectRPC compatibility API remains available; new Agent Network API is the A2A 1.0 HTTP+JSON boundary and uses `888a2a Agent` terminology. |
| `backend/manager/store/**`, `backend/manager/migration/**` | Provider metadata, assignment, A2A work, task graph and audit persistence | New tables, columns and event names use `a2a888` technical identifiers where a namespace is required; existing data is migrated additively. |
| `proto/v1/v1/{agent,machine,command,api_provider_service}.proto` | Existing Agent/Machine/command/provider contracts used by the runtime | Do not add new legacy package or product identifiers. New Agent Network contracts belong in a dedicated `a2a_agent_network.proto` using `a2a888.v1`. |
| `proto/store/store/{agent,machine}.proto` | Durable Agent/Machine store messages | New store contracts use `a2a888.store`; current generated code is compatibility input until the master Proto migration. |
| `backend/generated-go/**`, `proto/gen/**` | Generated API and documentation output | Regenerate from approved `a2a888` sources; do not hand-edit generated files or introduce new legacy names. |
| `frontend/src/stores/{agent,machine,api-provider,task,chat}.ts`, `frontend/src/pages/dashboard/{agents,agent-profile,machines,machine-profile,settings-agents,settings-api-providers}.tsx` | Agent Network status, Provider readiness and Machine UI | Visible labels use `888a2a` and `888a2a Agent`; API client names follow the A2A/`a2a888` contract. |
| `scripts/build-embedded-machines.sh`, `scripts/docker/{Dockerfile.manager,Dockerfile.machine,machine-entrypoint.sh}`, `scripts/build_*_docker.sh` | Cross-build, image and container entrypoint surfaces | Artifacts/images are `888a2a/manager`, `888a2a/machine`, `888a2a-machine-<platform>`; product-scoped build/runtime variables use `A2A888_*`. |
| `backend/manager/server/install.{sh,ps1}.tmpl` | Machine installer and upgrade surface | Installer downloads and installs `888a2a-machine` / `888a2a-machine.exe`; help and errors use `888a2a Machine`. |
| `docs/deploy.md`, `docs/plan/machine-hosts-many-agents-design.md`, `docs/plan/agent-acp-provider-discovery-design.md`, `docs/plan/888a2a-provider-gateway-fork-sdd.md` | Operator, Machine and Provider documentation | Titles, commands and new examples use `888a2a`; this inventory is the Agent Network naming reference. |

## Environment-variable mapping

These are the product-scoped names encountered on the Agent Network path. The
right-hand names are the only names to use for new code and documentation.

| Existing name | Target name | Classification |
| --- | --- | --- |
| `LAELIA_HOME` | `A2A888_HOME` | Machine data/config root. Read old value only in migration compatibility. |
| `LAELIA_MANAGER_URL` | `A2A888_MANAGER_URL` | Machine-to-Manager endpoint. |
| `LAELIA_INSECURE` | `A2A888_INSECURE` | Machine TLS compatibility flag. |
| `LAELIA_DEBUG` | `A2A888_DEBUG` | Machine/container debug flag. |
| `LAELIA_CODEX_HOME` | `A2A888_CODEX_HOME` | Product-owned override for the mounted Codex home; the provider's `CODEX_HOME` remains provider-owned. |
| `LAELIA_PG_URL` | `A2A888_PG_URL` | Manager PostgreSQL connection. |
| `LAELIA_BUILD_PROXY` | `A2A888_BUILD_PROXY` | Product build/download proxy. |
| `LAELIA_TEST_CACHE` | `A2A888_TEST_CACHE` | Test-server cache location. |
| `LAELIA_RUN_OPENCODE_ACP_TESTS` | `A2A888_RUN_OPENCODE_ACP_TESTS` | Opt-in ACP integration test. |
| `LAELIA_RUN_CODEX_ACP_TESTS` | `A2A888_RUN_CODEX_ACP_TESTS` | Opt-in Codex ACP integration test. |
| `LAELIA_TOKEN` | No new direct replacement | Legacy bootstrap-token input only; new setup uses persisted `A2A888_HOME` state and short-lived credentials. |

`RELEASE`, `EMBED_MACHINE`, `APT_MIRROR`, `CODEX_NPM_SPEC`, `CODEX_HOME`,
`HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` are generic/tool-owned variables.
They are not renamed by this inventory. Any new product-specific variable,
including future runtime cache or assignment controls, must begin with
`A2A888_`.

## API and documentation rules

- New Agent Network API documentation calls the service `888a2a Agent Network`
  and the role `888a2a Agent`.
- A2A 1.0 HTTP+JSON is the interoperable boundary. Agent Cards, task IDs,
  contexts, artifacts, status events and cancellation are A2A concepts; they
  must not be exposed as private legacy chat commands.
- Existing ConnectRPC Agent/Machine/Command services and current persisted
  records remain readable during migration. They are not permission to create
  new legacy package names, CLI help, logs, environment variables or docs.
- The focused change does not name or embed WuKongIM. Human IM/Message Plane
  selection belongs to the deferred omnichannel change.

## Review evidence

The mapping was checked against:

- `openspec/changes/build-888a2a-agent-network-foundation/{proposal,design,tasks}.md`;
- all four focused change specs, especially `product-identity-migration`;
- the current Provider, executor, client, command, supervisor, Manager
  dispatcher/API/store, Proto, build/install, frontend and plan-doc paths;
- the existing identity guidance in
  `openspec/changes/build-888a2a-omnichannel-agent-saas/design.md` and
  `docs/plan/omnichannel-human-agent-saas-wiki.md`.

Review conclusions:

1. Every new product-scoped environment variable named by this change uses
   `A2A888_`.
2. Existing legacy identifiers are explicitly classified as migration inputs,
   compatibility surfaces, or required attribution; they are not approved for
   new Agent Network code.
3. The inventory is limited to Agent Network files and does not pre-approve a
   whole-repository mechanical rename.
