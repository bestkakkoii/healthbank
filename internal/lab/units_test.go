// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package lab

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"strings"
	"testing"
)

func TestUnitSpellingAliasesNeverFoldPrefixes(t *testing.T) {
	for _, pair := range [][2]string{
		{"mg/dl", "mg/dL"}, {"µmol/l", "umol/L"}, {"μmol/L", "umol/L"},
		{"ng/ml", "ng/mL"}, {"pg/ml", "pg/mL"}, {"fl", "fL"}, {"U/l", "U/L"},
		{"x10^3/µl", "x10^3/uL"}, {"mL / min / 1.73m²", "mL/min/1.73m2"},
		{"ml/min/1.73m^2", "mL/min/1.73m2"},
	} {
		if unitKey(pair[0]) != unitKey(pair[1]) {
			t.Errorf("verified spelling alias rejected: %q and %q", pair[0], pair[1])
		}
	}
	for _, pair := range [][2]string{
		{"Mg/dL", "mg/dL"}, {"MG/DL", "mg/dL"}, {"ML/min/1.73m2", "mL/min/1.73m2"},
		{"Mmol/L", "mmol/L"}, {"UMOL/L", "umol/L"}, {"u/L", "U/L"},
		{"Ng/mL", "ng/mL"}, {"mg/gcr", "mg/gCr"}, {"/hpf", "/HPF"},
	} {
		if unitKey(pair[0]) == unitKey(pair[1]) {
			t.Errorf("unverified case alias accepted: %q and %q", pair[0], pair[1])
		}
	}
}

func TestAmbiguousUnitsNeverAcquireKnownRulesOrIdentity(t *testing.T) {
	catalog := gradingCatalog(t)
	for _, test := range []struct{ item, unit string }{
		{"serum_triglycerides", "Mg/dL"}, {"serum_triglycerides", "MG/DL"},
		{"egfr", "ML/min/1.73m2"}, {"serum_potassium", "Mmol/L"},
	} {
		settings := DefaultSettings()
		raw := syntheticMeasurement(t, catalog, test.item, "100", "[無參考值][]")
		raw.Unit, raw.OfficialFlag = test.unit, "SYNTHETIC_SOURCE_FLAG"
		row := normalizedOne(t, catalog, raw, &settings)
		if row.Level != nil || !strings.HasPrefix(row.ItemID, "custom_v2_") || row.Unit != raw.Unit || row.Result != raw.Result || row.OfficialFlag != raw.OfficialFlag {
			t.Fatalf("ambiguous unit inherited clinical interpretation or changed source: %+v", row)
		}
	}
	if dynamicID("SYNTHETIC", "Synthetic analyte", "mg/dL") == dynamicID("SYNTHETIC", "Synthetic analyte", "Mg/dL") {
		t.Fatal("unregistered measurements with ambiguous units share a personal configuration")
	}
}

func previousUnitFixtureKey(code, name, unit string) string {
	digest := sha256.Sum256([]byte(code + "\x00" + name + "\x00" + strings.ToLower(unit)))
	return "custom_" + hex.EncodeToString(digest[:10])
}

func TestPreviousCaseFoldedSettingsRemainIsolatedAndRecoverable(t *testing.T) {
	catalog := gradingCatalog(t)
	for _, unit := range []string{"mg/dL", "Mg/dL"} {
		t.Run(unit, func(t *testing.T) {
			settings := DefaultSettings()
			settings.Theme, settings.FontScale = "dark", 1.5
			id := previousUnitFixtureKey("09015C", "Creatinine(健檢)", unit)
			settings.Known[id] = KnownItem{ID: id, Key: id, Name: strings.Repeat("x", 300), Unit: unit, Category: "other"}
			settings.Overrides[id] = Override{Enabled: true, ReferenceHigh: pointer("1.600000000000000001")}
			settings.Overrides["serum_sodium"] = Override{Enabled: true, ReferenceHigh: pointer("144")}
			settings.Favorites = []string{id, "serum_sodium"}
			catalog.ReconcileSettings(&settings)
			retained := settings.Overrides[id]
			if retained.Enabled || retained.ReferenceHigh == nil || *retained.ReferenceHigh != "1.600000000000000001" || settings.Known[id].LegacyFor != unitReviewRequired {
				t.Fatal("ambiguous personal limits were deleted, promoted or left enabled")
			}
			if _, promoted := settings.Overrides["serum_creatinine"]; promoted {
				t.Fatal("a last-seen normal unit falsely proved the intent of a collided old key")
			}
			if settings.Theme != "dark" || settings.FontScale != 1.5 || !settings.Overrides["serum_sodium"].Enabled || !reflect.DeepEqual(settings.Favorites, []string{id, "serum_sodium"}) {
				t.Fatal("unrelated appearance, favorites or verified settings changed")
			}
			if err := ValidateKnown(settings.Known); err != nil {
				t.Fatalf("migration violated bounded setting metadata: %v", err)
			}
			before, _ := settings.Document()
			catalog.ReconcileSettings(&settings)
			after, _ := settings.Document()
			if string(before) != string(after) {
				t.Fatal("unit isolation is not idempotent")
			}
			retained.Enabled = true
			settings.Overrides[id] = retained
			if ValidateSettings(settings, catalog.ConfigurableItems(settings.Known)) == nil {
				t.Fatal("an isolated legacy key was re-enabled instead of requiring unit review")
			}
		})
	}
}

func TestIncompatibleObservedBuiltinUnitCannotCarryPersonalLimitsForward(t *testing.T) {
	catalog := gradingCatalog(t)
	settings := DefaultSettings()
	settings.Known["serum_creatinine"] = KnownItem{ID: "serum_creatinine", Key: "serum_creatinine", Name: "Synthetic creatinine", Unit: "Mg/dL", Category: "renal"}
	settings.Overrides["serum_creatinine"] = Override{Enabled: true, ReferenceHigh: pointer("1.6")}
	settings.Known["serum_sodium"] = KnownItem{ID: "serum_sodium", Key: "serum_sodium", Name: "Synthetic sodium", Unit: "mmol/L", Category: "electrolytes"}
	settings.Overrides["serum_sodium"] = Override{Enabled: true, ReferenceHigh: pointer("144")}
	settings.Favorites = []string{"serum_creatinine", "serum_sodium"}
	catalog.ReconcileSettings(&settings)
	if _, exists := settings.Overrides["serum_creatinine"]; exists || !settings.Overrides["serum_sodium"].Enabled {
		t.Fatal("incompatible old builtin limit leaked forward or correct settings were removed")
	}
	retained := 0
	for id, item := range settings.Known {
		if item.LegacyFor == unitReviewRequired {
			retained++
			if item.Unit != "Mg/dL" || settings.Overrides[id].Enabled || settings.Overrides[id].ReferenceHigh == nil || *settings.Overrides[id].ReferenceHigh != "1.6" {
				t.Fatal("source unit or retained personal limit changed")
			}
		}
	}
	if retained != 1 || len(settings.Favorites) != 2 {
		t.Fatal("migration lost the review entry or existing favorites")
	}
	before, _ := settings.Document()
	catalog.ReconcileSettings(&settings)
	after, _ := settings.Document()
	if string(before) != string(after) {
		t.Fatal("builtin isolation created duplicate retained limits")
	}
}

func TestOldDynamicSharedKeyCannotDisableVerifiedBuiltinSettings(t *testing.T) {
	catalog := gradingCatalog(t)
	settings := DefaultSettings()
	id := "custom_0123456789abcdefabcd"
	settings.Known[id] = KnownItem{ID: id, Key: "serum_sodium", Name: "Synthetic legacy shared setting", Unit: "mmol/L", Category: "electrolytes"}
	settings.Overrides["serum_sodium"] = Override{Enabled: true, ReferenceHigh: pointer("144")}
	settings.Favorites = []string{"serum_sodium"}
	if err := ValidateKnown(settings.Known); err != nil {
		t.Fatalf("fixture must be a permitted portable shared-key definition: %v", err)
	}
	if err := ValidateSettings(settings, catalog.ConfigurableItems(settings.Known)); err != nil {
		t.Fatalf("fixture must be a valid pre-migration settings document: %v", err)
	}
	catalog.ReconcileSettings(&settings)
	if item := settings.Known[id]; item.Key != id || item.LegacyFor != unitReviewRequired {
		t.Fatal("old shared-key metadata still owns a current verified configuration")
	}
	if override := settings.Overrides["serum_sodium"]; !override.Enabled || override.ReferenceHigh == nil || *override.ReferenceHigh != "144" {
		t.Fatal("isolating a legacy identity disabled an unrelated verified limit")
	}
	if !reflect.DeepEqual(settings.Favorites, []string{"serum_sodium"}) {
		t.Fatal("shared-key isolation changed existing favorites")
	}
	if err := ValidateSettings(settings, catalog.ConfigurableItems(settings.Known)); err != nil {
		t.Fatalf("legacy marker prevents the verified built-in limit from being saved: %v", err)
	}
}
