// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package lab

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html"
)

const (
	maxImportBytes    = 12 << 20
	maxExpandedHTML   = 6 << 20
	maxArchiveEntries = 4096
	maxHTMLToken      = 1 << 20
	maxCellBytes      = 64 << 10
	maxImportedRows   = 50000
)

const savedPageWarning = "另存網頁只包含當時頁面已有的資料，可能僅目前分頁；不代表完整年度或全部歷史紀錄。"

var detailedHeaders = []string{
	"檢查日期", "醫令代碼", "檢驗檢查醫囑名稱", "檢驗檢查項目名稱", "檢查結果",
	"單位", "參考值", "醫事機構", "就醫日期/入院日期", "資料來源",
}

// ParseImport reads one saved official report in memory. A browser's companion
// resource ZIP is supported; its scripts, assets and patient banner are never
// executed, extracted, interpreted as records or returned to the application.
// Result/date text is retained for the normalizer. Repeated report rows remain
// distinct observations; only alternate summary/detail representations of the
// same table are excluded by selecting the ten-column detail header.
func ParseImport(filename string, data []byte) ([]RawRecord, []string, error) {
	if len(data) == 0 {
		return nil, nil, errors.New("匯入檔案是空的。")
	}
	if len(data) > maxImportBytes {
		return nil, nil, errors.New("匯入檔案不可超過 12 MiB。")
	}
	extension := strings.ToLower(path.Ext(filename))
	if extension == ".json" {
		return parsePortableJSON(data)
	}
	if extension == ".xml" {
		return parsePortableXML(data)
	}
	if extension == ".zip" || bytes.HasPrefix(data, []byte{'P', 'K', 3, 4}) || bytes.HasPrefix(data, []byte{'P', 'K', 5, 6}) {
		return parseArchive(data)
	}
	if extension != ".html" && extension != ".htm" {
		return nil, nil, errors.New("請上傳另存HTML／ZIP，或Healthbank匯出的JSON／XML可攜報表。")
	}
	if len(data) > maxExpandedHTML {
		return nil, nil, errors.New("HTML 網頁內容不可超過 6 MiB。")
	}
	document, err := parseSavedHTML(data)
	if err != nil {
		return nil, nil, err
	}
	if !document.found {
		return nil, nil, missingDetailError()
	}
	return document.records, importWarnings(document), nil
}

func parseArchive(data []byte) ([]RawRecord, []string, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, nil, errors.New("無法讀取 ZIP；請確認是未加密、完整的瀏覽器另存網頁壓縮檔。")
	}
	if len(reader.File) > maxArchiveEntries {
		return nil, nil, errors.New("ZIP 內的項目不可超過 4096 個。")
	}
	remaining := int64(maxExpandedHTML)
	var selected parsedDocument
	for _, file := range reader.File {
		extension := strings.ToLower(path.Ext(file.Name))
		if (extension != ".html" && extension != ".htm") || !file.Mode().IsRegular() {
			continue
		}
		if file.Flags&1 != 0 {
			return nil, nil, errors.New("無法匯入加密的 HTML；請提供未加密的另存網頁。")
		}
		if file.UncompressedSize64 > uint64(remaining) {
			return nil, nil, errors.New("ZIP 內展開後的 HTML 總量不可超過 6 MiB。")
		}
		stream, err := file.Open()
		if err != nil {
			return nil, nil, errors.New("ZIP 中的 HTML 無法讀取，請重新另存網頁。")
		}
		body, readErr := io.ReadAll(io.LimitReader(stream, remaining+1))
		closeErr := stream.Close()
		if readErr != nil || closeErr != nil {
			return nil, nil, errors.New("ZIP 中的 HTML 已損壞或不完整，請重新另存網頁。")
		}
		if int64(len(body)) > remaining {
			return nil, nil, errors.New("ZIP 內展開後的 HTML 總量不可超過 6 MiB。")
		}
		remaining -= int64(len(body))
		document, parseErr := parseSavedHTML(body)
		clear(body)
		if parseErr != nil {
			return nil, nil, parseErr
		}
		if !document.found {
			continue
		}
		if selected.found {
			return nil, nil, errors.New("ZIP 內包含多份檢驗明細網頁；請分別匯入，以免混合不同人的資料或重複頁面。")
		}
		selected = document
	}
	if !selected.found {
		return nil, nil, missingDetailError()
	}
	return selected.records, importWarnings(selected), nil
}

type parsedDocument struct {
	records         []RawRecord
	found           bool
	additionalTable bool
}

func importWarnings(document parsedDocument) []string {
	warnings := []string{savedPageWarning}
	if document.additionalTable {
		warnings = append(warnings, "同一網頁包含多個明細表，只讀取第一個符合格式的完整十欄表格。")
	}
	if len(document.records) == 0 {
		warnings = append(warnings, "這份另存網頁的檢驗明細表沒有結果；未建立任何數值紀錄。")
	}
	return warnings
}

func missingDetailError() error {
	return errors.New("找不到「其他檢驗資料」的完整十欄明細表。請在官方頁面切換詳細資料後另存網頁；三欄摘要無法提供單位與參考值。")
}

type importCell struct {
	text string
	flag bool
}

type importTable struct {
	columns  map[string]int
	records  []RawRecord
	row      []importCell
	rowOpen  bool
	cellOpen bool
	cellText strings.Builder
	cellFlag bool
}

func (table *importTable) appendText(value string) error {
	if !table.cellOpen {
		return nil
	}
	if table.cellText.Len()+len(value) > maxCellBytes {
		return errors.New("明細表中的單一欄位過長，請確認另存網頁內容。")
	}
	table.cellText.WriteString(value)
	return nil
}

func (table *importTable) finishCell() {
	if !table.cellOpen {
		return
	}
	table.row = append(table.row, importCell{text: strings.Join(strings.Fields(table.cellText.String()), " "), flag: table.cellFlag})
	table.cellOpen, table.cellFlag = false, false
	table.cellText.Reset()
}

func (table *importTable) finishRow() error {
	if !table.rowOpen {
		return nil
	}
	table.finishCell()
	table.rowOpen = false
	defer func() { table.row = nil }()
	if columns := detailColumns(table.row); columns != nil {
		if table.columns == nil {
			table.columns = columns
			table.records = []RawRecord{}
		}
		return nil // Repeated header rows are display headers, not results.
	}
	if table.columns == nil || len(table.row) == 0 {
		return nil
	}
	if len(table.row) == 1 && noDataText(table.row[0].text) {
		return nil
	}
	allEmpty := true
	for _, cell := range table.row {
		if cell.text != "" {
			allEmpty = false
			break
		}
	}
	if allEmpty {
		return nil
	}
	if len(table.row) != len(detailedHeaders) {
		return errors.New("明細資料有缺少欄位或結構不完整的資料列；未匯入部分結果。")
	}
	get := func(header string) string { return table.row[table.columns[header]].text }
	if get("檢驗檢查項目名稱") == "" {
		return errors.New("明細資料缺少檢驗項目名稱，請確認另存的是完整詳細資料表格。")
	}
	if len(table.records) >= maxImportedRows {
		return errors.New("匯入的檢驗資料列超過上限，請拆分檔案。")
	}
	row := RawRecord{
		CheckDate: get("檢查日期"), OrderCode: get("醫令代碼"), OrderName: get("檢驗檢查醫囑名稱"),
		ItemName: get("檢驗檢查項目名稱"), Result: get("檢查結果"), Unit: get("單位"), Reference: get("參考值"),
		Institution: get("醫事機構"), VisitDate: get("就醫日期/入院日期"), Source: get("資料來源"),
	}
	if table.row[table.columns["檢查結果"]].flag {
		row.OfficialFlag = "1"
	}
	table.records = append(table.records, row)
	return nil
}

func detailColumns(cells []importCell) map[string]int {
	if len(cells) != len(detailedHeaders) {
		return nil
	}
	columns := make(map[string]int, len(detailedHeaders))
	for i, cell := range cells {
		key := headerKey(cell.text)
		if _, duplicate := columns[key]; duplicate {
			return nil
		}
		columns[key] = i
	}
	for _, required := range detailedHeaders {
		if _, ok := columns[required]; !ok {
			return nil
		}
	}
	return columns
}

func headerKey(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '\ufeff' || r == '\u200b' {
			return -1
		}
		if r == '／' {
			return '/'
		}
		return r
	}, value)
}

func noDataText(value string) bool {
	switch strings.ToLower(headerKey(value)) {
	case "無資料", "查無資料", "目前無資料", "沒有資料", "nodata", "noresults":
		return true
	}
	return false
}

func blockedHTMLTag(tag string) bool {
	switch tag {
	case "script", "style", "template", "noscript", "iframe", "object":
		return true
	}
	return false
}

func blockSpacingTag(tag string) bool {
	switch tag {
	case "p", "div", "br", "li":
		return true
	}
	return false
}

func hasOfficialFlag(attributes []html.Attribute) bool {
	for _, attribute := range attributes {
		if attribute.Key == "class" {
			for _, name := range strings.Fields(attribute.Val) {
				if name == "sign-red" {
					return true
				}
			}
		}
	}
	return false
}

// parseSavedHTML uses the tokenizer rather than building a document tree.
// This bounds allocations and treats all uploaded code as inert data.
func parseSavedHTML(data []byte) (parsedDocument, error) {
	if !utf8.Valid(data) {
		return parsedDocument{}, errors.New("HTML 不是有效的 UTF-8 網頁，請用瀏覽器重新另存。")
	}
	tokenizer := html.NewTokenizer(bytes.NewReader(data))
	tokenizer.SetMaxBuf(maxHTMLToken)
	var document parsedDocument
	var table *importTable
	tableDepth := 0
	blockedTag, blockedDepth := "", 0
	finishTable := func() error {
		if table == nil {
			return nil
		}
		if err := table.finishRow(); err != nil {
			return err
		}
		if table.columns != nil {
			if document.found {
				document.additionalTable = true
			} else {
				document.found, document.records = true, table.records
			}
		}
		table = nil
		return nil
	}
	for {
		tokenType := tokenizer.Next()
		if tokenType == html.ErrorToken {
			if tokenizer.Err() != io.EOF {
				return parsedDocument{}, errors.New("HTML 內容過長或結構無法讀取，請重新另存官方詳細資料頁面。")
			}
			if err := finishTable(); err != nil {
				return parsedDocument{}, err
			}
			return document, nil
		}
		if tokenType == html.TextToken {
			if blockedDepth == 0 && tableDepth == 1 && table != nil {
				if err := table.appendText(string(tokenizer.Text())); err != nil {
					return parsedDocument{}, err
				}
			}
			continue
		}
		if tokenType != html.StartTagToken && tokenType != html.SelfClosingTagToken && tokenType != html.EndTagToken {
			continue
		}
		token := tokenizer.Token()
		start := tokenType != html.EndTagToken
		if blockedDepth > 0 {
			if token.Data == blockedTag {
				if start {
					blockedDepth++
				} else if !start {
					blockedDepth--
				}
			}
			continue
		}
		if start && blockedHTMLTag(token.Data) {
			// In HTML, the self-closing marker does not close these non-void
			// elements. Treat <script/> like <script>, as a browser would.
			blockedTag, blockedDepth = token.Data, 1
			continue
		}
		if token.Data == "table" {
			if start {
				if tableDepth == 0 {
					table = &importTable{}
				}
				tableDepth++
			} else if tableDepth > 0 {
				tableDepth--
				if tableDepth == 0 {
					if err := finishTable(); err != nil {
						return parsedDocument{}, err
					}
				}
			}
			continue
		}
		if table == nil || tableDepth != 1 {
			continue
		}
		switch token.Data {
		case "tr":
			if err := table.finishRow(); err != nil {
				return parsedDocument{}, err
			}
			table.rowOpen = start
		case "td", "th":
			table.finishCell()
			if start {
				table.rowOpen, table.cellOpen = true, true
				table.cellFlag = hasOfficialFlag(token.Attr)
			}
		default:
			if start && table.cellOpen && hasOfficialFlag(token.Attr) {
				table.cellFlag = true
			}
			if blockSpacingTag(token.Data) {
				if err := table.appendText(" "); err != nil {
					return parsedDocument{}, err
				}
			} else if start && token.Data == "sup" {
				if err := table.appendText("^"); err != nil {
					return parsedDocument{}, err
				}
			}
		}
	}
}
