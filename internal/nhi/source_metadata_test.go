// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestSourceDegradationDistinguishesMissingNullAndEmptyWithoutDroppingRows(t *testing.T) {
	s := providerFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(t, w, map[string]any{
			"sp_IHKE3410S01":           []any{map[string]any{"real_INSPECT_DATE": "115/10/01", "assay_VALUE": "<0.000000000000000001", "inspect_MODE": "合成檢體", "assay_METHOD": "合成方法"}},
			"sp_IHKE3410S02":           nil,
			"sp_IHKE3410S04":           []any{},
			"sp_IHKE3410S05":           []any{map[string]any{"inoculate_D": "115/10/02", "code_cname": "合成疫苗"}},
			"PRIVATE_UNKNOWN_PROPERTY": "DO_NOT_FORWARD_SYNTHETIC_VALUE",
		})
	})
	report, err := s.FetchProvider(context.Background(), "covid", Query{})
	if err != nil || len(report.Entries) != 2 || report.Complete || report.ContractRevision != ProviderContractRevision {
		t.Fatalf("partial response lost valid evidence: %v", err)
	}
	observed := map[string]string{}
	for _, item := range report.Diagnostics {
		observed[item.Section] = item.Code
	}
	for section, code := range map[string]string{"nhi.covid.sp_ihke3410s02": "section_null", "nhi.covid.sp_ihke3410s04": "section_empty", "nhi.covid.sp_ihke3410s09": "section_missing"} {
		if observed[section] != code {
			t.Fatalf("source states conflated: %s = %s", section, observed[section])
		}
	}
	fields := map[string]SupplementField{}
	for _, field := range report.Entries[0].Fields {
		fields[field.Code] = field
	}
	if fields["assay_value"].Value != "<0.000000000000000001" || fields["inspect_mode"].Role != "specimen" || fields["assay_method"].Role != "method" || report.Entries[0].DateRole != "examination" {
		t.Fatal("stable field identity, source precision or date meaning lost")
	}
	data, _ := json.Marshal(report)
	if strings.Contains(string(data), "DO_NOT_FORWARD") || strings.Contains(string(data), "PRIVATE_UNKNOWN") {
		t.Fatal("diagnostics disclosed unknown source content")
	}
}

func TestSourceKnownResultMissingAndOptionalAbsenceRemainExplicit(t *testing.T) {
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "3720") {
			respondJSON(t, w, map[string]any{"sP_IHKE3720S01": map[string]any{"status": "N"}})
			return
		}
		respondJSON(t, w, map[string]any{"maindata": []any{map[string]any{"real_INSPECT_DATE": "115/10/28", "hosp_ABBR": "合成院所"}}})
	})
	partial, err := s.FetchProvider(context.Background(), "hepatitis_test", Query{})
	if err != nil || len(partial.Entries) != 1 || len(partial.Diagnostics) != 1 || partial.Diagnostics[0].Code != "known_result_missing" || partial.Diagnostics[0].Count == nil || *partial.Diagnostics[0].Count != 1 {
		t.Fatalf("missing known value not disclosed: %v", err)
	}
	optional, err := s.FetchProvider(context.Background(), "family_doctor", Query{})
	if err != nil || len(optional.Diagnostics) != 1 || optional.Diagnostics[0].Code != "section_optional_absent" || optional.Diagnostics[0].Severity != "info" {
		t.Fatalf("optional absence misclassified: %v", err)
	}
}

func TestMedicationExtractionUsesStableCodesNotTranslatedLabels(t *testing.T) {
	fields, err := appendKnownFields(wireRow{"drug_code": json.RawMessage(`"SYNTHETIC"`), "drug_days": json.RawMessage(`"7.000"`)}, []fieldSpec{{"drug_CODE", "changed display label", ""}, {"drug_DAYS", "另一個顯示文字", "日"}})
	if err != nil {
		t.Fatal(err)
	}
	medicationFieldRoles(fields)
	if fields[0].Code != "drug_code" || fields[0].Role != "medication_code" || fields[1].Role != "prescribed_days" || fields[1].Value != "7.000" || fields[1].UnitBasis != "public_contract" {
		t.Fatal("source semantics depend on translated display text")
	}
}

func TestDateRoleDoesNotInventCollectionOrDiseasePeriods(t *testing.T) {
	if providerDateRole("diabetes_care", "case_DATE", false) != "unknown" || providerDateRole("catastrophic", "valid_S_DATE", true) != "valid_from" || providerDateRole("allergies", "upload_D", false) != "uploaded" {
		t.Fatal("administrative or ambiguous source dates reinterpreted")
	}
	entry := SupplementEntry{Date: "115/10/01", EndDate: "115/10/03"}
	medicalEntryMetadata(&entry, "hospital", "header", false)
	if entry.DateRole != "admission" || entry.EndDateRole != "discharge" {
		t.Fatal("explicit hospitalization interval lost")
	}
	entry = SupplementEntry{Date: "115/10/01"}
	medicalEntryMetadata(&entry, "medicine", "header", false)
	if entry.DateRole != "unknown" {
		t.Fatal("ambiguous medicine date became a prescription or dispensing assertion")
	}
	medicalEntryMetadata(&entry, "medicine", "header", true)
	if entry.DateRole != "dispensing" {
		t.Fatal("explicit source dispensing flag ignored")
	}
}
