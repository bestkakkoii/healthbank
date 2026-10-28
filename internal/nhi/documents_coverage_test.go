// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestPrescriptionCalendarBoundToUnfilledSourceRefillOnly(t *testing.T) {
	var downloads atomic.Int32
	const calendar = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nSUMMARY:合成建議領藥\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/ihke3000/IHKE3212S01/page_load":
			respondJSON(t, w, map[string]any{"main_data": []any{map[string]any{"treat_NUM": "PRIVATE-TREAT", "recipe_TYPE": "C", "func_DATE": "115/10/01", "refillList": []any{map[string]any{"recipE_TIMES": "1", "reL_DATE_STR": "115/10/01"}, map[string]any{"recipE_TIMES": "2", "reL_DATE_STR": "", "recipE_MEMO": "合成建議期間"}}}}, "hist_list": []any{}})
		case "/api/ihke3000/IHKE3212S02/page_load":
			respondJSON(t, w, map[string]any{"treatData": map[string]any{"treat_NUM": "PRIVATE-TREAT", "recipe_TYPE": "C", "func_DATE": "115/10/01"}, "list": []any{}})
		case "/api/ihke3000/ihke3212s01/IHKE3212_ics":
			downloads.Add(1)
			if r.Method != http.MethodGet || len(r.URL.Query()) != 3 || r.URL.Query().Get("tid") != "PRIVATE-TREAT" || r.URL.Query().Get("recipeType") != "C" || r.URL.Query().Get("rtimes") != "2" {
				t.Error("calendar selectors were not bound to the source refill")
			}
			w.Header().Set("Content-Type", "text/calendar;charset=UTF-8")
			_, _ = io.WriteString(w, calendar)
		default:
			t.Error("unobserved request or implicit document mutation")
			http.NotFound(w, r)
		}
	})
	list, err := s.FetchProvider(context.Background(), "e_prescriptions", Query{})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := s.FetchProviderDetail(context.Background(), "e_prescriptions", list.Entries[0].DetailID)
	if err != nil {
		t.Fatal(err)
	}
	attachments := detail.Entries[0].Attachments
	if len(attachments) != 2 || !strings.HasSuffix(attachments[1].Name, ".ics") || !strings.Contains(attachments[1].Name, "非已領或已服用") || downloads.Load() != 0 {
		t.Fatal("calendar capability missing, automatically downloaded, or includes a confirmed dispense")
	}
	encoded, _ := json.Marshal(detail)
	if strings.Contains(string(encoded), "PRIVATE-TREAT") {
		t.Fatal("private source prescription identifier exposed")
	}
	var info DocumentInfo
	var output bytes.Buffer
	if err := s.DownloadDocument(context.Background(), "e_prescriptions", attachments[1].ID, collectDocument(&info, &output)); err != nil {
		t.Fatal(err)
	}
	if output.String() != calendar || info.ContentType != "text/calendar;charset=utf-8" || downloads.Load() != 1 {
		t.Fatal("original UTF-8 calendar was changed or incomplete")
	}
	if _, err := s.FetchProvider(context.Background(), "e_prescriptions", Query{}); err != nil {
		t.Fatal(err)
	}
	if err := s.DownloadDocument(context.Background(), "e_prescriptions", attachments[1].ID, collectDocument(&info, &output)); err == nil || downloads.Load() != 1 {
		t.Fatal("retired calendar reference remained usable")
	}
}

func TestPrescriptionCalendarRejectsIncompleteAndErrorBodies(t *testing.T) {
	for _, body := range []string{"<html>login</html>", "BEGIN:VCALENDAR\nVERSION:2.0\n", "BEGIN:VCALENDAR\n\xff\nEND:VCALENDAR", "BEGIN:VCALENDAR\n\x00\nEND:VCALENDAR"} {
		s := providerFixture(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/calendar")
			_, _ = io.WriteString(w, body)
		})
		s.documentReferences = map[string]documentReference{"calendar": {category: "e_prescriptions", kind: "prescription_calendar", rowID: "synthetic", sourceType: "C", sequence: "2"}}
		called := false
		err := s.DownloadDocument(context.Background(), "e_prescriptions", "calendar", func(DocumentInfo, io.Reader) error { called = true; return nil })
		if err == nil || called {
			t.Fatal("invalid calendar offered to consumer")
		}
	}
}

func TestSelfPaidOriginalFilesAreBoundAndPreserveDuplicates(t *testing.T) {
	var downloads atomic.Int32
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/ihke3000/ihke3403s05/IHKE3403S05":
			var payload map[string]string
			if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&payload) != nil || len(payload) != 1 || payload["DATA02_SEQ"] != "PRIVATE-PARENT" {
				t.Error("self-paid read used a mutation or unbound selector")
			}
			respondJSON(t, w, map[string]any{"DATA02_SEQ": "PRIVATE-PARENT", "REAL_INSPECT_DATE": "115/09/01", "HOSP_NAME": "合成院所", "ReportDataDownloadList": []any{map[string]any{"FILE_SEQ": "PRIVATE-FILE", "RANK_SEQ": "1"}, map[string]any{"FILE_SEQ": "PRIVATE-FILE", "RANK_SEQ": "1"}}})
		case "/api/ihke3000/ihke3403S07/download":
			downloads.Add(1)
			if r.Method != http.MethodGet || len(r.URL.Query()) != 2 || r.URL.Query().Get("DATA02_SEQ") != "PRIVATE-PARENT" || r.URL.Query().Get("FILE_SEQ") != "PRIVATE-FILE" {
				t.Error("self-paid file identifiers escaped source binding")
			}
			w.Header().Set("Content-Type", "text/plain;charset=UTF-8") // Source Blob type is not the actual PDF type.
			_, _ = io.WriteString(w, "%PDF-1.7\nsynthetic\n%%EOF\n")
		default:
			t.Error("self-paid original invoked an unobserved route")
			http.NotFound(w, r)
		}
	})
	s.selfPaidReferences = map[string]string{"parent": "PRIVATE-PARENT"}
	report, err := s.FetchSelfPaidDetail(context.Background(), "parent")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Entries) != 2 || report.Entries[0].Key == report.Entries[1].Key || downloads.Load() != 0 {
		t.Fatal("source original file rows collapsed or auto-downloaded")
	}
	first, second := report.Entries[0].Attachments[0], report.Entries[1].Attachments[0]
	if first.ID == second.ID || first.ContentType != "application/octet-stream" {
		t.Fatal("file capability was collapsed or source format guessed")
	}
	encoded, _ := json.Marshal(report)
	if strings.Contains(string(encoded), "PRIVATE-") {
		t.Fatal("self-paid private selectors exposed")
	}
	var info DocumentInfo
	var output bytes.Buffer
	if err := s.DownloadDocument(context.Background(), "e_prescriptions", first.ID, collectDocument(&info, &output)); err == nil || downloads.Load() != 0 {
		t.Fatal("cross-category self-paid download accepted")
	}
	if err := s.DownloadDocument(context.Background(), "self_paid", first.ID, collectDocument(&info, &output)); err != nil {
		t.Fatal(err)
	}
	if info.ContentType != "application/pdf" || !strings.HasSuffix(info.Name, ".pdf") || output.String() != "%PDF-1.7\nsynthetic\n%%EOF\n" {
		t.Fatal("self-paid actual file type or bytes changed")
	}
	if _, err := s.FetchSelfPaidDetail(context.Background(), "parent"); err != nil {
		t.Fatal(err)
	}
	if err := s.DownloadDocument(context.Background(), "self_paid", first.ID, collectDocument(&info, &output)); err == nil || downloads.Load() != 1 {
		t.Fatal("self-paid file refresh retained a retired opaque capability")
	}
}

func TestSelfPaidFilesDetectTypesAndRejectLoginOrBinaryGarbage(t *testing.T) {
	for _, tc := range []struct{ body, contentType string }{
		{"合成報告 <3；\n仍保留文字。", "text/plain;charset=utf-8"},
		{"\x89PNG\r\n\x1a\nsynthetic", "image/png"},
		{"\xff\xd8\xffsynthetic", "image/jpeg"},
		{"PK\x03\x04synthetic", "application/zip"},
		{"<html>synthetic sign-in</html>", ""},
		{"\x00\x01\x02\x03\x04unknown", ""},
		{"{\"status\":\"Y\"}", ""},
	} {
		s := providerFixture(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = io.WriteString(w, tc.body)
		})
		s.documentReferences = map[string]documentReference{"file": {category: "self_paid", kind: "self_paid_file", rowID: "synthetic-parent", sequence: "synthetic-file"}}
		var info DocumentInfo
		var output bytes.Buffer
		err := s.DownloadDocument(context.Background(), "self_paid", "file", collectDocument(&info, &output))
		if tc.contentType == "" {
			if err == nil || output.Len() > 0 {
				t.Fatal("invalid source artifact accepted")
			}
		} else if err != nil || info.ContentType != tc.contentType || output.String() != tc.body {
			t.Fatalf("source file bytes/type changed: %v", err)
		}
	}
}

func TestSelfPaidStructuredRowsNeverSilentlyDisappear(t *testing.T) {
	var unknown atomic.Bool
	s := providerFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		rows := []any{map[string]any{"ASSAY_VALUE": nil}, map[string]any{"ASSAY_VALUE": ""}, map[string]any{"ASSAY_VALUE": "0"}, map[string]any{"ASSAY_VALUE": "0"}, map[string]any{"ASSAY_ITEM_NAME": "合成數值區塊標題", "LEVEL_01": "001", "LEVEL_02": "000"}}
		if unknown.Load() {
			rows = []any{map[string]any{"unknown_schema": "synthetic changed source"}}
		}
		respondJSON(t, w, map[string]any{"DATA02_SEQ": "synthetic-report", "REAL_INSPECT_DATE": "115/10/01", "OrdinaryInspection": map[string]any{}, "NumericalData": map[string]any{"合成群組": rows}, "ReportData": map[string]any{}})
	})
	s.selfPaidReferences = map[string]string{"parent": "synthetic-report"}
	report, err := s.FetchSelfPaidDetail(context.Background(), "parent")
	if err != nil || len(report.Entries) != 5 {
		t.Fatal("known absent/empty self-paid source rows were silently dropped", err)
	}
	seen := map[string]bool{}
	for _, entry := range report.Entries {
		if entry.Key == "" || seen[entry.Key] {
			t.Fatal("self-paid duplicate source row identity collapsed")
		}
		seen[entry.Key] = true
		group := false
		for _, field := range entry.Fields {
			group = group || field.Name == "來源群組（原文）" && field.Value == "合成群組"
		}
		if !group {
			t.Fatal("source visible group heading was discarded")
		}
	}
	if report.Entries[4].Kind != "source_heading" || report.Entries[4].Title != "合成數值區塊標題" {
		t.Fatal("source hierarchy heading became a missing clinical observation")
	}
	missing, empty := 0, 0
	for _, diagnostic := range report.Diagnostics {
		if diagnostic.Code == "known_result_missing" && diagnostic.Section == "nhi.self_paid.numericaldata" && diagnostic.Count != nil {
			missing += *diagnostic.Count
		}
		if diagnostic.Code == "section_empty" && (diagnostic.Section == "nhi.self_paid.ordinaryinspection" || diagnostic.Section == "nhi.self_paid.reportdata") {
			empty++
		}
	}
	if missing != 2 || empty != 2 {
		t.Fatal("empty sections/missing rows were not distinguished from explicit zero results")
	}
	unknown.Store(true)
	if _, err := s.FetchSelfPaidDetail(context.Background(), "parent"); err == nil {
		t.Fatal("unrecognized self-paid rows were reported as a successful empty response")
	}
}

func TestOfficialWholeBundleUsesItsReturnedFileDateAndReadyState(t *testing.T) {
	var ready atomic.Bool
	var downloads atomic.Int32
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/ihke3000/ihke3801s01/page_load":
			respondJSON(t, w, map[string]any{"sp_IHKE3801S01_data": []any{map[string]any{"seq_NO": "PRIVATE-JOB", "title": "JSON", "txt_DATE": "115/10/27", "file_txt_DATE": "115/10/28", "status": "來源狀態", "flag": "4"}}})
		case "/api/ihke3000/ihke3801s02/page_load":
			if r.URL.Query().Get("txtdate") != "115/10/27" || r.URL.Query().Get("seq_no") != "PRIVATE-JOB" {
				t.Error("job detail did not preserve the source job date")
			}
			status := "0"
			if ready.Load() {
				status = "1"
			}
			respondJSON(t, w, map[string]any{"status": status})
		case "/api/ihke3000/ihke3801s01/download":
			downloads.Add(1)
			if r.Method != http.MethodGet || len(r.URL.Query()) != 3 || r.URL.Query().Get("txtdate") != "115/10/28" || r.URL.Query().Get("seq_no") != "PRIVATE-JOB" || r.URL.Query().Get("type") != "JSON" {
				t.Error("whole bundle used guessed selectors or the job's different date")
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = io.WriteString(w, "PK\x05\x06\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")
		default:
			t.Error("whole source bundle invoked an unobserved route")
			http.NotFound(w, r)
		}
	})
	list, err := s.FetchOfficialFiles(context.Background())
	if err != nil || len(list.Entries) != 1 {
		t.Fatal("whole source job list unavailable", err)
	}
	parent := list.Entries[0].DetailID
	pending, err := s.FetchOfficialFileDetail(context.Background(), parent)
	if err != nil || len(pending.Entries[0].Attachments) != 0 {
		t.Fatal("pending source job acquired a download capability", err)
	}
	ready.Store(true)
	available, err := s.FetchOfficialFileDetail(context.Background(), parent)
	if err != nil || len(available.Entries[0].Attachments) != 2 || downloads.Load() != 0 {
		t.Fatal("ready whole bundle missing or implicitly downloaded", err)
	}
	bundle := available.Entries[0].Attachments[1]
	var info DocumentInfo
	var output bytes.Buffer
	if err := s.DownloadDocument(context.Background(), "official_files", bundle.ID, collectDocument(&info, &output)); err != nil || info.ContentType != "application/zip" || output.Len() != 22 {
		t.Fatal("whole bundle did not preserve its actual format/bytes", err)
	}
	if diagnostic := s.Snapshot(); diagnostic == nil || diagnostic.Operation != OperationDocumentDownload || diagnostic.Phase != PhaseComplete {
		t.Fatal("whole bundle diagnostic did not finish the fixed download operation")
	}
	if _, err := s.FetchOfficialFiles(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.DownloadDocument(context.Background(), "official_files", bundle.ID, collectDocument(&info, &output)); err == nil || downloads.Load() != 1 {
		t.Fatal("old whole bundle reference survived list refresh")
	}
}

func TestNamedOriginalTextIsInertAndJSONStatusStillExpires(t *testing.T) {
	for _, tc := range []struct {
		name, body, media, disposition, wantCode string
		wantOK                                   bool
	}{
		{"json", "{\"合成結果\":62.000000000000000001}", "application/json", "attachment; filename=synthetic.json", "", true},
		{"json_plain", "\xef\xbb\xbf{\"合成結果\":[\"<1\",\"<1\"]}", "text/plain", "attachment; filename=synthetic.json", "", true},
		{"json_array", "[{\"合成結果\":\"原文\"}]", "application/json", "attachment; filename=synthetic.json", "", true},
		{"json_case_sensitive", "{\"Result\":1,\"result\":2}", "application/json", "attachment; filename=synthetic.json", "", true},
		{"json_report_markup", "{\"report\":\"<html>合成原始報告<script>neverExecuted()</script></html>\"}", "application/json", "attachment; filename=synthetic.json", "", true},
		{"html_original", "<!doctype html><html><body>合成來源<script>neverExecuted()</script></body></html>", "text/html", "attachment; filename=synthetic.html", "", true},
		{"xml_original", "<?xml version=\"1.0\"?><合成結果>&lt;1</合成結果>", "application/xml", "attachment; filename=synthetic.xml", "", true},
		{"short_text", "好", "text/plain", "attachment; filename=synthetic.txt", "", true},
		{"html_login", "<!doctype html><html>login</html>", "text/html", "", "DOCUMENT_INVALID", false},
		{"json_status", "{\"status\":\"Y\"}", "application/json", "", "DOCUMENT_INVALID", false},
		{"json_incomplete", "{\"synthetic\":", "application/json", "attachment; filename=synthetic.json", "DOCUMENT_INVALID", false},
		{"json_unsafe_name", "{\"synthetic\":1}", "application/json", "attachment; filename=\"../synthetic.json\"", "DOCUMENT_INVALID", false},
		{"json_expired", "{\"error_code\":\"9999\"}", "application/json", "attachment; filename=synthetic.json", "SESSION_EXPIRED", false},
		{"json_repeated_expiry", "{\"error_code\":\"0000\",\"ERROR_CODE\":\"9999\",\"error_code\":\"0000\"}", "application/json", "attachment; filename=synthetic.json", "SESSION_EXPIRED", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := providerFixture(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tc.media)
				if tc.disposition != "" {
					w.Header().Set("Content-Disposition", tc.disposition)
				}
				_, _ = io.WriteString(w, tc.body)
			})
			s.documentReferences = map[string]documentReference{"file": {category: "official_files", kind: "official_bundle", sequence: "synthetic-job", format: "JSON", date: "115/10/28"}}
			var info DocumentInfo
			var output bytes.Buffer
			err := s.DownloadDocument(context.Background(), "official_files", "file", collectDocument(&info, &output))
			if tc.wantOK {
				if err != nil || output.String() != tc.body || info.ContentType != "text/plain;charset=utf-8" || strings.HasSuffix(info.Name, ".html") || strings.Contains(info.Name, "synthetic") {
					t.Fatalf("original text bytes/type/name did not remain inert: %v", err)
				}
				if diagnostic := s.Snapshot(); diagnostic == nil || diagnostic.Phase != PhaseComplete {
					t.Fatal("successful text download was not marked complete")
				}
			} else {
				var upstream *Error
				if !errors.As(err, &upstream) || upstream.Code != tc.wantCode || output.Len() != 0 {
					t.Fatalf("status or incomplete original delivered as a successful artifact: %v", err)
				}
				if tc.wantCode == "SESSION_EXPIRED" && (s.token != "" || len(s.documentReferences) != 0) {
					t.Fatal("named source file concealed explicit authentication expiry")
				}
			}
		})
	}
}
