// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package lab

import (
	"strings"
	"testing"
)

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
