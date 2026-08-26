## Why

888a2a 的近期核心價值是先讓現有十幾個 LLM Agent 穩定上線、發現彼此、委派工作並交換結果。目前平台已有多 Agent Machine、Channel/DM、cursor、Codex/Claude Code/OpenCode 執行基礎，但缺少正式 A2A 1.0 介面、通用 pinned npm／`npx` runtime、可靠 assignment，以及大量 Agent 所需的邊界與驗收。

## What Changes

- 完成此 Agent Network 對外與執行面所需的 888a2a／888a2a Agent naming，避免新增舊產品識別。
- 新增 Provider manifest，統一 runtime type、protocol、platform、version、integrity、capability、session 與 permission profile。
- 新增受控的 npm／`npx` runtime preparation：固定版本、integrity、atomic cache、offline launch、quarantine、rollback，turn 執行期間不得隱式下載或解析 `latest`。
- 將 Codex、Claude Code、OpenCode 與 embedded Pi 納入 manifest-backed Runtime Gateway。
- 強化一台 Machine 同時託管多個 Agent 的 assignment、config/remove、workspace、session、env 與 credential isolation。
- 將 Machine assignment 改為 durable sequence＋ack＋retry＋replay，Manager/Machine 重連後自動恢復 Agent roster。
- 新增 A2A 1.0 Agent Card、Discovery、Send、Stream、Get、List、Cancel 與 authenticated tenant routing。
- 建立內部 Agent Directory 與 skill/capability discovery，使 888a2a Agent 能選擇適合的 peer。
- 將 Agent 委派表示為 A2A-compatible durable work record，連結現有 Conversation/Task 並保留 requester、executor、context、artifact 與 trace。
- 新增 Agent 端 A2A tools：列出 peers、發送 task、查詢/訂閱、回覆結果與取消；Agent 不需要輪詢其他 Agent process。
- 新增最小多 Agent graph：parent/child、bounded fan-out、join、cycle detection、depth、concurrency、time/token budget 與 cancellation propagation。
- 移除 ACP permission 無條件允許路徑，第一版採 default-deny 高風險 action；完整 Organization Approval 留給後續 change。
- 建立 12-Agent／2-Machine E2E gate，驗證 discovery、delegation、parallel work、artifact/result、session resume、Manager restart、Machine reconnect、cancel、dedup 與 isolation。
- 本 change 不導入 WuKongIM、完整多租戶 SaaS、Web Widget 或 Slack/Teams/LINE/WhatsApp Connector；現階段使用既有 PostgreSQL collaboration/message path 作為內部 Agent network 的 durable transport。

## Capabilities

### New Capabilities

- `product-identity-migration`: 完成 Agent Network 新介面、binary、CLI、runtime、env 與文件所需的 888a2a product identity，並阻止新增舊識別。
- `agent-runtime-foundation`: Provider manifest、pinned npm／npx preparation、多 Agent Machine、session/workspace isolation、durable assignment 與 compatibility evidence。
- `a2a-agent-network`: A2A 1.0 Agent Card、Discovery、Task/Message/Artifact、stream/list/get/cancel、Agent Directory、peer delegation 與 12-Agent interoperability gate。
- `agent-network-safety`: Default-deny runtime permission、graph depth/fan-out/cycle/concurrency/budget、cancellation、dedup、audit 與 tenant-ready isolation boundary。

### Modified Capabilities

目前沒有已封存的 OpenSpec main capability；本 focused change 建立可獨立驗收的 Agent Network 規格。

## Impact

- **Agent**：`backend/agent/provider`、`executor`、`client`、`chattools`、`cmd`、`daemon`、`home`、`state`、`supervisor`。
- **Manager**：dispatcher、Machine/Agent streams、store、task/conversation mapping、auth、audit、runtime policy 與 durable outbox。
- **Proto/API**：Provider manifest/status、A2A boundary、Agent Directory、work/task graph、assignment ack/replay、budget/cancel/audit contracts。
- **Persistence**：Provider/runtime metadata、durable assignment、A2A work/context/artifact references、task edges、idempotency and audit events。
- **Dependencies**：official A2A Go SDK；npm runtime tools already available on Machine images but require controlled preparation.
- **Compatibility**：existing internal messages/tasks remain readable; A2A work maps to current conversations/tasks through additive fields/tables during this change.
- **Deferred**：Organization-first SaaS migration, mature IM Message Plane and omnichannel connectors remain in `build-888a2a-omnichannel-agent-saas` after the Agent Network gate.
