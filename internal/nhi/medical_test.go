// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package nhi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// These fixtures contain invented records and the read-only request/field
// contracts observed in the public official bundles. They do not authenticate
// a real person or certify official account completeness.
func TestMedicalCategoryListAndDetails(t *testing.T) {
	for _, tc := range []struct {
		category, listPath, listRoot, searchPath, detailPath, detailRoot, section string
		item                                                                      map[string]any
		wantDate, wantEnd, wantKind                                               string
	}{
		{"medicine", "/api/ihke3000/ihke3306s01/page_load", "sp_IHKE3306S01_data", "/api/ihke3000/ihke3306s01/search", "/api/ihke3000/IHKE3306S02/page_load", "ihke3306S02_main_data", "sp_IHKE3306S03_data_list", map[string]any{"order_code": "SYNTHETIC-M", "drug_name": "合成藥品||Synthetic", "act": "合成分類", "order_qty": "28.00", "order_drug_day": 14}, "115/10/01", "", "medicine"},
		{"chronic", "/api/ihke3000/ihke3307s01/page_load", "sp_IHKE3307S01_data", "", "/api/ihke3000/IHKE3306S02/page_load", "ihke3306S02_main_data", "sp_IHKE3306S03_data_list", map[string]any{"order_code": "SYNTHETIC-M", "drug_name": "合成慢箋", "act": "合成分類", "order_drug_day": "28"}, "115/10/01", "", "medicine"},
		{"western", "/api/ihke3000/ihke3303s01/page_load", "sp_IHKE3303S01_data", "/api/ihke3000/ihke3303s01/search", "/api/ihke3000/ihke3303s02/page_load", "ihke3303S02_main_data", "sp_IHKE3302S04_data", map[string]any{"order_code": "SYNTHETIC-M", "drug_name": "合成西藥", "order_drug_day": "7"}, "115/10/01", "", "medicine"},
		{"chinese", "/api/ihke3000/ihke3304s01/page_load", "sp_IHKE3304S01_data", "/api/ihke3000/ihke3304s01/search", "/api/ihke3000/ihke3303s02/page_load", "ihke3303S02_main_data", "sp_IHKE3302S04_data", map[string]any{"order_code": "SYNTHETIC-TCM", "drug_name": "合成中藥", "order_drug_day": "7"}, "115/10/01", "", "medicine"},
		{"dental", "/api/ihke3000/ihke3305s01/page_load", "sp_IHKE3305S01_data", "/api/ihke3000/ihke3305s01/search", "/api/ihke3000/ihke3305s02/page_load", "ihke3305S02_main_data", "sp_IHKE3305S07", map[string]any{"order_code": "SYNTHETIC-D", "cure_CNAME": "合成牙科醫令", "teethname": "合成牙位", "order_qty": "1"}, "115/10/01", "", "order"},
		{"procedure", "/api/ihke3000/ihke3308s01/page_load", "sp_IHKE3308S01_data", "/api/ihke3000/IHKE3308s01/search", "/api/ihke3000/IHKE3308S02/page_load", "ihke3308S02_main_data", "sp_IHKE3308S04_data_list", map[string]any{"order_CODE": "SYNTHETIC-P", "order_CODE_NAME": "合成手術", "exe_S_DATE": "115/10/02", "code_CNAME": "合成部位", "assay_VALUE": "<3"}, "115/10/02", "", "procedure"},
		{"hospital", "/api/ihke3000/ihke3309s01/page_load", "sp_IHKE3309S01_data", "/api/ihke3000/ihke3309s01/search", "/api/ihke3000/IHKE3309S02/page_load", "ihke3309S02_main_data", "sp_IHKE3302S11_data", map[string]any{"order_code": "SYNTHETIC-H", "drug_NAME": "合成住院藥品", "exe_S_DATE": "115/10/02", "exe_E_DATE": "115/10/03", "order_qty": "DO_NOT_INFER", "order_drug_day": "DO_NOT_INFER"}, "115/10/02", "115/10/03", "medicine"},
	} {
		t.Run(tc.category, func(t *testing.T) {
			var listCalls, searchCalls, detailCalls atomic.Int32
			row := map[string]any{"row_ID": "synthetic-private-row", "ori_TYPE": "SYNTHETIC", "func_DATE": "115/10/01", "in_DATE": "115/09/30", "out_DATE": "115/10/04", "hosp_ABBR": "合成院所", "ori_type_cname": "合成資料來源", "ori_TYPE_NAME": "合成就醫類別", "ID_NO": "DO_NOT_FORWARD"}
			s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer synthetic-medical" {
					t.Error("medical request lost read-only authenticated transport")
				}
				switch r.URL.Path {
				case tc.listPath, tc.searchPath:
					q := r.URL.Query()
					if tc.searchPath == "" {
						if len(q) != 0 {
							t.Error("invented a date filter for the chronic list")
						}
					} else if q.Get("s_date") != "2010-01-01" || q.Get("e_date") != "2026-10-28" || q.Get("s_sort") != "A1" {
						t.Error("explicit medical date range or sort changed")
					}
					if tc.category == "medicine" && q.Get("s_type") != "A" {
						t.Error("medicine query must use the official all-types selector")
					}
					if tc.category != "medicine" && q.Has("s_type") {
						t.Error("unsupported source filter was added")
					}
					if r.URL.Path == tc.listPath {
						listCalls.Add(1)
						respondJSON(t, w, map[string]any{tc.listRoot: []any{row}, "sdate": "2025-01-01", "edate": "2026-10-01"})
					} else {
						searchCalls.Add(1)
						respondJSON(t, w, []any{row, row})
					}
				case tc.detailPath:
					detailCalls.Add(1)
					q := r.URL.Query()
					if len(q) != 2 || q.Get("crid") != "synthetic-private-row" || q.Get("ctype") != "SYNTHETIC" {
						t.Error("detail did not use only the session-bound official row identity")
					}
					respondJSON(t, w, map[string]any{tc.detailRoot: []any{map[string]any{"rowid": "synthetic-private-row", "main_tit": "合成官方標題", "type_name_isr": true, "func_DATE": "115/10/01", "cure_E_DATE": "115/09/28", "appl_S_DATE": "115/09/01", "appl_E_DATE": "115/09/30", tc.section: []any{tc.item}, "ID_NO": "DO_NOT_FORWARD"}}})
				default:
					t.Error("unexpected official endpoint")
					http.NotFound(w, r)
				}
			})
			s.token, s.tokenObtainedAt = "synthetic-medical", time.Now()
			list, err := s.FetchMedical(context.Background(), tc.category, Query{StartDate: "2010-01-01", EndDate: "2026-10-28"})
			if err != nil {
				t.Fatal(err)
			}
			wantRows, wantSearches := 2, int32(1)
			if tc.searchPath == "" {
				wantRows, wantSearches = 1, 0
				if list.StartDate != "" || list.EndDate != "" || !strings.Contains(list.Coverage, "未提供日期搜尋") {
					t.Fatal("chronic response overstated date coverage")
				}
			} else if list.StartDate != "2010-01-01" || list.EndDate != "2026-10-28" {
				t.Fatal("initial dates replaced the explicit search interval")
			}
			if len(list.Entries) != wantRows || listCalls.Load() != 1 || searchCalls.Load() != wantSearches || list.Complete {
				t.Fatal("search rows lost, requests repeated, or completeness fabricated")
			}
			if wantRows == 2 && (list.Entries[0].DetailID == list.Entries[1].DetailID || list.Entries[0].Key == list.Entries[1].Key) {
				t.Fatal("same-date source rows collapsed")
			}
			entry := list.Entries[0]
			wantListDate := "115/10/01"
			if tc.category == "hospital" {
				wantListDate = "115/09/30"
			}
			if len(entry.DetailID) != 48 || entry.Date != wantListDate || entry.EndDate != "115/10/04" {
				t.Fatal("opaque reference or original dates were lost")
			}
			detail, err := s.FetchMedicalDetail(context.Background(), tc.category, entry.DetailID)
			if err != nil {
				t.Fatal(err)
			}
			if len(detail.Entries) != 2 || detailCalls.Load() != 1 || detail.Complete {
				t.Fatal("missing header/order or fabricated detail completeness")
			}
			order := detail.Entries[1]
			if order.Date != tc.wantDate || order.EndDate != tc.wantEnd || order.Kind != tc.wantKind {
				t.Fatalf("medical order date/kind changed: %+v", order)
			}
			if tc.wantKind == "medicine" && (order.Medicine == nil || order.Medicine.Name != tc.item["drug_name"] && tc.category != "hospital") {
				t.Fatal("source medication name missing")
			}
			if tc.category == "medicine" && (order.Medicine.Quantity != "28.00" || order.Medicine.Days != "14") {
				t.Fatal("source quantity precision or numeric day text changed")
			}
			if tc.category == "hospital" && (order.Medicine.Days != "" || order.Medicine.Quantity != "") {
				t.Fatal("inpatient source dates were used to invent dose or days")
			}
			data, err := json.Marshal([]SupplementReport{list, detail})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "DO_NOT_FORWARD") || strings.Contains(string(data), "synthetic-private-row") {
				t.Fatal("unneeded personal/source identifiers were forwarded")
			}
			if tc.category == "medicine" || tc.category == "chronic" {
				if !strings.Contains(string(data), "合成分類") || !strings.Contains(string(data), "調劑日期（原文）") || !strings.HasSuffix(order.Source, "／調劑") {
					t.Fatal("source pharmacology or confirmed dispensing label missing")
				}
			} else if strings.HasSuffix(order.Source, "／調劑") {
				t.Fatal("dispensing flag from another category was applied")
			}
			if tc.category == "hospital" && !strings.Contains(string(data), "申報起日（原文，非入院日期）") {
				t.Fatal("claim dates were discarded or conflated with admission")
			}
		})
	}
}

func TestMedicalDispensingRequiresExplicitSourceFlag(t *testing.T) {
	for _, flag := range []any{nil, false, "true", 1} {
		s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
			respondJSON(t, w, map[string]any{"ihke3306S02_main_data": []any{map[string]any{"rowid": "synthetic-row", "type_name_isr": flag, "func_DATE": "115/10/01", "sp_IHKE3306S03_data_list": []any{map[string]any{"drug_name": "合成藥品"}}}}})
		})
		s.token, s.tokenObtainedAt = "synthetic", time.Now()
		s.medicalReferences = map[string]map[string]medicalReference{"medicine": {"synthetic-reference": {category: "medicine", rowID: "synthetic-row", sourceType: "A"}}}
		report, err := s.FetchMedicalDetail(context.Background(), "medicine", "synthetic-reference")
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range report.Entries {
			if strings.HasSuffix(entry.Source, "／調劑") {
				t.Fatal("unconfirmed source flag became a dispensing event")
			}
			for _, field := range entry.Fields {
				if field.Name == "調劑日期（原文）" {
					t.Fatal("unconfirmed source flag changed date semantics")
				}
			}
		}
	}
}

func TestMedicalReferencesAreCategoryAndSessionBound(t *testing.T) {
	var details atomic.Int32
	s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/ihke3000/IHKE3306S02/page_load" {
			details.Add(1)
			respondJSON(t, w, map[string]any{"ihke3306S02_main_data": []any{map[string]any{"rowid": "synthetic-row", "sp_IHKE3306S03_data_list": []any{}}}})
			return
		}
		respondJSON(t, w, map[string]any{"sp_IHKE3307S01_data": []any{map[string]any{"row_ID": "synthetic-row", "ori_TYPE": "A", "func_DATE": "115/10/01"}}})
	})
	s.token, s.tokenObtainedAt = "synthetic", time.Now()
	list, err := s.FetchMedical(context.Background(), "chronic", Query{})
	if err != nil {
		t.Fatal(err)
	}
	reference := list.Entries[0].DetailID
	for _, tc := range []struct{ category, reference string }{{"medicine", reference}, {"chronic", "synthetic-row"}, {"imaging", reference}} {
		if _, err := s.FetchMedicalDetail(context.Background(), tc.category, tc.reference); err == nil {
			t.Fatal("caller selected an unlisted/cross-category official row")
		}
	}
	if details.Load() != 0 {
		t.Fatal("invalid references reached the official detail endpoint")
	}
	if _, err := s.FetchMedical(context.Background(), "chronic", Query{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FetchMedicalDetail(context.Background(), "chronic", reference); err == nil || details.Load() != 0 {
		t.Fatal("superseded list reference remained valid")
	}
	s.clearAuth()
	s.token, s.tokenObtainedAt = "another-synthetic-session", time.Now()
	if _, err := s.FetchMedicalDetail(context.Background(), "chronic", reference); err == nil || len(s.medicalReferences) != 0 {
		t.Fatal("old medical reference survived authentication reset")
	}
}

func TestMedicalRejectsInvalidDateBeforeRefreshing(t *testing.T) {
	var calls atomic.Int32
	s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "unexpected synthetic request", http.StatusInternalServerError)
	})
	s.token, s.tokenObtainedAt = "synthetic", time.Now().Add(-9*time.Minute)
	_, err := s.FetchMedical(context.Background(), "medicine", Query{StartDate: "2026-02-30", EndDate: "2026-10-28"})
	var upstream *Error
	if !errors.As(err, &upstream) || upstream.Code != "INVALID_INPUT" || calls.Load() != 0 {
		t.Fatal("invalid medical input reached refresh or list endpoint")
	}
}

func TestMedicalRejectsChangedAndExpiredResponses(t *testing.T) {
	for _, tc := range []struct {
		name, body, code string
		clearAuth        bool
	}{
		{"null", `null`, "UPSTREAM_CHANGED", false},
		{"wrong_root", `{"other":[]}`, "UPSTREAM_CHANGED", false},
		{"wrong_row", `{"ihke3306S02_main_data":[{"rowid":"different"}]}`, "UPSTREAM_CHANGED", false},
		{"empty_detail", `{"ihke3306S02_main_data":[]}`, "UPSTREAM_CHANGED", false},
		{"invalid_section", `{"ihke3306S02_main_data":[{"rowid":"synthetic-row","sp_IHKE3306S03_data_list":{}}]}`, "UPSTREAM_CHANGED", false},
		{"expired", `{"error_code":"9999"}`, "SESSION_EXPIRED", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if _, err := w.Write([]byte(tc.body)); err != nil {
					t.Fatal(err)
				}
			})
			s.token, s.tokenObtainedAt = "synthetic", time.Now()
			s.medicalReferences = map[string]map[string]medicalReference{"medicine": {"synthetic-reference": {category: "medicine", rowID: "synthetic-row", sourceType: "A"}}}
			_, err := s.FetchMedicalDetail(context.Background(), "medicine", "synthetic-reference")
			var upstream *Error
			if !errors.As(err, &upstream) || upstream.Code != tc.code {
				t.Fatalf("unexpected medical failure: %v", err)
			}
			if tc.clearAuth && (s.token != "" || len(s.medicalReferences) != 0) {
				t.Fatal("expired medical response retained authentication/references")
			}
		})
	}
}

func TestMedicalNullSearchPreservesPreviousListReferences(t *testing.T) {
	s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/page_load") {
			respondJSON(t, w, map[string]any{"sp_IHKE3306S01_data": []any{}})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte("null")); err != nil {
			t.Fatal(err)
		}
	})
	s.token, s.tokenObtainedAt = "synthetic", time.Now()
	s.medicalReferences = map[string]map[string]medicalReference{"medicine": {"previous-reference": {category: "medicine", rowID: "synthetic-row", sourceType: "A"}}}
	_, err := s.FetchMedical(context.Background(), "medicine", Query{})
	var upstream *Error
	if !errors.As(err, &upstream) || upstream.Code != "UPSTREAM_CHANGED" {
		t.Fatal("null search was treated as a successful empty list")
	}
	if _, ok := s.medicalReferences["medicine"]["previous-reference"]; !ok {
		t.Fatal("failed refresh discarded references belonging to the previous successful list")
	}
}

func TestMedicalDetailLimitIncludesHeaderEntries(t *testing.T) {
	rows := make([]map[string]any, MaxReportRecords)
	for i := range rows {
		rows[i] = map[string]any{"rowid": "synthetic-row"}
	}
	rows[0]["sp_IHKE3306S03_data_list"] = []any{map[string]any{"drug_name": "合成藥品"}}
	s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
		respondJSON(t, w, map[string]any{"ihke3306S02_main_data": rows})
	})
	s.token, s.tokenObtainedAt = "synthetic", time.Now()
	s.medicalReferences = map[string]map[string]medicalReference{"medicine": {"synthetic-reference": {category: "medicine", rowID: "synthetic-row", sourceType: "A"}}}
	_, err := s.FetchMedicalDetail(context.Background(), "medicine", "synthetic-reference")
	var upstream *Error
	if !errors.As(err, &upstream) || upstream.Code != "RESPONSE_TOO_LARGE" {
		t.Fatal("header-only rows exceeded the combined medical detail limit")
	}
}
