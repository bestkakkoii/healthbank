// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package server

import (
	"strings"
	"testing"

	"github.com/bestkakkoii/healthbank/internal/lab"
)

func TestLDLMethodWarningUsesVerifiedTGIdentityAndExactContext(t *testing.T) {
	catalog, err := lab.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	app := &App{catalog: catalog}
	for _, tc := range []struct {
		name, unit, result string
		change             func(*lab.RawRecord, *lab.RawRecord)
		knownTG, warning   bool
	}{
		{name: "canonical_boundary", unit: "mg/dL", result: "400", knownTG: true, warning: true},
		{name: "reviewed_liter_spelling", unit: "mg/dl", result: "400", knownTG: true, warning: true},
		{name: "reviewed_spacing", unit: " mg / dl ", result: "400", knownTG: true, warning: true},
		{name: "different_case_sensitive_prefix", unit: "Mg/dL", result: "400"},
		{name: "unreviewed_uppercase_unit", unit: "MG/DL", result: "400"},
		{name: "different_scale", unit: "mmol/L", result: "400"},
		{name: "missing_unit", result: "400"},
		{name: "exactly_below_boundary", unit: "mg/dl", result: "399.999999999999999999999", knownTG: true},
		{name: "exactly_above_boundary", unit: "mg/dl", result: "400.000000000000000000001", knownTG: true, warning: true},
		{name: "exclusive_lower_boundary", unit: "mg/dl", result: ">400", knownTG: true, warning: true},
		{name: "inclusive_lower_boundary", unit: "mg/dl", result: ">=400", knownTG: true, warning: true},
		{name: "range_wholly_at_least_boundary", unit: "mg/dl", result: "400-450", knownTG: true, warning: true},
		{name: "range_crosses_boundary", unit: "mg/dl", result: "399-450", knownTG: true},
		{name: "exclusive_lower_bound_below_threshold", unit: "mg/dl", result: ">399.999999999999999999999", knownTG: true},
		{name: "upper_bound_only", unit: "mg/dl", result: "<500", knownTG: true},
		{name: "missing_result", unit: "mg/dl", result: "-", knownTG: true},
		{name: "different_examination_date", unit: "mg/dl", result: "400", knownTG: true, change: func(_ *lab.RawRecord, ldl *lab.RawRecord) { ldl.CheckDate = "2002-04-07" }},
		{name: "unknown_examination_date", unit: "mg/dl", result: "400", knownTG: true, change: func(tg, _ *lab.RawRecord) { tg.CheckDate = "" }},
		{name: "invalid_examination_date", unit: "mg/dl", result: "400", knownTG: true, change: func(tg, _ *lab.RawRecord) { tg.CheckDate = "unknown" }},
		{name: "different_institution", unit: "mg/dl", result: "400", knownTG: true, change: func(_ *lab.RawRecord, ldl *lab.RawRecord) { ldl.Institution = "Synthetic clinic B" }},
		{name: "missing_institution", unit: "mg/dl", result: "400", knownTG: true, change: func(tg, ldl *lab.RawRecord) { tg.Institution, ldl.Institution = "", "" }},
		{name: "code_alone_does_not_match", unit: "mg/dl", result: "400", change: func(tg, _ *lab.RawRecord) { tg.ItemName = "Synthetic unverified analyte" }},
		{name: "ldl_requires_verified_identity", unit: "mg/dl", result: "400", knownTG: true, change: func(_ *lab.RawRecord, ldl *lab.RawRecord) { ldl.Unit = "Mg/dL" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tg := lab.RawRecord{CheckDate: "2002-04-06", VisitDate: "2002-04-06", OrderCode: "09004C", ItemName: "Triglyceride", Result: tc.result, Unit: tc.unit, Institution: "Synthetic clinic A"}
			ldl := lab.RawRecord{CheckDate: "2002-04-06", OrderCode: "09044C", ItemName: "LDL-Cholesterol", Result: "74.000000000000000000002", Unit: "mg/dL", Institution: "Synthetic clinic A", OfficialFlag: "SYNTHETIC_SOURCE_FLAG"}
			if tc.change != nil {
				tc.change(&tg, &ldl)
			}
			s := &session{raw: []lab.RawRecord{tg, ldl, ldl}, settings: lab.DefaultSettings(), warnings: []string{"Synthetic unrelated warning"}}
			got := app.currentResponse(s)
			if len(got.Records) != 3 || (got.Records[0].ItemID == "serum_triglycerides") != tc.knownTG {
				t.Fatal("source matching or duplicate record retention changed")
			}
			if got.Records[0].Unit != tg.Unit || got.Records[0].Result != tg.Result || got.Records[1].Unit != ldl.Unit || got.Records[1].Result != ldl.Result || got.Records[1].OfficialFlag != ldl.OfficialFlag {
				t.Fatal("contextual warning changed original source values, units or flags")
			}
			count := 0
			for _, warning := range got.Warnings {
				if strings.Contains(warning, "Friedewald") {
					count++
					if !strings.Contains(warning, "須核對是否同次採檢及LDL方法") {
						t.Fatal("same-day context was changed into a confirmed same-collection assertion")
					}
				}
			}
			want := 0
			if tc.warning {
				want = 1
			}
			if count != want || len(got.Warnings) != want+1 || got.Warnings[0] != "Synthetic unrelated warning" {
				t.Fatalf("LDL method warnings = %d, want %d; unrelated warnings must survive", count, want)
			}
		})
	}
}
