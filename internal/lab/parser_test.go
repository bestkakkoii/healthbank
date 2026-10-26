// Copyright (c) 2026 bestkakkoi llc. SPDX-License-Identifier: MIT

package lab

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"testing"
)

const syntheticHeader = `<tr><th>檢查日期</th><th>醫令代碼</th><th>檢驗檢查醫囑名稱</th><th>檢驗檢查項目名稱</th><th>檢查結果</th><th>單位</th><th>參考值</th><th>醫事機構</th><th>就醫日期／入院日期</th><th>資料來源</th></tr>`

const syntheticDetailRow = `<tr><td>115/01/20</td><td>TEST01</td><td>合成檢驗組合</td><td>合成測試項目</td><td class="text-end sign-red"><p>&gt;=30</p><script>this_is_not_report_text()</script></td><td>x10<sup>3</sup>/uL</td><td>[1][10]</td><td><a href="https://example.invalid/">合成測試院所</a></td><td>114/12/30</td><td>A</td></tr>`

func syntheticTable(rows string) string {
	return "<table><thead>" + syntheticHeader + "</thead><tbody>" + rows + "</tbody></table>"
}

func syntheticArchive(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range entries {
		file, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestSavedDetailOnlyPreservesDuplicatesAndDates(t *testing.T) {
	summary := `<table><tr><th>檢查日期</th><th>檢驗檢查項目名稱</th><th>檢查結果</th></tr><tr><td>115/01/20</td><td>合成測試項目</td><td>&gt;=30</td></tr><tr><td>115/01/20</td><td>合成測試項目</td><td>&gt;=30</td></tr></table>`
	// A patient banner, executable asset text and alternate summary rows must
	// never become observations. Two identical detail rows are both retained.
	document := `<!doctype html><html><body><div>此處是合成頁首，不是報告資料</div><script>document.write('not executed')</script>` + summary + syntheticTable(syntheticDetailRow+syntheticDetailRow) + `</body></html>`
	rows, warnings, err := ParseImport("synthetic.html", []byte(document))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || len(warnings) == 0 {
		t.Fatalf("want two detail rows and the completeness warning, got %d rows", len(rows))
	}
	if rows[0] != rows[1] {
		t.Fatal("duplicate rows changed instead of being preserved")
	}
	row := rows[0]
	if row.CheckDate != "115/01/20" || row.VisitDate != "114/12/30" || NormalizeDate(row.CheckDate) != "2026-01-20" || NormalizeDate(row.VisitDate) != "2025-12-30" {
		t.Fatal("ROC inspection/visit dates were mixed or lost")
	}
	if row.Result != ">=30" || row.Unit != "x10^3/uL" || row.Reference != "[1][10]" || row.OfficialFlag != "1" || row.OrderCode != "TEST01" {
		t.Fatal("original result, unit, reference or official flag was not preserved")
	}
	if strings.Contains(row.Result, "this_is_not_report_text") {
		t.Fatal("script text entered an observation")
	}
}

func TestArchiveIgnoresAssetsAndUsesNoExtractionPaths(t *testing.T) {
	archive := syntheticArchive(t, map[string]string{
		"../../synthetic-report.HTML": syntheticTable(syntheticDetailRow),
		"assets/app.js":               `throw new Error("never execute uploaded JavaScript")`,
		"assets/styles.css":           "body { color: red; }",
		"assets/empty-frame.htm":      "<html><body>unrelated embedded page</body></html>",
	})
	rows, warnings, err := ParseImport("synthetic.zip", archive)
	if err != nil || len(rows) != 1 || len(warnings) == 0 {
		t.Fatalf("in-memory archive import failed: %v", err)
	}
	if rows[0].ItemName != "合成測試項目" {
		t.Fatal("unexpected archive data")
	}
}

func TestEmptyResultsAreNotFabricatedObservations(t *testing.T) {
	for _, body := range []string{"", `<tr><td colspan="10">無資料</td></tr>`} {
		rows, warnings, err := ParseImport("empty.html", []byte(syntheticTable(body)))
		if err != nil || rows == nil || len(rows) != 0 || len(warnings) < 2 {
			t.Fatalf("empty detailed table did not remain empty: %v", err)
		}
	}
	missing := strings.Replace(syntheticDetailRow, "&gt;=30", "-", 1)
	rows, _, err := ParseImport("missing.html", []byte(syntheticTable(missing)))
	if err != nil || len(rows) != 1 || rows[0].Result != "-" {
		t.Fatalf("missing result changed or disappeared: %v", err)
	}
}

func TestUnusableOrAmbiguousTablesAreRejected(t *testing.T) {
	for _, document := range []string{
		`<table><tr><th>檢查日期</th><th>檢驗檢查項目名稱</th><th>檢查結果</th></tr><tr><td>115/01/20</td><td>合成</td><td>1</td></tr></table>`,
		syntheticTable(strings.Replace(syntheticDetailRow, `<td>A</td>`, "", 1)),
		syntheticTable(strings.Replace(syntheticDetailRow, `<td>合成測試項目</td>`, `<td></td>`, 1)),
		"<html><body>not a laboratory report</body></html>",
	} {
		if rows, _, err := ParseImport("unusable.html", []byte(document)); err == nil || len(rows) != 0 {
			t.Fatal("unusable report silently produced observations")
		}
	}
	archive := syntheticArchive(t, map[string]string{"one.html": syntheticTable(syntheticDetailRow), "two.html": syntheticTable(syntheticDetailRow)})
	if rows, _, err := ParseImport("ambiguous.zip", archive); err == nil || len(rows) != 0 {
		t.Fatal("independent reports were silently combined")
	}
}

func TestImportResourceLimits(t *testing.T) {
	if _, _, err := ParseImport("too-large.zip", make([]byte, maxImportBytes+1)); err == nil {
		t.Fatal("compressed input size limit not enforced")
	}
	if _, _, err := ParseImport("too-large.html", bytes.Repeat([]byte(" "), maxExpandedHTML+1)); err == nil {
		t.Fatal("plain HTML size limit not enforced")
	}
	expanded := syntheticArchive(t, map[string]string{"large.html": strings.Repeat(" ", maxExpandedHTML+1)})
	if _, _, err := ParseImport("expansion.zip", expanded); err == nil {
		t.Fatal("expanded HTML size limit not enforced")
	}
	entries := make(map[string]string, maxArchiveEntries+1)
	for i := 0; i <= maxArchiveEntries; i++ {
		entries[fmt.Sprintf("asset-%d.txt", i)] = ""
	}
	if _, _, err := ParseImport("entries.zip", syntheticArchive(t, entries)); err == nil {
		t.Fatal("archive entry count limit not enforced")
	}
}
