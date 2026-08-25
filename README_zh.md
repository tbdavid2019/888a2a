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

- [ ] **產品識別與遷移** — 將公開與營運介面統一為 888a2a，建立相容性映射，
  並安全遷移既有設定、憑證與資料。
- [ ] **Agent Runtime 基礎** — 完善 Provider manifest、本機 npm/npx runtime、
  ACP v1/v2 執行、Session resume 與多 Machine hosting。
- [ ] **A2A 1.0 互通** — 透過官方協議邊界支援 Agent Card、Discovery、Task、
  Streaming、Polling、Artifact、取消、授權與 durable work record。
- [ ] **多 Agent Orchestration** — 加入受限制的委派、Fan-out/Fan-in、Task graph、
  Retry、Budget、Timeout，以及可追蹤的父子工作關係。
- [ ] **安全與 Approval** — 落實 Organization、Workspace、Agent、Skill 與資料
  邊界；隔離工作區與憑證；加入風險分級、審批與升級流程。
- [ ] **Organization-ready SaaS** — 建立 Organization tenancy、Entitlement、
  Usage metering、Quota、可銜接 Billing 的邊界、稽核能力與無狀態 Manager 擴展。
- [ ] **全通路協作** — 整合 Slack、Teams、LINE、WhatsApp、Web Widget 等入口，
  同時保留一致且受治理的對話與任務模型。
- [ ] **可靠性與營運** — 補齊共享即時事件、Tracing、Metrics、SLO、Load test、
  斷線恢復、備份、升級與 production deployment 文件。

第一個里程碑是 **Agent Network Foundation**：至少 12 個 Agent 分布在 2 台
Machine 上，可以互相發現、交換受限制的工作、回傳 Artifact、恢復 Session、
承受斷線重連並取消工作，同時不跨越 Workspace 或憑證邊界。

## 快速開始

### 一鍵測試環境

啟動包含嵌入式 PostgreSQL 與預置測試帳號的本機瀏覽器測試環境：

```bash
scripts/test-server.sh run --workdir /tmp/888a2a-test
```

指令會輸出網址與預置帳號。停止服務：

```bash
scripts/test-server.sh stop --workdir /tmp/888a2a-test
```

選項與注意事項請見 [`docs/test-server.md`](docs/test-server.md)。

### 開發

```bash
# Backend
go run ./backend/manager/bin/server/main.go --port 8181 --debug

# Frontend
pnpm --dir frontend dev

# Build
go build -ldflags "-w -s" -p=16 -o ./build/laelia ./backend/manager/bin/server/main.go
```

完整的建置、測試、Lint 與發布流程請見 [`AGENTS.md`](AGENTS.md)。

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
