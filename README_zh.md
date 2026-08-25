# 888a2a

> **語言 / Language:** [繁體中文](README_zh.md) | [English](README.md)

888a2a 是一個開源、自架式的人類與 AI Agent 協作平台，將真人、Coding
Agent、工具、任務與外部 Agent 網路放進同一個受治理的工作空間。

本專案是以 [Laelia](https://github.com/Ranxy/laelia) 為基礎的持續開發版本。
長期方向是打造以 A2A 為核心、適合組織使用的協作平台，連接本機 Agent、內部
Bot、外部 A2A Agent，以及各種全通路對話入口。

> **專案狀態：** 888a2a 目前仍在積極開發中，Agent Network Foundation
> 正在建置，尚未達到 production-ready。產品識別遷移完成前，程式與文件中仍
> 可能出現 Laelia 名稱與相容性介面。

## 目前基礎

上游平台已提供以下重要基礎：

- 人類對 Agent、人類對人類、Agent 對 Agent 的對話
- Channel、私訊、Thread、Task 與排程提醒
- Manager 與只發起 outbound 連線的 Machine 元件
- 基於 ACP 的本機 Agent 執行與 Provider 整合
- MCP 擴充、工作區檔案、IAM 與稽核紀錄
- PostgreSQL 狀態儲存，以及 React/TypeScript 網頁介面
- 初步的 durable work contract、Provider manifest 與多 Agent testkit

## Roadmap / TODO

以下是接下來的大方向；詳細提案與驗收條件請見
[`openspec/changes/`](openspec/changes/)。

- [x] **產品識別與基準** — 建立 888a2a 命名防護閘門、盤點清單與遷移邊界。
- [x] **Agent Runtime 基礎** — 實作安全驗證的 Provider Manifest、固定運行環境快取（`@agentclientprotocol/claude-agent-acp@0.70.0`）、防篡改隔離區、啟動指紋與多 Machine 狀態回放。
- [x] **A2A 1.0 互通** — 透過標準官方協議（`github.com/a2aproject/a2a-go/v2`）支援 Agent Card 投影、目錄發現、PostgreSQL durable work 與重啟復原。
- [x] **多 Agent Orchestration** — 加入 DAG 循環依賴防護、並行扇出/聚合、預算與併發限制，以及根任務級聯取消。
- [x] **安全與聚焦政策** — 落實工作區路徑限制、所有權斷言、預設拒絕運行權限，以及脫敏審計日誌。
- [x] **Twelve-Agent 驗收閘門** — 跨 2 台 Machine 運行 12 個 Agent（1 Coordinator, 10 Specialists, 1 Reviewer），通過確定性扇出、丟包重試、Manager 重啟復原與跨 Agent 滲透隔離驗證。
- [ ] **Organization-ready SaaS** — 建立 Organization tenancy、Entitlement、Usage metering、Quota、可銜接 Billing 的邊界、稽核能力與無狀態 Manager 擴展。
- [ ] **全通路協作** — 整合 Slack、Teams、LINE、WhatsApp、Web Widget 等入口，同時保留一致且受治理的對話與任務模型。
- [ ] **可靠性與營運** — 補齊共享即時事件、Tracing、Metrics、SLO、Load test、備份與零停機升級。

第一個里程碑 **Agent Network Foundation** 已全數建置並通過驗收。
如需多節點部署指南，請參閱 [Agent Network 運維指南](docs/guide/agent-network-operator-guide.md)。

## 快速開始

### 伺服器生產環境 Docker 部署（免安裝 Go / Node.js）

對於一般伺服器（未安裝 Go 或 Node.js），可直接使用 Docker Compose 快速啟動全套服務：

```bash
# 取得程式庫並啟動所有服務 (PostgreSQL + Manager + Machine)
git clone https://github.com/tbdavid2019/888a2a.git
cd 888a2a
docker compose up -d
```

啟動完成後，在瀏覽器開啟 `http://<YOUR_SERVER_IP>:8181` 即可使用。
詳細配置、資料卷持久化與備份方式請見 [`docs/guide/docker-deployment-guide.md`](docs/guide/docker-deployment-guide.md)。

### 本機一鍵測試環境

啟動包含嵌入式 PostgreSQL 與預置測試帳號的本機瀏覽器測試環境：

```bash
scripts/test-server.sh run --workdir /tmp/888a2a-test
```

指令會輸出網址與預置帳號。停止服務：

```bash
scripts/test-server.sh stop --workdir /tmp/888a2a-test
```

選項與注意事項請見 [`docs/test-server.md`](docs/test-server.md)。

### 從原始碼開發

```bash
# Backend
go run ./backend/manager/bin/server/main.go --port 8181 --debug

# Frontend
pnpm --dir frontend dev

# Build Manager & Machine 二進位檔
go build -ldflags "-w -s" -p=16 -o ./build/888a2a ./backend/manager/bin/server/main.go
go build -ldflags "-w -s" -p=16 -o ./build/888a2a-machine ./backend/agent/bin/agent/main.go
```

完整的建置、測試、Lint 與開發流程請見 [`AGENTS.md`](AGENTS.md)。
多 Agent 網路架構與部署指引請參閱 [`docs/guide/agent-network-operator-guide.md`](docs/guide/agent-network-operator-guide.md)。

## 架構

- **Manager** — Web UI、API、IAM、持久化狀態、排程、派工與 Organization 層級政策。
- **Machine** — 只發起 outbound 連線的 Agent 主機，執行一個或多個本機 Agent 與
  Provider runtime。
- **A2A boundary** — Agent Discovery 與工作交換的標準介面；內部 Chat 保留作為
  人類可讀的協作介面。

## 致謝

888a2a 建立在 [Ranxy/laelia](https://github.com/Ranxy/laelia) 的優秀成果上。
我們誠摯感謝 Ranxy 與 Laelia 貢獻者提供原始平台、架構、實作與靈感，讓本專案
能夠繼續發展。

上游專案採用 [Apache License 2.0](LICENSE)。重新發布或修改程式碼時，請保留
原有的版權與授權聲明。本專案也受到 [raft.build](https://raft.build/) 啟發。

## 授權

本專案採 Apache License 2.0 授權，完整條文請見 [`LICENSE`](LICENSE)。
