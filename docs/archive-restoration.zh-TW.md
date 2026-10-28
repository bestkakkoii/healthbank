# 分批資料與原始檔 ZIP 還原

Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT.

本文件說明裝置端保存檔案的開啟與校驗。官方取得流程、產製限制及串流下載契約見 [資料工作區](data-workspace.zh-TW.md)。檔案內容由使用者保管；SHA-256 能檢查內容是否符合所附描述，不能證明檔案來自官方或屬於某個人。

## 按年／按月的保存索引

「按年／按月分批取得與保存」仍逐批產生獨立的 `healthbank.dataset.v2` JSON。下載各批後，可另外下載 `healthbank-batches-<collection_id>-index.json`。計畫結束後，索引保留至該頁面／工作階段被清除或下一個分批計畫取代為止；請與原始資料 JSON 一起保存。

索引使用 `healthbank.batch.collection.v1`：

| 欄位 | 內容 |
| --- | --- |
| `collection_id`、`created_at` | 本次裝置端計畫識別與索引建立時間，不是官方身分 |
| `query`、`period` | 使用者指定的完整期間與 `year`／`month` 分批方式 |
| `categories`、`include_laboratory` | 本計畫的來源選擇與是否納入本批檢驗 |
| `batches[].index`、`query` | 全部預定批次的順序與精確起迄日；尚未匯出批次也保留 |
| `batches[].dataset` | 該批上次匯出 JSON 的檔名、精確位元組長度、SHA-256、匯出時間與使用者保存確認 |
| `dataset.acquisition` | 該批來源／檢驗筆數、未完成清單與明細、來源完整性及本批可讀取項目是否已處理 |

索引最多 2 MiB／600 批，期間必須是完整、連續、不重疊的原計畫切分。每個 JSON 仍受既有 128 MiB 資料包上限約束。重新匯出同一批會更新該批描述；應重新保存索引。

### 開啟方式

1. 在「保存／還原資料包 → 還原分批資料與原始檔 ZIP」選擇分批索引。
2. 複選各批 JSON。唯一的相符檔名與長度只用來協助配對；同名候選不會任意擇一。
3. 檔案改名時，可以在該批列上明確指定 JSON。
4. 點選「開啟此批資料」。程式核對位元組長度、SHA-256、v2 格式、查詢期間、匯出時間與來源／完整性摘要後，才以獨立匯入模式開啟。
5. 切換批次保留已選的本機 File 參照，逐批重新校驗；不需一次讀入所有批次。

各批是獨立快照。同一個 `archive_1` 在不同批次可能代表完全不同的明細或附件，不能跨批當成同一筆。程式不以日期、藥名、院所或相似數值去重，也不將不同查詢的完整性資訊拼成「終身資料完整」。每次開啟只替換目前工作區快照；先前批次由原始 JSON 保存。

## 原始檔 ZIP 的直接還原

使用者可以複選全面取得流程下載的 ZIP，也可以稍後加入其他部分及補取包。讀取器只支援 Healthbank 產生的 `healthbank.acquisition.package.v1` 契約，並要求：

- 每個 ZIP 含 `README.txt` 與最後一個成員 `manifest.json`。
- `dataset.included=true` 時必須含 `healthbank.dataset.v2.json`，長度及 SHA-256 與 manifest 一致。
- `dataset.included=false` 時必須另外選擇原始完整 JSON；可改名，仍以精確長度與 SHA-256 核對。
- 原始檔路徑為 `originals/<archive_id>/<安全檔名>`，必須逐一對應 manifest 與同份 v2 JSON 內的來源類別／附件關聯。
- 不同部分與補取包只能在完整 JSON 的位元組長度及 SHA-256 完全相同時一起處理。不同快照須分別開啟。

同一計畫的同一部分不能重複加入。跨計畫的補取包可以提供原先失敗的檔案；若相同附件識別卻提供不同 SHA-256、長度或 MIME，程式標示內容衝突，不自行選擇「較新」版本。相同描述的多份副本會分別驗證；若某份損壞而另一份通過，只採用通過的副本，損壞副本仍列為失敗。

### 大檔與記憶體界限

ZIP 以 `File.slice()` 讀取尾端、中央目錄、成員標頭與指定資料範圍。支援超過 4 GiB 的 ZIP64 目錄偏移；不呼叫整個 ZIP 的 `arrayBuffer()`。原始附件使用 Store 成員，以最多 1 MiB 的切片逐段計算 SHA-256／CRC32；metadata 的 Deflate 成員使用原生 `DecompressionStream('deflate-raw')`，輸出受宣告長度與各項上限約束。

| 項目 | 還原界限 |
| --- | --- |
| 單一 ZIP | 9 GiB 結構上限；來源原始內容沿用每包 8 GiB 的取得限制 |
| 單一原始檔 | 500 MiB |
| 單一 manifest／中央目錄 | 各 4 MiB |
| 同次選取 | 最多 1,000 個 ZIP、manifest 合計 32 MiB、最多 150,000 個取得結果（包括補取副本） |
| 不同原始附件關聯 | 最多 50,000 個 |
| 工作區內保留的附件 | 合計 32 MiB 且最多 10,000 個，與既有完整資料包一致 |

超過附件保留量的成功檔案標為「已驗證；保留在 ZIP，未加入記憶體」。使用者仍可按「另存此原始檔」，從原始 ZIP 的 Blob 範圍交給瀏覽器下載，不建立另一份整包記憶體副本。頁面無法證明瀏覽器最後把檔案寫入磁碟，使用者仍須確認下載結果。

讀取器拒絕重複或不安全路徑、符號連結／非一般檔案、加密、多磁碟 ZIP、未知壓縮方法、不相符的本機與中央標頭、不完整資料描述符、重疊或越界範圍、超限輸出及損壞校驗。ZIP 不會自行解壓縮寫入指定路徑；原始 HTML／腳本也不會被執行。建議領藥日的 ICS 檔只作為原始檔保存，不會自動匯入行事曆或代表已實際領藥。

不支援原生 `deflate-raw` 的瀏覽器會明確顯示限制。使用者可用裝置的解壓縮工具取出完整 JSON，再走既有 JSON 還原功能；此情況不宣稱應用程式已驗證或恢復 ZIP 內全部附件。

### 逐檔狀態與接續

| 狀態 | 意義 |
| --- | --- |
| `not_checked` | 尚未完成本次校驗；停止或中斷不算成功 |
| `loaded` | 採用副本已通過 SHA-256、CRC32 及內容格式檢查，並保留在本次還原記憶體中 |
| `verified_external` | 採用副本通過校驗，留在使用者選取的 ZIP，可直接另存 |
| `source_unusable` | manifest 說明來源取得失敗或未完成；即使 ZIP 有部分位元組也不當作可用檔案 |
| `missing_part` | JSON 有附件描述，但已選的 ZIP 尚未提供對應取得結果 |
| `conflict` | 相同附件關聯出現不同內容或不同型別，不選邊合併 |
| `failed` | 沒有可採用的校驗成功副本 |

「停止檔案讀取」保留已完成校驗及有界附件結果，之後可接續；未完成成員會從該檔開頭重新校驗。這是本頁面的讀取進度，不是伺服器排程，也不保證關閉分頁後保留。清除所選檔案會釋放本次 File 參照；目前已開啟的工作區資料不因此宣稱已從裝置其他下載位置刪除。切換無關工作階段／帳號時會清除選取上下文，舊的非同步讀取不能安裝到新使用者工作區。

還原時保留原始來源完整性、查詢日期與取得限制。`completed` 只表示該包所列檔案當時傳輸完成；不能推導所有官方年度、其他未選來源或無法關聯的產製工作都已取得。這個讀取器逐成員校驗 metadata 與原始檔；另外匯出的 acquisition collection 索引中的整包 SHA-256 仍是獨立的整檔校驗描述，不以成員校驗冒充整包雜湊已比對。

## 實作依據與可重現驗證

- [PKWARE APPNOTE 6.3.10](https://pkware.cachefly.net/webdocs/casestudies/APPNOTE.TXT)：ZIP／ZIP64 中央目錄、資料描述符及成員格式。
- [NIST FIPS 180-4](https://nvlpubs.nist.gov/nistpubs/FIPS/NIST.FIPS.180-4.pdf)：增量 SHA-256；僅用於本機檔案完整性，不用於登入或金鑰保存。
- [MDN SubtleCrypto.digest](https://developer.mozilla.org/en-US/docs/Web/API/SubtleCrypto/digest)：介面不支援串流輸入，所以大型原始檔改用受測的增量計算。
- [WHATWG Compression Streams](https://compression.spec.whatwg.org/) 與 [MDN DecompressionStream](https://developer.mozilla.org/en-US/docs/Web/API/DecompressionStream/DecompressionStream)：原生 Deflate 解壓縮及平台能力界限。

聚焦測試檔：`archive-integrity.test.mjs`、`archive-zip.test.mjs`、`batch-collection.test.mjs`、`acquisition-restore.test.mjs`，以及實際編譯版瀏覽器情境 `browser/collection-restore-acceptance.mjs`。全部資料為合成資料；包括超過 4 GiB 的稀疏 ZIP64 偏移、超過 32 MiB 的原始檔、批次識別隔離、取消後接續、損壞內容與補取合併。真實 iPhone／官方帳號資料不是這些合成測試的證據。
