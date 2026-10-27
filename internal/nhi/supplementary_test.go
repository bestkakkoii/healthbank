// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package nhi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSupplementSearchKeepsExplicitDatesAndDropsIdentifiers(t *testing.T) {
	calls := []string{}
	s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		if r.URL.Query().Get("s_date") != "2010-01-01" || r.URL.Query().Get("e_date") != "2026-10-27" {
			t.Error("explicit interval overwritten")
		}
		if r.Header.Get("Authorization") == "" {
			t.Error("unauthenticated official request")
		}
		if strings.HasSuffix(r.URL.Path, "page_load") {
			respondJSON(t, w, map[string]any{"sp_IHKE3303S01_data": []any{}, "sdate": "2025-01-01", "edate": "2026-10-27"})
			return
		}
		respondJSON(t, w, []any{map[string]any{"funC_DATE": "100/01/01", "hosP_ABBR": "SYNTHETIC", "icd9cm_CODE_CNAME": "synthetic classification", "ID_NO": "DO_NOT_FORWARD", "PASSWORD": "DO_NOT_FORWARD"}, map[string]any{"funC_DATE": "100/01/01", "hosP_ABBR": "SYNTHETIC", "icd9cm_CODE_CNAME": "repeat kept"}})
	})
	s.token = "synthetic"
	s.tokenObtainedAt = time.Now()
	report, err := s.FetchSupplement(context.Background(), "western", Query{StartDate: "2010-01-01", EndDate: "2026-10-27"})
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || len(report.Entries) != 2 || report.Complete || report.StartDate != "2010-01-01" {
		t.Fatalf("wrong coverage: %#v", report)
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "DO_NOT_FORWARD") {
		t.Fatal("unneeded identifiers forwarded")
	}
}
func TestSupplementAdultAndRecentScopes(t *testing.T) {
	s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "3402") {
			respondJSON(t, w, map[string]any{"sP_IHKE3402S01_Data": []any{map[string]any{"firsT_DIAG_DATE": "115/10/01", "hosP_ABBR": "SYNTHETIC", "bloD_CREAT": "90", "waistline": "32"}}})
			return
		}
		respondJSON(t, w, map[string]any{"main_data": []any{map[string]any{"func_date": "115/10/01", "hosp_abbr": "SYNTHETIC", "icd9cm_name": "summary"}}, "chinese_data": []any{map[string]any{"func_date": "115/09/01", "hosp_abbr": "SYNTHETIC TCM"}}})
	})
	s.token = "synthetic"
	s.tokenObtainedAt = time.Now()
	adult, err := s.FetchSupplement(context.Background(), "adult", Query{})
	if err != nil {
		t.Fatal(err)
	}
	if adult.Complete || !strings.Contains(adult.Coverage, "最近兩次") || len(adult.Entries) != 1 {
		t.Fatal("adult scope overstated")
	}
	fields := adult.Entries[0].Fields
	if len(fields) != 2 || fields[0].Unit != "吋或公分" || fields[1].Unit != "µmol/L" {
		t.Fatalf("source units changed: %#v", fields)
	}
	recent, err := s.FetchSupplement(context.Background(), "recent_medicine", Query{})
	if err != nil {
		t.Fatal(err)
	}
	if len(recent.Entries) != 2 || recent.Complete || !strings.Contains(recent.Coverage, "摘要") {
		t.Fatal("recent sections lost or overstated")
	}
}
func TestSelfPaidDetailRequiresSessionBoundListReference(t *testing.T) {
	var detailCalls int
	s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatal("source read-only POST contract changed")
		}
		if strings.Contains(r.URL.Path, "3403s01") {
			respondJSON(t, w, []any{map[string]any{"DATA02_SEQ": "fixture-private-sequence", "REAL_INSPECT_DATE": "115/10/01", "HOSP_NAME": "SYNTHETIC", "DATA_FLAG": "S"}})
			return
		}
		detailCalls++
		var input map[string]string
		if json.NewDecoder(r.Body).Decode(&input) != nil || input["DATA02_SEQ"] != "fixture-private-sequence" {
			t.Error("wrong detail reference")
		}
		respondJSON(t, w, map[string]any{"DATA02_SEQ": "fixture-private-sequence", "REAL_INSPECT_DATE": "115/10/01", "HOSP_NAME": "SYNTHETIC", "NumericalData": map[string]any{"test": []any{map[string]any{"ASSAY_ITEM_NAME": "Synthetic", "ASSAY_VALUE": "<3", "UNIT_DATA": "test-unit", "CONSULT_VALUE": "[1][5]"}}}, "ReportData": map[string]any{"text": []any{map[string]any{"ASSAY_ITEM_NAME": "Synthetic text", "INSPECT_RESULT": "<script>plain text only</script>"}}}})
	})
	s.token = "synthetic"
	s.tokenObtainedAt = time.Now()
	if _, err := s.FetchSelfPaidDetail(context.Background(), "fixture-private-sequence"); err == nil || detailCalls != 0 {
		t.Fatal("caller chose upstream record")
	}
	list, err := s.FetchSupplement(context.Background(), "self_paid", Query{})
	if err != nil {
		t.Fatal(err)
	}
	reference := list.Entries[0].DetailID
	if len(reference) != 48 {
		t.Fatal("missing opaque detail reference")
	}
	detail, err := s.FetchSelfPaidDetail(context.Background(), reference)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Entries) != 2 || detail.Entries[0].Fields[0].Value != "<3" {
		t.Fatal("detail original results changed")
	}
	s.clearAuth()
	s.token = "synthetic-new"
	s.tokenObtainedAt = time.Now()
	if _, err := s.FetchSelfPaidDetail(context.Background(), reference); err == nil {
		t.Fatal("previous person's reference survived auth reset")
	}
}
func TestSupplementRejectsUnrecognizedContract(t *testing.T) {
	for _, body := range []string{`{}`, `[{"ID_NO":"identifier only"}]`, `[{"func_date":{},"hosp_abbr":"x"}]`, `[{"func_date":"x","FUNC_DATE":"y"}]`, `{"main_data":{}}`} {
		t.Run(body, func(t *testing.T) {
			s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, err := w.Write([]byte(body))
				if err != nil {
					t.Fatal(err)
				}
			})
			s.token = "synthetic"
			s.tokenObtainedAt = time.Now()
			if _, err := s.FetchSupplement(context.Background(), "recent_medicine", Query{}); err == nil {
				t.Fatal("unrecognized contract accepted as empty/success")
			}
		})
	}
}
func TestWesternOrdersUseObservedReadOnlyContract(t *testing.T) {
	s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/ihke3000/ihke3303s02/page_load" {
			if r.URL.Query().Get("crid") != "fixture-visit" || r.URL.Query().Get("ctype") != "A" {
				t.Error("wrong visit contract")
			}
			respondJSON(t, w, map[string]any{"ihke3303S02_main_data": []any{map[string]any{"rowid": "fixture-visit", "sp_IHKE3302S04_data": []any{map[string]any{"order_code": "SYNTHETIC", "drug_name": "Synthetic Medicine||合成藥品", "order_drug_day": 14, "ID_NO": "DO_NOT_FORWARD"}}, "sp_IHKE3302S05": []any{}}}})
			return
		}
		if strings.HasSuffix(r.URL.Path, "page_load") {
			respondJSON(t, w, map[string]any{"sp_IHKE3303S01_data": []any{}})
			return
		}
		respondJSON(t, w, []any{map[string]any{"row_ID": "fixture-visit", "ori_TYPE": "A", "funC_DATE": "115/10/01", "hosP_ABBR": "SYNTHETIC"}})
	})
	s.token = "synthetic"
	s.tokenObtainedAt = time.Now()
	list, err := s.FetchSupplement(context.Background(), "western", Query{})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := s.FetchWesternDetail(context.Background(), list.Entries[0].DetailID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Entries) != 1 || detail.Entries[0].Fields[2].Value != "14" || detail.Complete {
		t.Fatal("medication source fields lost")
	}
	s.clearAuth()
	s.token = "new"
	s.tokenObtainedAt = time.Now()
	if _, err := s.FetchWesternDetail(context.Background(), list.Entries[0].DetailID); err == nil {
		t.Fatal("old visit reference survived")
	}
}
