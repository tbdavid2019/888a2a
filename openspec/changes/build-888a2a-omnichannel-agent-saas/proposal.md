## Why

888a2a 已具備多人與多 Agent 協作、Machine 執行、ACP、MCP、Channel、Task 與 IAM 的基礎，但目前仍是單一 workspace、單進程即時通知與自有聊天模型，無法直接支撐可商業化的大型多租戶 SaaS。產品機會是將它發展為組織級 Omnichannel Human＋Agent Collaboration Platform，讓企業把真人團隊、內部 Bot、外部 A2A Agent、本機 Coding Agent，以及 Slack、Teams、LINE、WhatsApp、網站 Widget 等入口放在同一個受治理的工作空間。

## What Changes

- **BREAKING**：將產品、Agent、CLI、Go module、Proto namespace、環境變數、資料目錄、Docker image、release artifact、UI 與文件統一更名為 `888a2a`／`888a2a Agent`，並以明確 migration 處理既有安裝資料。
- **BREAKING**：將資料與權限根邊界從單一 workspace 改為 `Organization → Workspace/Project → Resource`，所有 principal、conversation、task、agent、machine、connector、approval、audit 與 usage 都必須帶有 tenant scope。
- **BREAKING**：將目前的 Chat API 升級為 Collaboration Service；採成熟 IM engine 承擔 per-channel ordering、idempotency、multi-device、offline sync、presence 與 realtime fan-out，888a2a 保存組織政策、業務投影、搜尋、稽核與 Agent 關聯。
- **BREAKING**：將 Agent 間的工作委派標準化為 A2A 1.0 Task／Message／Artifact／Agent Card；888a2a 自有 Channel／DM 保留做人類協作介面，不再充當公開 A2A 協議。
- 新增 Multi-Agent Orchestrator，支援 parent/child task graph、fan-out/fan-in、cycle detection、delegation depth、budget、quota、cancel propagation 與 trace。
- 新增 Omnichannel Connector Gateway，提供 Slack、Teams、LINE、WhatsApp、Web Widget 的安裝、webhook 驗證、事件去重、格式正規化、能力宣告、rate-limit queue、retry 與 outbound delivery。
- 保留 Manager／Machine 架構，將 Machine 擴充為 Provider Runtime Gateway，透過 ACP v1/v2、固定版本 npm／`npx`、MCP 與 embedded runtime 接入多種 Coding Agent。
- 將 Approval 改為每個 Organization 自主管理的 policy：支援 approver user/group/role、resource/skill/action/risk scope、quorum、timeout、escalation 與 immutable decision record，並映射 A2A `AUTH_REQUIRED`。
- 預留 SaaS 商業模型：billing account、plan、subscription、entitlement、quota、usage event；本變更不串接付款供應商。
- 將 realtime notification、connector ingestion、Machine assignment、approval 與 usage 改為 durable inbox/outbox/event processing，移除關鍵路徑的單進程或 best-effort 唯一狀態。
- 新增組織級 credential vault、connector token rotation、webhook signature verification、tenant rate limiting、audit 與 retention policy。
- 分階段推出：先完成 888a2a Agent Network Foundation，包括 Provider manifest、本機 npm／`npx`、多 Agent Machine、A2A 1.0 與最小安全限制，驗證 10+ Agent 穩定互通後，再完成 Organization SaaS、IM、Web Widget 與外部 Connector。

## Capabilities

### New Capabilities

- `product-identity-migration`: 將所有公開與內部 legacy product identifiers 遷移為 888a2a，涵蓋品牌、CLI、module、Proto、env、storage、images、artifacts 與相容資料匯入。
- `organization-tenancy`: Organization、workspace、human/agent/service principal、membership、group、role、tenant isolation 與資源命名。
- `collaboration-messaging`: 人與人、人與 Agent、多人成員 Channel／DM／Thread、訊息事件、排序、冪等、編輯、撤回、多裝置、離線同步與即時傳遞。
- `omnichannel-connectors`: Slack、Teams、LINE、WhatsApp、Web Widget 的 tenant installation、identity/conversation mapping、webhook ingestion、能力差異、outbound delivery 與 retry。
- `a2a-collaboration`: A2A 1.0 Agent Card、Task、Message、Artifact、stream/push、multi-turn context，以及多 Agent task graph orchestration。
- `agent-runtime-gateway`: ACP v1/v2、npm／`npx` Provider manifest、package pin/integrity、session resume、MCP、runtime policy 與 compatibility matrix。
- `organization-approval`: 組織級 approval policy、approver routing、quorum、expiry、escalation、decision binding 與 A2A authorization-required workflow。
- `usage-entitlements`: Plan-independent entitlement、quota、usage event、aggregate 與未來 billing provider 邊界。
- `platform-reliability`: Durable inbox/outbox、idempotent consumers、multi-instance realtime、audit、observability、retention、backpressure、reconciliation 與 disaster recovery。

### Modified Capabilities

目前沒有既有 OpenSpec capability；本變更建立第一組主規格。

## Impact

- **Backend**：Manager store、IAM、dispatcher、conversation/task model、Machine control、provider/executor、MCP、auth、audit、notification 與新 Connector/A2A/Approval/Usage components。
- **Proto/API**：資源名稱加入 organization/workspace scope；新增 A2A server/client boundary、connector、approval、entitlement、usage 與 messaging event contracts。
- **Frontend**：Organization switcher、workspace、member/group/role、omnichannel inbox、connector onboarding、approval center、Agent Card／task graph、runtime catalog、usage visibility 與 embeddable widget。
- **Persistence**：PostgreSQL tenant schema 與 projections、object storage tenant prefix、durable event/outbox；另評估 WuKongIM 等成熟 IM engine。
- **Infrastructure**：stateless Manager replicas、shared realtime/event infrastructure、secret vault、API gateway、connector webhook endpoints、A2A endpoints、tenant metrics 與 rate limiting。
- **Dependencies**：官方 A2A Go SDK；IM engine 與各通路官方 SDK／protocol 需經 spike、license、版本與 production-readiness 審查後決定。
- **Compatibility**：需要單一 workspace 資料遷移、舊 API compatibility window、feature flags 與 staged rollout；不得直接破壞既有部署資料。
- **Product identity**：所有新增介面與 UI 只使用 `888a2a`；legacy identifiers 只可出現在受控 migration mapping、license attribution 與移除驗證中。
- **Prior plan**：`docs/plan/888a2a-provider-gateway-fork-sdd.md` 的多租戶與 A2A 階段排序被本 proposal 取代；可重用其中的 Provider Gateway、安全與 upstream/downstream 策略。
