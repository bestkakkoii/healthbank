# 資料保護與問題回報

Copyright © 2026 bestkakkoii llc. SPDX-License-Identifier: MIT.

Healthbank 是自架的檢驗資料閱讀工具。設定資料庫不保存檢驗結果，但登入資料、官方工作階段與報告仍會經過服務記憶體；「只保存設定」不代表沒有需要保護的資料。

## 部署與備份

- 對外使用 HTTPS，將 Go 服務保留在本機監聽位址，由此 APP 的反向代理轉送。
- 執行帳號只需程式與資料目錄的必要權限；不要將 `data/` 設為可公開下載的靜態目錄。
- 設定備份須一起保留資料庫與 `data/identity.key`。假名化身分對應仍可連結到個人設定，不宣稱完全匿名。
- 保護 MySQL DSN、設定匯出與主機備份。應用不保存的資料，仍可能被外部代理、主機日誌或備份工具額外記錄，部署者須核對自己的設定。

應用採固定官方 HTTPS 來源、各使用階段獨立 Cookie、手動驗證碼、同源／CSRF 寫入檢查與請求大小限制；匯入網頁只解析資料，不執行其中腳本。這些措施的實際驗證範圍見[驗證紀錄](docs/verification.zh-TW.md)。

## 一般錯誤回報

可在 [Issues](https://github.com/bestkakkoii/healthbank/issues) 提供：

- `healthbank --version` 顯示的版本與提交。
- 作業系統／CPU 架構、瀏覽器、相關部署方式。
- 重現步驟、預期與實際行為。
- 經檢查的錯誤代碼、查詢代碼、HTTP 狀態或 `diagnose-nhi` 診斷結果。

**不要上傳證號、健保卡號、密碼、Cookie／權杖、真實檢驗報告、設定資料庫、金鑰或未檢查的網路封包紀錄。** 畫面截圖與日誌也要先確認沒有這些內容。需要重現資料時，使用合成範例。

## 疑似安全漏洞

不要在公開 Issue 揭露可利用的敏感細節或真實使用者資料。若尚無維護者明確提供的私人管道，可先提出不含漏洞細節與敏感資訊的聯絡請求，待維護者確認管道後再提供必要內容。

私人聯絡管道與回應安排以維護者確認的內容為準。版本修正與已知限制以 [Releases](https://github.com/bestkakkoii/healthbank/releases)、[CHANGELOG](CHANGELOG.md) 及驗證紀錄為準。
