// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"context"
	"net/http"
	"testing"
)

func TestMedicalDuplicateParentRowsPreserveEveryNestedOccurrence(t *testing.T) {
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/ihke3000/IHKE3306S02/page_load" || r.URL.Query().Get("crid") != "synthetic-parent" {
			t.Error("detail used an unbound source route")
		}
		order := map[string]any{"order_code": "SYNTHETIC-X", "drug_name": "合成藥品", "order_qty": "14.000000000000000001", "order_drug_day": "7.00"}
		row := map[string]any{"rowid": "synthetic-parent", "sp_IHKE3306S03_data_list": []any{order, order}}
		respondJSON(t, w, map[string]any{"ihke3306S02_main_data": []any{row, row}})
	})
	s.medicalReferences = map[string]map[string]medicalReference{"medicine": {"parent": {category: "medicine", rowID: "synthetic-parent", sourceType: "A"}}}
	report, err := s.FetchMedicalDetail(context.Background(), "medicine", "parent")
	if err != nil || len(report.Entries) != 6 {
		t.Fatalf("source parent/child row multiplicity lost: %v", err)
	}
	seen := map[string]bool{}
	orders := 0
	for _, entry := range report.Entries {
		if entry.Key == "" || seen[entry.Key] {
			t.Fatal("distinct returned parent/child occurrences share an output key")
		}
		seen[entry.Key] = true
		if entry.Medicine != nil {
			orders++
			if entry.Medicine.Quantity != "14.000000000000000001" || entry.Medicine.Days != "7.00" {
				t.Fatal("duplicate order source precision changed")
			}
		}
	}
	if orders != 4 {
		t.Fatal("a duplicate source medication occurrence disappeared")
	}
}
