# 健康映覽 · Healthbank

![Healthbank：自架檢驗報告比較與長期趨勢](docs/assets/healthbank-banner.svg)

**把健康存摺逐筆列出的檢驗資料，整理成易讀、可比較、可追溯的年度表格與趨勢。**

[下載與發行](https://github.com/bestkakkoii/healthbank/releases) · [部署](docs/deployment.zh-TW.md) · [資料處理](docs/privacy.zh-TW.md) · [驗證](docs/verification.zh-TW.md)

獨立 MIT 開源專案，非健保署官方服務。Go 執行檔內嵌 React 前端；SQLite 預設、MySQL 選用。
不要求特定主機面板、額外會員帳號或日常打卡。前端不載入外部腳本、字型或廣告分析服務。

## 功能

| 功能 | 邊界 |
| --- | --- |
| 全歷史查詢 | 明確送出日期範圍、保留來源及同日多筆；官方實際留存範圍仍適用，不把收到陣列當作終身完整證明 |
| 年度比較 | 日期為列、項目為欄；搜尋、收藏、年度篩選及可見橫向控制 |
| 趨勢 | 真實日期間隔、精確數據點及逐筆明細；不把不等式或範圍捏成單點 |
| 年齡／性別情境 | 本人與比較條件並列、即時生效；目前提供血液肌酸酐／ALP具名範本，符合條件後可估算CKD-EPI 2021；不覆寫原院數據 |
| 個人基準 | 未匯入也顯示內建項目，支援個別／全部還原；官方驗證後跨裝置同步 |
| 登入記憶 | 私人裝置自選記住多位使用者的證號／卡號；密碼由瀏覽器管理，驗證碼由本人輸入 |
| 匯入與輸出 | HTML／ZIP匯入、CSV、SVG及瀏覽器列印；JSON／XML匯入及其他官方類別仍待接入 |

淺色／深色／跟隨系統，支援應用字級放大。手機不固定日期欄，桌面保留固定日期。
實體iOS、密碼管理員、真實官方資料完整性與實際部署各自驗收，不用模擬測試替代。

![年度比較；合成示範資料，非真實個人報告](docs/screenshots/history-comparison.jpg)

## 部署

下載符合CPU架構的Linux發行包，解壓並執行 `healthbank`；對外由HTTPS反向代理轉送。
預設監聽 `127.0.0.1:8787`，在執行檔旁建立 `data/`。不用安裝Go、Node.js、Python、Chromium或SQLite服務。
部署方式、系統服務與代理配置見[部署文件](docs/deployment.zh-TW.md)。

| 範圍 | 支援與查核 |
| --- | --- |
| 主要目標 | Debian 13（trixie）x86_64；Linux amd64靜態執行檔 |
| 其他架構 | Linux arm64交叉編譯；原生機器驗收另列 |
| 瀏覽器 | 持續更新的Chrome、Edge、Firefox、Safari；需要JavaScript與短效工作階段Cookie |
| 行動裝置 | 以iOS Safari為重點；原生縮放、密碼自動填入、鍵盤與橫直向須實機驗收 |
| 網路與作業系統 | 有效CA信任、正確時間、DNS及對外HTTPS；部署平台不限定面板 |
| 開發 | 工具來源、版本、提交及相容性限制集中在鎖定檔與[依賴文件](DEPENDENCIES.md) |

尚未建立通用最低瀏覽器版號或硬體下限，不杜撰可用範圍。預覽版在Releases明確標示；Latest不一定指向預覽版。

## 資料保存

- **SQL只保存假名化識別碼與設定**，不保存原始證號、卡號、密碼、官方權杖或檢驗結果。假名化不等於匿名，設定仍需保護。
- **官方工作階段與報告只在記憶體**。API回應不快取；代理、CDN、交換空間、轉存及備份須由實際營運者另行稽核。
- **裝置記憶由使用者選擇**，可忘記單人或全部。密碼不寫入網站儲存；密碼管理員可能要求生物辨識並依其設定同步。
- **年齡／性別比較與生日只在頁面記憶體**，本版未實作其跨裝置保存。使用者主動匯出的檔案由本人管理。

更新前停止服務，保留並備份 `data/`，尤其 `identity.key` 與設定資料庫須一起保留。
登入逾時、登出或服務重啟後需重新取得報告。詳見[隱私與資料處理](docs/privacy.zh-TW.md)。

## 開發與狀態

Agent從[README_AI.md](README_AI.md)開始；建置與嚴格檢查見[CONTRIBUTING.md](CONTRIBUTING.md)。

[原始判讀規則](docs/clinical-reference.zh-TW.md) · [情境計算契約](docs/comparison-engine.md) · [官方協定](docs/nhi_protocol.md) · [變更紀錄](CHANGELOG.md) · [第三方授權](THIRD_PARTY_NOTICES.md)

新增資料類別、廣泛的分齡常量、全表情境著色、全局分析設定同步及選配使用品項紀錄，仍須逐項完成與驗證，不能視為已提供。

原始碼採[MIT License](LICENSE)。使用者資料、官方文件與第三方元件不因此改變其權利。

Copyright © 2026 **bestkakkoii llc.**
