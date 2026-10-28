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
	"time"
)

func TestUpstreamRejectsAmbiguousAndLossyLaboratoryJSON(t *testing.T) {
	for _, body := range []string{
		`[{"assaY_ITEM_NAME":"合成檢驗","assaY_VALUE":"1","assaY_VALUE":"2"}]`,
		`[{"assaY_ITEM_NAME":"合成檢驗","assaY_VALUE":"1","ASSAY_VALUE":"2"}]`,
		`[{"assaY_ITEM_NAME":"合成檢驗","assaY_VALUE":"1","assaY_VAL\u0055E":"2"}]`,
		`[{"assaY_ITEM_NAME":"合成檢驗","assaY_VALUE":"\ud800"}]`,
		`[{"assaY_ITEM_NAME":"合成檢驗","assaY_VALUE":"\udc00"}]`,
		"[{\"assaY_ITEM_NAME\":\"合成檢驗\",\"assaY_VALUE\":\"\xff\"}]",
	} {
		rows, err := parseRecords([]byte(body))
		var upstream *Error
		if !errors.As(err, &upstream) || upstream.Code != "UPSTREAM_CHANGED" || len(rows) != 0 {
			t.Fatal("ambiguous or repaired laboratory value was accepted")
		}
	}
	body := []byte("\xef\xbb\xbf" + `[{"assaY_ITEM_NAME":"合成中文","assaY_VALUE":"\ud842\udfb7，原字元�","consulT_VALUE":"\\ud800"}]`)
	rows, err := parseRecords(body)
	if err != nil || len(rows) != 1 || rows[0].Result != "𠮷，原字元�" || rows[0].Reference != `\ud800` {
		t.Fatalf("valid original Unicode or literal escapes changed: %v", err)
	}
}

func TestProviderRejectsDuplicateFieldsWithoutFoldingDataKeys(t *testing.T) {
	for _, body := range []string{
		`{"sP_IHKE3202S01":[],"sP_IHKE3202S01":[{"drug_NAME":"合成"}]}`,
		`{"sP_IHKE3202S01":[{"drug_NAME":"合成一","drug_NAME":"合成二"}]}`,
		`{"sP_IHKE3202S01":[{"drug_NAME":"\ud800"}]}`,
	} {
		if _, err := providerObject([]byte(body)); err == nil {
			t.Fatal("provider JSON overwrote or repaired source text")
		}
	}
	object, err := providerObject([]byte(`{"dietRecordList":{"Meal":{"eatDate":"115/01/01"},"meal":{"eatDate":"115/01/02"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	rows, present, err := providerSectionRows(object, providerSection{root: "dietRecordList", keyed: true})
	if err != nil || !present || len(rows) != 2 {
		t.Fatal("case-sensitive source dictionary keys were collapsed")
	}
}

func TestLoginRejectsConflictingSuccessAndTokenFields(t *testing.T) {
	for _, body := range []string{
		`{"error_code":"Z001","error_code":"0000","jwt_token":"synthetic"}`,
		`{"error_code":"Z001","ERROR_CODE":"0000","jwt_token":"synthetic"}`,
		`{"error_code":"0000","jwt_token":"first","JWT_TOKEN":"second"}`,
		`{"error_code":"0000","jwt_token":"synthetic\ud800"}`,
	} {
		s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != loginPath || r.Method != http.MethodPost {
				t.Error("unexpected synthetic login request")
			}
			_, _ = io.WriteString(w, body)
		})
		s.challengeID, s.challengeExpiry = "synthetic-challenge", time.Now().Add(time.Minute)
		err := s.Login(context.Background(), Credentials{NationalID: "A123456789", CardNumber: "000012345678", Password: "synthetic-password", Captcha: "synthetic-answer"})
		var upstream *Error
		if !errors.As(err, &upstream) || upstream.Code != "UPSTREAM_CHANGED" || s.token != "" || s.challengeID != "" {
			t.Fatal("conflicting authentication fields established a session or replayable challenge")
		}
	}
}

func TestExerciseDetailHonorsFailureAndPreservesSourceTime(t *testing.T) {
	for _, tc := range []struct {
		name, body, code string
	}{
		{"failed", `{"ResultStatus":false,"oneRecordList":[]}`, "REQUEST_FAILED"},
		{"missing_status", `{"oneRecordList":[{"CODE":"SYNTHETIC","TOT_HR":"1"}]}`, "UPSTREAM_CHANGED"},
		{"empty", `{"ResultStatus":true,"oneRecordList":[]}`, "UPSTREAM_CHANGED"},
		{"wrong_row", `{"ResultStatus":true,"SPORTDATE_DATE":"2026-10-01","oneRecordList":[{"ROW_ID":"SYNTHETIC-OTHER","CODE":"SYNTHETIC","TOT_HR":"1"}]}`, "UPSTREAM_CHANGED"},
		{"valid", `{"ResultStatus":true,"SPORTDATE_DATE":"2026-10-01","SPORTDATE_HOUR":"08","SPORTDATE_MINUTE":"05","oneRecordList":[{"ROW_ID":"SYNTHETIC-EXERCISE","CODE":"SYNTHETIC","CODE_LEVEL":"2","TOT_HR":"1.000","TOT_MIN":"30"}]}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/ihke3000/ihke3207s02/SP_IHKE3207S04" || r.Method != http.MethodPost {
					t.Error("exercise read used an unexpected route")
				}
				var selector map[string]string
				if json.NewDecoder(r.Body).Decode(&selector) != nil || len(selector) != 1 || selector["c_rowid"] != "SYNTHETIC-EXERCISE" {
					t.Error("exercise detail selector escaped its parent binding")
				}
				_, _ = io.WriteString(w, tc.body)
			})
			s.providerReferences = map[string]map[string]providerReference{"exercise": {"ref": {rowID: "SYNTHETIC-EXERCISE", date: "115/10/01", title: "合成運動"}}}
			report, err := s.FetchProviderDetail(context.Background(), "exercise", "ref")
			if tc.code != "" {
				var upstream *Error
				if !errors.As(err, &upstream) || upstream.Code != tc.code || len(report.Entries) != 0 {
					t.Fatalf("source failure became successful exercise detail: %v", err)
				}
				return
			}
			if err != nil || len(report.Entries) != 2 || report.Entries[0].Date != "2026-10-01" || report.Complete {
				t.Fatalf("valid exercise detail was not preserved: %v", err)
			}
			data, err := json.Marshal(report)
			if err != nil || !bytes.Contains(data, []byte(`"value":"08"`)) || !bytes.Contains(data, []byte(`"value":"05"`)) || !bytes.Contains(data, []byte(`"value":"1.000"`)) || bytes.Contains(data, []byte("SYNTHETIC-EXERCISE")) {
				t.Fatal("source time/duration lost precision or private selector leaked")
			}
		})
	}
}

func TestOfficialFilesBindSourceDownloadIDsToFixedRoutes(t *testing.T) {
	for _, tc := range []struct {
		name, format, detailPath, downloadPath, payload string
		image                                           bool
	}{
		{"archive", "JSON", "page_load", "app_download", "PK\x03\x04synthetic-source", false},
		{"pdf", "PDF", "page_load", "app_download", "%PDF-synthetic-source", false},
		{"dicom", "DICOM", "imgF_page_load", "app_imgF_download", "PK\x03\x04synthetic-dicom", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/ihke3000/ihke3801s02/" + tc.detailPath:
					respondJSON(t, w, map[string]any{"status": "1", "did": "private-synthetic-gid", "sp_IHKE3801S02_data": []any{}})
				case "/api/ihke3000/ihke3801s02/" + tc.downloadPath:
					calls.Add(1)
					if r.Method != http.MethodGet || len(r.URL.Query()) != 1 || r.URL.Query().Get("gid") != "private-synthetic-gid" || r.Header.Get("Authorization") != "Bearer synthetic-provider-token" {
						t.Error("download ID route/selector escaped its bound source contract")
					}
					w.Header().Set("Content-Type", "application/octet-stream")
					_, _ = io.WriteString(w, tc.payload)
				default:
					t.Error("source-selected download ignored in favor of another route")
					http.NotFound(w, r)
				}
			})
			s.officialFileReferences = map[string]officialFileReference{"ref": {sequence: "private-synthetic-sequence", format: tc.format, date: "115/10/28", image: tc.image}}
			report, err := s.FetchOfficialFileDetail(context.Background(), "ref")
			if err != nil || len(report.Entries) != 1 || len(report.Entries[0].Attachments) != 1 {
				t.Fatalf("source-selected document reference missing: %v", err)
			}
			encoded, err := json.Marshal(report)
			if err != nil || strings.Contains(string(encoded), "private-synthetic") {
				t.Fatal("private official selector leaked to the browser")
			}
			id := report.Entries[0].Attachments[0].ID
			err = s.DownloadDocument(context.Background(), "official_files", id, func(info DocumentInfo, reader io.Reader) error {
				data, err := io.ReadAll(reader)
				if string(data) != tc.payload {
					t.Error("source artifact bytes changed")
				}
				return err
			})
			if err != nil || calls.Load() != 1 {
				t.Fatalf("fixed source-selected download failed: %v", err)
			}
		})
	}
}

func TestProviderPreservesSourceScreeningReportsAndSpecimenContext(t *testing.T) {
	for _, tc := range []struct {
		category, path, root, field, value string
	}{
		{"screen_lung", "/api/ihke3000/ihke3404s06/page_load", "sp_IHKE3404S06_Data", "lung_RADS", "合成報告：<b>原始標記僅作文字</b>"},
		{"screen_breast", "/api/ihke3000/ihke3404s04/page_load", "sP_IHKE3404S04_Data", "diagnosis_CODE", "合成判讀原文"},
		{"screen_hpv", "/api/ihke3000/ihke3404s07/page_load", "sp_IHKE3404S07_Data", "hpv_diagnosis_code_rpt", "合成 HPV 來源報告"},
		{"screen_hpsa", "/api/ihke3000/ihke3404s08/page_load", "sp_IHKE3404S08_Data", "assaY_RESULT", "1"},
		{"covid", "/api/ihke3000/IHKE3410S01/page_load", "sp_IHKE3410S04", "inspect_MODE", "合成檢體種類"},
		{"covid", "/api/ihke3000/IHKE3410S01/page_load", "sp_IHKE3410S04", "assay_DISEA", "合成送驗疾病"},
		{"covid", "/api/ihke3000/IHKE3410S01/page_load", "sp_IHKE3410S04", "recv_DATE", "115/10/03 09:10:11"},
	} {
		t.Run(tc.category+"_"+tc.field, func(t *testing.T) {
			s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path || r.Method != http.MethodGet {
					t.Error("source projection changed acquisition route")
				}
				respondJSON(t, w, map[string]any{tc.root: []any{map[string]string{"func_DATE": "115/10/01", "real_INSPECT_DATE": "115/10/01", "code_CNAME": "合成結果", tc.field: tc.value}}})
			})
			report, err := s.FetchProvider(context.Background(), tc.category, Query{})
			if err != nil || len(report.Entries) != 1 || report.Entries[0].Date != "115/10/01" {
				t.Fatalf("source report failed or confused report and collection dates: %v", err)
			}
			found := false
			for _, field := range report.Entries[0].Fields {
				found = found || field.Value == tc.value
			}
			if !found {
				t.Fatal("source clinical report/specimen field was silently discarded")
			}
		})
	}
}

func TestProviderParentContextCannotValidateEmptyClinicalDetails(t *testing.T) {
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(t, w, map[string]any{"sP_IHKE3504S02_Data": []any{map[string]string{"unknown_schema_field": "not a recognized clinical record"}}})
	})
	s.providerReferences = map[string]map[string]providerReference{"diabetes_care": {"ref": {rowID: "synthetic-care", date: "115/10/01", institution: "合成院所"}}}
	report, err := s.FetchProviderDetail(context.Background(), "diabetes_care", "ref")
	var upstream *Error
	if !errors.As(err, &upstream) || upstream.Code != "UPSTREAM_CHANGED" || len(report.Entries) != 0 {
		t.Fatal("inherited institution made an unrecognized source detail look successful")
	}
}

func TestDocumentJSONExpiryClearsConnectorBeforeDeliveringArtifact(t *testing.T) {
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(t, w, map[string]string{"error_code": "9999", "message": "synthetic private error text must not leak"})
	})
	s.documentReferences = map[string]documentReference{"ref": {category: "official_files", kind: "official_archive", sequence: "synthetic"}}
	s.providerReferences = map[string]map[string]providerReference{"exercise": {"detail": {rowID: "synthetic"}}}
	delivered := false
	err := s.DownloadDocument(context.Background(), "official_files", "ref", func(DocumentInfo, io.Reader) error {
		delivered = true
		return nil
	})
	var upstream *Error
	if !errors.As(err, &upstream) || upstream.Code != "SESSION_EXPIRED" || delivered || s.token != "" || len(s.documentReferences) != 0 || len(s.providerReferences) != 0 {
		t.Fatalf("JSON source expiry retained authentication or delivered an artifact: %v", err)
	}
	if strings.Contains(err.Error(), "synthetic private") {
		t.Fatal("raw official error leaked")
	}
}

func TestProviderInvalidDateStopsBeforeOfficialTokenRefresh(t *testing.T) {
	var requests atomic.Int32
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		respondJSON(t, w, map[string]string{"error_code": "9999"})
	})
	s.tokenObtainedAt = time.Now().Add(-10 * time.Minute)
	_, err := s.FetchProvider(context.Background(), "glucose", Query{StartDate: "2026-02-30", EndDate: "2026-10-28"})
	var upstream *Error
	if !errors.As(err, &upstream) || upstream.Code != "INVALID_INPUT" || requests.Load() != 0 || s.token != "synthetic-provider-token" {
		t.Fatalf("invalid range performed an upstream operation before validation: %v", err)
	}
}

func TestProviderErrorCodeCasingCannotHideExpiry(t *testing.T) {
	for _, tc := range []struct{ body, code string }{
		{`{"ERROR_CODE":"9999","sP_IHKE3404S03_Data":[]}`, "SESSION_EXPIRED"},
		{`{"error_code":"0000","ERROR_CODE":"9999","sP_IHKE3404S03_Data":[]}`, "UPSTREAM_CHANGED"},
	} {
		s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, tc.body) })
		_, err := s.FetchProvider(context.Background(), "screen_oral", Query{})
		var upstream *Error
		if !errors.As(err, &upstream) || upstream.Code != tc.code || tc.code == "SESSION_EXPIRED" && s.token != "" {
			t.Fatalf("reserved error field was mistaken for a successful empty source: %v", err)
		}
	}
}

func TestProjectedSourceBoundsRejectBeforeReplacingReferences(t *testing.T) {
	t.Run("physiology_additional_index", func(t *testing.T) {
		rows := make([]map[string]string, 50000)
		for i := range rows {
			rows[i] = map[string]string{"value": "1"}
		}
		s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
			respondJSON(t, w, map[string]any{"maindata": []any{}, "maindata_10568": rows})
		})
		s.providerReferences = map[string]map[string]providerReference{"physiology": {"previous": {rowID: "10568"}}}
		_, err := s.FetchProvider(context.Background(), "physiology", Query{})
		var upstream *Error
		if !errors.As(err, &upstream) || upstream.Code != "RESPONSE_TOO_LARGE" || len(s.providerReferences["physiology"]) != 1 || s.providerReferences["physiology"]["previous"].rowID != "10568" {
			t.Fatalf("extra source index exceeded the report bound or retired earlier references: %v", err)
		}
	})
	t.Run("official_file_combined_fields", func(t *testing.T) {
		labels := make([]string, 50000)
		for i := range labels {
			labels[i] = "合成項目"
		}
		s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
			respondJSON(t, w, map[string]any{"status": "1", "code_cname": labels})
		})
		s.officialFileReferences = map[string]officialFileReference{"parent": {sequence: "synthetic", format: "JSON"}}
		s.documentReferences = map[string]documentReference{"previous": {category: "official_files", parent: "parent", kind: "official_archive", sequence: "synthetic"}}
		_, err := s.FetchOfficialFileDetail(context.Background(), "parent")
		var upstream *Error
		if !errors.As(err, &upstream) || upstream.Code != "RESPONSE_TOO_LARGE" || len(s.documentReferences) != 1 || s.documentReferences["previous"].sequence != "synthetic" {
			t.Fatalf("combined source labels exceeded portable bounds or retired earlier attachment: %v", err)
		}
	})
	t.Run("projected_title_prefix", func(t *testing.T) {
		s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
			respondJSON(t, w, map[string]any{"sp_IHKE3801S01_data": []any{map[string]string{"seq_NO": "synthetic-new", "title": strings.Repeat("A", 65536)}}})
		})
		s.officialFileReferences = map[string]officialFileReference{"previous": {sequence: "synthetic-old"}}
		_, err := s.FetchOfficialFiles(context.Background())
		var upstream *Error
		if !errors.As(err, &upstream) || upstream.Code != "RESPONSE_TOO_LARGE" || len(s.officialFileReferences) != 1 || s.officialFileReferences["previous"].sequence != "synthetic-old" {
			t.Fatalf("title prefix bypassed scalar bounds or replaced previous references: %v", err)
		}
	})
	t.Run("html_combined_scope_fields", func(t *testing.T) {
		months := make([]map[string]string, 50000)
		for i := range months {
			months[i] = map[string]string{"text": "合成月份", "value": "synthetic"}
		}
		s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
			respondJSON(t, w, map[string]any{"sp_IHKE3802S01_data": []any{map[string]any{"code_CNAME": "合成原始檔", "has_all": false, "all_sdate": "19120101", "all_edate": "20261028", "details_items": []any{map[string]any{"details": months}}}}})
		})
		s.documentReferences = map[string]documentReference{"previous": {category: "official_export_html", parent: "options", kind: "prepare_export", payload: []byte("synthetic-previous-selector")}}
		_, err := s.FetchOfficialExportOptions(context.Background(), "html")
		var upstream *Error
		if !errors.As(err, &upstream) || upstream.Code != "RESPONSE_TOO_LARGE" || len(s.documentReferences) != 1 || string(s.documentReferences["previous"].payload) != "synthetic-previous-selector" {
			t.Fatalf("combined export-option fields exceeded portable bounds or replaced earlier action: %v", err)
		}
	})
}

func TestOfficialLaboratoryScalarLimitMatchesPortableDataset(t *testing.T) {
	for _, length := range []int{65536, 65537} {
		body, err := json.Marshal([]map[string]string{{"assaY_ITEM_NAME": "合成檢驗", "assaY_VALUE": strings.Repeat("A", length)}})
		if err != nil {
			t.Fatal(err)
		}
		rows, err := parseRecords(body)
		if length == 65536 && (err != nil || len(rows) != 1 || len(rows[0].Result) != length) {
			t.Fatal("valid boundary-length source value was changed or rejected")
		}
		if length > 65536 && (err == nil || len(rows) != 0) {
			t.Fatal("oversized source value cannot round-trip the portable dataset")
		}
	}
}

func TestHospitalPDFCleanupExpiryPropagatesBeforeDelivery(t *testing.T) {
	for _, tc := range []struct {
		name, document string
		cleanupStatus  int
	}{
		{"json_expiry", "%PDF-1.7\nsynthetic\n%%EOF", http.StatusOK},
		{"http_expiry", "%PDF-1.7\nsynthetic\n%%EOF", http.StatusUnauthorized},
		{"expiry_during_failed_document_cleanup", "<html>synthetic invalid file</html>", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var cleanup atomic.Int32
			s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/ihke3000/IHKE3309S02/getpdf":
					respondJSON(t, w, map[string]string{"file_name": "synthetic-cleanup.pdf"})
				case "/ihke3000/ViewPdf/synthetic-cleanup.pdf":
					_, _ = io.WriteString(w, tc.document)
				case "/api/ihke3000/IHKE3309S02/delpdf":
					cleanup.Add(1)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(tc.cleanupStatus)
					_, _ = io.WriteString(w, `{"error_code":"9999","message":"SYNTHETIC_PRIVATE_ERROR"}`)
				default:
					t.Error("unexpected synthetic PDF request")
					http.NotFound(w, r)
				}
			})
			s.documentReferences = map[string]documentReference{"ref": {category: "hospital", kind: "hospital_pdf", rowID: "synthetic", sourceType: "A"}}
			consumed := false
			err := s.DownloadDocument(context.Background(), "hospital", "ref", func(DocumentInfo, io.Reader) error {
				consumed = true
				return nil
			})
			var upstream *Error
			if !errors.As(err, &upstream) || upstream.Code != "SESSION_EXPIRED" || consumed || s.token != "" || len(s.documentReferences) != 0 || cleanup.Load() != 1 {
				t.Fatalf("temporary cleanup expiry did not invalidate the application before document delivery: %v", err)
			}
			if strings.Contains(err.Error(), "SYNTHETIC_PRIVATE_ERROR") {
				t.Fatal("cleanup error leaked provider content")
			}
		})
	}
}

func TestHospitalPDFOrdinaryCleanupFailurePreservesDocumentError(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var cleanup atomic.Int32
			s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/ihke3000/IHKE3309S02/getpdf":
					respondJSON(t, w, map[string]string{"file_name": "synthetic-invalid.pdf"})
				case "/ihke3000/ViewPdf/synthetic-invalid.pdf":
					_, _ = io.WriteString(w, "<html>synthetic invalid file</html>")
				case "/api/ihke3000/IHKE3309S02/delpdf":
					cleanup.Add(1)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(status)
					_, _ = io.WriteString(w, `{"error_code":"D002"}`)
				default:
					t.Error("unexpected synthetic PDF request")
					http.NotFound(w, r)
				}
			})
			s.documentReferences = map[string]documentReference{"ref": {category: "hospital", kind: "hospital_pdf", rowID: "synthetic", sourceType: "A"}}
			consumed := false
			err := s.DownloadDocument(context.Background(), "hospital", "ref", func(DocumentInfo, io.Reader) error {
				consumed = true
				return nil
			})
			var upstream *Error
			if !errors.As(err, &upstream) || upstream.Code != "DOCUMENT_INVALID" || consumed || s.token != "synthetic-provider-token" || len(s.documentReferences) != 1 || cleanup.Load() != 1 {
				t.Fatalf("ordinary cleanup failure replaced the document diagnostic or changed authentication: %v", err)
			}
		})
	}
}
