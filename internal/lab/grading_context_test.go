// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package lab

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestGradingContextsExposeActualBasisAndUnverifiedApplicability(t *testing.T) {
	catalog := gradingCatalog(t)
	for _, tc := range []struct {
		id, value, reference, unit, kind, ruleID, note string
	}{
		{"serum_triglycerides", "180", "[0][150]", "", "profile", "adult_triglycerides", "年齡與空腹狀態未確認"},
		{"egfr", "70", "", "mL/min/1.73m2", "profile", "egfr_categories", "來源估算公式與慢性病程未確認"},
		{"urine_acr_semiquantitative", ">=300", "", "", "profile", "acr_categories", "採檢條件與異常持續性未確認"},
		{"urine_pcr", "160", "", "", "profile", "pcr_categories", "兒童正常範圍不同"},
		{"serum_bun", "9", "[7][25]", "", "source_reference", "report_reference", "未獨立驗證"},
		{"urine_protein_qualitative", "Trace", "", "", "qualitative", "qualitative_reference", "方法及採檢條件"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			settings := DefaultSettings()
			raw := syntheticMeasurement(t, catalog, tc.id, tc.value, tc.reference)
			if tc.unit != "" {
				raw.Unit = tc.unit
			}
			row := normalizedOne(t, catalog, raw, &settings)
			if len(row.GradingContexts) != 1 {
				t.Fatalf("want one actual base context, got %+v", row.GradingContexts)
			}
			context := row.GradingContexts[0]
			if context.Kind != tc.kind || context.RuleID != tc.ruleID || context.Label == "" || !strings.Contains(context.Note, tc.note) {
				t.Fatalf("missing applied profile qualification: %+v", context)
			}
			if row.Result != raw.Result || row.ReferenceText != raw.Reference {
				t.Fatal("qualification changed source values")
			}
		})
	}
}

func TestGradingContextsReplacePersonalBaseAndPreserveIndependentAlerts(t *testing.T) {
	catalog := gradingCatalog(t)
	for _, value := range []string{"180", "1000", ">=500"} {
		t.Run(value, func(t *testing.T) {
			settings := DefaultSettings()
			settings.Overrides["serum_triglycerides"] = Override{Enabled: true, Bands: []Band{{Lower: pointer("0"), LowerInclusive: true, Level: 1}}}
			raw := syntheticMeasurement(t, catalog, "serum_triglycerides", value, "[0][150]")
			raw.OfficialFlag = "SYNTHETIC_FLAG"
			row := normalizedOne(t, catalog, raw, &settings)
			wantLevel, wantContexts := 1, 1
			if value == "1000" {
				wantLevel, wantContexts = 5, 2
			} else if value == ">=500" {
				wantLevel, wantContexts = 4, 2
			}
			if row.Level == nil || *row.Level != wantLevel || len(row.GradingContexts) != wantContexts {
				t.Fatalf("personal base or independent alert changed: %+v", row.GradingContexts)
			}
			if row.GradingContexts[0].Kind != "personal" || row.GradingContexts[0].RuleID != "personal" {
				t.Fatal("replaced adult base context remains attached to a personal grade")
			}
			if wantContexts == 2 {
				context := row.GradingContexts[1]
				if context.Kind != "independent_alert" || context.RuleID != "adult_triglycerides" || !strings.Contains(context.Label, "獨立提醒") || !strings.Contains(context.Note, "未確認") {
					t.Fatal("independent adult alert lost its unverified-applicability qualification")
				}
			}
			if row.ReferenceStatus != "high" || row.OfficialFlag != raw.OfficialFlag || row.Result != raw.Result {
				t.Fatal("context metadata changed immutable source status or values")
			}
			document, err := settings.Document()
			if err != nil || strings.Contains(string(document), "grading_contexts") || strings.Contains(string(document), "SYNTHETIC_FLAG") {
				t.Fatal("derived interpretation or source observation entered settings")
			}
		})
	}
}

func TestGradingContextsDoNotPromoteScenarioAgeOrUnknownUnits(t *testing.T) {
	catalog := gradingCatalog(t)
	raw := syntheticMeasurement(t, catalog, "serum_triglycerides", "180", "[0][150]")
	initial := DefaultSettings()
	baseline := normalizedOne(t, catalog, raw, &initial)
	for _, age := range []int{1, 10, 40, 100} {
		settings := DefaultSettings()
		settings.Analysis = &AnalysisPreferences{Mode: "primary", Primary: AnalysisScenario{Age: pointer(age), Sex: "female"}, Comparison: AnalysisScenario{Age: pointer(30), Sex: "male"}}
		row := normalizedOne(t, catalog, raw, &settings)
		if !reflect.DeepEqual(row.GradingContexts, baseline.GradingContexts) || !reflect.DeepEqual(row.Level, baseline.Level) {
			t.Fatal("hypothetical scenario age was promoted into verified patient applicability")
		}
	}
	settings := DefaultSettings()
	unitless := syntheticMeasurement(t, catalog, "egfr", "25", "")
	row := normalizedOne(t, catalog, unitless, &settings)
	if row.Level != nil || len(row.GradingContexts) != 0 {
		t.Fatal("unverified eGFR unit acquired an applied profile context")
	}
	unitless.Reference = "[30][90]"
	row = normalizedOne(t, catalog, unitless, &settings)
	if len(row.GradingContexts) != 1 || row.GradingContexts[0].Kind != "source_reference" || row.Level == nil || *row.Level != 3 {
		t.Fatal("missing profile units erased the independent source reference context")
	}
	for _, tc := range []struct{ id, value string }{{"manual_smear_status", "0"}, {"serum_triglycerides", "-1"}, {"serum_triglycerides", "-"}} {
		row = normalizedOne(t, catalog, syntheticMeasurement(t, catalog, tc.id, tc.value, ""), &settings)
		if row.Level != nil || len(row.GradingContexts) != 0 {
			t.Fatal("metadata, invalid or missing result acquired a grading context")
		}
	}
}

func TestGradingConditionsSurvivePublicCatalogSerialization(t *testing.T) {
	catalog := gradingCatalog(t)
	response := catalog.ForSettings(DefaultSettings())
	var decoded struct {
		Items []struct {
			ID   string `json:"id"`
			Rule Rule   `json:"default_rule"`
		} `json:"items"`
	}
	data, err := json.Marshal(response)
	if err != nil || json.Unmarshal(data, &decoded) != nil {
		t.Fatal("catalog metadata serialization failed")
	}
	found := false
	for _, item := range decoded.Items {
		if item.ID != "serum_triglycerides" {
			continue
		}
		found = true
		if !reflect.DeepEqual(item.Rule.Conditions, catalog.Rules["adult_triglycerides"].Conditions) || len(item.Rule.Conditions) == 0 || !strings.Contains(item.Rule.ScopeLabel, "條件未確認") || item.Rule.ApplicabilityNote == "" {
			t.Fatal("public catalog discarded source applicability conditions")
		}
	}
	if !found {
		t.Fatal("built-in TG profile absent")
	}
	for id, rule := range catalog.Rules {
		if rule.Kind != "unclassified" && (rule.ScopeLabel == "" || rule.ApplicabilityNote == "") {
			t.Fatalf("clinical rule %s lacks explicit scope metadata", id)
		}
	}
}
