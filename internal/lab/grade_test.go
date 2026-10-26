// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package lab

import (
	"strings"
	"testing"
)

func gradingCatalog(t *testing.T) *Catalog {
	t.Helper()
	catalog, err := LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func syntheticMeasurement(t *testing.T, catalog *Catalog, id, value, reference string) RawRecord {
	t.Helper()
	for _, definition := range catalog.Items {
		if definition.ID == id {
			return RawRecord{
				CheckDate: "115/01/20", VisitDate: "115/01/02", OrderCode: definition.Match.Code,
				OrderName: "Synthetic panel", ItemName: definition.Match.Name, Result: value,
				Unit: definition.Match.Unit, Reference: reference, Institution: "Synthetic institution", Source: "A",
			}
		}
	}
	t.Fatalf("missing catalog definition %s", id)
	return RawRecord{}
}

func normalizedOne(t *testing.T, catalog *Catalog, raw RawRecord, settings *Settings) Record {
	t.Helper()
	rows := catalog.Normalize([]RawRecord{raw}, settings)
	if len(rows) != 1 {
		t.Fatal("normalization did not preserve one observation")
	}
	return rows[0]
}

func TestExactGuidelineBoundaries(t *testing.T) {
	catalog := gradingCatalog(t)
	for _, tc := range []struct {
		item  string
		value string
		unit  string
		level int
	}{
		{"serum_triglycerides", "149.999999999999999999999", "", 1},
		{"serum_triglycerides", "150", "", 2},
		{"serum_triglycerides", "199.999999999999999999999", "", 2},
		{"serum_triglycerides", "200", "", 3},
		{"serum_triglycerides", "499.999999999999999999999", "", 3},
		{"serum_triglycerides", "500", "", 4},
		{"serum_triglycerides", "999.999999999999999999999", "", 4},
		{"serum_triglycerides", "1000", "", 5},
		{"urine_acr_quantitative", "29.999999999999999999999", "", 1},
		{"urine_acr_quantitative", "30", "", 3},
		{"urine_acr_quantitative", "300", "", 3},
		{"urine_acr_quantitative", "300.000000000000000000001", "", 4},
		{"urine_pcr", "150", "", 3},
		{"urine_pcr", "500", "", 3},
		{"urine_pcr", "500.000000000000000000001", "", 4},
		{"egfr", "14.999999999999999999999", "mL/min/1.73m2", 5},
		{"egfr", "15", "mL/min/1.73m2", 4},
		{"egfr", "30", "mL/min/1.73m2", 3},
		{"egfr", "45", "mL/min/1.73m2", 3},
		{"egfr", "60", "mL/min/1.73m2", 2},
		{"egfr", "90", "mL/min/1.73m2", 1},
	} {
		t.Run(tc.item+"/"+tc.value, func(t *testing.T) {
			settings := DefaultSettings()
			raw := syntheticMeasurement(t, catalog, tc.item, tc.value, "[無參考值][]")
			if tc.unit != "" {
				raw.Unit = tc.unit
			}
			row := normalizedOne(t, catalog, raw, &settings)
			if row.Level == nil || *row.Level != tc.level || row.Result != tc.value {
				t.Fatalf("boundary classified incorrectly; want application level %d", tc.level)
			}
		})
	}
}

func TestReferencePrecisionAndInclusiveBounds(t *testing.T) {
	for _, tc := range []struct {
		reference string
		result    string
		status    string
	}{
		{"≥0.0000001", "0", "low"},
		{"≥0.0000001", "0.0000001", "within"},
		{"0.0000001-0.0000002", "0.00000015", "within"},
		{"0.0000001-0.0000002", "0.00000001", "low"},
		{"≤0.0000001", "0.0000002", "high"},
		{"[3.5][5.1]", "3.5", "within"},
		{"[3.5][5.1]", "5.1", "within"},
		{"[3.5][5.1]", "3.499999999999999999999", "low"},
		{"[3.5][5.1]", "5.100000000000000000001", "high"},
		{"[3.5][5.1]", ">5.1", "high"},
		{"[3.5][5.1]", ">=5.1", "unknown"},
		{"[3.5][5.1]", "<3.5", "low"},
		{"[3.5][5.1]", "<=3.5", "unknown"},
		{"[3.5][5.1]", "3-6", "unknown"},
		{"<0.0000001", "0.0000001", "unknown"},
		{"[10][1]", "5", "unknown"},
		{"[無參考值][]", "0", "unknown"},
	} {
		t.Run(tc.reference+"/"+tc.result, func(t *testing.T) {
			low, high := parseReference(tc.reference)
			if got := referenceStatus(parseValue(tc.result), low, high); got != tc.status {
				t.Fatalf("want %s, got %s", tc.status, got)
			}
		})
	}
	low, high := parseReference("0.0000001-0.0000002")
	if low == nil || high == nil || *low != "0.0000001" || *high != "0.0000002" {
		t.Fatal("reference precision was rounded")
	}
	catalog := gradingCatalog(t)
	settings := DefaultSettings()
	row := normalizedOne(t, catalog, syntheticMeasurement(t, catalog, "serum_triglycerides", "150", "[0][150]"), &settings)
	if row.ReferenceStatus != "within" || row.Level == nil || *row.Level != 2 {
		t.Fatal("report reference bounds were conflated with guideline category bounds")
	}
}

func TestInequalitiesAndRangesDoNotBecomePointGuesses(t *testing.T) {
	catalog := gradingCatalog(t)
	for _, tc := range []struct {
		value string
		level *int
	}{
		{"大於等於300", nil},
		{">300", pointer(4)},
		{"≥301", pointer(4)},
		{"<30", pointer(1)},
		{"≤30", nil},
		{"30-300", pointer(3)},
		{"29-31", nil},
	} {
		t.Run(tc.value, func(t *testing.T) {
			settings := DefaultSettings()
			row := normalizedOne(t, catalog, syntheticMeasurement(t, catalog, "urine_acr_semiquantitative", tc.value, "[無參考值][]"), &settings)
			if row.NumericValue != nil || row.Result != tc.value {
				t.Fatal("interval was converted to a fabricated point observation")
			}
			if (row.Level == nil) != (tc.level == nil) || (row.Level != nil && *row.Level != *tc.level) {
				t.Fatal("interval crossing a category boundary was assigned an unjustified level")
			}
		})
	}
	settings := DefaultSettings()
	unitless := normalizedOne(t, catalog, syntheticMeasurement(t, catalog, "egfr", "25", "[無參考值][]"), &settings)
	if unitless.Level != nil || !strings.Contains(unitless.Interpretation, "單位") {
		t.Fatal("unverified eGFR unit received guideline grading")
	}
}

func TestInvalidDomainsMetadataAndMissingValues(t *testing.T) {
	catalog := gradingCatalog(t)
	for _, tc := range []struct{ item, value string }{
		{"venous_ph", "72.43"},
		{"serum_triglycerides", "-1"},
		{"blood_hba1c", "101"},
		{"serum_potassium", "-1-2"},
		{"serum_potassium", "20-10"},
	} {
		settings := DefaultSettings()
		row := normalizedOne(t, catalog, syntheticMeasurement(t, catalog, tc.item, tc.value, "[0][100]"), &settings)
		if row.ValueKind != "invalid" || row.NumericValue != nil || row.Level != nil {
			t.Fatalf("invalid physical domain or reversed range was graded for %s", tc.item)
		}
	}
	settings := DefaultSettings()
	for _, id := range []string{"venous_blood_gas_panel", "manual_smear_status"} {
		row := normalizedOne(t, catalog, syntheticMeasurement(t, catalog, id, "0", "[0][1]"), &settings)
		if row.ValueKind != "metadata" || row.Level != nil || row.NumericValue != nil {
			t.Fatal("metadata acquired a measurement level")
		}
	}
	missing := normalizedOne(t, catalog, syntheticMeasurement(t, catalog, "serum_potassium", "-", "[3.5][5.1]"), &settings)
	if missing.ValueKind != "missing" || missing.NumericValue != nil || missing.Level != nil || len(settings.Known) != 0 {
		t.Fatal("metadata or absent values created a numeric result/configuration")
	}
}

func TestPersonalOverridesCannotHideIndependentAlerts(t *testing.T) {
	catalog := gradingCatalog(t)
	for _, tc := range []struct {
		item      string
		value     string
		reference string
		level     int
		source    string
	}{
		{"serum_triglycerides", "500", "[0][150]", 4, "acc_hypertriglyceridemia"},
		{"serum_triglycerides", "1000", "[0][150]", 5, "acc_hypertriglyceridemia"},
		{"fasting_plasma_glucose", "53", "[70][100]", 5, "ada_hypoglycemia"},
		{"fasting_plasma_glucose", "54", "[70][100]", 4, "ada_hypoglycemia"},
	} {
		t.Run(tc.item+"/"+tc.value, func(t *testing.T) {
			settings := DefaultSettings()
			settings.Overrides[tc.item] = Override{Enabled: true, Bands: []Band{{Lower: pointer("0"), LowerInclusive: true, Level: 1}}}
			raw := syntheticMeasurement(t, catalog, tc.item, tc.value, tc.reference)
			raw.OfficialFlag = "1"
			row := normalizedOne(t, catalog, raw, &settings)
			if err := ValidateSettings(settings, settings.Known); err != nil {
				t.Fatal(err)
			}
			if row.Level == nil || *row.Level != tc.level || !strings.Contains(row.RuleSource, "personal") || !strings.Contains(row.RuleSource, tc.source) {
				t.Fatal("personal green override hid an independent alert or its source")
			}
			if row.ReferenceStatus == "within" || row.OfficialFlag != "1" || row.ReferenceText != tc.reference {
				t.Fatal("personal override rewrote immutable source references")
			}
		})
	}
}

func TestNonexactAlertsKeepTheirGuaranteedMinimum(t *testing.T) {
	catalog := gradingCatalog(t)
	for _, tc := range []struct {
		item      string
		value     string
		reference string
		level     int
		crosses   bool
	}{
		{"serum_triglycerides", ">500", "[0][150]", 4, true},
		{"serum_triglycerides", ">=500", "[0][150]", 4, true},
		{"serum_triglycerides", "500-1500", "[0][150]", 4, true},
		{"serum_triglycerides", ">999.9", "[0][150]", 4, true},
		{"serum_triglycerides", ">=1000", "[0][150]", 5, false},
		{"fasting_plasma_glucose", "<=60", "[70][100]", 4, true},
		{"fasting_plasma_glucose", "<=54", "[70][100]", 4, true},
		{"fasting_plasma_glucose", "0-54", "[70][100]", 4, true},
		{"fasting_plasma_glucose", "<54", "[70][100]", 5, false},
		{"fasting_plasma_glucose", "<70", "[70][100]", 4, true},
	} {
		for _, override := range []bool{false, true} {
			name := tc.item + "/" + tc.value
			if override {
				name += "/personal-green"
			}
			t.Run(name, func(t *testing.T) {
				settings := DefaultSettings()
				if override {
					settings.Overrides[tc.item] = Override{Enabled: true, Bands: []Band{{Lower: pointer("0"), LowerInclusive: true, Level: 1}}}
				}
				raw := syntheticMeasurement(t, catalog, tc.item, tc.value, tc.reference)
				raw.OfficialFlag = "1"
				row := normalizedOne(t, catalog, raw, &settings)
				if row.Level == nil || *row.Level != tc.level || row.NumericValue != nil || row.Result != tc.value {
					t.Fatal("nonexact result lost its guaranteed alert or became a fabricated point")
				}
				if !strings.Contains(row.Interpretation, "至少達本應用 L") || !strings.Contains(row.Interpretation, "不等式或區間") {
					t.Fatal("the alert minimum was presented as an exact-value classification")
				}
				if tc.crosses && !strings.Contains(row.Interpretation, "跨越來源分類界限") {
					t.Fatal("ambiguous source category was lost")
				}
				if row.OfficialFlag != "1" || row.ReferenceText != tc.reference {
					t.Fatal("alert floor changed the official reference context")
				}
			})
		}
	}
	for _, tc := range []struct{ item, value string }{
		{"serum_triglycerides", ">499"},
		{"serum_triglycerides", "499-1500"},
		{"fasting_plasma_glucose", "<=70"},
		{"fasting_plasma_glucose", "53-70"},
	} {
		settings := DefaultSettings()
		row := normalizedOne(t, catalog, syntheticMeasurement(t, catalog, tc.item, tc.value, "[無參考值][]"), &settings)
		if row.Level != nil {
			t.Fatal("a possible alert was incorrectly presented as a guaranteed alert")
		}
	}
}

func TestAlertUnionDoesNotBridgeExcludedEndpoints(t *testing.T) {
	bands := []Band{
		{Lower: pointer("500"), Upper: pointer("1000"), LowerInclusive: true, UpperInclusive: false, Level: 4},
		{Lower: pointer("1000"), LowerInclusive: false, Level: 5},
	}
	if _, ok := guaranteedAlertFloor(parseValue("999-1001"), bands); ok {
		t.Fatal("an uncovered shared endpoint was silently bridged")
	}
	bands[1].LowerInclusive = true
	if level, ok := guaranteedAlertFloor(parseValue("999-1001"), bands); !ok || *level != 4 {
		t.Fatal("adjacent covered bands did not establish their minimum level")
	}
}

func TestPersonalEGFROverrideRequiresVerifiedResultUnits(t *testing.T) {
	catalog := gradingCatalog(t)
	settings := DefaultSettings()
	settings.Overrides["egfr"] = Override{Enabled: true, Bands: []Band{{Lower: pointer("0"), LowerInclusive: true, Level: 1}}}
	raw := syntheticMeasurement(t, catalog, "egfr", "25", "[無參考值][]")
	unitless := normalizedOne(t, catalog, raw, &settings)
	if unitless.Level != nil || unitless.RuleSource == "personal" || !strings.Contains(unitless.Interpretation, "個人設定尚未套用") {
		t.Fatal("personal thresholds bypassed the missing-unit guard")
	}
	// A source interval still retains its own distinct reference context.
	raw.Reference = "[30][90]"
	withReference := normalizedOne(t, catalog, raw, &settings)
	if withReference.ReferenceStatus != "low" || withReference.RuleSource != "live_report" || withReference.Level == nil || *withReference.Level != 3 {
		t.Fatal("missing units erased an independent original report flag")
	}
	raw.Reference, raw.Unit = "[無參考值][]", "mL/min/1.73m2"
	verified := normalizedOne(t, catalog, raw, &settings)
	if verified.Level == nil || *verified.Level != 1 || verified.RuleSource != "personal" {
		t.Fatal("verified matching units did not permit the personal target")
	}
}

func TestPersonalIntervalsRejectDoubleIncludedEndpoints(t *testing.T) {
	settings := DefaultSettings()
	settings.Known["synthetic_measurement"] = KnownItem{ID: "synthetic_measurement", Key: "synthetic_measurement", Name: "Synthetic measurement"}
	first := Band{Lower: pointer("0"), Upper: pointer("10"), LowerInclusive: true, UpperInclusive: true, Level: 1}
	second := Band{Lower: pointer("10"), Upper: pointer("20"), LowerInclusive: true, UpperInclusive: true, Level: 2}
	settings.Overrides["synthetic_measurement"] = Override{Enabled: true, Bands: []Band{first, second}}
	if err := ValidateSettings(settings, settings.Known); err == nil {
		t.Fatal("overlapping inclusive endpoints were accepted")
	}
	first.UpperInclusive = false
	settings.Overrides["synthetic_measurement"] = Override{Enabled: true, Bands: []Band{first, second}}
	if err := ValidateSettings(settings, settings.Known); err != nil {
		t.Fatal(err)
	}
}

func TestSharedACRConfigurationAndSettingsExcludeObservations(t *testing.T) {
	catalog := gradingCatalog(t)
	settings := DefaultSettings()
	rows := []RawRecord{
		syntheticMeasurement(t, catalog, "urine_acr_semiquantitative", "大於等於300", "[無參考值][]"),
		syntheticMeasurement(t, catalog, "urine_acr_quantitative", "345.6789", "[無參考值][]"),
	}
	normalized := catalog.Normalize(rows, &settings)
	configuration := catalog.ForSettings(settings)
	if len(normalized) != 2 || normalized[0].ID == normalized[1].ID || normalized[0].ConfigurationKey != normalized[1].ConfigurationKey || len(configuration.Items) != 1 {
		t.Fatal("shared ACR configuration was duplicated or separate observations were merged")
	}
	document, err := settings.Document()
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"345.6789", "115/01/20", "115/01/02", "Synthetic institution", "大於等於300"} {
		if strings.Contains(string(document), value) {
			t.Fatal("an observation value, date or institution entered persisted settings")
		}
	}
}
