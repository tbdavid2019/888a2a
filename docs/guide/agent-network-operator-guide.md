# 888a2a Agent Network Operator Guide

This guide describes how to deploy, configure, secure, and operate a multi-machine, multi-agent network on 888a2a.

---

## 1. Network Architecture Overview

The 888a2a Agent Network connects decentralized agent runtimes across physical or virtual machines using the official A2A 1.0 HTTP+JSON protocol and durable PostgreSQL-backed task orchestration.

```
                    ┌────────────────────────────────────────────────────────┐
                    │               888a2a Manager Cluster                   │
                    │                                                        │
 外部 A2A 請求 ─────┼─► [A2A 1.0 Gateway & Task Store]                      │
 (HTTP+JSON)        │   • Agent Directory & Card Projections                 │
                    │   • a2a888_work / event CRUD & Recovery                │
                    │   • DAG Cycle Prevention & Budget Enforcement          │
                    │                     │                                  │
                    │                     ▼                                  │
                    │   [Dispatcher & Assignment Store] ─────────────────────┼─► Machine Daemons
                    │   • Monotonic Sequence Log                             │   (gRPC / ConnectRPC)
                    │   • Idempotent Ack & Replay                            │
                    └────────────────────────────────────────────────────────┘
                                   │                 │
                ┌──────────────────┘                 └──────────────────┐
                ▼                                                       ▼
 ┌──────────────────────────────┐                        ┌──────────────────────────────┐
 │    Machine 01 (Host 1)       │                        │    Machine 02 (Host 2)       │
 │ • Coordinator Agent (01)     │                        │ • Specialist Agents (06-10)  │
 │ • Specialist Agents (01-05)  │                        │ • Reviewer Agent (12)        │
 │ • Workspace Confinement      │                        │ • Workspace Confinement      │
 │ • Pinned Runtime Cache       │                        │ • Pinned Runtime Cache       │
 └──────────────────────────────┘                        └──────────────────────────────┘
```

---

## 2. Environment Variables & Identity

Every product-scoped configuration uses the `A2A888_` prefix:

| Environment Variable | Description | Default |
| :--- | :--- | :--- |
| `A2A888_HOME` | Machine local data root (workspaces, logs, runtime binaries) | `~/.888a2a` |
| `A2A888_MANAGER_URL` | Endpoint of the 888a2a Manager cluster | `http://localhost:8181` |
| `A2A888_TOKEN` | Machine authentication bearer token | *(Required)* |
| `A2A888_DAEMON_SOCKET` | Local domain socket for IPC | `<A2A888_HOME>/daemon.sock` |
| `A2A888_AGENT` | Resource ID of the executing agent | Injected per subprocess |
| `A2A888_SESSION_TOKEN` | Ephemeral subprocess auth token | Injected per subprocess |

---

## 3. Provider Manifests & Pinned Runtimes

888a2a eliminates runtime drifting and turn-time network dependencies via immutable, sha256-verified manifests:

1. **Claude Code Provider (`claude-code`)**:
   - Package: `@agentclientprotocol/claude-agent-acp@0.70.0`
   - Integrity: SRI sha512 hash verified prior to execution.
   - Cache Directory: `<A2A888_HOME>/cache/npm/@agentclientprotocol/claude-agent-acp@0.70.0/`
2. **OpenCode Provider (`opencode`)**:
   - System executable probed with minimum version check.
3. **Codex ACP v2 Provider (`codex`)**:
   - JSON-RPC over stdio with bidirectional thread session management.
4. **Embedded Pi Provider (`pi`)**:
   - Embedded native agent binary with cross-platform release hashes.

---

## 4. Security Defaults & Process Isolation

1. **Path Confinement**:
   - Subprocesses are strictly confined to `<A2A888_HOME>/workspaces/<agent_id>/`.
   - Symlink traversal (`../`) and dangling link escapes are denied with `ErrPathEscape`.
2. **Workspace & Ownership Assertions**:
   - Cross-agent file access and impersonation attempts are rejected via `AssertAgentOwnership`.
3. **Default-Deny Runtime Policy**:
   - High-risk operations (unapproved shell, raw disk write, credentials probing) are denied by default.
4. **Audit & Trace Redaction**:
   - All `a2a888_work_event` logs automatically strip credentials (`token`, `secret`, `bearer`) and hidden reasoning (`thought`, `thinking`).

---

## 5. Deployment Step-by-Step

### Step 1: Start Manager & Apply PostgreSQL Migrations
```bash
# Set database connection string
export A2A888_PG_URL="postgres://dev:dev@localhost:5432/888a2a?sslmode=disable"

# Start manager
./build/888a2a --port 8181
```

### Step 2: Connect Machine Nodes
```bash
# On Machine 1
export A2A888_HOME="/opt/888a2a/mach-01"
export A2A888_MANAGER_URL="http://manager.internal:8181"
export A2A888_TOKEN="mach-token-1"
./build/888a2a-machine

# On Machine 2
export A2A888_HOME="/opt/888a2a/mach-02"
export A2A888_MANAGER_URL="http://manager.internal:8181"
export A2A888_TOKEN="mach-token-2"
./build/888a2a-machine
```

### Step 3: Verify Twelve Agents Online & READY
```bash
# Query agent directory via A2A 1.0 Gateway
curl -s -H "A2A-Version: 1.0" http://localhost:8181/api/v1/a2a/directory/peers | jq .
```

---

## 6. Troubleshooting & Recovery Runbook

| Symptom | Cause | Resolution |
| :--- | :--- | :--- |
| `RuntimeStatus == QUARANTINED` | Binary on disk was modified or corrupted | Trigger `RepairRuntime` via API to re-extract verified package. |
| `ErrCyclicDelegation` | Delegation DAG has a loop ($A \to B \to A$) | Check parent-child edge hierarchy in coordinator subtask definition. |
| `ErrPolicyLimitExceeded` | Work exceeded max depth, child count, or budget | Increase coordinator budget allocation or decompose workflow. |
| Machine Disconnected | Temporary network disruption | Reconnect daemon; Manager replays all missed assignment sequences automatically. |
| Manager Restarted | Server restart during active tasks | `RecoveryService` automatically scans in-flight tasks and resets them to `SUBMITTED` for safe re-dispatch. |

---

## 7. Acceptance Gate Test Suite

The 12-agent deterministic topology and fault injection test suite can be run at any time:

```bash
# Run acceptance test suite
go test -v -count=1 ./backend/agent/testkit -run TestTwelveAgentAcceptanceGate
```
