# 官方文件與原始檔連接契約

Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT.

查核日期：**2026-10-28（臺灣時間）**。本模組根據健保署網站公開前端程式的實際請求實作，
不是健保署承諾支援的公開 API。公開程式與合成測試可驗證請求、解析及隔離，不能證明個別真實帳號的文件可用性。
Healthbank 使用官方授權工作階段取得資料，以自己的資料工作區呈現；不複製官方行政、通知或表單管理介面。

## 應用 API

| 方法與路徑 | 輸入 | 回傳與用途 |
| --- | --- | --- |
| `GET /api/data/catalog` | 無 | `{categories:[{id,label,group,range_mode,has_details}]}`；公開的程式能力目錄，不是個人的資料清單，不建立登入工作階段。 |
| `GET /api/supplementary` | `category`；可選 `detail` | 類別清單或明細；影像、官方檔案及產製範圍使用下表所列類別。 |
| `GET /api/documents` | `category`、`id` | 已取得明細所列附件；`Content-Disposition: attachment`。不接受上游 URL。`HEAD` 不會啟動文件取得。 |
| `POST /api/documents/prepare` | JSON `{category,id}`，同源及 `X-CSRF-Token` | 明確要求官方產製文件；成功為 HTTP 202、`{status:"requested",message:"…"}`，不是產製完成。 |

除能力目錄外，均須已完成官方身分驗證。匯入資料或示範資料不能授權存取官方文件。
全部 API 繼承 `Cache-Control: no-store, private`、禁止代理緩衝及不嗅探內容類型標頭。

清單、明細及附件識別使用隨機 24 bytes 的 48 位十六進位字串，綁定當次連線、類別及父項目。
重新成功取得清單會使舊明細與文件識別失效；失敗刷新不會替換原成功結果。登出、重新驗證或官方逾時會清除識別。
產製操作的識別只使用一次；即使請求因網路錯誤無法確認是否送達，也不自動重送。重新讀取官方狀態後才能再次提出。

附件欄位：

```json
{
  "attachments": [{"id":"<當次文件識別>","name":"官方附件.pdf","content_type":"application/pdf"}],
  "document_actions": [{"id":"<當次操作識別>","name":"請官方產製原始檔","kind":"prepare_export"}]
}
```

`name` 是可直接下載的安全檔名，保留 `.pdf`、`.zip` 或 `.txt`；移除路徑及跨平台不允許的字元。
`size` 只有可確認時才提供。`kind` 為 `prepare_jpeg`、`prepare_dicom` 或 `prepare_export`。
取得清單、批次取得明細及匯出 Healthbank 資料包都不會自動執行這些產製操作。

## 資料與上游流程

下列上游路徑均固定在 `https://myhealthbank.nhi.gov.tw`，不能由瀏覽器替換。

| 應用類別 | 已觀察上游契約 | 實際行為 |
| --- | --- | --- |
| `imaging` | `ihke3408s01/page_load` 後接 `search`；`IHKE3408S02/page_load?crid&ctype` | 以全部來源 `s_type=` 讀取清單，再取得報告原文。清單沒有可自行指定的日期搜尋或獨立總數。 |
| `imaging` JPEG | `IHKE3408S03/page_load?IPL_CASE_SEQ_NO`，回傳 `pics[]` | 僅在來源狀態可讀取時提供附件，驗證 JPEG 編碼與尺寸，按原始順序打包；不重新壓縮或變更影像。 |
| `imaging` 產製 | POST `IHKE3408S02/add` | JPEG 使用來源 `rownum`；DICOM 使用來源 `ipl_CASE_SEQ_NO`，兩者送入已觀察的同名請求欄位。 |
| `osteoporosis` | `IHKE3411S02/page_load`；POST `IHKE3411S02/add`；`IHKE3411S03/page_load` | 骨密度報告沿用自己的來源端點及狀態，不能套用一般影像類別的識別。 |
| `hospital` 病摘 | `IHKE3309S02/getxml?crid&ctype`，`file_name` | 官方 `showXML` 將該字串直接作為 HTML 顯示。Healthbank 保留原顯示內容並以 `.txt` 下載，不執行標記，也不稱為原始 XML。 |
| `hospital` PDF | `IHKE3309S02/getpdf` → 官方回傳 PDF 位址 → `IHKE3309S02/delpdf` | 取得原 PDF 後要求清理此次上游暫存。只有相同官方來源、限定目錄內的 PDF 位址可讀取；不傳 JWT 給靜態 PDF，禁止重新導向。 |
| `e_prescriptions` | `ihke3212s01/IHKE3212_pdf?tid&tid2&recipeType` | 兩個 `tid` 均來自已驗證清單的同一處方識別。此方法存在於公開元件，但目前元件畫面未呼叫該按鈕；真實帳號可用性另須驗收。 |
| `official_files` | `ihke3801s01/page_load`；`ihke3801s02/page_load`；`app_download?gid` 或 `download?seq_no` | 既有原始檔產製工作及檔案，只有來源狀態 `1` 才建立下載識別；有 `did` 時綁定來源 `gid` 路徑，否則使用原序號路徑。 |
| `official_files` DICOM | `ihke3801s02/imgF_page_load`；`app_imgF_download?gid` 或 `imgF_download?seq_no` | 下載官方準備的原始影像 ZIP；不在 Healthbank 伺服器解壓或進行診斷閱片。 |
| `official_export_html` | `ihke3802s01/page_load`；POST `add` 的 `row_id` | 每類選用官方提供的全部範圍；沒有全部選項時使用來源列出的各月份，保留可產製範圍供使用者檢視。 |
| `official_export_xml` | `ihke3803s01/page_load`；POST `add` 的 `dat` | 精確保留官方提供的產製參數，不自行推測格式或年度選項。 |
| `official_export_json` | `ihke3804s01/page_load`；POST `add` 的 `dat` | 同上；這是官方原始格式，不等同 Healthbank 自己的可攜資料包。 |
| `official_export_pdf` | `ihke3805s01/page_load`；POST `add` 的 `row_ID` | 每個操作只對應官方回傳的報告選項，不接受任意來源識別。 |

表中省略共同 API 前綴 `/api/ihke3000/`。詳細來源欄名以實作中的明確白名單為準；不轉送整個未知 JSON 物件。

影像清單的公開程式只有初始 `page_load` 會把可選路由年度 `query.y` 傳為 `c_year`；
兩個 `search` 呼叫都只有 `s_type`、`s_sort`。Healthbank 一律接續全部來源搜尋，沒有把單一年度捷徑當成完整清單。
這仍不能證明官方保有全部終身歷史。

官方原始檔清單標示最近 **7 天的產製紀錄**；這不是醫療資料保存年限。
產製通常不是即時完成。重新取得明細或原始檔清單即可更新狀態，程式不無限輪詢、重新產製或取消已提交的官方工作。
官方原始壓縮檔可能有密碼保護；下載保留原始位元組，不自動解密，也不保存解壓密碼。

## 下載與隱私界線

| 項目 | 應用上限／處理 |
| --- | --- |
| 一般上游 JSON | 16 MiB；原清單及明細筆數界限仍適用。 |
| JPEG／病摘來源 JSON | 48 MiB 的編碼回應上限。 |
| 單份緩衝 PDF 內容／JPEG 解碼後原始資料總量 | 32 MiB；JPEG 至多 500 張，每張尺寸及編碼須可辨識。超限會拒絕，不回傳部分影像組。 |
| 官方原始 ZIP／PDF／DICOM | 串流上限 500 MiB、整次最多 5 分鐘；不以整份大型檔案占用記憶體。 |
| 文件及產製識別 | 當次連線合計最多 50,000 個，列表更新及登入重設清除。 |
| 原始檔產製參數 | 每次產製選項回應合計最多 1 MiB，僅留於工作階段記憶體，使用或失效時清除引用。 |

上述為內容與傳輸上限，不是處理程序記憶體峰值上限；編碼 JSON、解碼字串、ZIP 中繼資料與緩衝仍會占用額外記憶體。

下載檢查實際 PDF／ZIP／JPEG 標頭、可核對的傳輸長度與配置上限；JPEG 另驗證編碼與尺寸。來源 HTML／JSON 錯誤頁不會冒充原始檔，HTTP 200 且 Content-Type 為 JSON 的有界回應若明示官方逾時，同樣傳遞登入失效。錯誤 MIME 的非文件內容拒絕為文件格式錯誤，不因此推斷登入狀態。這些檢查不等於完整 PDF／ZIP 內部結構驗證；上游若以正常結束的傳輸回傳內部損壞內容，尤其沒有可核對長度或屬加密原始檔時，不能僅憑傳輸成功證明檔案語意正確。
串流途中遇到取消、大小超限或內容截斷，會中止 HTTP 回應，不能把部分內容標為下載成功。
PDF 的回傳位址額外拒絕跨來源、目錄穿越、編碼反斜線、控制字元及再次百分比解碼路徑。

官方 PDF 需要建立其自己的暫存檔。程式會執行一次觀察到的 `delpdf` 清理請求；
公開程式沒有提供可核對的正向清理回應契約，所以 HTTP 200 也不被宣稱為已證明刪除。
PDF 下載以 `X-Healthbank-Document-Warning: official_temporary_cleanup_unconfirmed` 表示此限制。
清理回應若明示登入失效，會在交付緩衝 PDF 前回報 `SESSION_EXPIRED`；失敗下載的延後清理也會傳遞此失效。一般清理故障保留原下載錯誤或未確認警告，不吞掉失效，也不虛報已刪除。
應用的記憶體與不落地處理，不等於能保證官方系統、代理、主機交換空間或備份的行為。

Healthbank 不寫入醫療文件、官方 Cookie／權杖、產製參數或下載歷史至 SQL、應用日誌或伺服器檔案。
使用者明確下載到自己的裝置後，由使用者管理。原始醫療文件及第三方內容不因本專案的 MIT 授權而改變授權。

## 驗證範圍

合成 TLS 夾具驗證原文及 JPEG 位元組保留、所有附件類別的端點、官方產製參數、
不自動產製、不可重播操作、過期及跨類別識別拒絕、非法來源位址拒絕、
檔案類型／上限／截斷／取消／重新導向、暫存清理嘗試與下載錯誤碼。
伺服器案例驗證身分門檻、CSRF、禁止任意 URL、隱私標頭及不新增 SQL 設定。

仍須真實環境驗收：帳號實際文件／年度範圍、上游產製可用性與保留時間、
官方 PDF 位址的實際格式、原始加密壓縮檔內容及裝置下載行為。
這些未知不會被合成測試標為通過，也不阻止通過獨立驗證的版本發行。

## 公開來源雜湊

下列為本輪實際讀取的公開程式，沒有以真實登入資料取得私人 API 回應。
公開資產存在不代表每個帳號目前都有相同權限或相同資料。

| 官方來源 | SHA-256 |
| --- | --- |
| [IHKE3408S01.0daa885f.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3408S01.0daa885f.js) | `aedb9fff65514ddde8d10e4020e64d81d8568169dc8f204736103da7d2946d3e` |
| [IHKE3408S02.a93b9a4e.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3408S02.a93b9a4e.js) | `c0ceac7a19b4e0c5107df11d7077d93deffbcc8f2d30a48e7673e8c5672128b5` |
| [IHKE3408S03.7a152af9.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3408S03.7a152af9.js) | `e7e085f99cf14893855dc113232a454a13205f7f058a287158e06cda9c46c69c` |
| [IHKE3411S02.cc4529c5.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3411S02.cc4529c5.js) | `d70a6a67b2ce1c2ab3ad0df6fc54d4e211c100a910207febfb02d09c7118861c` |
| [IHKE3411S03.4e01b861.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3411S03.4e01b861.js) | `7bdfa6d474b001f3d81c9c224361adc938ca5afcb538fce9cc85dd477f70068e` |
| [IHKE3212S01.647a1fc8.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3212S01.647a1fc8.js) | `4a31db2e715245c2db4152461bfd49148518be60ee3516d1e508059e5efb152d` |
| [showXML.90ff2238.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/showXML.90ff2238.js) | `c9a968efec8f6a3c10c7f00042c1f66f77eac9749bd3e911359d1ed15b182353` |
| [showPDF.19095ca4.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/showPDF.19095ca4.js) | `9f024e5218b5cb5ae40ec6c7f09c14de231bebe88075cf9eb96f93f94945379e` |
| [IHKE3801S01.0332b698.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3801S01.0332b698.js) | `ed38d7325cf6f91c24a253254de8cad50696cbd7a60388d3072fea5eba39c7ab` |
| [IHKE3801S02.5603c61d.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3801S02.5603c61d.js) | `ca0f399f821487bfc8b049de8dc7e8e8183d0e185f82791b3ad48a6089eb0a49` |
| [IHKE3802S01.a1e95a48.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3802S01.a1e95a48.js) | `77a6a2bce43b7427ea6c04bb2ad236c88b3489249a0f78d5535dbd4d24b9f8d8` |
| [IHKE3803S01.61a663da.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3803S01.61a663da.js) | `5f5e55c72ff0638887b5ae1e51477456f770bc19d70ac466e5e962a2cfc7cdac` |
| [IHKE3804S01.7191a746.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3804S01.7191a746.js) | `33754793788641305604f91eee9e28f9214c00285f4444fefcc6921b6e2864b4` |
| [IHKE3805S01.5f157684.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3805S01.5f157684.js) | `d2695c2d381b90fe72db2def9ede091603079971b71825d4862f5e1203aa1ea8` |
| [IHKE3807S01.f0ab8096.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3807S01.f0ab8096.js) | `ba073173be6a658d4652536db90a1fd895a450a6451ce2bb139ac5870b388b77` |
| [pdfviewer.html](https://myhealthbank.nhi.gov.tw/ihke3000/ViewPdf/pdfviewer.html) | `18a3e2c5b1aeb80effdb73839ae6c8ea396e161bc44c90282aedf2c5bf07a875` |
| [pdf-viewer.js](https://myhealthbank.nhi.gov.tw/ihke3000/ViewPdf/viewer.js) | `cf6781805e4bdb91548366163361b4f618ef9b665f24d46f2510f9a7dc9e81ba` |
| [IHKE3309S02.deb370e8.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3309S02.deb370e8.js) | `f09984c1c3f46a0dc3eecf58163f95718295abd0af7d6fea74665b3df51047d0` |

RC9 額外重新取得並核對的下載元件：

| 官方來源 | SHA-256 |
| --- | --- |
| [IHKE3800download.87e94a32.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3800download.87e94a32.js) | `7073bc1ab99b3b92c1351bc6b90944d38e3e209fe7d3b12345beedfad22bd7dd` |
| [IHKE3800download_IMG.df47eafd.js](https://myhealthbank.nhi.gov.tw/IHKE3000/js/IHKE3800download_IMG.df47eafd.js) | `8fb68daedc85efb4736c9a093de6d29547586f307e89e8725653af4c4983a4ff` |
