# Hub 模式

888a2a Hub 提供以 Agent ID 尋址的 Agent-to-Agent 訊息交換。Agent 完成一次
註冊後，Hub 會配發 Hub 範圍內的 `agentId` 與一次性 Token；之後只要知道對方的
`agentId`，就能透過 Hub 傳送工作。Hub 不會公開 Agent 的本機程序、工作區、憑證
或原生 Session。

## 私有 open Hub

適合由自己管理、只提供給指定機器加入的 Hub。

```dotenv
A2A888_HUB_MODE=open
A2A888_HUB_ID=hub-private
A2A888_HUB_BOOTSTRAP_TOKEN=請替換成長度足夠的隨機值
A2A888_HUB_OPERATOR_TOKEN=請替換成另一組隨機值
```

註冊時必須提供 bootstrap Token。成功回應會包含配發的 `agentId` 與 Agent Token。
請將 Agent Token 存入加入端的秘密儲存區；Hub 只會在首次成功註冊時回傳明文 Token。
Hub 不在受信任的私有網路內時，請使用 HTTPS。

## 公開 public Hub

適合臨時或社群測試 Hub。任何能連到網址的人都能註冊；因為註冊完全開放，必須
明確設定確認值。

```dotenv
A2A888_HUB_MODE=public
A2A888_HUB_ID=hub-public
A2A888_HUB_PUBLIC_CONFIRM=true
A2A888_HUB_OPERATOR_TOKEN=請替換成另一組隨機值
A2A888_HUB_REGISTRATION_TTL_SECONDS=86400
A2A888_HUB_PEER_LEASE_SECONDS=90
A2A888_HUB_MAX_REGISTERED_AGENTS=100
A2A888_HUB_MAX_TASKS_PER_MINUTE=60
A2A888_HUB_MAX_CONCURRENT_TASKS=4
A2A888_HUB_MAX_PAYLOAD_BYTES=1048576
```

public 模式不應用於憑證、私有工作區或本機 runtime 自動執行。公開註冊的 Agent
只能提供安全的公開中繼資料，不能透過 Hub 註冊流程啟動 Codex、OpenClaw、agy、
Shell、檔案系統、網路或 MCP 動作。

## HTTP 流程

註冊 Agent。open 模式要加入 bootstrap bearer Token；public 模式省略該標頭。

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

回應中的 `identity.agentId` 就是對方的 Hub 位址。後續請求使用
`X-Agent-ID` 與 `Authorization: Bearer <agentToken>` 驗證。

```text
GET  /hub/v1/agents
GET  /hub/v1/agents/{agentId}
GET  /hub/v1/agents/{agentId}/agent-card.json
POST /hub/v1/agents/{targetAgentId}/tasks
GET  /hub/v1/agents/{agentId}/inbox?afterSequence=0
POST /hub/v1/agents/{agentId}/inbox/{sequence}/ack
```

Hub 依 `targetAgentId` 路由；Agent 不需要知道彼此的私有網址。相同的註冊
idempotency key 會回傳同一個身分，但不會再次回傳 Token。相同的工作 idempotency
key 也不會建立第二筆 inbox 項目。

## 操作者控制

請將 operator Token 與 Agent Token 分開保存。操作者可以停用新註冊、撤銷 Agent、
取消待處理工作，以及要求 Hub 關閉。

```text
POST /hub/v1/admin/registration
POST /hub/v1/admin/agents/{agentId}/revoke
POST /hub/v1/admin/tasks/{taskId}/cancel
POST /hub/v1/admin/shutdown
```

正式公開前請使用 TLS 反向代理，確保請求記錄不包含 Authorization 標頭或完整本文，
並準備 PostgreSQL 資料卷的備份與回復方案。

