// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestMedicalDetailKeepsIndependentSourceAndProviderText(t *testing.T) {
	for _, tc := range []struct{ category, path, root string }{
		{"dental", "/api/ihke3000/ihke3305s02/page_load", "ihke3305S02_main_data"},
		{"medicine", "/api/ihke3000/IHKE3306S02/page_load", "ihke3306S02_main_data"},
		{"chronic", "/api/ihke3000/IHKE3306S02/page_load", "ihke3306S02_main_data"},
	} {
		t.Run(tc.category, func(t *testing.T) {
			s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != tc.path || r.URL.Query().Get("crid") != "private-synthetic-row" || r.URL.Query().Get("ctype") != "A" {
					t.Error("medical source context read escaped its bound parent")
				}
				respondJSON(t, w, map[string]any{tc.root: []any{map[string]any{
					"rowid": "private-synthetic-row", "hosp_ABBR": "明細獨立回傳合成院所", "ori_TYPE": "明細來源原文||Source channel",
					"func_SEQ_NO": "0007", "func_DATE": "115/01/03", "cure_E_DATE": "114/12/28", "type_name_isr": true,
					"part_AMT": "0.0000", "part_AMT_BASIC": "1.0001", "part_AMT_DRUG": "2.0002", "part_AMT_ASSAY": "3.0003",
					"appl_DOT": "4.0004", "exp_DOT": "5.0005", "drug_DOT": "6.0006", "dsvc_DOT": "7.0007",
					"diag_DOT": "8.0008", "met_DOT": "9.0009", "cure_DOT": "10.0010", "assay_AMIN": "11.0011",
					"hosp_url": "https://private-synthetic.invalid/", "ID_NO": "private-synthetic-identity",
				}}})
			})
			s.medicalReferences = map[string]map[string]medicalReference{tc.category: {"ref": {category: tc.category, rowID: "private-synthetic-row", sourceType: "A", date: "115/01/01", institution: "原清單合成院所"}}}
			report, err := s.FetchMedicalDetail(context.Background(), tc.category, "ref")
			if err != nil || len(report.Entries) != 1 {
				t.Fatalf("source detail failed: %v", err)
			}
			header := report.Entries[0]
			if header.Institution != "原清單合成院所" || header.Date != "115/01/01" {
				t.Fatal("detail source fields overwrote independent list context")
			}
			fields := map[string]string{}
			for _, field := range header.Fields {
				fields[field.Code] = field.Value
				if strings.Contains(field.Value, "private-synthetic") {
					t.Fatal("private selector, identity or link escaped source projection")
				}
			}
			for code, value := range map[string]string{
				"hosp_abbr": "明細獨立回傳合成院所", "ori_type": "明細來源原文||Source channel", "func_seq_no": "0007",
				"func_date": "115/01/03", "cure_e_date": "114/12/28", "part_amt": "0.0000", "part_amt_basic": "1.0001",
				"part_amt_drug": "2.0002", "part_amt_assay": "3.0003", "appl_dot": "4.0004", "exp_dot": "5.0005",
				"drug_dot": "6.0006", "dsvc_dot": "7.0007", "diag_dot": "8.0008", "met_dot": "9.0009", "cure_dot": "10.0010", "assay_amin": "11.0011",
			} {
				if fields[code] != value {
					t.Fatalf("independent source field %s lost its original text", code)
				}
			}
		})
	}
}
