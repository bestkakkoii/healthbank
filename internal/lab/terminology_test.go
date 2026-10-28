// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package lab

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestTerminologyPreservesRawRecordsIdentitiesAndRules(t *testing.T) {
	catalog := gradingCatalog(t)
	withoutGlossary := *catalog
	withoutGlossary.NameGlossary = nil
	raw := []RawRecord{
		{OrderCode: "SYNTHETIC_RATIO_A", ItemName: "ACR", Unit: "mg/g", Result: "<10.000000000000001", CheckDate: "113/02/03", VisitDate: "113/01/31", OrderName: "SYNTHETIC_ORDER_A", OfficialFlag: "SYNTHETIC_FLAG"},
		{OrderCode: "SYNTHETIC_RATIO_B", ItemName: "ACR", Unit: "mg/g", Result: "10.000000000000002", CheckDate: "113/02/03", OrderName: "SYNTHETIC_ORDER_B"},
		{OrderCode: "09016C", ItemName: "ACR", Unit: "mg/dL", Result: "0"},
		{OrderCode: "09071C", ItemName: "CK-MB", Unit: "ng/mL", Result: "1.0"},
		{OrderCode: "SYNTHETIC_UNKNOWN", ItemName: "acr", Unit: "mg/g", Result: "2.0"},
		{OrderCode: "SYNTHETIC_UNKNOWN", ItemName: "Unreviewed synthetic analyte", Unit: "mg/L", Result: "3.0"},
	}
	settings, originalSettings := DefaultSettings(), DefaultSettings()
	rows := catalog.Normalize(raw, &settings)
	originalRows := withoutGlossary.Normalize(raw, &originalSettings)
	if !reflect.DeepEqual(rows, originalRows) || !reflect.DeepEqual(settings, originalSettings) {
		t.Fatal("display terminology changed normalization or stored settings")
	}
	seen := map[string]bool{}
	for i, row := range rows {
		if !strings.HasPrefix(row.ItemID, "custom_v2_") || seen[row.ItemID] || row.ConfigurationKey != row.ItemID || row.ItemName != raw[i].ItemName || row.OrderName != raw[i].OrderName || row.OrderCode != raw[i].OrderCode || row.Result != raw[i].Result || row.Unit != raw[i].Unit || row.RawCheckDate != raw[i].CheckDate || row.RawVisitDate != raw[i].VisitDate || row.OfficialFlag != raw[i].OfficialFlag || row.Level != nil {
			t.Fatalf("terminology changed a synthetic source identity or observation at %d", i)
		}
		seen[row.ItemID] = true
	}
	before, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	response := catalog.ForSettings(settings)
	for i, row := range rows {
		item := terminologyCatalogItem(t, response, row.ConfigurationKey)
		if item.Builtin || item.Key != row.ConfigurationKey || item.ID != row.ItemID || item.Unit != row.Unit || item.Category != row.Category || item.Rule != nil || item.Reference != nil || len(item.Alerts) != 0 {
			t.Fatalf("display-only glossary enabled a clinical contract for row %d", i)
		}
		if i < 4 {
			if item.Name == raw[i].ItemName || item.EnglishName == "" || !slices.Contains(item.Aliases, raw[i].ItemName) {
				t.Fatalf("reviewed display name or raw search alias is missing for row %d", i)
			}
		} else if item.Name != raw[i].ItemName || item.EnglishName != "" || item.TermNote != "" {
			t.Fatalf("an unreviewed exact spelling was guessed for row %d", i)
		}
	}
	mass := terminologyCatalogItem(t, response, rows[3].ConfigurationKey)
	if strings.Contains(mass.EnglishName, "activity") || !strings.Contains(mass.TermNote, "活性") {
		t.Fatal("name-only CK-MB translation asserted an activity assay")
	}
	after, err := json.Marshal(settings)
	if err != nil || string(before) != string(after) {
		t.Fatal("reading translated catalog mutated persisted definitions")
	}
}

func TestTerminologyKeepsBillingProceduresSeparateFromAnalytes(t *testing.T) {
	catalog := gradingCatalog(t)
	settings := DefaultSettings()
	raw := []RawRecord{
		{OrderCode: "09016C", ItemName: "ACR", Unit: "mg/g", Result: "2.123456789"},
		{OrderCode: "09016C", ItemName: "Creatinine-spot urine", Unit: "mg/dL", Result: "3.123456789"},
		{OrderCode: "09016C", ItemName: "Creatinine(健檢)", Unit: "mg/dL", Result: "4.123456789"},
		{OrderCode: "12193C", ItemName: "NT-ProBNP", Unit: "pg/mL", Result: "5.123456789"},
	}
	rows := catalog.Normalize(raw, &settings)
	if rows[0].ItemID != "urine_acr_quantitative" || rows[1].ItemID != "spot_urine_creatinine" || !strings.HasPrefix(rows[2].ItemID, "custom_v2_") || rows[3].ItemID != "plasma_nt_probnp" {
		t.Fatal("procedure names replaced result-specific identities")
	}
	response := catalog.ForSettings(settings)
	acr := terminologyCatalogItem(t, response, "urine_albumin_creatinine_ratio")
	protein := terminologyCatalogItem(t, response, "urine_protein_dipstick")
	if acr.EnglishName != "Urine albumin-to-creatinine ratio (UACR)" || strings.Contains(acr.EnglishName, "quantitative") || protein.EnglishName != "Urine protein (qualitative)" || strings.Contains(protein.EnglishName, "dipstick") {
		t.Fatal("shared settings asserted a single assay method")
	}
	if !strings.Contains(terminologyCatalogItem(t, response, "plasma_nt_probnp").EnglishName, "NT-proBNP") || !strings.Contains(terminologyCatalogItem(t, response, rows[2].ConfigurationKey).EnglishName, "specimen unspecified") {
		t.Fatal("billing vocabulary erased analyte or specimen uncertainty")
	}
	for i, row := range rows {
		if row.ItemName != raw[i].ItemName || row.Result != raw[i].Result {
			t.Fatal("source spelling or precision changed")
		}
	}
}

func TestTerminologyDoesNotInferBloodGasSpecimen(t *testing.T) {
	catalog := gradingCatalog(t)
	for _, d := range catalog.Items {
		if !strings.HasPrefix(d.ID, "venous_") {
			continue
		}
		if d.ID == "venous_blood_gas_panel" {
			if d.Specimen != "venous_blood" || d.Match.Name != "Blood gas analysis-vein" || !strings.Contains(d.Name, "靜脈") || !strings.Contains(d.EnglishName, "Venous") {
				t.Fatal("the source-explicit venous panel lost its meaning")
			}
			continue
		}
		if d.Specimen != "blood_unconfirmed" || strings.Contains(d.Name, "靜脈") || strings.Contains(strings.ToLower(d.EnglishName), "venous") || !strings.Contains(d.TermNote, "未確認") || d.Key != d.ID || d.Reference != nil && d.Reference.AutomaticFallback {
			t.Fatalf("generic blood-gas field asserts specimen or changes identity: %s", d.ID)
		}
	}
	settings := DefaultSettings()
	rows := catalog.Normalize([]RawRecord{
		{OrderCode: "SYNTHETIC_UNKNOWN", ItemName: "PH", Unit: "無單位", Result: "6.0"},
		{OrderCode: "SYNTHETIC_UNKNOWN", ItemName: "pH", Unit: "無單位", Result: "7.0"},
	}, &settings)
	if rows[0].ItemID == rows[1].ItemID {
		t.Fatal("translated pH spelling merged source identities")
	}
	for _, row := range rows {
		item := terminologyCatalogItem(t, catalog.ForSettings(settings), row.ConfigurationKey)
		if !strings.Contains(item.EnglishName, "specimen unspecified") || !strings.Contains(item.TermNote, "大小寫") {
			t.Fatal("name-only pH translation inferred a specimen")
		}
	}
}

func TestTerminologyRetainsLegacyConflictLabels(t *testing.T) {
	catalog := gradingCatalog(t)
	settings := DefaultSettings()
	settings.Known["legacy_synthetic"] = KnownItem{ID: "legacy_synthetic", Key: "legacy_synthetic", Name: "ACR", Unit: "mg/g", Category: "other", LegacyFor: "urine_albumin_creatinine_ratio"}
	item := terminologyCatalogItem(t, catalog.ForSettings(settings), "legacy_synthetic")
	if item.Name != "ACR" || item.EnglishName != "" || item.TermNote != "" || !strings.Contains(item.Description, "舊版") {
		t.Fatal("display glossary relabelled an isolated legacy setting")
	}
}

func TestTerminologyCatalogExposesNamesWithoutInventingMeasurements(t *testing.T) {
	catalog := gradingCatalog(t)
	settings := DefaultSettings()
	withoutGlossary := *catalog
	withoutGlossary.NameGlossary = nil
	response := catalog.ForSettings(settings)
	baseline := withoutGlossary.ForSettings(settings)
	if !reflect.DeepEqual(response.Items, baseline.Items) || len(settings.Known) != 0 || response.TermNotice == "" {
		t.Fatal("reading the public glossary invented observed items or omitted its display-only notice")
	}
	terms := map[string]CatalogTerm{}
	for _, term := range response.NameGlossary {
		for _, name := range term.Names {
			terms[name] = term
		}
	}
	for _, name := range []string{"BIOCHEM", "Hematology", "是否人工閱片"} {
		term, ok := terms[name]
		if !ok || term.Name == "" || term.EnglishName == "" || term.Note == "" || term.Basis != "literal_source_translation" {
			t.Fatalf("non-measurement label %q lost its explicit literal-translation basis", name)
		}
	}
	acr, ok := terms["ACR"]
	if !ok || acr.Basis != "primary_source_terms" || len(acr.Sources) == 0 || !strings.Contains(acr.Note, "未") {
		t.Fatal("source-name ACR fallback lost its attribution or specimen/method uncertainty")
	}
	// The public DTO is a copy: a consumer must not be able to change the
	// embedded exact spellings or attribution seen by subsequent requests.
	before, err := json.Marshal(catalog.NameGlossary)
	if err != nil {
		t.Fatal(err)
	}
	for i := range response.NameGlossary {
		if len(response.NameGlossary[i].Names) > 0 {
			response.NameGlossary[i].Names[0] = "synthetic consumer mutation"
		}
		if len(response.NameGlossary[i].Sources) > 0 {
			response.NameGlossary[i].Sources[0] = "synthetic attribution mutation"
		}
	}
	after, err := json.Marshal(catalog.NameGlossary)
	if err != nil || string(before) != string(after) {
		t.Fatal("public glossary shares mutable source metadata with the embedded catalog")
	}
}

func TestTerminologyKeepsSpecimenAndFastingUncertainty(t *testing.T) {
	catalog := gradingCatalog(t)
	response := catalog.ForSettings(DefaultSettings())
	for key, name := range map[string]string{
		"serum_creatinine":       "血液肌酸酐",
		"serum_albumin":          "血液白蛋白",
		"fasting_plasma_glucose": "飯前血糖（來源標示 AC）",
	} {
		item := terminologyCatalogItem(t, response, key)
		if item.Name != name || item.TermNote == "" {
			t.Fatalf("display name %s asserts an unverified specimen or fasting condition", key)
		}
	}
	for _, d := range catalog.Items {
		if d.ID != "venous_oxygen_saturation" && d.ID != "venous_po2" {
			continue
		}
		settings := DefaultSettings()
		raw := RawRecord{OrderCode: d.Match.Code, ItemName: d.Match.Name, Unit: d.Match.Unit, Result: "10.000000000000001", CheckDate: "113/02/03"}
		row := catalog.Normalize([]RawRecord{raw}, &settings)[0]
		if row.ItemID != d.ID || row.ItemName != raw.ItemName || row.Result != raw.Result || row.Unit != raw.Unit || !strings.Contains(row.Interpretation, "動脈／靜脈檢體未經來源確認") || strings.Contains(row.Interpretation, "此為靜脈血氣") {
			t.Fatalf("generic blood-gas result %s still asserts an unverified collection specimen", d.ID)
		}
	}
}

func terminologyCatalogItem(t *testing.T, response CatalogResponse, key string) CatalogItem {
	t.Helper()
	for _, item := range response.Items {
		if item.Key == key {
			return item
		}
	}
	t.Fatalf("missing catalog key %s", key)
	return CatalogItem{}
}
