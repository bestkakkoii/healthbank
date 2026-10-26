# Healthbank v1：寶塔部署與資料庫遷移

© 2026 bestkakkoi llc. All rights reserved. 原始碼採 MIT 授權，詳見 `LICENSE`。

專案倉庫：[bestkakkoii/healthbank](https://github.com/bestkakkoii/healthbank)。Linux 套件與校驗清單的發行入口：[GitHub Releases](https://github.com/bestkakkoii/healthbank/releases)。

## 一、第一次部署：上傳、選執行檔、綁域名、啟動

**本次正式部署平台為 Debian 13（trixie）x64。使用 `healthbank_1.0.1_linux_amd64.tar.gz`；`amd64` 在發行檔名中同時代表 Intel／AMD 的 64 位元 x86 主機。ARM64 包供其他主機使用，這台 Debian 主機不選它。**

發行包內的 `healthbank` 已包含編譯完成的前端與 Go 後端。執行主機不需要安裝 Node.js、Python、Go 編譯器、SQLite 服務或瀏覽器自動化程式。使用預設 SQLite 時也不用先建立資料庫。

1. 在寶塔「檔案」把對應架構的壓縮檔上傳到應用程式目錄並解壓縮，例如 `/www/wwwroot/healthbank/`。一般 Intel／AMD 64 位元主機選 `healthbank_1.0.1_linux_amd64.tar.gz`；ARM 64 位元主機選 `healthbank_1.0.1_linux_arm64.tar.gz`。版本號以實際下載檔名為準。
2. 開啟「網站 → Go 項目 → 添加項目」，依下表填寫。寶塔若自動產生完整執行命令，保留指向該執行檔的命令即可，不需要 `serve` 參數。
3. 綁定已解析到本機的域名，使用寶塔提供的域名／外網映射功能，讓 Nginx 轉送到 `127.0.0.1:8787`。在站點 SSL 設定啟用 HTTPS，供手機與筆電正式登入使用。
4. 點「啟動」，開啟 `https://你的域名/`。SQLite 與必要的服務設定會自動建立在執行檔旁的 `data/` 目錄；不論寶塔啟動時採用哪一個工作目錄，都會使用同一個位置。

| 寶塔欄位 | 填寫內容 |
| --- | --- |
| 項目執行檔 | 實際解壓後的 `healthbank` 完整路徑 |
| 項目名稱 | `healthbank` 或你習慣的名稱 |
| 項目端口 | `8787` |
| 執行命令／參數 | 只執行 `healthbank`；若欄位只收附加參數則留空 |
| 執行帳號 | 可讀取及執行程式、並可寫入該目錄的帳號，例如 `www` |
| 域名 | 你已解析到這部主機的域名 |
| 開機啟動 | 開啟 |

發行壓縮檔會保留程式的 `0755` 執行權限。若透過其他方式複製後權限遺失，在寶塔檔案管理的「權限」恢復執行權限即可。應用程式目錄須讓執行帳號可以建立 `data/`；不需要設成 `777`。

寶塔不同版本的按鈕名稱略有差異。Go 項目的執行檔、端口、執行帳號與域名欄位可參考[寶塔官方 Go 項目教學](https://www.bt.cn/bbs/thread-93034-1-1.html)。本程式已把該教學範例原本另行執行的安裝與資料庫建立步驟收進自動啟動流程。

## 二、檔案與預設行為

| 檔案／目錄 | 用途 |
| --- | --- |
| `healthbank` | 完整 Go 服務與內嵌前端，主要部署檔案 |
| `data/settings.sqlite` | SQLite 個人檢驗項目設定、顯示偏好及版本號 |
| `data/settings.sqlite-wal`、`data/settings.sqlite-shm` | SQLite 執行期間可能出現的工作檔，不要在服務運行時單獨刪除 |
| `data/identity.key` | 身分對應密鑰，讓同一位已驗證使用者在手機與電腦得到同一組設定；備份、搬家時需一起保留 |
| `deploy/nginx.healthbank.conf` | 此應用程式專用的選用 Nginx 轉送片段 |
| `VERSION`、`BUILDINFO` | 版本、建置時間、提交識別及執行檔架構 |
| `SHA256SUMS` | 同一版發行檔案的 SHA-256 校驗清單，位於發行下載目錄 |

資料庫只保存經驗證身分對應的不可直接識別代碼、檢驗項目設定、介面偏好與設定修改時間。瀏覽報告不會建立登入歷史；檢驗數值、健保卡號、身分證字號、密碼及官方網站 Cookie 不寫入此資料庫。連線期間需要的官方工作階段與報告以暫存方式處理；應用程式重啟後需重新登入、拉取或匯入。

瀏覽器的「記住登入欄位」與密碼管理員屬於使用者裝置上的選擇。它們和伺服器上的設定資料庫是不同的儲存位置；密碼由瀏覽器／手機密碼管理員處理。

## 三、只有需要時才使用 Nginx 片段

寶塔 Go 項目若已產生可用的轉送設定，直接使用該設定即可。包內 `deploy/nginx.healthbank.conf` 用於手動設定或修正此應用程式的站點；不是全機 Nginx 設定。

在寶塔「此域名的站點設定 → 設定檔」內，保留面板管理的 `server_name`、`listen`、SSL 憑證及 ACME 驗證區塊，將現有的 `location /` 轉送區塊替換成包內片段。不要新增第二個 `location /`，也不要覆蓋全域 `nginx.conf`。

片段會將所有應用程式請求轉送到 `127.0.0.1:8787`，傳遞原始域名及 HTTPS 資訊，允許最多 13 MiB 的完整請求，容納應用程式上限為 12 MiB 的檔案及表單包裝，並為官方網站查詢保留 130 秒轉送等候時間。前端由 Go 提供，不需另行設定靜態檔案的 `root` 或 `alias`。服務預設只監聽本機；透過域名訪問時不需要對外開放 `8787`。

## 四、進階設定：預設全部可以不填

以下環境變數可在寶塔 Go 項目的環境設定填入；修改後重啟應用程式。

| 名稱 | 預設值 | 說明 |
| --- | --- | --- |
| `HEALTHBANK_ADDR` | `127.0.0.1:8787` | 完整監聽位址，若設定則優先使用 |
| `PORT` | `8787` | 未指定完整位址時使用的端口；寶塔填入的端口須一致 |
| `HEALTHBANK_DATA_DIR` | 執行檔旁的 `data/` | 設定資料及身分對應密鑰的位置；自訂時建議使用絕對路徑 |
| `HEALTHBANK_DB_DRIVER` | `sqlite` | 可選 `sqlite` 或 `mysql` |
| `HEALTHBANK_MYSQL_DSN` | 空白 | 僅 MySQL 使用；格式見下一節 |

若改成其他端口，例如 `8788`，寶塔項目端口及 Nginx 的 `proxy_pass` 也要同時改為 `8788`。一般家庭使用不需要改動以上設定。

## 五、更新與備份

### 更新程式

在寶塔停止此 Go 項目，保留原本的整個 `data/` 目錄，把新的 `healthbank` 放到原位置，再點啟動。新的前端已在新執行檔內，不需手動替換前端檔案或執行 npm。資料庫結構升級由程式自動處理；若資料庫已由未來版本升級，舊程式會拒絕使用不支援的結構，避免誤讀。

更新前保留上一版執行檔及整個 `data/` 備份，便於回復。這是設定備份，不會包含檢驗報告歷史。

### 備份設定

最簡單的方式是在寶塔停止此 Go 項目，將整個 `data/` 目錄打包備份，再啟動。停止後一起複製可以避免只備份到 SQLite 主檔、漏掉尚未合併的 WAL 內容。不要把 `data/` 放進公開 GitHub 倉庫或程式發行包。

搬到另一台機器時，使用同版或較新版執行檔，保留整個 `data/`，再依第一次部署方式綁定域名啟動。身分對應密鑰需要原樣保留，否則原來的設定無法再對應到登入者。

## 六、SQLite 遷移到 MySQL

### 支援範圍

預設 SQLite 使用純 Go 驅動，因此 Linux 發行執行檔不依賴系統的 `libsqlite3`。MySQL 使用相同的設定文件與版本控制介面、專用的 MySQL 建表語法及交易式匯入。程式不會把 SQLite SQL 檔直接送進 MySQL。

MySQL 資料庫本身須先在寶塔「資料庫」建立，使用 `utf8mb4`；程式第一次連線時會自動建立自己的表。連線帳號需要該資料庫的建表、讀取、新增、更新與刪除權限。

範例 DSN（以下使用的是佔位文字）：

```text
healthbank:YOUR_DATABASE_PASSWORD@tcp(127.0.0.1:3306)/healthbank?charset=utf8mb4&timeout=10s
```

DSN 語法依 [Go MySQL Driver 官方說明](https://github.com/go-sql-driver/mysql#dsn-data-source-name)。只連接本機的寶塔 MySQL 可使用上述形式；若 MySQL 在其他主機，應依該服務的 TLS 設定調整 DSN。

### 遷移步驟

遷移是一次性的維運工作；平常部署與使用不需要執行命令。

1. 在寶塔停止 Go 項目，完整備份 `data/`。
2. 保留目前 SQLite 環境設定，在寶塔終端執行以下匯出。輸出路徑可自行選擇；不要放在可公開下載的站點靜態目錄。

   ```bash
   /www/wwwroot/healthbank/healthbank settings export /tmp/healthbank-settings.json
   ```

3. 在寶塔建立新的、供本應用程式專用的空白 MySQL 資料庫。設定 `HEALTHBANK_DB_DRIVER=mysql` 及 `HEALTHBANK_MYSQL_DSN`，並使用原本的 `HEALTHBANK_DATA_DIR`。密鑰不包含在匯出 JSON 內，須保留原有 `data/`；跨機遷移時也須帶走原密鑰。
4. 在同一組 MySQL 環境設定下執行匯入。寶塔項目的環境變數未必自動套用到終端，請確認終端也使用相同設定，再執行：

   ```bash
   /www/wwwroot/healthbank/healthbank settings import /tmp/healthbank-settings.json
   ```

5. 點啟動。用原來的使用者登入，確認手機與筆電看到相同設定。確認完成後可移走維運用的匯出檔，原 SQLite 備份則保留到你確認新資料庫運作正常為止。

匯入只接受空白目的資料庫；若已有個人設定會直接停止，不合併、不覆蓋。每份文件的版本號及修改時間會保留。檔案先完整驗證，再放進同一筆資料庫交易；任何錯誤都不會留下部分完成的匯入。

### 可攜式匯出格式

以下是合成示例，沒有真實個人資訊或檢驗結果：

```json
{
  "schema_version": 1,
  "exported_at": "2026-10-26T00:00:00Z",
  "entries": [
    {
      "subject_key": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
      "revision": 2,
      "settings": {},
      "updated_at": "2026-10-26T00:00:00Z"
    }
  ]
}
```

`settings` 的內容由應用程式的設定格式決定，不接受以此欄位儲存檢驗紀錄。匯出檔是設定備份，不是健保署原始資料格式。

MySQL 驅動、建表與匯入路徑均已實作。若該發行版的驗證紀錄未記載實際 MySQL 連線測試，請把 MySQL 視為已實作但尚待你的資料庫環境驗證的選項；SQLite 為預設路徑。開發者可用獨立的空白測試資料庫設定 `HEALTHBANK_TEST_MYSQL_DSN` 執行 `go test ./internal/store -run TestMySQLIntegration -v`，測試不會刪除或截斷既有資料表。

## 七、常見狀況

| 現象 | 檢查位置與處理 |
| --- | --- |
| 顯示 `cannot execute binary file` 或 `Exec format error` | amd64／arm64 架構是否選對；是否把原始碼包當成執行包 |
| 顯示 `permission denied` | 執行檔是否有執行權限；寶塔執行帳號是否能讀寫應用程式資料目錄 |
| 顯示 `address already in use` | 8787 是否被另一個程序占用；改端口後需同步修改寶塔與 Nginx |
| 域名顯示 502 | Go 項目是否啟動；Nginx 是否轉送到正確的 `127.0.0.1:8787`；查看該項目啟動訊息 |
| 頁面出現但前端檔案 404 | 寶塔是否加了另行讀取磁碟的靜態檔案規則；本應用程式的前端檔案也應轉送到 Go |
| 重新啟動後設定不見 | 是否換了執行檔目錄或 `HEALTHBANK_DATA_DIR`；是否遺失原身分對應密鑰 |
| 設定儲存顯示版本衝突 | 另一部裝置已修改同一份設定；重新讀取最新設定後再修改 |
| 官方網站登入失敗、驗證碼過期 | 重新取得驗證碼並由使用者填寫；這不等同於本服務或資料庫故障 |
| 官方查詢暫時無回應 | 稍後重新登入查詢，或先使用已下載資料的匯入功能；不會把失敗結果當成新的檢驗數值 |

### 網站可開啟，但圖形驗證碼回傳 502

登入視窗會自動建立工作階段並取得官方驗證碼圖片；使用者只需辨識及輸入圖片文字。若 `/api/health` 與 `/api/session` 正常，只有 `/api/auth/captcha` 失敗，不能直接歸因於 Go 未啟動。Cloudflare 的錯誤頁可能取代原始 502 回應，需在部署主機取得原始錯誤碼。

在寶塔終端執行以下唯讀檢查，直接呼叫 Go 的本機端點，不經 Nginx 或 Cloudflare。它不需要登入資料，最多等待 45 秒；成功時會省略圖片及挑戰識別字串：

```bash
curl --noproxy '*' -sS --connect-timeout 5 --max-time 45 \
  -w '\nHTTP %{http_code} | %{time_total}s\n' \
  http://127.0.0.1:8787/api/auth/captcha |
sed -E 's/"image_url":"[^"]*/"image_url":"[image omitted]/g;s/"challenge_id":"[^"]*/"challenge_id":"[omitted]/g'
```

| 原始結果 | 下一個檢查方向 |
| --- | --- |
| HTTP 200 且圖片已取得 | 本機取圖成功；核對同時段公開請求、Nginx 與 Cloudflare 的轉送回應。 |
| `NETWORK` | 部署主機對官方的 DNS、TLS、代理、連線或回應讀取失敗；此版本將這些原因歸在同一錯誤碼。 |
| `UPSTREAM_UNAVAILABLE` | 官方請求收到非成功 HTTP 狀態；不能單憑此碼宣稱官網全面停機。 |
| `UPSTREAM_CHANGED` | 官方請求回傳成功狀態，但內容不是支援的驗證碼圖片。 |
| 連線拒絕或 HTTP 000 | 核對實際 Go 監聽位址與程序狀態；若改過端口，指令也要使用該端口。 |

應用回傳的 `request_id` 可對照寶塔 Go 項目日誌中的 `request failed`、`code` 與 `request_id`。只提供錯誤 JSON、HTTP 狀態及對應錯誤日誌即可定位；不要貼出登入請求、Cookie、密碼或檢驗資料。

## 八、給開發者：建立 Linux Release

先按專案 README 完成相依套件準備、測試及前端建置，確認 `internal/webui/dist/index.html` 已存在，再執行：

```bash
bash scripts/release.sh v1.0.1
```

腳本產生：

```text
dist/releases/healthbank_1.0.1_linux_amd64.tar.gz
dist/releases/healthbank_1.0.1_linux_arm64.tar.gz
dist/releases/healthbank_1.0.1_source.tar.gz
dist/releases/SHA256SUMS
```

兩個 Linux 執行檔以 `CGO_ENABLED=0` 建置，前端隨 Go 內嵌。原始碼包只收錄本專案明確列出的程式目錄與專案文件，排除上層上傳檔、`data/`、資料庫、密鑰、`.env`、報表、快取、`node_modules`、`.tools` 及編譯產物。已釘選的 `web/vendor/` React 套件封裝會保留，供原始碼重建使用。編譯工具可用 `GO=/完整路徑/go` 指定；`SOURCE_DATE_EPOCH` 可固定發行時間，`COMMIT` 可指定提交識別，`RELEASE_DIR` 可改輸出目錄。腳本不會推送程式碼或發布 GitHub Release。

可使用 `sha256sum -c SHA256SUMS` 檢查下載檔案。GitHub 自動生成的 source.zip／source.tar.gz 與此處的有版本原始碼包屬於不同檔案，雜湊不會相同。

### GitHub Actions

`CI` 工作流程會在推送到 `main`／`master`、Pull Request 或手動執行時，依專案釘選資訊建立工具與 React 套件，完成前端編譯、Go 測試、靜態檢查及兩種 Linux 架構的編譯。`Linux release` 工作流程只在手動執行時上傳 Actions 建置附件，保留 7 天。推送 `v*` 版本標籤時，若對應 Release 已存在，會保留其發布狀態及檔案；若不存在，則建立附有封裝檔的草稿供維護者檢查。API 或認證查詢錯誤會明確失敗，不會被當成沒有 Release。本機封裝、Git 推送、工作流程執行與 Release 發布是不同操作。

工作流程所使用的 GitHub 官方 Actions 皆釘選完整提交識別，不使用浮動的 `@main` 或版本標籤；Node.js 執行環境固定為 `24.22.1`。工作流程狀態應以本專案的 [Actions 執行紀錄](https://github.com/bestkakkoii/healthbank/actions)為準；本機成功編譯與 GitHub Actions 的實際執行結果分開記錄。

## 官方技術依據

- [寶塔 Go 項目教學](https://www.bt.cn/bbs/thread-93034-1-1.html)：執行檔、端口、執行帳號及域名設定。
- [SQLite WAL 文件](https://sqlite.org/wal.html)：WAL 與共同記憶體工作檔的角色。
- [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite)：純 Go 驅動及連線 PRAGMA。
- [Go MySQL Driver](https://github.com/go-sql-driver/mysql)：DSN、連線設定及 `database/sql` 驅動介面。
- [Nginx proxy 模組](https://nginx.org/en/docs/http/ngx_http_proxy_module.html)：轉送標頭、逾時與代理快取設定。
