# 888a2a Docker 部署與安裝手冊 (Docker Deployment Guide)

本文件提供在一般伺服器（無需安裝 Go 或 Node.js 開發環境）使用 Docker 及 Docker Compose 快速部署 888a2a 的完整流程。

---

## 1. 系統需求 (Prerequisites)

* **作業系統**：Linux (Ubuntu 20.04+, Debian 11+, RHEL/Rocky 8+, AlmaLinux), macOS 或 Windows (WSL2)
* **軟體環境**：
  * [Docker](https://docs.docker.com/engine/install/) (v24.0+)
  * [Docker Compose](https://docs.docker.com/compose/install/) (v2.20+)
* **硬體建議**：
  * CPU: 2 Core 以上
  * Memory: 4 GB RAM 以上
  * Disk: 20 GB 以上可用空間

---

## 2. 一鍵啟動 (One-Command Quick Start)

### 步驟 1：取得程式庫或 Compose 設定檔
```bash
git clone https://github.com/tbdavid2019/888a2a.git
cd 888a2a
```

*(若僅下載 compose 檔)*：
```bash
curl -O https://raw.githubusercontent.com/tbdavid2019/888a2a/main/docker-compose.example.yml
cp docker-compose.example.yml docker-compose.yml
```

### 步驟 2：啟動所有服務 (PostgreSQL + Manager + Machine)
```bash
# 先建立只存在於本機的密碼檔，勿提交至 Git
printf 'A2A888_DB_PASSWORD=%s\n' "$(openssl rand -hex 24)" > .env
DB_PASSWORD="$(sed -n 's/^A2A888_DB_PASSWORD=//p' .env)"
printf 'A2A888_PG_URL=postgres://dev:%s@db:5432/888a2a?sslmode=disable\n' "$DB_PASSWORD" >> .env
chmod 600 .env
docker compose up -d
```

### 步驟 3：檢查服務狀態
```bash
docker compose ps
```

Compose 會建立 `objectdata` 永久資料卷，掛載到 Manager 的
`/data/objects`。未設定 S3 時，檔案與頭像會寫入這個資料卷；容器重建或
`docker compose down` 不會刪除資料。請勿使用 `docker compose down -v`，除非
你確定要刪除資料庫、工作區與物件檔案。

輸出範例：
```text
NAME                IMAGE                               COMMAND                  SERVICE   CREATED         STATUS                   PORTS
888a2a-db          postgres:16-alpine                  "docker-entrypoint.s…"   db        2 minutes ago   Up 2 minutes (healthy)   0.0.0.0:5432->5432/tcp
888a2a-manager     tbdavid2019/888a2a:latest           "888a2a --port 8181"     manager   2 minutes ago   Up 2 minutes (healthy)   0.0.0.0:8181->8181/tcp
888a2a-machine     tbdavid2019/888a2a-machine:latest   "machine-entrypoint"     machine   2 minutes ago   Up 2 minutes             
```

### 步驟 4：開啟瀏覽器
開啟瀏覽器訪問：
👉 **`http://<YOUR_SERVER_IP>:8181`**

---

## 3. 服務配置與環境變數

在 `docker-compose.yml` 中可調整之關鍵環境變數：

### Manager (888a2a 核心服務)

| 環境變數 | 說明 | 預設值 |
| :--- | :--- | :--- |
| `A2A888_PG_URL` | PostgreSQL 資料庫連線字串 | 由 `.env` 設定，必須包含隨機密碼 |
| `PORT` | Manager 監聽埠號 | `8181` |
| `A2A888_OBJECT_STORAGE_DIR` | 本機物件儲存目錄 | `/data/objects` |

### 物件儲存

沒有設定 S3 時，888a2a 會使用本機儲存，無需額外服務：

```yaml
volumes:
  - objectdata:/data/objects
```

要使用雲端儲存時，請在管理介面設定完整的 endpoint 與 bucket。AWS S3、
Cloudflare R2 與 GCP Cloud Storage 都可透過 S3-compatible endpoint 使用：

| 服務 | Endpoint 範例 | Region | 注意事項 |
| :--- | :--- | :--- | :--- |
| AWS S3 | 留空或 AWS S3 endpoint | `us-east-1` | 使用 AWS credentials |
| Cloudflare R2 | `https://<account>.r2.cloudflarestorage.com` | `auto` | 使用 R2 Access Key 與 Secret Key |
| GCP Cloud Storage | `https://storage.googleapis.com` | `auto` | 使用 GCS HMAC interoperability key |

設定完成後，endpoint 與 bucket 會切換到遠端 S3-compatible backend；清空兩者
則回到本機儲存。請勿把 cloud secret 寫入 Git 或放進物件 key。

設定依據請見 [Cloudflare R2 S3 API](https://developers.cloudflare.com/r2/api/s3/api/)、
[Google Cloud Storage interoperability](https://cloud.google.com/storage/docs/interoperability)
與 [GCS HMAC keys](https://cloud.google.com/storage/docs/authentication/hmackeys)。

### Machine (Agent 執行環境主機)

| 環境變數 | 說明 | 預設值 |
| :--- | :--- | :--- |
| `A2A888_MANAGER_URL` | Manager 連線端點 | `http://manager:8181` |
| `A2A888_HOME` | Machine 資料持久化目錄 | `/data` |
| `A2A888_CODEX_HOME` | 可選之 Codex 設定檔掛載路徑 | `/home/a2a888/.codex` |

---

## 4. 單獨建立 Docker 映像檔 (Manual Build)

若需要從源始碼自行打包 Docker Image：

```bash
# 1. 構建 Manager 映像檔 (含前端嵌入)
docker build -t tbdavid2019/888a2a:latest .

# 2. 構建 Machine 映像檔 (含 Python, Node.js, Pi, Codex 運行環境)
docker build -f scripts/docker/Dockerfile.machine -t tbdavid2019/888a2a-machine:latest .
```

---

## 5. 常見維護指令 (Operations & Troubleshooting)

### 檢視即時日誌
```bash
# 查看 Manager 日誌
docker compose logs -f manager

# 查看 Machine 與 Agent 執行日誌
docker compose logs -f machine
```

### 停止與重啟服務
```bash
# 停止服務 (資料保留在 volume)
docker compose down

# 停止並清除所有持久化資料 (警告：將清除資料庫、工作區與物件檔案)
docker compose down -v
```

### 備份與資料庫維護
```bash
# 備份 PostgreSQL
docker compose exec db pg_dump -U dev 888a2a > 888a2a_backup_$(date +%Y%m%d).sql

# 還原 PostgreSQL
docker compose exec -T db psql -U dev 888a2a < 888a2a_backup.sql
```

本機物件資料卷也必須備份。先找出卷名稱，再打包 `/data/objects`：

```bash
docker volume inspect 888a2a_objectdata
docker run --rm -v 888a2a_objectdata:/objects -v "$PWD":/backup alpine \
  tar czf /backup/888a2a_objects_$(date +%Y%m%d).tar.gz -C /objects .
```

還原前請停止 Manager，避免同時寫入：

```bash
docker compose stop manager
docker run --rm -v 888a2a_objectdata:/objects -v "$PWD":/backup alpine \
  tar xzf /backup/888a2a_objects_YYYYMMDD.tar.gz -C /objects
docker compose start manager
```

完整的 custom-format 備份、校驗與隔離資料庫災難復原演練，請參閱
[`docs/guide/backup-restore.md`](backup-restore.md)。
