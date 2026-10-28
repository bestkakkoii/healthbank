// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestMedicalAndImagingKeepIndependentHomepageRowsAndSourceFloor(t *testing.T) {
	for _, category := range []string{"western", "imaging"} {
		t.Run(category, func(t *testing.T) {
			root, module := "sp_IHKE3303S01_data", "ihke3303s01"
			if category == "imaging" {
				root, module = "sp_IHKE3408S01_data", "ihke3408s01"
			}
			s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if !strings.Contains(r.URL.Path, module) {
					t.Error("wrong fixed source route")
				}
				row := map[string]any{"func_DATE": "104/02/03", "real_INSPECT_DATE": "104/02/01", "hosp_ABBR": "合成院所", "row_ID": "PRIVATE_SNAPSHOT", "ori_TYPE": "A", "jpg_STATUS": "SOURCE_READY", "img_ORI_SIZE": "12.000 MB"}
				if strings.HasSuffix(r.URL.Path, "/page_load") {
					respondJSON(t, w, map[string]any{root: []any{row, row}, "c_sdate": "來源提供起日"})
				} else {
					respondJSON(t, w, []any{row})
				}
			})
			var report SupplementReport
			var err error
			if category == "imaging" {
				report, err = s.FetchSupplement(context.Background(), category, Query{})
			} else {
				report, err = s.FetchMedical(context.Background(), category, Query{StartDate: "2010-01-01", EndDate: "2020-12-31"})
			}
			if err != nil || len(report.Entries) != 4 || report.Complete {
				t.Fatalf("homepage, search or source floor lost: %v", err)
			}
			rows := []SupplementEntry{}
			for _, entry := range report.Entries {
				if entry.Kind == "source_metadata" {
					if entry.Date != "" || entry.EndDate != "" || fieldValues([]SupplementEntry{entry}, "c_sdate")[0] != "來源提供起日" {
						t.Fatal("source availability floor was applied as a clinical date")
					}
				} else {
					rows = append(rows, entry)
				}
			}
			if len(rows) != 3 || rows[0].Key == rows[1].Key || rows[0].ContractID == rows[2].ContractID || rows[0].DetailID == rows[2].DetailID {
				t.Fatal("source snapshots or duplicate multiplicity collapsed")
			}
			if category == "imaging" && (len(fieldValues(rows, "jpg_status")) != 3 || fieldValues(rows, "img_ori_size")[0] != "12.000 MB") {
				t.Fatal("original image readiness/size context dropped")
			}
		})
	}
}
