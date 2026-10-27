# 健康映覽 · Healthbank

![Healthbank：自架檢驗報告比較與長期趨勢](docs/assets/healthbank-banner.svg)

**把健康存摺逐筆列出的檢驗結果，整理成容易閱讀的年度比較、趨勢圖與個人基準。**

[下載與發行說明](https://github.com/bestkakkoii/healthbank/releases) · [寶塔部署](docs/deployment.zh-TW.md) · [更新紀錄](CHANGELOG.md) · [醫學判讀原則](docs/clinical-reference.zh-TW.md) · [驗證紀錄](docs/verification.zh-TW.md)

自架的繁體中文網頁工具，適合家人以筆電或手機瀏覽器使用。Go 後端內嵌已編譯的 React 前端與參考目錄；**上傳一個執行檔即可運行，SQLite 為預設，MySQL 為選用。**

這是獨立開源專案，非健保署官方服務。官方連接器仍須依實際帳號與部署環境驗收；取圖成功、合成協定測試與真實登入／歷史完整性是不同證據，詳見[驗證紀錄](docs/verification.zh-TW.md)。

## 功能

| 畫面 | 可以做什麼 |
| --- | --- |
| 檢驗總覽 | 查看最近檢查、涵蓋年度、關注項目、來源與查詢範圍 |
| 年度比較 | **日期為列、檢驗項目為欄**；依年份、類別、關注或異常篩選，同日多筆結果分開保留 |
| 數值趨勢 | 依真實日期間隔呈現完整所選期間；查看原院／個人基準、單筆明細與對數刻度 |
| 個人設定 | 尚未匯入即可查看內建基準；額外項目隨實際資料新增，支援單項／全部還原與跨裝置設定 |

支援淺色、深色與跟隨系統外觀，以及最高 200% 的應用字級。手機比較表所有欄位都可滑走；桌面保留固定日期欄，並提供可見的橫向操作按鈕。另有 CSV、SVG 圖表及瀏覽器列印／PDF 功能；各功能實際驗收範圍見[驗證紀錄](docs/verification.zh-TW.md)。

![年度比較與歷史查詢控制；畫面為合成示範資料，非真實個人報告](docs/screenshots/history-comparison.jpg)

## 快速部署：寶塔

1. 到 [Releases](https://github.com/bestkakkoii/healthbank/releases) 選擇發行版。**預覽版也列在這裡；GitHub 的 Latest 標記只代表正式版，請核對發行說明。**
2. Debian 13 x64 主機下載檔名結尾為 **`_linux_amd64.tar.gz`** 的套件，上傳並解壓縮。
3. 在寶塔「網站 → Go 項目」選擇解壓後的 **`healthbank`** 執行檔，端口填 **`8787`**，啟動參數留空。
4. 綁定網域並啟用 HTTPS，點「啟動」。程式會在執行檔旁建立 `data/`。

一般部署不需要 Go、Node.js、Python、Chromium、外部 SQLite 服務或 MySQL。執行帳號需要程式執行權限，以及資料目錄的寫入權限。若複製時遺失執行權限，將 `healthbank` 設為 `755`。

寶塔已有可用映射時可直接沿用；手動設定可使用 [APP 專用 Nginx 片段](deploy/nginx.healthbank.conf)。詳細欄位、備份、連線診斷與 MySQL 遷移見[部署說明](docs/deployment.zh-TW.md)。

### 執行需求與平台範圍

| 範圍 | 需求與驗證界限 |
| --- | --- |
| 主要伺服器目標 | **Debian 13（trixie）x86_64**，對應 Linux amd64 靜態執行檔；寶塔主機須依部署步驟驗收 |
| 其他發行架構 | Linux arm64 提供交叉編譯套件；尚未完成原生 ARM64 主機驗收 |
| 桌面瀏覽器 | 目標為持續更新的 Chrome、Edge、Firefox 與 Safari；已做 Chromium 操作／版面檢查，不能等同所有瀏覽器均已驗收 |
| 手機瀏覽器 | 以 iOS Safari 為使用目標；已檢查窄螢幕、橫向與放大字級版面，實體 iPhone、系統字級與鍵盤仍須實機驗收 |
| 網路 | 主機需有效的系統 CA 信任憑證、正確時間，以及 DNS／對外 HTTPS 連線；對外網域由 HTTPS 反向代理轉送 |
| 開發建置 | Bootstrap 以 Linux x86_64 為目標；一般使用者下載執行包即可，開發工具清單見 [CONTRIBUTING.md](CONTRIBUTING.md) |

瀏覽器需啟用 JavaScript 與網站 Cookie；裝置欄位記憶功能另使用瀏覽器儲存空間。最低瀏覽器版號及 CPU／記憶體下限尚未建立驗收基準；目前僅提供 Linux 原生服務發行檔。

## 取得資料與閱讀原則

登入視窗會自動取得官方圖形驗證碼，**圖片文字由本人輸入**。健保卡號可輸入完整 12 碼，或輸入 8 碼由程式補上前導 `0000`；密碼可交由瀏覽器密碼管理員處理。

登入後預設執行「取得全部歷史」，也可指定自訂日期。查詢會明確送出擴大後的期間，保留全部資料來源與檢驗項目，取得官方搜尋回傳的完整陣列；不只擷取目前畫面的第一頁。**官方實際可提供的歷史範圍仍適用**，完整回傳陣列不等於已證明終身資料齊全。畫面會顯示送出的範圍；只有同一範圍具有獨立官方總筆數時，才宣稱完成筆數核對。詳見[連接協定與完整性界限](docs/nhi_protocol.md)。

也可匯入已下載的 HTML／ZIP；另存網頁可能只有一頁，因此匯入不保證完整歷史，也不會藉此取得官方身分或連結持久個人設定。

每筆原始結果、單位、原院參考值、來源旗標及檢查／就醫日期各自保留。不等式、區間、加號、定性結果與缺值不會被轉成假的精確趨勢點。**顏色是來源規則或個人基準的提示，不是通用疾病危險分期**；個人覆寫不能消除仍適用的獨立警示。完整規則與來源見[醫學判讀說明](docs/clinical-reference.zh-TW.md)。

## 保存內容與更新

| 位置 | 保存內容 |
| --- | --- |
| SQLite／選用 MySQL | 經官方驗證後的假名化身分對應、項目設定、個人基準、外觀與設定版本 |
| `data/identity.key` | 穩定的身分對應金鑰；需和設定資料庫一起備份 |
| 使用者瀏覽器 | 自行選擇記住的證號／卡號與外觀；密碼由瀏覽器密碼管理員管理 |
| 目前使用階段的記憶體 | 官方 Cookie／權杖與檢驗結果；不寫入設定資料庫、報告檔案或應用日誌 |

更新時停止 Go 項目，**備份並保留完整 `data/`**，替換執行檔後重新啟動。`identity.key` 遺失會使舊設定無法對應到登入者。檢驗報告不在設定備份中；登入到期、登出或服務重啟後需重新取得。詳見[更新與備份](docs/deployment.zh-TW.md#更新與備份)及[資料保護與問題回報](SECURITY.md)。

## 開發與文件

開發者與 Agent 從 **[README_AI.md](README_AI.md)** 開始，再依 [CONTRIBUTING.md](CONTRIBUTING.md) 建置與檢查。工具版本、上游提交、雜湊與相容性例外集中於 [DEPENDENCIES.md](DEPENDENCIES.md)、[dependencies.lock.json](dependencies.lock.json) 與套件鎖定檔；精確工具與依賴版本以這些鎖定資料為準。

- [部署、更新、備份、MySQL 遷移](docs/deployment.zh-TW.md)
- [官方協定與歷史資料完整性](docs/nhi_protocol.md)
- [檢驗判讀、單位與參考來源](docs/clinical-reference.zh-TW.md)
- [實際驗證紀錄](docs/verification.zh-TW.md) · [品質審查](docs/quality-audit.zh-TW.md)
- [後續功能評估與優先順序](docs/roadmap.md)（提案，尚未實作）
- [變更紀錄](CHANGELOG.md) · [第三方授權](THIRD_PARTY_NOTICES.md)

## 授權

原始碼採 [MIT License](LICENSE)，第三方元件保留各自授權。

Copyright © 2026 **bestkakkoii llc.**
