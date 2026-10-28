// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestSourceDiseaseAssessmentsKeepEveryNestedEvidenceAndDateMeaning(t *testing.T) {
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/ihke3000/IHKE3723S01/Get_Disease_Data" || r.ContentLength > 0 || r.URL.RawQuery != "" {
			t.Error("assessment read changed into a calculation, write or invented search")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sP_IHKE3723S01_Data":[{"disease":"synthetic","diseasE_NAME":"合成評估","diseasE_STAGE":"0","crT_DATE":"115/10/01","edU_INFO":"<script>synthetic source text</script>","sP_IHKE3723S01_Append_Data":[{"assaY_DATE":"114/02/03","assaY_VALUE":1.000000000000000001,"uniT_DATA":"來源單位","consulT_VALUE_LOWER":"<0.01","hosP_ABBR":"合成院所","hosP_ID":"PRIVATE_MUST_NOT_LEAK"},{"assaY_DATE":"114/02/03","assaY_VALUE":1.000000000000000001,"uniT_DATA":"來源單位"}],"sP_IHKE3723S06_Data":[{"url":"https://PRIVATE_MUST_NOT_LEAK"}]},{"disease":"synthetic","diseasE_NAME":"合成評估","crT_DATE":"115/10/01","sP_IHKE3723S01_Append_Data":[]}]} `))
	})
	report, err := s.FetchProvider(context.Background(), "source_disease_assessments", Query{StartDate: "2026-01-01", EndDate: "2026-01-02"})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Entries) != 2 || report.Entries[0].Key == report.Entries[1].Key || report.StartDate != "" || report.EndDate != "" || report.Complete {
		t.Fatal("source window or duplicate assessment rows changed")
	}
	entry := report.Entries[0]
	if entry.Date != "115/10/01" || entry.DateRole != "recorded" || entry.Origin != "official_summary" {
		t.Fatal("assessment date became an examination or diagnosis")
	}
	results, examDates, referenceFields := 0, 0, 0
	for _, field := range entry.Fields {
		if field.Code == "assay_value" {
			results++
			if field.Value != "1.000000000000000001" || field.Role != "result" || !strings.Contains(field.Name, "關鍵檢驗第 ") {
				t.Fatal("nested result precision or row association lost")
			}
		}
		if field.Code == "assay_date" && field.Value == "114/02/03" && field.Role == "date" {
			examDates++
		}
		if field.Code == "consult_value_lower" && field.Role == "reference" {
			referenceFields++
		}
	}
	if results != 2 || examDates != 2 || referenceFields != 1 {
		t.Fatal("same-date nested evidence was collapsed or omitted")
	}
	for _, field := range report.Entries[1].Fields {
		if field.Code == "disease_stage" {
			t.Fatal("missing assessment stage filled with a synthetic zero")
		}
	}
	encoded, _ := json.Marshal(report)
	if strings.Contains(string(encoded), "PRIVATE_MUST_NOT_LEAK") || !strings.Contains(string(encoded), "synthetic source text") {
		t.Fatal("source text lost or private/link field forwarded")
	}
}

func TestSourceDiabetesSnapshotRequiresExplicitAvailabilityWithoutDefaults(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		wantRows   int
		wantError  bool
	}{
		{"available", `{"hasData":true,"period":"來源期間","grade":"2","dcsi":0,"r1":"0.000","pr1":"0","keyobservationcontent":"<b>來源 AI 文字</b>","name":"PRIVATE_MUST_NOT_LEAK"}`, 1, false},
		{"unavailable", `{"hasData":false,"grade":"2"}`, 0, false},
		{"nullAvailability", `{"hasData":null,"grade":"2"}`, 0, true},
		{"missingAvailability", `{"grade":"2"}`, 0, true},
		{"changedAvailability", `{"hasData":"true","grade":"2"}`, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/ihke3000/ihke3518s01/page_load" || r.URL.RawQuery != "" {
					t.Error("unexpected diabetes source read")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			})
			report, err := s.FetchProvider(context.Background(), "source_diabetes_assessment", Query{})
			if (err != nil) != tc.wantError {
				t.Fatalf("availability validation: %v", err)
			}
			if err != nil {
				return
			}
			if len(report.Entries) != tc.wantRows {
				t.Fatal("availability fabricated a source record")
			}
			if tc.wantRows == 0 {
				return
			}
			entry := report.Entries[0]
			if entry.Date != "" || entry.Origin != "official_summary" || !strings.Contains(entry.Source, "官方 AI") {
				t.Fatal("source period or AI origin misrepresented")
			}
			for _, field := range entry.Fields {
				if field.Code == "p4p" || field.Code == "ht" || field.Code == "r2" {
					t.Fatal("source missing value replaced with absence/zero")
				}
			}
			encoded, _ := json.Marshal(report)
			if strings.Contains(string(encoded), "PRIVATE_MUST_NOT_LEAK") || !strings.Contains(string(encoded), "0.000") {
				t.Fatal("precise source code lost or identity leaked")
			}
		})
	}
}

func TestAnnualOverviewAndCollectionEventsRetainSourceSemantics(t *testing.T) {
	for _, tc := range []struct{ category, path, body, date, code, value, unit string }{
		{"annual_overview", "/api/ihke3000/ihke3302s01/page_load", `{"maindata":[{"year":"113","cnt":"0","o_cnt":"2","part_AMT":"0010","radi_MSV":"0.000000000000000001"}]}`, "", "radi_msv", "0.000000000000000001", "mSv"},
		{"mask_collections", "/api/ihke3000/ihke3702s01/SP_IHKE3702S01", `{"sP_IHKE3702S01":[{"purchasE_DATE":"109/04/01","paY_DATE":"109/03/30","qty":0,"childreN_QTY":"9","hosP_ABBR":"合成地點"}]}`, "109/04/01", "qty", "0", "片"},
		{"rapid_test_collections", "/api/ihke3000/ihke3706s01/pageload", `{"ihke3706S01_maindata":[{"purchase_DATE":"111/06/01","firm_NAME":"合成廠牌","show_0to6":false,"show_up65":true}]}`, "111/06/01", "show_0to6", "false", ""},
	} {
		t.Run(tc.category, func(t *testing.T) {
			s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != tc.path || r.URL.RawQuery != "" {
					t.Error("source-only list got an invented date filter")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			})
			report, err := s.FetchProvider(context.Background(), tc.category, Query{})
			if err != nil {
				t.Fatal(err)
			}
			if len(report.Entries) != 1 || report.Entries[0].Date != tc.date || report.Entries[0].Origin != "official_summary" || report.Complete {
				t.Fatal("summary/collection semantics changed")
			}
			found := false
			for _, field := range report.Entries[0].Fields {
				if field.Code == tc.code {
					found = field.Value == tc.value && field.Unit == tc.unit && field.Role == ""
				}
			}
			if !found {
				t.Fatal("source scalar lost, rounded, or misclassified as clinical test result")
			}
		})
	}
}

func TestCoverageRejectsMalformedNestedRowsAndCollectionFlags(t *testing.T) {
	for _, tc := range []struct{ category, body string }{
		{"source_disease_assessments", `{"sP_IHKE3723S01_Data":[{"diseasE_NAME":"合成","sP_IHKE3723S01_Append_Data":{}}]}`},
		{"source_disease_assessments", `{"sP_IHKE3723S01_Data":[{"diseasE_NAME":"合成","sP_IHKE3723S01_Append_Data":[{"unknown":"changed"}]}]}`},
		{"rapid_test_collections", `{"ihke3706S01_maindata":[{"purchase_DATE":"111/06/01","show_0to6":"false"}]}`},
	} {
		s := providerFixture(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(tc.body))
		})
		if _, err := s.FetchProvider(context.Background(), tc.category, Query{}); err == nil {
			t.Fatalf("%s accepted a changed source contract", tc.category)
		}
	}
}
func TestSourceDiseaseMissingEvidenceResultsStayExplicit(t *testing.T) {
	s := providerFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(t, w, map[string]any{"sP_IHKE3723S01_Data": []any{map[string]any{"diseasE_NAME": "合成來源評估", "sP_IHKE3723S01_Append_Data": []any{
			map[string]any{"assaY_NAME": "合成缺值檢驗", "assaY_VALUE": nil},
			map[string]any{"assaY_NAME": "合成缺值檢驗"},
			map[string]any{"assaY_NAME": "合成零值檢驗", "assaY_VALUE": "0"},
		}}}})
	})
	report, err := s.FetchProvider(context.Background(), "source_disease_assessments", Query{})
	if err != nil || len(report.Entries) != 1 {
		t.Fatal("source assessment evidence rows were dropped", err)
	}
	missing := 0
	for _, diagnostic := range report.Diagnostics {
		if diagnostic.Code == "known_result_missing" && diagnostic.Section == "nhi.source_disease_assessments.sp_ihke3723s01_append_data" && diagnostic.Count != nil {
			missing += *diagnostic.Count
		}
	}
	if missing != 2 {
		t.Fatal("nested absent/null source results lacked an explicit warning or zero was treated as absent")
	}
	if len(report.Entries[0].Fields) != 5 {
		t.Fatal("missing-result diagnostics changed the retained evidence fields")
	}
}
