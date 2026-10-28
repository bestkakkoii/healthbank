// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRecentSummariesRetainDatesAndBoundMedicalDetails(t *testing.T) {
	for _, tc := range []struct{ category, list, detail, root string }{
		{"recent_medicine", "/api/ihke3000/ihke3301s03/page_load", "/api/ihke3000/IHKE3306S02/page_load", "ihke3306S02_main_data"},
		{"recent_chronic", "/api/ihke3000/ihke3301s04/page_load", "/api/ihke3000/IHKE3306S02/page_load", "ihke3306S02_main_data"},
		{"recent_procedure", "/api/ihke3000/ihke3301s05/page_load", "/api/ihke3000/IHKE3308S02/page_load", "ihke3308S02_main_data"},
		{"recent_hospital", "/api/ihke3000/ihke3301s06/page_load", "/api/ihke3000/IHKE3309S02/page_load", "ihke3309S02_main_data"},
	} {
		t.Run(tc.category, func(t *testing.T) {
			var calls atomic.Int32
			large := false
			s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path == tc.list {
					if r.Method != http.MethodGet || r.URL.RawQuery != "" {
						t.Error("invented date selector on recent snapshot")
					}
					row := map[string]any{"row_id": "PRIVATE_RECENT", "ori_type": "A", "func_date": "104/02/03", "in_date": "104/02/01", "out_date": "104/02/05", "hosp_abbr": "合成院所", "icd9cm_name": "來源分類"}
					rows := []any{row, row}
					if large {
						rows = append(rows, row, row)
					}
					object := map[string]any{"main_data": rows}
					if tc.category == "recent_medicine" {
						object["chinese_data"] = []any{row}
					}
					respondJSON(t, w, object)
					return
				}
				if r.URL.Path != tc.detail || r.Method != http.MethodGet || len(r.URL.Query()) != 2 || r.URL.Query().Get("crid") != "PRIVATE_RECENT" || r.URL.Query().Get("ctype") != "A" {
					t.Error("unbound or cross-category detail read")
				}
				respondJSON(t, w, map[string]any{tc.root: []any{map[string]any{"rowid": "PRIVATE_RECENT", "func_DATE": "104/02/03", "in_DATE": "104/02/01", "out_DATE": "104/02/05", "hosp_ABBR": "合成院所"}}})
			})
			list, err := s.FetchSupplement(context.Background(), tc.category, Query{})
			want := 2
			if tc.category == "recent_medicine" {
				want++
			}
			if err != nil || len(list.Entries) != want || list.Entries[0].DetailID == "" || list.Entries[0].Key == list.Entries[1].Key || list.Entries[0].EndDate != "104/02/05" {
				t.Fatalf("recent source context/duplicates/details lost: %v", err)
			}
			if tc.category == "recent_hospital" && (list.Entries[0].Date != "104/02/01" || list.Entries[0].DateRole != "admission" || list.Entries[0].EndDateRole != "discharge") {
				t.Fatal("admission was replaced with another source date")
			}
			encoded, _ := json.Marshal(list)
			if strings.Contains(string(encoded), "PRIVATE_RECENT") {
				t.Fatal("upstream record selector escaped opaque handle")
			}
			old := list.Entries[0].DetailID
			before := calls.Load()
			other := "recent_hospital"
			if tc.category == other {
				other = "recent_medicine"
			}
			if _, err := s.FetchRecentMedicalDetail(context.Background(), other, old); err == nil || calls.Load() != before {
				t.Fatal("cross-owner recent handle accepted")
			}
			detail, err := s.FetchRecentMedicalDetail(context.Background(), tc.category, old)
			if err != nil || len(detail.Entries) == 0 || detail.Category != tc.category+"_detail" {
				t.Fatalf("source-bound recent detail unavailable: %v", err)
			}
			previous := s.referenceBytes
			s.referenceBudget = func(previous, next int64) bool { return next <= previous }
			large = true
			if _, err := s.FetchSupplement(context.Background(), tc.category, Query{}); err == nil || s.referenceBytes != previous {
				t.Fatal("over-budget recent refresh replaced prior tree")
			}
			if _, err := s.FetchRecentMedicalDetail(context.Background(), tc.category, old); err != nil {
				t.Fatal("rejected refresh damaged old details", err)
			}
			large = false
			if _, err := s.FetchSupplement(context.Background(), tc.category, Query{}); err != nil {
				t.Fatal(err)
			}
			before = calls.Load()
			if _, err := s.FetchRecentMedicalDetail(context.Background(), tc.category, old); err == nil || calls.Load() != before {
				t.Fatal("old recent handle survived refresh")
			}
		})
	}
}
