// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package lab

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestPortableImportSupportsHistoryAboveTwelveMiB(t *testing.T) {
	// The former upload cap rejected valid complete-history exports even though
	// every row and scalar remained well inside the parser's independent limits.
	const count = 400
	source := strings.Repeat("SYNTHETIC_SOURCE_", 2048)
	row := portableRow{CheckDate: "115/01/02", ItemName: "合成項目", Result: ">400.000000000000000001", Source: source}
	rows := make([]portableRow, count)
	for i := range rows {
		rows[i] = row
	}
	jsonData, err := json.Marshal(struct {
		Schema  string        `json:"schema"`
		Records []portableRow `json:"records"`
	}{portableSchema, rows})
	if err != nil {
		t.Fatal(err)
	}
	var xmlData strings.Builder
	xmlData.WriteString(`<healthbank_report schema="healthbank.lab.v1">`)
	for range count {
		xmlData.WriteString(`<record><check_date>115/01/02</check_date><item_name>合成項目</item_name><result>&gt;400.000000000000000001</result><source>`)
		xmlData.WriteString(source)
		xmlData.WriteString(`</source></record>`)
	}
	xmlData.WriteString(`</healthbank_report>`)
	for _, input := range []struct {
		name string
		data []byte
	}{{"history.json", jsonData}, {"history.xml", []byte(xmlData.String())}} {
		t.Run(input.name, func(t *testing.T) {
			if len(input.data) <= 12<<20 || len(input.data) >= 32<<20 {
				t.Fatal("fixture must exercise a valid portable file between the old and new bounds")
			}
			records, warnings, err := ParseImport(input.name, input.data)
			if err != nil || len(records) != count || len(warnings) == 0 {
				t.Fatalf("large portable history failed: %v; rows=%d", err, len(records))
			}
			if records[0] != row.raw() || records[count-1] != row.raw() {
				t.Fatal("large import truncated, reformatted or deduplicated original rows")
			}
		})
	}
}

func TestPortableImportThirtyTwoMiBBoundAndFieldLimits(t *testing.T) {
	for _, ext := range []string{"json", "xml"} {
		t.Run(ext+"_file", func(t *testing.T) {
			if _, _, err := ParseImport("oversized."+ext, bytes.Repeat([]byte(" "), (32<<20)+1)); err == nil || !strings.Contains(err.Error(), "32 MiB") {
				t.Fatal("portable import did not reject a file above 32 MiB before parsing")
			}
		})
	}
	for _, input := range []struct{ name, data string }{
		{"field.json", `{"schema":"healthbank.lab.v1","records":[{"item_name":"synthetic","result":"` + strings.Repeat("x", (64<<10)+1) + `"}]}`},
		{"field.xml", `<healthbank_report schema="healthbank.lab.v1"><record><item_name>synthetic</item_name><result>` + strings.Repeat("x", (64<<10)+1) + `</result></record></healthbank_report>`},
		{"rows.json", `{"schema":"healthbank.lab.v1","records":[` + strings.TrimSuffix(strings.Repeat(`{"item_name":"synthetic"},`, 50001), ",") + `]}`},
		{"rows.xml", `<healthbank_report schema="healthbank.lab.v1">` + strings.Repeat(`<record><item_name>synthetic</item_name></record>`, 50001) + `</healthbank_report>`},
	} {
		t.Run(input.name, func(t *testing.T) {
			if records, _, err := ParseImport(input.name, []byte(input.data)); err == nil || len(records) != 0 {
				t.Fatal("larger portable file allowance relaxed scalar or row limits")
			}
		})
	}
}

func TestPortableReportsPreserveEveryOriginalRow(t *testing.T) {
	row := `{"check_date":"115/10/26","item_name":"Synthetic test","result":">300","unit":"mg/L","reference":"[][30]","official_flag":"H"}`
	data := []byte(`{"schema":"healthbank.lab.v1","records":[` + row + `,` + row + `]}`)
	records, warnings, err := ParseImport("test.json", data)
	if err != nil || len(records) != 2 || len(warnings) == 0 || records[0].Result != ">300" || records[0].OfficialFlag != "H" {
		t.Fatalf("portable JSON: %v %#v", err, records)
	}
	xml := `<?xml version="1.0"?><healthbank_report schema="healthbank.lab.v1"><record><check_date>115/10/26</check_date><item_name>Synthetic test</item_name><result>&gt;300</result><unit>mg/L</unit><reference>[][30]</reference><official_flag>H</official_flag></record></healthbank_report>`
	fromXML, _, err := ParseImport("test.xml", []byte(xml))
	if err != nil || len(fromXML) != 1 || fromXML[0] != records[0] {
		t.Fatalf("portable XML differs: %v %#v", err, fromXML)
	}
}
func TestPortableImportRejectsAmbiguityAndUnsupportedSources(t *testing.T) {
	cases := map[string][]string{"json": {
		`{"schema":"healthbank.lab.v1","schema":"other","records":[]}`,
		`{"schema":"healthbank.lab.v1","records":[{"item_name":"x","result":"1","result":"99"}]}`,
		`{"schema":"healthbank.lab.v1","records":[],"patient":"private"}`,
		`{"resourceType":"Bundle","entry":[]}`,
		`{"schema":"healthbank.lab.v1","records":[{"item_name":""}]}`,
		`{"schema":"healthbank.lab.v1","records":[]} {}`,
		strings.Repeat("[", 40) + strings.Repeat("]", 40),
	}, "xml": {
		`<!DOCTYPE report [<!ENTITY x SYSTEM "file:///etc/passwd">]><healthbank_report schema="healthbank.lab.v1"/>`,
		`<healthbank_report schema="healthbank.lab.v1"><record><item_name>x</item_name><result>1</result><result>2</result></record></healthbank_report>`,
		`<healthbank_report schema="healthbank.lab.v1"><record><patient>private</patient></record></healthbank_report>`,
		`<healthbank_report schema="healthbank.lab.v1"><record><result><nested/></result></record></healthbank_report>`,
		`<healthbank_report schema="healthbank.lab.v1"/><healthbank_report schema="healthbank.lab.v1"/>`,
		`<healthbank_report schema="healthbank.lab.v1"><?evil data?></healthbank_report>`,
	}}
	for ext, documents := range cases {
		for _, document := range documents {
			if _, _, err := ParseImport("synthetic."+ext, []byte(document)); err == nil {
				t.Fatalf("accepted invalid %s", ext)
			}
		}
	}
}

func TestPortableJSONRejectsLossyStringsAndSchemaAliases(t *testing.T) {
	for _, document := range []string{
		`{"schema":"healthbank.lab.v1","records":[{"item_name":"synthetic","result":"\ud800"}]}`,
		`{"schema":"healthbank.lab.v1","records":[{"item_name":"synthetic","result":"\udc00"}]}`,
		`{"schema":"healthbank.lab.v1","records":[{"item_name":"synthetic","result":"1","Result":"2"}]}`,
		`{"schema":"healthbank.lab.v1","Records":[]}`,
		`{"schema":"healthbank.lab.v1","records":[{"Item_Name":"synthetic"}]}`,
		`{"schema":"healthbank.lab.v1","records":[{"item_name":"synthetic","result":null}]}`,
	} {
		if records, _, err := ParseImport("synthetic.json", []byte(document)); err == nil || len(records) != 0 {
			t.Errorf("accepted ambiguous or lossy portable schema: %s", document)
		}
	}
	document := `{"schema":"healthbank.lab.v1","records":[{"item_name":"synthetic","result":"\ud842\udfb7，原文�與\\ud800"}]}`
	records, _, err := ParseImport("synthetic.json", []byte(document))
	if err != nil || len(records) != 1 || records[0].Result != "𠮷，原文�與\\ud800" {
		t.Fatal("valid non-BMP, replacement character or literal escape changed")
	}
}
