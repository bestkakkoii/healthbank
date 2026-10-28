// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package lab

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPortableV2PreservesOriginalSourceMetadataAndDuplicates(t *testing.T) {
	row := `{"check_date":"113/06/07","visit_date":"113/06/01","upload_date":"113/07/02 09:15","source":"A","source_name":"合成來源 <原文> & 說明","item_name":"ALT","order_code":"09026C","result":"17.000000000000000001","unit":"U/L","official_flag":"H"}`
	xmlRow := `<record><check_date>113/06/07</check_date><visit_date>113/06/01</visit_date><upload_date>113/07/02 09:15</upload_date><source>A</source><source_name>合成來源 &lt;原文&gt; &amp; 說明</source_name><item_name>ALT</item_name><order_code>09026C</order_code><result>17.000000000000000001</result><unit>U/L</unit><official_flag>H</official_flag></record>`
	var previous []RawRecord
	for _, input := range []struct{ name, text string }{
		{"synthetic.json", `{"schema":"healthbank.lab.v2","records":[` + row + `,` + row + `]}`},
		{"synthetic.xml", `<healthbank_report schema="healthbank.lab.v2">` + xmlRow + xmlRow + `</healthbank_report>`},
	} {
		raw, warnings, err := ParseImport(input.name, []byte(input.text))
		if err != nil || len(raw) != 2 || len(warnings) == 0 || raw[0] != raw[1] {
			t.Fatalf("v2 source metadata import lost duplicate rows or provenance warning: %v", err)
		}
		if raw[0].UploadDate != "113/07/02 09:15" || raw[0].SourceName != "合成來源 <原文> & 說明" || raw[0].Source != "A" || raw[0].Result != "17.000000000000000001" || raw[0].OfficialFlag != "H" {
			t.Fatal("source date/name/precision/flag was changed")
		}
		if previous != nil && previous[0] != raw[0] {
			t.Fatal("JSON and XML disagree about original source metadata")
		}
		previous = raw
		catalog, err := LoadCatalog()
		if err != nil {
			t.Fatal(err)
		}
		settings := DefaultSettings()
		rows := catalog.Normalize(raw, &settings)
		if len(rows) != 2 || rows[0].ID == rows[1].ID || rows[0].ExamDate != "2024-06-07" || rows[0].VisitDate != "2024-06-01" || rows[0].UploadDate != "2024-07-02" || rows[0].RawUploadDate != raw[0].UploadDate || rows[0].SourceName != raw[0].SourceName {
			t.Fatal("normalization merged duplicates or conflated source date roles")
		}
		payload, err := json.Marshal(rows[0])
		if err != nil || !strings.Contains(string(payload), `"raw_upload_date":"113/07/02 09:15"`) || !strings.Contains(string(payload), `"upload_date":"2024-07-02"`) {
			t.Fatal("API record omitted the normalized or original upload date")
		}
	}
}

func TestSourceUploadDateNeverSuppliesMissingExaminationDate(t *testing.T) {
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	settings := DefaultSettings()
	rows := catalog.Normalize([]RawRecord{{ItemName: "ALT", Result: "17.125", Unit: "U/L", UploadDate: "113/07/02", SourceName: "合成來源"}}, &settings)
	if len(rows) != 1 || rows[0].ExamDate != "" || rows[0].VisitDate != "" || rows[0].RawCheckDate != "" || rows[0].RawVisitDate != "" || rows[0].UploadDate != "2024-07-02" {
		t.Fatal("source metadata fabricated a clinical date")
	}
}

func TestPortableV2KeepsLegacySchemaStrictAndBoundsNewFields(t *testing.T) {
	for _, schema := range []string{"healthbank.lab.v1", "healthbank.lab.v2"} {
		for _, ext := range []string{"json", "xml"} {
			text := `{"schema":"` + schema + `","records":[{"item_name":"SYNTHETIC_LEGACY","result":"0"}]}`
			if ext == "xml" {
				text = `<healthbank_report schema="` + schema + `"><record><item_name>SYNTHETIC_LEGACY</item_name><result>0</result></record></healthbank_report>`
			}
			rows, _, err := ParseImport("legacy."+ext, []byte(text))
			if err != nil || len(rows) != 1 || rows[0].UploadDate != "" || rows[0].SourceName != "" || rows[0].Result != "0" {
				t.Fatalf("legacy fields stopped being accepted without invented metadata: %v", err)
			}
		}
	}
	for _, field := range []string{"upload_date", "source_name"} {
		for _, ext := range []string{"json", "xml"} {
			for _, schema := range []string{"healthbank.lab.v1", "healthbank.lab.v2"} {
				value := "SYNTHETIC_METADATA"
				if schema == "healthbank.lab.v2" {
					value = strings.Repeat("x", maxCellBytes+1)
				}
				text := `{"schema":"` + schema + `","records":[{"item_name":"SYNTHETIC_BOUNDS","` + field + `":"` + value + `"}]}`
				if ext == "xml" {
					text = `<healthbank_report schema="` + schema + `"><record><item_name>SYNTHETIC_BOUNDS</item_name><` + field + `>` + value + `</` + field + `></record></healthbank_report>`
				}
				if rows, _, err := ParseImport("invalid."+ext, []byte(text)); err == nil || len(rows) != 0 {
					t.Fatal("v1 accepted new fields, or v2 bypassed their scalar limit")
				}
			}
		}
		for _, value := range []string{"null", "42", "true", "[]", "{}"} {
			text := `{"schema":"healthbank.lab.v2","records":[{"item_name":"SYNTHETIC_TYPE","` + field + `":` + value + `}]}`
			if _, _, err := ParseImport("type.json", []byte(text)); err == nil {
				t.Fatal("source metadata accepted a non-string field")
			}
		}
	}
	for _, text := range []string{
		`{"schema":"healthbank.lab.v2","records":[{"item_name":"SYNTHETIC","source_name":"A","Source_Name":"B"}]}`,
		`{"schema":"healthbank.lab.v2","records":[{"item_name":"SYNTHETIC","raw_upload_date":"113/07/02"}]}`,
		`{"schema":"healthbank.lab.v2","records":[{"item_name":"SYNTHETIC","patient_id":"FORBIDDEN_SYNTHETIC"}]}`,
	} {
		if _, _, err := ParseImport("invalid.json", []byte(text)); err == nil {
			t.Fatal("v2 relaxed exact field names or identity exclusion")
		}
	}
}
