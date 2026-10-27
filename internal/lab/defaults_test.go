// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package lab

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestBuiltinSettingsWithoutObservations(t *testing.T) {
	catalog := gradingCatalog(t)
	settings := DefaultSettings()
	response := catalog.ForSettings(settings)
	if len(response.Items) != 76 || len(settings.Known) != 0 {
		t.Fatalf("want 76 unique built-ins and no fabricated encounters, got %d/%d", len(response.Items), len(settings.Known))
	}
	seen := map[string]bool{}
	for _, item := range response.Items {
		if seen[item.Key] || item.Key == "" || !item.Builtin || item.Low != nil || item.High != nil || item.Aliases == nil || item.Alerts == nil || len(item.OrderCodes) == 0 {
			t.Fatalf("invalid pre-import configuration contract: %+v", item)
		}
		seen[item.Key] = true
		if item.ID == "manual_smear_status" || item.ID == "venous_blood_gas_panel" {
			t.Fatal("metadata entered measurement settings")
		}
	}
	settings.Overrides["serum_sodium"] = Override{Enabled: true, ReferenceHigh: pointer("144")}
	settings.Favorites = []string{"serum_total_vitamin_b12"}
	if err := ValidateSettings(settings, catalog.ConfigurableItems(settings.Known)); err != nil {
		t.Fatalf("unobserved built-in settings rejected: %v", err)
	}
	settings.Favorites = []string{"manual_smear_status"}
	if err := ValidateSettings(settings, catalog.ConfigurableItems(settings.Known)); err == nil {
		t.Fatal("metadata accepted as a favorite")
	}
}

func TestNewSourceMappingsPreserveClinicalIdentity(t *testing.T) {
	catalog := gradingCatalog(t)
	cases := []struct{ code, name, unit, id, key string }{
		{"09071C", "CK-MB", "U/L", "blood_ck_mb_activity", "blood_ck_mb_activity"},
		{"08079B", "D-D diamer", "ng/mL", "plasma_d_dimer_unspecified", "plasma_d_dimer_unspecified"},
		{"12151C", "Homocysteine", "µmol/L", "blood_total_homocysteine", "blood_total_homocysteine"},
		{"09122C", "I-PTH", "pg/mL", "blood_intact_pth", "blood_intact_pth"},
		{"09129C", "Vitamin B12", "pg/mL", "serum_total_vitamin_b12", "serum_total_vitamin_b12"},
		{"09015C", "Creatinine(健檢)", "mg/dL", "serum_creatinine", "serum_creatinine"},
		{"09005C", "Glucose", "mg/dL", "blood_glucose_unspecified", "blood_glucose_unspecified"},
		{"09140C", "Glucose-PC(飯後)-微血管採血法", "mg/dL", "capillary_postprandial_glucose", "capillary_postprandial_glucose"},
		{"06003C", "Protein(qualitative)", "無單位", "urine_protein_qualitative", "urine_protein_dipstick"},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			d := catalog.match(RawRecord{OrderCode: tc.code, ItemName: tc.name, Unit: tc.unit})
			if d.ID != tc.id || d.Key != tc.key {
				t.Fatalf("unexpected canonical identity: %+v", d)
			}
		})
	}
	for _, raw := range []RawRecord{
		{OrderCode: "09071C", ItemName: "CK-MB", Unit: "ng/mL"},
		{OrderCode: "08079B", ItemName: "D-D diamer", Unit: "ng/mL FEU"},
		{OrderCode: "09129C", ItemName: "Vitamin B12", Unit: "pmol/L"},
		{OrderCode: "09016C", ItemName: "Creatinine(健檢)", Unit: "mg/dL"},
	} {
		if !strings.HasPrefix(catalog.match(raw).ID, "custom_") {
			t.Fatal("unsupported specimen, method or unit inherited another configuration")
		}
	}
	settings := DefaultSettings()
	protein := normalizedOne(t, catalog, syntheticMeasurement(t, catalog, "urine_protein_qualitative", "1+", "[無參考值][]"), &settings)
	if protein.Level == nil || *protein.Level != 3 || protein.NumericValue != nil {
		t.Fatal("qualitative protein was not flagged without inventing a concentration")
	}
	for _, result := range []string{"Trace", "trace", "±", "+/-"} {
		raw := syntheticMeasurement(t, catalog, "urine_protein_qualitative", result, "Negative or Trace")
		raw.OfficialFlag = "source-flag"
		row := normalizedOne(t, catalog, raw, &settings)
		if row.Level != nil || row.NumericValue != nil || row.Result != raw.Result || row.ReferenceText != raw.Reference || row.OfficialFlag != raw.OfficialFlag || !strings.Contains(row.Interpretation, "依檢驗方法") {
			t.Fatalf("method-dependent trace was classified or source context was lost: %+v", row)
		}
	}
}

func TestFullwidthComparatorsPreserveBounds(t *testing.T) {
	catalog := gradingCatalog(t)
	for _, result := range []string{"＜18", "＜＝18", "≦18", "≤18"} {
		t.Run(result, func(t *testing.T) {
			settings := DefaultSettings()
			row := normalizedOne(t, catalog, syntheticMeasurement(t, catalog, "blood_ck_mb_activity", result, "[0][25]"), &settings)
			if row.Result != result || row.ValueKind != "inequality" || row.NumericValue != nil || row.Level == nil || *row.Level != 1 {
				t.Fatalf("comparison glyph lost numeric meaning or fabricated a point: %+v", row)
			}
		})
	}
	lo, hi := parseReference("≧ 12")
	if lo == nil || *lo != "12" || hi != nil {
		t.Fatal("inclusive fullwidth reference was not parsed")
	}
	lo, hi = parseReference("＜ 12")
	if lo != nil || hi != nil {
		t.Fatal("strict reference was broadened to an inclusive bound")
	}
}

func TestNamedAlertsRetainExactEndpointsAndOverrideFloors(t *testing.T) {
	catalog := gradingCatalog(t)
	cases := []struct {
		id, value string
		level     int
	}{
		{"serum_potassium", "2", 5}, {"serum_potassium", "2.0001", 4},
		{"serum_potassium", "2.5", 4}, {"serum_potassium", "2.5001", 1},
		{"serum_potassium", "6.49", 1}, {"serum_potassium", "6.5", 5},
		{"serum_potassium", "≦2.5", 4}, {"serum_potassium", "＜2", 5},
		{"serum_sodium", "115", 4}, {"serum_sodium", "115.0001", 1},
		{"serum_sodium", "159.9999", 1}, {"serum_sodium", "160", 4},
		{"serum_total_calcium", "6.5", 4}, {"serum_total_calcium", "13", 4},
		{"serum_total_calcium", "14.9999", 4}, {"serum_total_calcium", "15", 5},
		{"serum_total_calcium", "13-18", 4},
		{"serum_chloride", "75", 4}, {"serum_chloride", "125", 4},
		{"blood_hemoglobin", "5", 4}, {"blood_hematocrit", "16", 4},
		{"blood_hematocrit", "60", 4}, {"blood_platelet_count", "30", 4},
		{"blood_wbc_count", "1.2", 4}, {"blood_wbc_count", "100", 1},
		{"blood_wbc_count", "100.0001", 4},
		{"blood_glucose_unspecified", "54", 4}, {"blood_glucose_unspecified", "53.9999", 5},
		{"capillary_postprandial_glucose", "69.9999", 4}, {"capillary_postprandial_glucose", "70", 1},
	}
	for _, tc := range cases {
		t.Run(tc.id+"/"+tc.value, func(t *testing.T) {
			settings := DefaultSettings()
			settings.Overrides[tc.id] = Override{Enabled: true, Bands: []Band{{Lower: pointer("0"), LowerInclusive: true, Level: 1}}}
			raw := syntheticMeasurement(t, catalog, tc.id, tc.value, "[0][1000]")
			raw.OfficialFlag = "SYNTHETIC_ORIGINAL_FLAG"
			row := normalizedOne(t, catalog, raw, &settings)
			if row.Level == nil || *row.Level != tc.level || row.OfficialFlag != raw.OfficialFlag || row.ReferenceText != raw.Reference {
				t.Fatalf("alert floor or immutable source fields changed: %+v", row)
			}
		})
	}
	settings := DefaultSettings()
	wrongUnit := syntheticMeasurement(t, catalog, "serum_potassium", "2", "[無參考值][]")
	wrongUnit.Unit = "mg/dL"
	if row := normalizedOne(t, catalog, wrongUnit, &settings); row.Level != nil {
		t.Fatal("potassium mmol/L alert applied to unverified units")
	}
	urine := syntheticMeasurement(t, catalog, "urine_glucose_dipstick", "4+", "[無參考值][]")
	if row := normalizedOne(t, catalog, urine, &settings); row.Level == nil || *row.Level != 3 {
		t.Fatal("blood glucose alert leaked into urine")
	}
}

func TestReferenceTemplatesDoNotInventApplicability(t *testing.T) {
	catalog := gradingCatalog(t)
	for _, id := range []string{"blood_ck_mb_activity", "blood_intact_pth", "blood_total_homocysteine", "serum_total_vitamin_b12", "plasma_d_dimer_unspecified", "capillary_postprandial_glucose"} {
		settings := DefaultSettings()
		row := normalizedOne(t, catalog, syntheticMeasurement(t, catalog, id, "200", "[無參考值][]"), &settings)
		if row.Level != nil || row.ReferenceLow != nil || row.ReferenceHigh != nil {
			t.Fatalf("%s silently used a named method, screening or treatment template", id)
		}
	}
}

func TestPreviouslyDynamicAliasesPreserveSettingsAndResets(t *testing.T) {
	catalog := gradingCatalog(t)
	oldID := dynamicID("09015C", "Creatinine(健檢)", "mg/dL")
	makeSettings := func() Settings {
		s := DefaultSettings()
		s.Known[oldID] = KnownItem{ID: oldID, Key: oldID, Name: "Creatinine(健檢)", Unit: "mg/dL", Category: "other"}
		s.Overrides[oldID] = Override{Enabled: true, ReferenceHigh: pointer("1.6")}
		s.Favorites = []string{oldID, "serum_creatinine"}
		return s
	}
	settings := makeSettings()
	catalog.ReconcileSettings(&settings)
	if len(settings.Known) != 1 || len(settings.Overrides) != 1 || settings.Overrides["serum_creatinine"].ReferenceHigh == nil || !reflect.DeepEqual(settings.Favorites, []string{"serum_creatinine"}) {
		t.Fatal("a recognized legacy alias lost its settings or duplicated favorites")
	}
	settings = makeSettings()
	settings.Overrides["serum_creatinine"] = Override{Enabled: true, ReferenceHigh: pointer("1.8")}
	catalog.ReconcileSettings(&settings)
	if len(settings.Overrides) != 2 || settings.Known[oldID].LegacyFor != "serum_creatinine" {
		t.Fatal("conflicting legacy settings were silently overwritten")
	}
	delete(settings.Overrides, "serum_creatinine")
	document, err := settings.Document()
	if err != nil {
		t.Fatal(err)
	}
	settings, err = DecodeSettings(document, 2)
	if err != nil {
		t.Fatal(err)
	}
	catalog.ReconcileSettings(&settings)
	if _, resurrected := settings.Overrides["serum_creatinine"]; resurrected {
		t.Fatal("restoring the canonical default resurrected a conflicting legacy override")
	}
	delete(settings.Overrides, oldID)
	catalog.ReconcileSettings(&settings)
	if _, retained := settings.Known[oldID]; retained {
		t.Fatal("reset legacy entry did not consolidate into the canonical setting")
	}
	serialized, err := json.Marshal(catalog.ForSettings(settings))
	if err != nil || strings.Contains(string(serialized), oldID) {
		t.Fatal("reset legacy alias remains in catalog")
	}
}
