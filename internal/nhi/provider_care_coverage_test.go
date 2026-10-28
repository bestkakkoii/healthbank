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

func TestCarePlanSnapshotsPreserveAllRowsRawValuesAndSourceReferences(t *testing.T) {
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/ihke3000/IHKE3213S01/page_load" || r.URL.RawQuery != "" {
			t.Error("care-plan snapshot used an invented search or calculation")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"myplan":[{"mhbt_name":"合成照護計畫","case_date":"112/02/03","close_date":"113/04/05","hosp_abbr":"合成院所","hospurl":"PRIVATE_MUST_NOT_LEAK"}],"physiology":[{"code_name":"合成收縮壓","value":120.000000000000001,"unit_data":"mmHg","consult_min_value":"<1.000","consult_max_value":"140.000"},{"code_name":"合成缺值","value":null,"unit_data":"來源單位"}],"bloodsugar":[{"assaY_VALUE":"5.9000%"},{"assaY_VALUE":"5.9000%"},{"assaY_VALUE":null}],"booldlipid":[{"curE_CNAME":"合成血脂","assaY_VALUE":"<0.0001","uniT_DATA":"mg/dL","consult":"來源原文 <99.00","ordeR_CODE":"SYNTHETIC_TEST"},{"curE_CNAME":"合成血脂","assaY_VALUE":"<0.0001","uniT_DATA":"mg/dL"}]}`))
	})
	report, err := s.FetchProvider(context.Background(), "care_plans", Query{StartDate: "2026-01-01", EndDate: "2026-02-01"})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Entries) != 8 || report.StartDate != "" || report.EndDate != "" || report.Complete {
		t.Fatal("source summary rows or source-only date scope changed")
	}
	keys := map[string]bool{}
	for index, entry := range report.Entries {
		if keys[entry.Key] || entry.Origin != "official_summary" {
			t.Fatal("duplicate source rows collapsed or snapshot origin changed")
		}
		keys[entry.Key] = true
		if index > 0 && (entry.Date != "" || entry.DateRole != "" || entry.Institution != "") {
			t.Fatal("snapshot inherited enrollment date or institution without source evidence")
		}
	}
	if report.Entries[0].Date != "112/02/03" || report.Entries[0].EndDate != "113/04/05" || report.Entries[0].DateRole != "period_start" || report.Entries[0].EndDateRole != "period_end" {
		t.Fatal("enrollment period semantics changed")
	}
	if values := personalFieldValues(report, "value"); len(values) != 1 || values[0] != "120.000000000000001" {
		t.Fatal("source number precision was rounded")
	}
	if values := personalFieldValues(report, "assay_value"); len(values) != 4 || values[0] != "5.9000%" || values[1] != "5.9000%" || values[2] != "<0.0001" || values[3] != "<0.0001" {
		t.Fatal("source unit text, inequality or repeated source result was changed")
	}
	for _, field := range report.Entries[1].Fields {
		if strings.HasPrefix(field.Code, "consult_") && field.Role != "reference" {
			t.Fatal("source reference bound lost its field role")
		}
	}
	missing := map[string]int{}
	for _, diagnostic := range report.Diagnostics {
		if diagnostic.Code == "known_result_missing" && diagnostic.Count != nil {
			missing[diagnostic.Section] = *diagnostic.Count
		}
		if diagnostic.Code == "section_empty" && diagnostic.Section == "nhi.care_plans.bloodsugar" {
			t.Fatal("a null-result source row was misrepresented as an empty array")
		}
	}
	if missing["nhi.care_plans.physiology"] != 1 || missing["nhi.care_plans.bloodsugar"] != 1 || len(report.Entries[5].Fields) != 0 {
		t.Fatal("missing results were dropped or fabricated")
	}
	encoded, _ := json.Marshal(report)
	if strings.Contains(string(encoded), "PRIVATE_MUST_NOT_LEAK") || strings.Contains(string(encoded), "4-6%") {
		t.Fatal("external source URL or front-end-only HbA1c threshold escaped")
	}
}

func TestFamilyFollowupSnapshotsRetainNestedResultsIndependentReferencesAndNotes(t *testing.T) {
	for _, category := range []string{"family_followup", "family_health_followup"} {
		t.Run(category, func(t *testing.T) {
			module := "3721"
			if category == "family_health_followup" {
				module = "3726"
			}
			indexRoot, nestedRoot, referenceRoot := "sP_IHKE"+module+"S02", "sP_IHKE"+module+"S04", "sP_IHKE"+module+"S03"
			path := "/api/ihke3000/ihke" + module + "s01/SP_IHKE" + module
			var detailCalls atomic.Int32
			s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case path + "S01":
					if r.Method != http.MethodGet || r.URL.RawQuery != "" {
						t.Error("source snapshot used invented search parameters")
					}
					result := map[string]any{"assaY_DATE": "87/02/03", "assaY_NAME": "SYNTHETIC-A", "assaY_VALUE": json.Number("70.000000000000000001"), "hosP_ABBR": "合成院所", "assaY_SOURCE_NAME": "合成來源", "hosP_ID": "PRIVATE-HOSPITAL-MUST-NOT-LEAK"}
					body := map[string]any{
						"hasillCategory": false, "title": "合成來源對象", "non_case_desc": "<b>來源未適用原文</b>",
						"memo": []string{"重複合成註記", "重複合成註記", ""}, "health_edu": []string{`<a href="javascript:source">來源原文</a>`},
						indexRoot:     []any{map[string]any{"assaY_NAME": "SYNTHETIC-A", "assaY_ZH_NAME": "合成追蹤甲", "nexT_CHK_DATE": "115/12/01", nestedRoot: []any{result, result, map[string]any{"assaY_NAME": "SYNTHETIC-DIFFERENT", "assaY_DATE": "88/01/01", "assaY_VALUE": "<0.00001"}, map[string]any{"assaY_VALUE": nil}, map[string]any{"assaY_VALUE": json.Number("0")}, map[string]any{"assaY_DATE": "89/01/01"}}}},
						referenceRoot: []any{map[string]any{"diseasE_STAGE_NAME": "合成來源分類", "assaY_NAME": "SYNTHETIC-UNMATCHED", "assaY_ZH_NAME": "不同參考項目", "consulT_VALUE_MAX": "<9.000", "consulT_VALUE_LOWER": json.Number("0.000000000000000001"), "uniT_DATA": "SOURCE-REFERENCE-UNIT"}, map[string]any{"uniT_DATA": nil}, map[string]any{"assaY_NAME": "SYNTHETIC-EXTRA"}},
					}
					if category == "family_health_followup" {
						body["bp_IHKE3726S02"] = map[string]any{"assaY_NAME": "合成血壓顯示名", "nexT_CHK_DATE": "115/12/02", "chk_memo": "來源註記", nestedRoot: []any{map[string]any{"assaY_NAME": "合成血壓顯示名", "assaY_DATE": "87/02/04", "c_ASSAY_VALUE": "120.0000/70.0000 <b>原文</b>"}}}
						body["bp_IHKE3726S03"] = map[string]any{"diseasE_STAGE_NAME": nil, "consulT_VALUE_MAX": "SOURCE-BP-LIMIT", "uniT_DATA": "SOURCE-BP-UNIT"}
					}
					respondJSON(t, w, body)
				case path + "S07":
					detailCalls.Add(1)
					var payload map[string]string
					if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&payload) != nil || len(payload) != 3 || payload["dStr"] != "2000-01-01" || payload["dEnd"] != "2002-12-31" || payload["dType"] != "SYNTHETIC-A" && payload["dType"] != "EBP" {
						t.Error("bound source history selector or requested period changed")
					}
					respondJSON(t, w, map[string]any{"sP_IHKE" + module + "S07_Data": []any{map[string]any{"assaY_DATE": "90/02/03", "assaY_VALUE": "999.0001"}}})
				default:
					t.Error("unexpected family read route")
				}
			})
			report, err := s.FetchProvider(context.Background(), category, Query{StartDate: "2000-01-01", EndDate: "2002-12-31"})
			if err != nil {
				t.Fatal(err)
			}
			if report.Complete || report.StartDate != "2000-01-01" || report.EndDate != "2002-12-31" {
				t.Fatal("source snapshots invented completeness or overwrote the requested history period")
			}
			keys, resultCount, references, missing := map[string]bool{}, 0, 0, 0
			for _, entry := range report.Entries {
				if keys[entry.Key] {
					t.Fatal("independent source rows collapsed")
				}
				keys[entry.Key] = true
				if entry.ContractID == sourceContractID(category, nestedRoot) {
					resultCount++
					for _, field := range entry.Fields {
						if field.Unit != "" || field.Code == "unit_data" || field.Role == "reference" || field.Value == "SOURCE-REFERENCE-UNIT" || field.Value == "SOURCE-BP-UNIT" {
							t.Fatal("source reference row was assigned to an unverified clinical row")
						}
					}
					if entry.Date != "" && entry.DateRole != "examination" || entry.Origin != "mixed" {
						t.Fatal("nested examination provenance changed")
					}
				}
				if entry.ContractID == sourceContractID(category, referenceRoot) {
					references++
				}
				if entry.Kind == "source_metadata" && (entry.Date != "" || entry.EndDate != "" || entry.DateRole != "" || entry.Institution != "") {
					t.Fatal("source context became a clinical measurement")
				}
				if entry.Kind == "source_index" {
					if entry.Date != "" || entry.DetailID == "" {
						t.Fatal("index lost history reference or fabricated an observation date")
					}
					detail, err := s.FetchProviderDetail(context.Background(), category, entry.DetailID)
					if err != nil || len(detail.Entries) != 1 || detail.Entries[0].Date != "90/02/03" || detail.StartDate != report.StartDate {
						t.Fatalf("independent requested history changed: %v", err)
					}
				}
			}
			wantResults, wantCalls := 6, int32(1)
			if category == "family_health_followup" {
				wantResults, wantCalls = 7, 2
			}
			if resultCount != wantResults || references != 3 || detailCalls.Load() != wantCalls {
				t.Fatal("default results, unmatched references, or fixed pressure history were omitted")
			}
			for _, diagnostic := range report.Diagnostics {
				if diagnostic.Code == "known_result_missing" && diagnostic.Section == sourceContractID(category, nestedRoot) && diagnostic.Count != nil {
					missing = *diagnostic.Count
				}
			}
			if missing != 2 {
				t.Fatal("known null/absent results were dropped or an explicit zero became missing")
			}
			if values := personalFieldValues(report, "assay_value"); len(values) != 4 || values[0] != "70.000000000000000001" || values[1] != values[0] || values[2] != "<0.00001" || values[3] != "0" {
				t.Fatal("nested precision, repeated values, or unmatched-name result changed")
			}
			if notes := personalFieldValues(report, "memo"); len(notes) != 3 || notes[0] != notes[1] || notes[2] != "" {
				t.Fatal("source notes were filtered or deduplicated")
			}
			if notes := personalFieldValues(report, "health_edu"); len(notes) != 1 || notes[0] != `<a href="javascript:source">來源原文</a>` {
				t.Fatal("inert source text was rewritten")
			}
			if values := personalFieldValues(report, "hasillcategory"); len(values) != 1 || values[0] != "false" {
				t.Fatal("source visibility state suppressed actual returned records")
			}
			encoded, _ := json.Marshal(report)
			if strings.Contains(string(encoded), "PRIVATE-HOSPITAL-MUST-NOT-LEAK") || strings.Contains(string(encoded), "999.0001") {
				t.Fatal("private navigation selector escaped or independent history was merged into the snapshot")
			}
		})
	}
}

func TestFamilyFollowupNullableAndChangedShapes(t *testing.T) {
	for _, tc := range []struct {
		name, body, code string
		wantError        bool
	}{
		{"unknownResponse", `{"unknown":[]}`, "", true},
		{"booleanString", `{"hasillCategory":"false","sP_IHKE3721S02":[]}`, "", true},
		{"unknownNested", `{"sP_IHKE3721S02":[{"assaY_NAME":"SYNTHETIC","sP_IHKE3721S04":[{"unknown":1}]}]}`, "", true},
		{"booleanResult", `{"sP_IHKE3721S02":[{"assaY_NAME":"SYNTHETIC","sP_IHKE3721S04":[{"assaY_VALUE":false}]}]}`, "", true},
		{"unknownReference", `{"sP_IHKE3721S03":[{"unknown":"changed"}]}`, "", true},
		{"numericNote", `{"memo":[1]}`, "", true},
		{"unknownNote", `{"health_edu":[{"text":"changed"}]}`, "", true},
		{"nullResult", `{"sP_IHKE3721S02":[{"assaY_NAME":"SYNTHETIC","sP_IHKE3721S04":[{"assaY_VALUE":null}]}]}`, "known_result_missing", false},
		{"emptyResult", `{"sP_IHKE3721S02":[{"assaY_NAME":"SYNTHETIC","sP_IHKE3721S04":[{"assaY_VALUE":""}]}]}`, "known_result_missing", false},
		{"nullNested", `{"sP_IHKE3721S02":[{"assaY_NAME":"SYNTHETIC","sP_IHKE3721S04":null}]}`, "section_null", false},
		{"emptyNested", `{"sP_IHKE3721S02":[{"assaY_NAME":"SYNTHETIC","sP_IHKE3721S04":[]}]}`, "section_empty", false},
		{"falseStateOnly", `{"hasillCategory":false}`, "", false},
		{"nullState", `{"hasillCategory":null}`, "section_null", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := providerFixture(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			})
			report, err := s.FetchProvider(context.Background(), "family_followup", Query{})
			if (err != nil) != tc.wantError {
				t.Fatalf("family source shape handling changed: %v", err)
			}
			if err != nil || tc.code == "" {
				return
			}
			for _, diagnostic := range report.Diagnostics {
				if diagnostic.Code == tc.code && (diagnostic.Section == "nhi.family_followup.sp_ihke3721s04" || diagnostic.Section == "nhi.family_followup.hasillcategory") {
					return
				}
			}
			t.Fatal("source empty/null/missing result distinction was lost")
		})
	}
}

func TestFamilyFollowupChangedNestedRefreshKeepsPreviousReference(t *testing.T) {
	var changed atomic.Bool
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "S07") {
			respondJSON(t, w, map[string]any{"sP_IHKE3721S07_Data": []any{map[string]any{"assaY_VALUE": "1.0001"}}})
			return
		}
		nested := map[string]any{"assaY_VALUE": "1.0000"}
		if changed.Load() {
			nested = map[string]any{"unknown_result": "schema changed"}
		}
		respondJSON(t, w, map[string]any{"sP_IHKE3721S02": []any{map[string]any{"assaY_NAME": "SYNTHETIC-INDEX", "sP_IHKE3721S04": []any{nested}}}})
	})
	report, err := s.FetchProvider(context.Background(), "family_followup", Query{})
	if err != nil {
		t.Fatal(err)
	}
	reference := report.Entries[0].DetailID
	changed.Store(true)
	if _, err := s.FetchProvider(context.Background(), "family_followup", Query{}); err == nil {
		t.Fatal("unknown nested response was accepted")
	}
	if _, err := s.FetchProviderDetail(context.Background(), "family_followup", reference); err != nil {
		t.Fatal("failed nested refresh retired the previously admitted history reference")
	}
}

func TestCareSourceTeamNamesAndWindowRemainSourceContext(t *testing.T) {
	for _, category := range []string{"home_care", "copd_care"} {
		t.Run(category, func(t *testing.T) {
			s := providerFixture(t, func(w http.ResponseWriter, _ *http.Request) {
				if category == "home_care" {
					respondJSON(t, w, map[string]any{"hasdata": true, "c_CASE_DATE": "112/01/01", "c_CLOSE_DATE": "113/01/01", "c_HOSP_ABBR": "合成收案院所", "c_MAIN_PRSN_NAME": "合成醫師", "c_NURSE_NAME": "合成護理師", "c_BRE_PRSN_HOSP_NAME": "合成治療師", "c_MAIN_PRSN_HOSP_url": "PRIVATE-URL-MUST-NOT-LEAK"})
					return
				}
				respondJSON(t, w, map[string]any{"main_Data": []any{}, "sdate": "112/01/01", "edate": "115/01/01"})
			})
			report, err := s.FetchProvider(context.Background(), category, Query{})
			if err != nil || len(report.Entries) != 1 {
				t.Fatalf("source context omitted: %v", err)
			}
			if category == "home_care" {
				for _, code := range []string{"c_main_prsn_name", "c_nurse_name", "c_bre_prsn_hosp_name"} {
					if len(personalFieldValues(report, code)) != 1 {
						t.Fatal("source care-team member omitted")
					}
				}
			} else if entry := report.Entries[0]; entry.Kind != "source_metadata" || entry.Date != "" || entry.EndDate != "" || entry.DateRole != "" || entry.EndDateRole != "" || len(entry.Fields) != 2 {
				t.Fatal("source availability window became an examination or enrollment period")
			}
			encoded, _ := json.Marshal(report)
			if strings.Contains(string(encoded), "PRIVATE-URL-MUST-NOT-LEAK") {
				t.Fatal("source navigation URL escaped")
			}
		})
	}
}

func TestCarePlanSnapshotUnknownNullAndEmptyStayDistinct(t *testing.T) {
	for _, tc := range []struct {
		name, body, code string
		rows             int
		wantError        bool
	}{
		{"unknown", `{"unknown":[]}`, "", 0, true},
		{"unknownResult", `{"bloodsugar":[{"unknown":"changed"}]}`, "", 0, true},
		{"invalidResult", `{"bloodsugar":[{"assaY_VALUE":true}]}`, "", 0, true},
		{"nullSection", `{"bloodsugar":null}`, "section_null", 0, false},
		{"emptySection", `{"bloodsugar":[]}`, "section_empty", 0, false},
		{"nullResult", `{"bloodsugar":[{"assaY_VALUE":null}]}`, "known_result_missing", 1, false},
		{"emptyResult", `{"bloodsugar":[{"assaY_VALUE":""}]}`, "known_result_missing", 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := providerFixture(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			})
			report, err := s.FetchProvider(context.Background(), "care_plans", Query{})
			if (err != nil) != tc.wantError {
				t.Fatalf("changed care snapshot shape: %v", err)
			}
			if err != nil {
				return
			}
			if len(report.Entries) != tc.rows {
				t.Fatal("nullable source row count changed")
			}
			found := false
			for _, diagnostic := range report.Diagnostics {
				if diagnostic.Section == "nhi.care_plans.bloodsugar" && diagnostic.Code == tc.code {
					found = true
				}
			}
			if !found {
				t.Fatal("source empty/null/missing result diagnostic omitted")
			}
		})
	}
}
