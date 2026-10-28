// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package lab

import (
	"encoding/json"
	"strings"
	"testing"
)

func FuzzBoundedImportPreservesParsedValues(f *testing.F) {
	f.Add(uint8(0), []byte(`{"schema":"healthbank.lab.v1","records":[{"item_name":"合成資料","result":">300.000000000000001","check_date":"115/10/28 08:30"}]}`))
	f.Add(uint8(1), []byte(`<healthbank_report schema="healthbank.lab.v1"><record><item_name>合成資料</item_name><result>&lt;0.18</result></record></healthbank_report>`))
	f.Add(uint8(2), []byte(`<table><tr><th>`+strings.Join(detailedHeaders, "</th><th>")+`</th></tr><tr><td>115/10/28</td><td>09004C</td><td>合成</td><td>Triglyceride</td><td>800.000000000000001</td><td>mg/dL</td><td>[][150]</td><td>SYNTHETIC</td><td>115/10/28</td><td>synthetic</td></tr></table>`))
	f.Add(uint8(3), []byte{'P', 'K', 5, 6})
	catalog, err := LoadCatalog()
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, format uint8, data []byte) {
		if len(data) > 64<<10 {
			t.Skip()
		}
		name := []string{"synthetic.json", "synthetic.xml", "synthetic.html", "synthetic.zip"}[format%4]
		raw, _, err := ParseImport(name, data)
		if err != nil {
			if len(raw) != 0 {
				t.Fatal("failed import returned a partial report")
			}
			return
		}
		settings := DefaultSettings()
		normalized := catalog.Normalize(raw, &settings)
		if len(normalized) != len(raw) {
			t.Fatal("normalization dropped observations")
		}
		seen := map[string]bool{}
		for i, row := range normalized {
			if row.Result != raw[i].Result || row.RawCheckDate != raw[i].CheckDate || row.RawVisitDate != raw[i].VisitDate || seen[row.ID] {
				t.Fatal("original values or duplicate identity were lost")
			}
			seen[row.ID] = true
			if row.ValueKind != "numeric" && row.NumericValue != nil {
				t.Fatal("nonexact source became a point")
			}
		}
		if _, err := json.Marshal(normalized); err != nil {
			t.Fatal("accepted source cannot be represented by the report API")
		}
	})
}
