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

## 反向代理與不留存邊界

[deploy/nginx.healthbank.conf](../deploy/nginx.healthbank.conf)是APP的location片段，不是全機配置。
保留網域、HTTPS憑證與ACME設定；不能新增兩個相同location。所有前端與API由Go提供。

片段停用代理快取、request/response buffering及proxy_store，並禁止回應寫入代理暫存檔。
上限內的請求緩衝配置於記憶體。依可用RAM限制總並行請求；不能用無上限記憶體交換「不落地」。
應用API另外回傳 `Cache-Control: no-store, private`、`Pragma: no-cache`及 `X-Accel-Buffering: no`。

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
確認新服務可讀取相同設定後，按維運保留政策處理匯出檔與舊備份。

## 診斷

```bash
./healthbank diagnose-nhi
```

只使用正式連接器讀登入頁與驗證碼，不提供帳密、不提交登入、不讀設定資料庫。
圖片完整解碼成功不等於實際登入、完整歷史或其他主機也成功。
診斷需使用與服務相同的帳號、DNS與代理環境，否則不能直接比較。

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
