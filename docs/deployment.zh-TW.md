# 部署、更新與設定遷移

Copyright © 2026 bestkakkoii llc. SPDX-License-Identifier: MIT.

## 啟動

1. 從[Releases](https://github.com/bestkakkoii/healthbank/releases)取得CPU對應的Linux包與SHA256SUMS，驗證後解壓。Intel／AMD 64位元選amd64；ARM64選arm64。
2. 使用專用、非root執行帳號。程式需可執行，資料目錄需可寫；不要設為777或放在公開靜態下載目錄。
3. 執行 `./healthbank`。預設本機 `127.0.0.1:8787`；由既有服務管理器管理生命週期即可。
4. 配置HTTPS反向代理，將該網域的請求送至Go服務。前端已內嵌，不需另外設定網站root或安裝Node、Go、Python、Chromium、SQLite服務。

Linux部署包不含版本名稱的外層目錄；解壓到選定目錄後，直接得到 `healthbank`、`LICENSE`、`docs/`、`deploy/` 等內容。
執行檔保留755權限。包內不附 `data/`、資料庫或身分金鑰，不會以空白設定覆蓋既有資料。
解壓工具自行提供的「建立同名目錄」選項不屬於壓縮包內容，直接部署時不要勾選。
原始碼包仍有獨立的原始碼根目錄，不是用來直接部署的執行包。

主要目標為Debian 13 x86_64。其他Linux與ARM64的實測情況以[驗證紀錄](verification.zh-TW.md)為準。
需要有效CA信任、正確時間、DNS與對外HTTPS；不限定特定面板。

## 環境變數

| 名稱 | 預設 | 用途 |
| --- | --- | --- |
| `HEALTHBANK_ADDR` | `127.0.0.1:8787` | 完整監聽地址，優先於PORT |
| `PORT` | `8787` | 未指定完整地址時使用 |
| `HEALTHBANK_DATA_DIR` | 執行檔旁`data/` | 設定資料庫與身分對應密鑰；遷移時保持一致 |
| `HEALTHBANK_DB_DRIVER` | `sqlite` | 可選mysql |
| `HEALTHBANK_MYSQL_DSN` | 空 | MySQL專用，不得寫入公開日誌或倉庫 |

SQLite首次啟動自動建立。MySQL需先建立專用空資料庫與最小必要權限帳號；遠端MySQL應配置TLS。
服務只信任來自loopback代理的 `X-Forwarded-Proto`，不要任意將應用埠公開。

### 容量設定

預設總工作階段 64 個，其中新匿名階段額度 16 個；匿名建立採程序內每秒補一個的配額，不保存 IP 指紋。
接受的連線最多 128 個；有狀態請求總量含排隊最多 32 個，同一階段一次執行一個、另容納 4 個等待者。
排隊最多等待 5 秒且接受取消。來源／匯入／文件等重工作同時最多 2 個，另須通過工作用量預留。

| 環境變數 | 預設 | 接受範圍 |
| --- | ---: | --- |
| `HEALTHBANK_MAX_SESSIONS` | 64 | 2–256 |
| `HEALTHBANK_ANONYMOUS_SESSIONS` | 16 | 1–64，且小於總階段數 |
| `HEALTHBANK_MAX_REQUESTS` | 32 | 1–1024 |
| `HEALTHBANK_SESSION_QUEUE` | 4 | 0–16 |
| `HEALTHBANK_HEAVY_REQUESTS` | 2 | 1–8，且不大於請求總量 |
| `HEALTHBANK_MAX_CONNECTIONS` | 128 | 16–4096 |
| `HEALTHBANK_WORKING_MEMORY_MIB` | 256 | 192–4096 |
| `HEALTHBANK_RETAINED_MEMORY_MIB` | 256 | 16–4096 |
| `HEALTHBANK_SESSION_MEMORY_MIB` | 64 | 至少 4，且不大於保留總預算 |

這些是操作預留及保留內容的邏輯計數，不是程序 RSS 的硬性上限或最低實體記憶體保證。
例如大型檢驗匯入預留 192 MiB，因此預設工作預算一次只允許一份大型匯入；Go 執行環境、GC、
連線、作業系統與反向代理仍需額外容量。來源引用另有每個連接器 32 MiB 上限。
超限回傳固定錯誤及 `Retry-After`，保留原有報告或來源引用，不交付截斷資料。
長時間下載占用同階段操作時，另一操作可能得到 `session_busy`；可等下載結束後重試。完整取得 ZIP 的狀態／取消不等待傳輸鎖；登出、模式切換及裝置快取刷新會先取消舊 ZIP 傳輸。
這不是分散式阻斷服務防護，也不取代主機實測。

## 反向代理與不留存邊界

[deploy/nginx.healthbank.conf](../deploy/nginx.healthbank.conf)是APP的location片段，不是全機配置。
保留網域、HTTPS憑證與ACME設定；不能新增兩個相同location。所有前端與API由Go提供。

片段停用代理快取、request/response buffering及proxy_store，並禁止回應寫入代理暫存檔。
上限內的請求緩衝配置於記憶體。依可用RAM限制總並行請求；不能用無上限記憶體交換「不落地」。
應用API另外回傳 `Cache-Control: no-store, private`、`Pragma: no-cache`及 `X-Accel-Buffering: no`。

Healthbank 可攜檢驗 JSON／XML 單檔上限為 **32 MiB**，HTTP 額外允許 **64 KiB** 的 multipart 包裝。
完整取得 ZIP 的計畫端點接受最大 **34 MiB** 本文，其中內嵌資料包最多 32 MiB，其餘為有界計畫及檔案描述。
Nginx 片段對應最大入口設定 `client_max_body_size 34m` 與 `client_body_buffer_size 35m`；各 Go 端點仍執行自己的較小上限。
請連同原本停用 request/response buffering、暫存回應檔、快取及 `proxy_store` 的設定一起更新，避免代理先以舊上限拒絕合法計畫。
依[Nginx 請求本文緩衝規則](https://nginx.org/en/docs/http/ngx_http_core_module.html#client_body_buffer_size)，
超過記憶體緩衝的本文可能寫入暫存檔；不能只提高請求上限而保留較小的緩衝設定。
另存 HTML／ZIP 的單檔限制仍為 **12 MiB**，純 HTML 及 ZIP 內 HTML 展開總量仍限 **6 MiB**；
筆數、欄位與解析結構限制也維持不變。完整資料包在瀏覽器還原後，只有其檢驗部分送至此匯入端點，
不會把整個資料包或附件交給伺服器保存。完整取得 ZIP 則可將上述有界資料包暫存在伺服器記憶體並串流封裝，仍不落地。

裝置健康快取保存在使用者瀏覽器的 IndexedDB，與伺服器設定庫分開；啟用、上限、身分隔離與清除見[裝置快取](device-health-cache.zh-TW.md)。更新服務時不要清除使用者瀏覽器資料；正式版本須保留已支援可攜格式的驗證與還原相容性。瀏覽器儲存失敗不應妨礙當次記憶體查詢。

這不是對任何主機的零留存證明。還須稽核：CDN/WAF快取、請求本文記錄、除錯日誌、錯誤追蹤、
swap、core dump、容器/VM快照及備份。保留不含憑證與醫療內容的必要診斷，不以吞錯或停用所有錯誤輸出取代保護。
無法控制的第三方保留行為須明示；登入與健康API不得被「快取所有內容」規則覆蓋。

## 更新與備份

停止服務，備份上一版執行檔與整個 `data/`，替換執行檔後啟動。前端已內嵌，不另跑npm。
`settings.sqlite`、其WAL工作檔及 `identity.key` 必須一致保存；先停止再備份可避免遺漏未合併寫入。
使用 `./healthbank --version` 與 `/api/health` 查核實際版本；新版本不會自動恢復記憶體中的報告。

設定備份包含可連結個人的假名化資料，不能公開。備份訂定有限保留期限與刪除程序；
備份不包含官方檢驗歷史。密鑰遺失不得悄悄重新產生來掩蓋原設定無法存取。

## SQLite／MySQL遷移

使用服務相同環境與資料目錄，停止服務後：

```bash
./healthbank settings export /secure-maintenance/settings.json
# 配置目的端HEALTHBANK_DB_DRIVER與HEALTHBANK_MYSQL_DSN後：
./healthbank settings import /secure-maintenance/settings.json
```

輸出路徑為示例，須先建立受保護的父目錄；輸出檔不能已存在。匯入只接受空白目的資料庫，
先驗證全部內容再交易式匯入。保留原 `identity.key`；匯出JSON不包含密鑰。
單一設定文件上限 256 KiB，整份設定快照的輸出與輸入均限 64 MiB；不能產生超過可還原上限的成功備份。匯入先拒絕重複／歧義欄位與損壞 Unicode，再將通過型別與白名單驗證的標準化設定寫入目的庫。修訂值須是 1 至 2^53−1 的精確整數，時間戳須符合 RFC3339 且不超過 MySQL 欄位的 35 bytes。這些限制同時適用 SQLite 與 MySQL。

確認新服務可讀取相同設定後，按維運保留政策處理匯出檔與舊備份。

### 金鑰配對與舊版升級

RC10 設定庫／設定快照使用 schema 2，新增用途分離的金鑰核對值與配對來源。
核對值不是金鑰；匯出仍不含 `identity.key`。新版備份匯入目的庫前，必須先放回相配的原金鑰，
不相配會拒絕，不能用舊格式選項繞過。

```bash
# 使用與服務相同的環境和資料目錄，先停止服務。
./healthbank settings verify-key
```

新建空庫會記錄配對。既有舊庫若已有設定但沒有核對值，升級仍能服務，並回報固定
`identity_key_unverified`；程式不會假稱能從舊庫證明目前金鑰正確。營運者確認目前檔案就是原金鑰、
並保留舊備份後，可明確執行 `./healthbank settings bind-legacy-key` 記錄此確認。
這個指令不是升級必做步驟，也不能把人工確認轉稱為獨立密碼學證明。

只有 schema 1 舊備份沒有核對值時，才使用明確的
`./healthbank settings import-legacy /secure-maintenance/settings.json`。
匯入後保留 `legacy_unverified` 來源；新版匯出會核對本次使用金鑰，同時保留原始配對尚未證明的狀態。
一般 `settings import` 不默默接受這種缺少核對值的舊備份。
需降回不支援 schema 2 的程式時，應還原升級前一致的資料庫與金鑰備份，不能只替換舊執行檔。

## 診斷

```bash
./healthbank diagnose-nhi
```

只使用正式連接器讀登入頁與驗證碼，不提供帳密、不提交登入、不讀設定資料庫。
圖片完整解碼成功不等於實際登入、完整歷史或其他主機也成功。
診斷需使用與服務相同的帳號、DNS與代理環境，否則不能直接比較。

`GET /api/health` 是公開的存活／版本查詢。`GET /api/ready` 僅供維運者直接連至本機 listener：

```bash
curl --fail http://127.0.0.1:8787/api/ready
```

readiness 檢查設定資料庫連通及資料表可讀，提供容量計數、Go 堆觀測與金鑰配對來源狀態；
不讀出設定個人內容、不輸出金鑰／核對值／DSN，也不保證資料庫寫入或健保署可用。
非 loopback、非本機 Host、帶來源或代理轉送標頭的請求會拒絕。
隨附 Nginx 片段明確封鎖此端點；其他反向代理也應保持私有。
資料庫檢查失敗時 readiness 回傳 503，存活端點仍可回應。

登入、清單、明細、產製、下載與清理診斷使用固定操作代碼。
傳輸中斷保留隨機請求識別與 `response_body` 階段，不印出上游本文或個人查詢內容。

| 狀況 | 檢查 |
| --- | --- |
| 程式不能執行 | CPU架構、執行權限與套件類型 |
| 502或連不上 | Go是否啟動、本機監聽埠、HTTPS代理目標 |
| DNS／TLS／連線重設 | 安全錯誤碼及最後連線階段；不可關閉憑證驗證 |
| 驗證碼失效 | 使用者更新圖片並重新填寫；不自動重試登入 |
| 設定衝突 | 另一裝置已修改，重讀最新版本 |
| 設定不見 | DATA_DIR與原identity.key是否一致 |

原始帳密、Cookie、真實報告及未檢查的封包檔不得上傳公開Issues。錯誤碼與操作階段通常足以初步定位。
公開服務前依[資料處理文件](privacy.zh-TW.md)補齊實際營運資訊與法律審查。
