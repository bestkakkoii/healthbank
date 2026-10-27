# 寶塔部署、更新與資料庫遷移

© 2026 bestkakkoii llc. All rights reserved. 原始碼採 MIT 授權，詳見 `LICENSE`。

專案倉庫：[bestkakkoii/healthbank](https://github.com/bestkakkoii/healthbank)。Linux 套件與校驗清單的發行入口：[GitHub Releases](https://github.com/bestkakkoii/healthbank/releases)。

## 第一次部署：上傳、選執行檔、綁域名、啟動

**主要部署目標為 Debian 13（trixie）x64。使用 `*_linux_amd64.tar.gz`；`amd64` 在發行檔名中同時代表 Intel／AMD 的 64 位元 x86 主機。ARM64 包供其他主機使用，這台 Debian 主機不選它。**

登入後預設執行全部歷史查詢，也可指定日期；實際可提供的歷史資料仍由健康存摺決定。伺服器與瀏覽器的使用目標見 [README 平台範圍](../README.md#執行需求與平台範圍)，實際測試環境與尚待驗收項目見 [驗證紀錄](verification.zh-TW.md)。

發行包內的 `healthbank` 已包含編譯完成的前端與 Go 後端。執行主機不需要安裝 Node.js、Python、Go 編譯器、SQLite 服務或瀏覽器自動化程式。使用預設 SQLite 時也不用先建立資料庫。主機仍需有有效的系統 CA 信任憑證、正確時間，以及 DNS／對外 HTTPS 連線；這些是連接官方服務的作業系統與網路條件。

1. 在寶塔「檔案」把對應架構的壓縮檔上傳到應用程式目錄並解壓縮，例如 `/www/wwwroot/healthbank/`。一般 Intel／AMD 64 位元主機選 `*_linux_amd64.tar.gz`；ARM 64 位元主機選 `*_linux_arm64.tar.gz`。版本號以實際下載檔名為準。
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

## 檔案與預設行為

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

## 只有需要時才使用 Nginx 片段

寶塔 Go 項目若已產生可用的轉送設定，直接使用該設定即可。包內 `deploy/nginx.healthbank.conf` 用於手動設定或修正此應用程式的站點；不是全機 Nginx 設定。

在寶塔「此域名的站點設定 → 設定檔」內，保留面板管理的 `server_name`、`listen`、SSL 憑證及 ACME 驗證區塊，將現有的 `location /` 轉送區塊替換成包內片段。不要新增第二個 `location /`，也不要覆蓋全域 `nginx.conf`。

片段會將所有應用程式請求轉送到 `127.0.0.1:8787`，傳遞原始域名及 HTTPS 資訊，允許最多 13 MiB 的完整請求，容納應用程式上限為 12 MiB 的檔案及表單包裝，並為官方網站查詢保留 130 秒轉送等候時間。前端由 Go 提供，不需另行設定靜態檔案的 `root` 或 `alias`。服務預設只監聽本機；透過域名訪問時不需要對外開放 `8787`。

## 進階設定：預設全部可以不填

以下環境變數可在寶塔 Go 項目的環境設定填入；修改後重啟應用程式。

| 名稱 | 預設值 | 說明 |
| --- | --- | --- |
| `HEALTHBANK_ADDR` | `127.0.0.1:8787` | 完整監聽位址，若設定則優先使用 |
| `PORT` | `8787` | 未指定完整位址時使用的端口；寶塔填入的端口須一致 |
| `HEALTHBANK_DATA_DIR` | 執行檔旁的 `data/` | 設定資料及身分對應密鑰的位置；自訂時建議使用絕對路徑 |
| `HEALTHBANK_DB_DRIVER` | `sqlite` | 可選 `sqlite` 或 `mysql` |
| `HEALTHBANK_MYSQL_DSN` | 空白 | 僅 MySQL 使用；格式見下一節 |

若改成其他端口，例如 `8788`，寶塔項目端口及 Nginx 的 `proxy_pass` 也要同時改為 `8788`。一般家庭使用不需要改動以上設定。

## 更新與備份

### 更新程式

在寶塔停止此 Go 項目，保留原本的整個 `data/` 目錄，把新的 `healthbank` 放到原位置，再點啟動。新的前端已在新執行檔內，不需手動替換前端檔案或執行 npm。資料庫結構升級由程式自動處理；若資料庫已由未來版本升級，舊程式會拒絕使用不支援的結構，避免誤讀。

更新前保留上一版執行檔及整個 `data/` 備份，便於回復。這是設定備份，不會包含檢驗報告歷史。啟動後重新整理瀏覽器，以頁面或 `./healthbank --version` 核對所下載發行版的版本與提交。原有可用的寶塔轉送設定可沿用。更新不會自動補回暫存報告；重新登入或按「取得全部歷史」查詢資料。

### 備份設定

最簡單的方式是在寶塔停止此 Go 項目，將整個 `data/` 目錄打包備份，再啟動。停止後一起複製可以避免只備份到 SQLite 主檔、漏掉尚未合併的 WAL 內容。不要把 `data/` 放進公開 GitHub 倉庫或程式發行包。

搬到另一台機器時，使用同版或較新版執行檔，保留整個 `data/`，再依第一次部署方式綁定域名啟動。身分對應密鑰需要原樣保留，否則原來的設定無法再對應到登入者。

## SQLite 遷移到 MySQL

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

設定匯出只含設定與假名化身分對應，不是健保署報告備份；原本的 `identity.key` 須另外保留。實際 MySQL 測試所用版本、隔離保護與遷移結果集中記於 [驗證紀錄](verification.zh-TW.md)，不代表已驗證其他部署主機。開發者的測試資料庫設定見 [CONTRIBUTING.md](../CONTRIBUTING.md)。

## 常見狀況

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

### 網站可開啟，但圖形驗證碼無法取得

登入視窗會自動建立工作階段並取得官方驗證碼圖片；使用者只需辨識及輸入圖片文字。若 `/api/health` 與 `/api/session` 正常，只有 `/api/auth/captcha` 失敗，不能直接歸因於 Go 未啟動。

官方連線失敗使用 HTTP 424 搭配結構化錯誤，區分 DNS、TLS、逾時、連線失敗及官方 HTTP 狀態。畫面會顯示安全的錯誤代碼、可取得的連線回應狀態與查詢代碼。Cloudflare／Nginx 的自訂錯誤規則仍可能替換錯誤回應；若畫面仍只有通用 502，請從部署主機讀取原始回應。

可把套件解壓到另一個目錄，在該目錄執行單次診斷，不需停機或覆蓋目前服務：

```bash
chmod 755 ./healthbank
./healthbank diagnose-nhi
```

這個指令直接使用正式連接器，先讀取登入頁，再於同一個 Cookie 工作階段取圖。它不開啟資料庫、不讀取既有使用者連線、不輸入帳密、不提交登入、不輸出圖片內容或驗證碼識別字串。`image_decoded: true` 只代表完整圖片解碼成功，**不代表登入或報告取得成功**；失敗會傳回非零退出碼，不做自動重試。

既有成功取圖與連線失敗的測試環境、時間及結果均保留在 [驗證紀錄](verification.zh-TW.md)。某次成功不能推論其他主機可連線，也不能證明已完成帳號登入或報告取得。

輸出的 `diagnostics.phase` 是中斷／最後完成階段，例如 `dns`、`tcp_connect`、`proxy_connect`、`tls_handshake`、`response_headers`、`response_body`。`proxy_selected` 只表示這次連線使用了環境設定的代理，不含代理位址。TLS 版本與協議只在實際觀察到時提供；若有代理，不能把它當成獨立證明已與官方來源完成端到端握手。指令與寶塔服務若使用不同執行帳號、DNS 或代理環境，結果也可能不同。

網頁會在錯誤訊息顯示中斷階段，API 及相同 `request_id` 的安全日誌也會帶上這些資訊。程式不會依重設錯誤自動停用憑證驗證、強制降級 TLS、偽造瀏覽器指紋或重送登入。

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
| `DNS_FAILED` | 部署主機的對外連線未完成 DNS 解析；檢查主機解析器與網路。 |
| `TIMEOUT` | 對外連線或回應逾時；核對對外 HTTPS 連線與官方服務的當時狀態。 |
| `TLS_CERTIFICATE`／`TLS_FAILED` | 檢查系統時間、CA 憑證或 TLS 連線；不可用關閉憑證驗證替代修復。 |
| `CONNECTION_REFUSED`／`CONNECTION_RESET`／`CONNECTION_FAILED` | 對外連線被拒絕、重設或無法建立；檢查路由、出口及主機連線設定。 |
| `NETWORK`／`RESPONSE_INTERRUPTED` | 未分類的連線失敗，或回應沒有完整收到。 |
| `UPSTREAM_UNAVAILABLE` | 連線收到非成功 HTTP 狀態；查看 `upstream_status`。不能單憑此碼宣稱官網全面停機。 |
| `UPSTREAM_ACCESS_DENIED` | 尚未登入的官方請求收到 401／403；這不表示使用者登入已逾時，也不能僅依狀態碼斷言是機器人阻擋。 |
| `CAPTCHA_INVALID_IMAGE` | 請求回傳成功狀態，但內容不是支援的驗證碼圖片。 |
| 連線拒絕或 HTTP 000 | 核對實際 Go 監聽位址與程序狀態；若改過端口，指令也要使用該端口。 |

應用回傳的 `request_id` 可對照寶塔 Go 項目日誌中的 `official request failed`、`code`、`request_id` 與可取得的 `upstream_status`。只需提供錯誤 JSON、HTTP 狀態及對應錯誤日誌；不需登入請求、Cookie、密碼或檢驗資料。網站管理者應依這些證據繼續定位；某一測試環境的取圖失敗不代表其他網路也失敗。

## 開發與發行

建置環境、品質檢查與封裝命令集中於 [CONTRIBUTING.md](../CONTRIBUTING.md)；Agent 的範圍與不變條件見 [README_AI.md](../README_AI.md)。工具版本、官方提交與相容性限制見 [DEPENDENCIES.md](../DEPENDENCIES.md) 及鎖定檔。

每次發行提供 Linux amd64／arm64 執行包、原始碼包與 `SHA256SUMS`。下載對應檔案及校驗清單後，可在同一個目錄使用 `sha256sum -c SHA256SUMS` 核對；只下載其中一個套件時，其他未下載檔案會顯示缺少，需核對已下載檔案的結果。GitHub 自動生成的原始碼壓縮檔與專案附上的來源包是不同檔案，雜湊也不同。

一般使用者只需對應架構的執行包。建置、測試、GitHub 工作流程與公開發行是不同步驟；狀態以 [Actions](https://github.com/bestkakkoii/healthbank/actions) 與 [Releases](https://github.com/bestkakkoii/healthbank/releases) 的實際結果為準。

## 官方技術依據

- [寶塔 Go 項目教學](https://www.bt.cn/bbs/thread-93034-1-1.html)：執行檔、端口、執行帳號及域名設定。
- [SQLite WAL 文件](https://sqlite.org/wal.html)：WAL 與共同記憶體工作檔的角色。
- [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite)：純 Go 驅動及連線 PRAGMA。
- [Go MySQL Driver](https://github.com/go-sql-driver/mysql)：DSN、連線設定及 `database/sql` 驅動介面。
- [Nginx proxy 模組](https://nginx.org/en/docs/http/ngx_http_proxy_module.html)：轉送標頭、逾時與代理快取設定。
