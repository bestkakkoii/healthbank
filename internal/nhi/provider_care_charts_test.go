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

func TestCareChartSourcesRetainIndependentRowsAndBoundAllSelectors(t *testing.T) {
	for _, tc := range []struct {
		category, tablePath, tableRoot, chartPath, chartRoot, selectors string
	}{
		{"diabetes_care", "/api/ihke3000/ihke3504s02/SP_IHKE3504S02", "sP_IHKE3504S02_Data", "/api/ihke3000/ihke3504s03/SP_IHKE3504S03", "sP_IHKE3504S03_Data", "03,04,05,06,07,08,09,10"},
		{"ckd_care", "/api/ihke3000/ihke3505s02/SP_IHKE3505S02", "sP_IHKE3505S02_Data", "/api/ihke3000/ihke3505s03/SP_IHKE3505S03", "sP_IHKE3505S03_Data", "03,04,05,06,07,08,09"},
		{"hepatitis_care", "/api/ihke3000/ihke3507s02/SP_IHKE3507S02", "sP_IHKE3507S02_Data", "/api/ihke3000/ihke3507s03/SP_IHKE3507S03", "sP_IHKE3507S03_Data", "03,04"},
		{"copd_care", "/api/ihke3000/ihke3513s02/page_load", "sp_IHKE3513S02_Data", "/api/ihke3000/ihke3513s02/get_highchar_val", "sp_IHKE3513S03_Data", "03,04,05,06,07"},
		{"diabetes_ckd_care", "/api/ihke3000/ihke3516s02/SP_IHKE3516S02", "sP_IHKE3516S02_Data", "/api/ihke3000/ihke3516s03/SP_IHKE3516S03", "sP_IHKE3516S03_Data", "03,04,05,06,07,08,09,10,11"},
		{"metabolic_care", "/api/ihke3000/ihke3517s02/SP_IHKE3517S02", "sP_IHKE3517S02_Data", "/api/ihke3000/ihke3517s03/SP_IHKE3517S03", "sP_IHKE3517S03_Data", "01,02,03,04,05,06,07,08,09"},
	} {
		t.Run(tc.category, func(t *testing.T) {
			var chartCalls atomic.Int32
			seen := map[string]bool{}
			s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
				var payload map[string]string
				if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&payload) != nil || payload["rid"] != "PRIVATE-SYNTHETIC-CASE" {
					t.Error("care read lost the bound source case or HTTP contract")
				}
				switch r.URL.Path {
				case tc.tablePath:
					if len(payload) != 1 {
						t.Error("source table used invented selectors")
					}
					respondJSON(t, w, map[string]any{tc.tableRoot: []any{}})
				case tc.chartPath:
					chartCalls.Add(1)
					selector := payload["ItemValue"]
					if len(payload) != 2 || !strings.Contains(","+tc.selectors+",", ","+selector+",") || seen[selector] {
						t.Error("source chart selector was missing, repeated or not publicly observed")
					}
					seen[selector] = true
					row := map[string]any{"casE_DATE": "99/01/02", "valuE1": json.Number("62.000000000000000001"), "valuE2": "<0.000000000000000001", "private_identity": "NEVER-EXPOSE"}
					respondJSON(t, w, map[string]any{tc.chartRoot: []any{row, row}})
				default:
					t.Error("unreviewed care chart route")
					http.NotFound(w, r)
				}
			})
			s.providerReferences = map[string]map[string]providerReference{tc.category: {"parent": {rowID: "PRIVATE-SYNTHETIC-CASE", date: "115/10/01", institution: "合成父層院所", originKey: "synthetic-source-key"}}}
			detail, err := s.FetchProviderDetail(context.Background(), tc.category, "parent")
			if err != nil || len(detail.Entries) != len(strings.Split(tc.selectors, ",")) || chartCalls.Load() != 0 {
				t.Fatalf("source chart indices hidden by an empty table or fetched implicitly: %v", err)
			}
			for _, entry := range detail.Entries {
				if entry.Kind != "source_index" || entry.DetailID == "" || entry.Date != "" {
					t.Fatal("chart index became a clinical observation")
				}
				chart, err := s.FetchProviderDetail(context.Background(), tc.category, entry.DetailID)
				if err != nil || len(chart.Entries) != 2 || chart.Complete || chart.StartDate != "" || chart.EndDate != "" {
					t.Fatalf("source chart independent rows unavailable or range invented: %v", err)
				}
				if chart.Entries[0].Key == chart.Entries[1].Key || chart.Entries[0].Date != "99/01/02" || chart.Entries[0].Institution != "" || chart.Entries[0].DetailID != "" {
					t.Fatal("chart duplicate collapsed, parent context invented or child recursion introduced")
				}
				values := map[string]string{}
				for _, field := range chart.Entries[0].Fields {
					values[field.Code] = field.Value
					if field.Unit != "" {
						t.Fatal("source chart acquired a guessed unit")
					}
				}
				if values["value1"] != "62.000000000000000001" || tc.category != "copd_care" && values["value2"] != "<0.000000000000000001" {
					t.Fatal("source chart raw precision or inequality changed")
				}
				encoded, _ := json.Marshal(chart)
				if strings.Contains(string(encoded), "PRIVATE-") || strings.Contains(string(encoded), "NEVER-EXPOSE") {
					t.Fatal("chart private selector or identity escaped projection")
				}
				if tc.category == "copd_care" && strings.Contains(string(encoded), "支氣管擴張") {
					t.Fatal("a source *_POST field name invented a verified bronchodilator procedure")
				}
			}
			if int(chartCalls.Load()) != len(strings.Split(tc.selectors, ",")) {
				t.Fatal("a reviewed source chart selector was not acquired")
			}
			before := chartCalls.Load()
			if _, err := s.FetchProviderDetail(context.Background(), tc.category, "parent"); err != nil {
				t.Fatal(err)
			}
			if _, err := s.FetchProviderDetail(context.Background(), tc.category, detail.Entries[0].DetailID); err == nil || chartCalls.Load() != before {
				t.Fatal("parent refresh retained a stale source chart handle")
			}
			if s.referenceGroups["provider/"+tc.category] != referenceMapSize(s.providerReferences[tc.category], providerReferenceSize) {
				t.Fatal("chart child replacement changed reference admission accounting")
			}
		})
	}
}

func TestCareChartReferenceAdmissionKeepsSuccessfulParentAndChild(t *testing.T) {
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/ihke3000/ihke3504s02/SP_IHKE3504S02":
			respondJSON(t, w, map[string]any{"sP_IHKE3504S02_Data": []any{}})
		case "/api/ihke3000/ihke3504s03/SP_IHKE3504S03":
			respondJSON(t, w, map[string]any{"sP_IHKE3504S03_Data": []any{map[string]any{"casE_DATE": "99/01/02", "valuE1": "120", "valuE2": "70"}}})
		default:
			http.NotFound(w, r)
		}
	})
	s.providerReferences = map[string]map[string]providerReference{"diabetes_care": {
		"parent":    {rowID: "synthetic-case", children: []string{"old-child"}},
		"old-child": {rowID: "synthetic-case", kind: "care_chart", sourceType: "03", parentReference: "parent"},
	}}
	s.initializeReferenceAccounting()
	before := s.referenceBytes
	s.referenceBudget = func(old, next int64) bool { return next <= old }
	if _, err := s.FetchProviderDetail(context.Background(), "diabetes_care", "parent"); err == nil {
		t.Fatal("chart child expansion bypassed reference admission")
	}
	if s.referenceBytes != before || len(s.providerReferences["diabetes_care"]) != 2 || s.providerReferences["diabetes_care"]["parent"].children[0] != "old-child" {
		t.Fatal("rejected source chart refresh changed successful reference state")
	}
	chart, err := s.FetchProviderDetail(context.Background(), "diabetes_care", "old-child")
	if err != nil || len(chart.Entries) != 1 {
		t.Fatalf("admission rejection retired a previously usable source chart: %v", err)
	}
	if _, err := providerCareChartRequest("diabetes_care", providerReference{rowID: "synthetic-case", kind: "care_chart", sourceType: "arbitrary"}); err == nil {
		t.Fatal("unreviewed chart selector accepted")
	}
}
