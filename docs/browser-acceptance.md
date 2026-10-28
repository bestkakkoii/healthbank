# 真實編譯程式的合成瀏覽器驗收

Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT.

此驗收直接啟動編譯後的 Go 執行檔，操作其內嵌 React 介面及真正的本機 API。它只產生明確標記的合成資料，不登入健保署、不使用個人報告，也不把資料寫入維護者原有的設定資料庫。瀏覽器僅供開發與 CI；Linux 部署包不需要 Node、Playwright、瀏覽器或字型下載。

## 固定的測試工具

`web/tests/browser/toolchain.json` 記錄 2026-10-28 核對的官方來源與 SHA-256：

| 工具 | 固定版本／來源 |
| --- | --- |
| Playwright | npm 開發依賴 `1.64.1`；官方原始碼提交 `cfe490d941f992655948eee8fd4fd94e1c4234bf` |
| Chromium Headless Shell | `156.0.8078.4`，對應 Playwright revision `1248`；Google 官方 Linux x64 壓縮包以 SHA-256 驗證 |
| 繁體中文字型 | Noto Sans CJK TC Regular；官方提交 `f8d157532fbfaeda587e826d4cd5b21a49186f7c`，字型與 SIL OFL 1.1 授權均核對 SHA-256 |
| CI 系統 | Ubuntu 24.04；Poppler 的 `pdftotext`、`pdfinfo` 版本記錄於驗收輸出，不宣稱鎖定所有系統套件 |

套件鎖定檔保留 npm 完整性雜湊。啟動前核對已安裝 Playwright 及其瀏覽器清單，啟動後再核對實際 Chromium 版本。字型使用測試專用 Fontconfig 目錄；瀏覽器透過實際渲染字型資訊確認中文字形存在。正式產品沒有新增遠端字型、外部腳本或追蹤請求。RC10 工具整合另以空白暫存前綴、鎖定套件與既有官方快取，通過 `npm ci --offline --ignore-scripts --no-audit --no-fund`，確認 147 個套件及兩個 Playwright 套件的精確版本可重新安裝；這不代表第一次安裝不需要網路。

官方來源：[Playwright 版本](https://github.com/microsoft/playwright/releases/tag/v1.64.1)、[該版本瀏覽器清單](https://github.com/microsoft/playwright/blob/v1.64.1/packages/playwright-core/browsers.json)、[瀏覽器安裝規則](https://playwright.dev/docs/browsers)、[Chrome for Testing](https://github.com/GoogleChromeLabs/chrome-for-testing)、[Noto CJK 固定來源](https://github.com/notofonts/noto-cjk/tree/f8d157532fbfaeda587e826d4cd5b21a49186f7c)。

## 執行方式

先依 `README_AI.md` 安裝專案鎖定工具，執行 `npm --prefix web ci`，完成前端編譯並建置 Linux amd64 執行檔。下列命令應使用專案鎖定的 Node，而不是任意系統版本：

```sh
node scripts/browser-acceptance.mjs --setup --with-deps
node scripts/browser-acceptance.mjs \
  --binary /absolute/path/to/healthbank \
  --evidence /absolute/path/to/synthetic-evidence
```

`--setup` 只安裝已核對的測試瀏覽器與字型；`--with-deps` 另透過 Playwright 安裝 Linux 系統依賴。環境也需要 `curl`、`unzip` 與 `poppler-utils`。CI 與 Release 工作流程會安裝它們。測試瀏覽器與下載快取位於排除發行的 `.tools/browser-acceptance/`；不從暫存目錄或其他工作站引用套件。

測試自行選用 loopback 連接埠，在獨立暫存目錄建立 SQLite／身分密鑰，最後關閉瀏覽器、停止 Go 程序並清除該目錄。`--evidence` 只保存合成輸出與驗證摘要；不得將此工具改成讀取真實資料或營運中的資料目錄。

## 驗收內容及可追溯性

- 實際還原 500 筆檢驗、120 筆來源清單及 1 筆明細；確認 API 世代識別相符，原始民國日期、時間字串與就醫日期保留，匯入模式沒有冒稱官方核對。
- 在 320×900、390×900、844×390 及 1280×900，以 200% 應用字級檢查五個主畫面、頁面橫向溢位、實際提示訊息邊界與繁體中文字形。資料表／圖表可在自己的容器內捲動；主要操作區畫面與暫時提示訊息分開保存。
- 搜尋及類別篩選後，從第二頁進入明細並返回；確認篩選、頁碼、焦點及可見位置保留，上下分頁均可操作。日期標記具有即時回饋及明確的趨勢導覽。
- 500 個圖表量測保留同日同值紀錄，只有一個圖表 Tab 入口；方向鍵、Home、End、Enter 和完整紀錄分頁均使用原始觀測識別。
- 透過真正的瀏覽器下載驗證 JSON、UTF-8 BOM、XML、篩選 CSV 及完整資料包；標示的範圍與實際筆數一致，原始日期、中文字串、重複紀錄與已取得明細沒有遺失，資料包不包含目前工作階段權杖。
- 專用列印報表由資料模型產生，另驗證完整／指定範圍／日期對照的 HTML、實際 PDF、原始日期及文字完整性；不把目前頁碼或畫面 DOM 當作完整報表。指定分享範圍不得包含被排除的來源紀錄、筆數或完整資料集警告。直接呼叫實際應用頁面的瀏覽器 PDF 路徑，確認原生 `beforeprint` 在年度篩選仍為 2016 時產生全部 500 筆檢驗與 121 筆來源紀錄；分享視窗開啟時則遵守已指定的 135 筆範圍，列印結束後清除暫存文件並保留介面狀態。
- 使用真實本機 API 模擬另一分頁切換匯入階段，確認舊世代讀寫被拒絕且介面清除舊資料。另延遲一個未變造的真實回應，在其間再切換相同模式，確認晚到資料不能套入新階段；最後實際登出並核對清除結果。
- 三組獨立、明確標記的合成 API 案例補足無法以真實登入自動測試的介面分支。計數案例保留真實匯入回應的資料、權杖與設定，只覆寫顯示模式及「官方回報 1 筆、保留 2 筆、未完整」描述，驗證兩處介面文案一致；匯入模式文案另外以未覆寫的真實工作階段驗證。
- 分批案例僅覆寫所選檢驗／來源清單／巢狀明細與官方顯示模式，所有請求仍帶有真正本機工作階段權杖，禁止任何健保來源流量。確認按月區間依序且不重疊、僅納入可查日期來源、巢狀明細未完成不能前進、實際匯出後仍需確認已保存；第二批失敗或取消時不會匯出前批資料，成功的同批來源與未選來源保留，恢復時不重抓已完成資料，結束計畫後恢復一般操作。最後分別在分批與一般取得流程，用真正的本機登出使工作階段失效，確認晚到檢驗回應不能繼續拉取來源或恢復舊資料，也不會重播登出動作。
- 完整取得案例檢查全部期間的清單、巢狀明細、一次性產製要求、同來源明細的六輪有界更新，以及官方未提供工作關聯時持續保留「未確認」。回應遺失不重送產製；原始檔包建立回應遺失時，以原計畫核對，重新讀取來源前則清除舊包並使用新計畫，避免新資料雜湊誤配舊包。合成部分成功 ZIP 確認保存後，只下載尚未完成的原始檔，保留同一份資料 JSON、附件關聯及 SHA-256；另外測試容量明確拒絕後的獨立 JSON 保存確認，以及真正本機登出期間晚到的檢驗回應不能繼續取得來源。

完整取得案例的來源及套件控制 API 為明確合成回應。Chromium 原生下載請求不能依賴頁面攔截，因此測試啟動僅監聽 loopback 的暫時代理，只有原始 ZIP 下載路徑回傳小型合成檔案；其他未覆寫的 API 仍轉至真正 Go 程序。驗收實際保存 ZIP、核對下載位元組及索引雜湊，沒有在產品加入測試登入、測試端點或驗證繞過。正式套件路由的工作階段隔離、串流、取消、容量、逐檔失敗與 ZIP 結構由 Go 測試另外驗證。上述合成案例均不代表成功官方登入或實際來源回傳。

`summary.json` 記錄被測執行檔 SHA-256、內嵌 JavaScript/CSS 雜湊、實際工具版本、逐項結果及使用的合成範圍。CI 保留摘要、合成畫面及合成 PDF，期限七天；不保存 Cookie、權杖、網路追蹤、HAR、程序資料目錄或資料庫。Release 工作流程在解壓實際 amd64 發行包後執行此驗收，通過後才進入既有發布步驟。

這些證據涵蓋 Chromium/Linux 的合成流程。真實健保登入、官方個人資料與歷史範圍、iPhone Safari、系統密碼管理員、實體印表機與營運環境仍須依 `docs/verification.zh-TW.md` 的界限進行實機驗收。

## 原始檔保存的瀏覽器能力界限

完整取得使用同來源的一次性原生下載連結，把 ZIP 交由瀏覽器保存；前端不先把所有附件讀成一個巨大 Blob。這條路徑不依賴 `showSaveFilePicker()`：MDN 將該方法標為尚未廣泛支援且需要安全環境，Chrome 文件也將其支援範圍限定於支援的 Chromium 瀏覽器。原生 [`download`](https://developer.mozilla.org/en-US/docs/Web/API/HTMLAnchorElement/download) 連結則已廣泛支援，但瀏覽器可以調整檔名或依下載設定處理檔案。參考：[MDN showSaveFilePicker](https://developer.mozilla.org/en-US/docs/Web/API/Window/showSaveFilePicker)、[Chrome File System Access](https://developer.chrome.com/docs/capabilities/web-apis/file-system-access)、[HTML 下載處理規則](https://html.spec.whatwg.org/multipage/links.html#downloading-resources)。

伺服器完成 ZIP 及算出 SHA-256，只能證明傳輸端產生了該內容。介面要求使用者查看瀏覽器下載結果，再確認每一部分已保存；超過內嵌上限的完整 JSON 也有獨立保存確認。整組索引分開記錄下載包狀態、已知原始檔狀態、使用者保存確認，以及來源範圍／產製未確認狀態。測試工具可以對合成檔使用 `download.saveAs()` 及檔案系統核對；正式網頁不宣稱擁有這種自動確認裝置檔案的能力。

## RC10 整合驗收紀錄（2026-10-28）

本次在整合階段的 `v1.0.3-rc.10-qa` 編譯執行檔上執行已入庫的驗收程式，命令以退出碼 0 結束。正式發行工作流程仍須對其重新建置、解壓後的實際 amd64 發行包執行同一套驗收。

| 證據 | 實際結果 |
| --- | --- |
| 瀏覽器驗收 | 10 組通過；20 組 200% 畫面檢查；未捕獲例外 0；禁止來源請求 0 |
| 工作階段回歸 | 分批與一般取得的晚到檢驗回應均停止後續來源請求；沒有對新階段重播登出 |
| 清理 | 瀏覽器與合成 Go 程序已停止，測試用 SQL／密鑰目錄已刪除 |
| 實際測試工具 | Playwright 1.64.1；Chromium 156.0.8078.4；pdftotext 24.02.0；pdfinfo 26.05.0 |

| 實際 PDF 路徑 | 檢驗筆數 | 醫療來源筆數 | 頁數 |
| --- | ---: | ---: | ---: |
| 專用完整逐筆報表 | 500 | 121 | 73 |
| 專用完整日期對照 | 500 | 0 | 73 |
| 專用指定分享範圍 | 135 | 0 | 14 |
| 應用頁面原生完整列印 | 500 | 121 | 70 |
| 分享視窗開啟時原生列印 | 135 | 0 | 14 |

所有 PDF 均核對原始日期、數值及預期紀錄文字；指定範圍另確認沒有被排除的來源筆數、來源覆蓋說明或完整資料集警告。代表性首末頁、中間頁及醫療紀錄段落已轉成影像檢視，確認中文字形、跨頁表頭與內容可讀。日期對照儲存格跨頁時仍保留原日期，完整頁尾段落依實際內容分頁。

可重查的 SHA-256：

```text
QA executable  87cfc59d70d392f96c8369baf14042a402b3bba812f3c4c6673538f10a2c97ce
embedded app.js  3e2353601cbcdb854d68702c08dd8fe84840b5e71e44f5245fce625eeb51f41d
embedded app.css d44758a7693b64afc94458412c1eddfc95cb701ce2ca7532eb0741b25d6ab88e
summary.json    2d13b49fe2a90286045a1f6400aabbba356c7bda87b936d195b5ad2e687ce5a2
```
