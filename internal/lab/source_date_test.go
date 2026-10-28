// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package lab

import (
	"encoding/json"
	"testing"
)

func TestNormalizedReportsKeepOriginalDateLexemes(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, original := range []string{"115/10/28 08:31:12", "2026-10-28T09:32:13+08:00", "日期待確認", ""} {
		settings := DefaultSettings()
		records := catalog.Normalize([]RawRecord{{ItemName: "Synthetic observation", CheckDate: original, VisitDate: "114/01/03 11:11:11", Result: "1.000000000000000001"}}, &settings)
		if len(records) != 1 || records[0].RawCheckDate != original || records[0].RawVisitDate != "114/01/03 11:11:11" {
			t.Fatal("normalization dropped an original date or timestamp")
		}
		if records[0].ExamDate != NormalizeDate(original) || records[0].VisitDate != "2025-01-03" {
			t.Fatal("source preservation changed normalized filter dates")
		}
		encoded, err := json.Marshal(records[0])
		if err != nil {
			t.Fatal(err)
		}
		var wire map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &wire); err != nil || wire["raw_check_date"] == nil || wire["raw_visit_date"] == nil {
			t.Fatal("source dates are missing from the API, including explicit empty values")
		}
	}
}
