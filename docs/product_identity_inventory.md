# 888a2a Product Identity Inventory & Migration Mapping

This inventory documents all legacy product identifiers across the codebase, their mapped `888a2a` targets, compatibility transition rules, and verification gates in accordance with OpenSpec change `build-888a2a-omnichannel-agent-saas` (Task 0.1 & Task 0.2).

Migration status is still in the compatibility-reader phase. The repository
does not yet have the end-to-end fixture required to accept Task 0.7, and
legacy aliases remain intentionally available until Task 0.9 is completed.

---

## 1. Mapping Inventory by Category

| Category | Legacy Identifier / Pattern | Target `888a2a` Identifier | Migration / Compatibility Strategy |
| :--- | :--- | :--- | :--- |
| **Product Brand** | `Laelia`, `Laelia AI` | `888a2a` | UI strings, headers, HTML titles, and operator guides updated to `888a2a`. |
| **Agent Brand** | `Laelia Agent`, `Agent` | `888a2a Agent` | Agent personas, prompts, documentation, and card projections use `888a2a Agent`. |
| **Go Module Path** | `github.com/Ranxy/laelia` | `github.com/tbdavid2019/888a2a` | Mechanical module refactor in `go.mod` and all Go package imports. |
| **Environment Prefix** | `LAELIA_*` (e.g. `LAELIA_TOKEN`) | `A2A888_*` (e.g. `A2A888_TOKEN`) | Dual-read reader in `config`: check `A2A888_*` first, fallback to `LAELIA_*` with deprecation notice. |
| **Server Binary** | `build/laelia` | `build/888a2a` | Build script produces `888a2a` with symlink / alias for legacy invocation. |
| **Machine Binary** | `build/laelia-machine` | `build/888a2a-machine` | Build scripts, Dockerfiles, and packaging updated to `888a2a-machine`. |
| **Agent CLI** | `backend/agent/bin/agent` | `888a2a-agent` | Agent supervisor and daemon references use `888a2a-agent`. |
| **Data Directory** | `~/.laelia` | `~/.888a2a` | One-time auto-migration on boot: read `~/.laelia` if `~/.888a2a` does not exist, copy state forward. |
| **Config Path** | `~/.config/laelia` | `~/.config/888a2a` | Fallback reader for config directory during transition. |
| **Runtime Sockets** | `/tmp/laelia-*.sock` | `/tmp/888a2a-*.sock` | Socket and IPC pipes prefixed with `888a2a-`. |
| **HTTP Cookies** | `access-token`, `laelia-*` | `888a2a-access-token` | Dual-read cookie support in auth interceptor. |
| **HTTP Headers** | `X-Laelia-Agent` | `X-888a2a-Agent` | Auth interceptor checks `X-888a2a-Agent` first, fallbacks to `X-Laelia-Agent`. |
| **LocalStorage** | `laelia-sidebar-collapsed` | `888a2a-sidebar-collapsed` | Dual-read from localStorage on frontend initialization. |
| **Permissions** | `laelia.conversations.*`, `laelia.settings.*` | `888a2a.conversations.*`, `888a2a.settings.*` | IAM evaluator supports both aliases seamlessly; new policies emit `888a2a.*`. |
| **Docker Images** | `laelia/manager`, `laelia/machine` | `888a2a/manager`, `888a2a/machine` | Build scripts and Docker compose manifests target `888a2a/*`. |
| **Frontend Package** | `laelia-frontend` (`package.json`) | `888a2a-frontend` | Update npm package metadata and app header branding. |
| **Proto Packages** | `proto/v1/laelia` | `proto/v1/a2a888` | New services defined under `a2a888`, legacy stubs aliased for compatibility. |
| **Attribution** | License headers, third-party NOTICE | Preserved in full | Upstream copyright and author attribution records are strictly preserved. |

---

## 2. Environment Variable Migration Reference

| Legacy Variable | Target Variable | Purpose |
| :--- | :--- | :--- |
| `LAELIA_PORT` | `A2A888_PORT` | Server listening port |
| `LAELIA_DEBUG` | `A2A888_DEBUG` | Enable debug logging |
| `LAELIA_PG_URL` | `A2A888_PG_URL` | PostgreSQL connection string |
| `LAELIA_MANAGER_URL` | `A2A888_MANAGER_URL` | Machine connecting to Manager URL |
| `LAELIA_TOKEN` | `A2A888_TOKEN` | Machine bootstrap/access token |
| `LAELIA_CODEX_HOME` | `A2A888_CODEX_HOME` | Codex runtime configuration directory |
| `LAELIA_DATA_DIR` | `A2A888_DATA_DIR` | Persistent state directory |
| `LAELIA_COOKIE_SAMESITE` | `A2A888_COOKIE_SAMESITE` | HTTP cookie SameSite policy |
| `LAELIA_RUN_OPENCODE_ACP_TESTS` | `A2A888_RUN_OPENCODE_ACP_TESTS` | Test flag for OpenCode integration |
| `LAELIA_RUN_CODEX_ACP_TESTS` | `A2A888_RUN_CODEX_ACP_TESTS` | Test flag for Codex integration |
| `LAELIA_RUN_MIGRATION_TESTS` | `A2A888_RUN_MIGRATION_TESTS` | Test flag for PostgreSQL migration tests |

---

## 3. Migration Stages & Verification Strategy

1. **Dual-Reader Phase**:
   - Configuration readers, auth interceptors, and local storage read target `888a2a` names first, falling back to legacy names if absent.
   - Newly written state always uses `888a2a` naming.
2. **Mechanical Code & Module Refactor**:
   - Refactor `go.mod`, package imports, CLI build scripts, and Docker images.
   - Run `go test ./...`, `pnpm biome:check`, `pnpm type-check`, and frontend test suites.
3. **Zero-Legacy-Residual Gate**:
   - `scripts/check_agent_network_naming.sh` validates that no newly written or modified files introduce unapproved legacy identifiers, ensuring zero regressions.
