// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"bytes"
	"regexp"
	"slices"
	"strings"
)

// ProviderContractRevision identifies Healthbank's observed SPA projection, not
// an official API version or evidence that a real account returned every section.
const ProviderContractRevision = "nhi-spa.2026-10-28.2"
const maxSourceDiagnostics = 128

var sourceIdentifier = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,127}$`)
var sourceFieldIdentifier = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

type SourceDiagnostic struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Section  string `json:"section"`
	Count    *int   `json:"count,omitempty"`
	Message  string `json:"message"`
}

var sourceDiagnosticMessages = map[string]struct{ severity, message string }{
	"section_missing":         {"warning", "來源未回傳已知資料區塊；目前資料可能缺漏，不能視為此區塊沒有紀錄。"},
	"section_optional_absent": {"info", "來源未提供此選用資料區塊；沒有足夠資訊判斷是否存在其他紀錄。"},
	"section_null":            {"warning", "來源將已知資料區塊回傳為 null；未當成已確認的零筆資料。"},
	"section_empty":           {"info", "來源已知資料區塊回傳空資料集合；僅代表本次回應，未證明完整歷史為零筆。"},
	"known_result_missing":    {"warning", "部分資料列未提供已知結果欄位；仍保留日期、其他原文與欄位識別，未補造數值。"},
}

func sourceContractID(category, section string) string {
	if section == "" {
		section = "root"
	}
	return "nhi." + category + "." + strings.ToLower(section)
}

func addSourceDiagnostic(report *SupplementReport, code, section string, count *int) error {
	definition, ok := sourceDiagnosticMessages[code]
	if !ok || !sourceIdentifier.MatchString(section) {
		return changedError()
	}
	for i := range report.Diagnostics {
		item := &report.Diagnostics[i]
		if item.Code == code && item.Section == section {
			if item.Count != nil && count != nil {
				total := *item.Count + *count
				if total > MaxReportRecords {
					return changedError()
				}
				item.Count = &total
			}
			return nil
		}
	}
	if len(report.Diagnostics) >= maxSourceDiagnostics {
		return failure("RESPONSE_TOO_LARGE", "來源資料品質訊息超過上限，未回傳不完整的取得結果。")
	}
	report.Diagnostics = append(report.Diagnostics, SourceDiagnostic{Code: code, Severity: definition.severity, Section: section, Count: count, Message: definition.message})
	return nil
}

func sourceSectionDiagnostic(report *SupplementReport, category, section string, raw []byte, present, optional bool, count int) error {
	id := sourceContractID(category, section)
	if !present {
		code := "section_missing"
		if optional {
			code = "section_optional_absent"
		}
		return addSourceDiagnostic(report, code, id, nil)
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return addSourceDiagnostic(report, "section_null", id, nil)
	}
	if count == 0 {
		return addSourceDiagnostic(report, "section_empty", id, &count)
	}
	return nil
}

// Roles apply only to these already allowlisted source keys. A Chinese display
// label, coincident date or unknown source property never establishes semantics.
func sourceFieldRole(code string) string {
	switch code {
	case "assay_value", "assay_value_1", "c_assay_value", "value", "value1", "value2", "value3", "value4", "value5", "value6", "value7", "value8", "value9", "check_val", "inspect_result", "assay_result":
		return "result"
	case "unit_data", "c_unit_data", "dose_unit", "drug_num_unit", "drug_unit":
		return "unit"
	case "consult_value", "c_consult_value", "consult":
		return "reference"
	case "assay_method":
		return "method"
	case "inspect_mode":
		return "specimen"
	case "real_inspect_date", "c_real_inspect_date", "assay_date", "assay_upload_date", "sample_date", "confirm_date", "recv_date", "blood_chk_date", "urine_chk_date", "chk_date", "c_lung_date", "c_cat_date", "c_mmrc_date", "create_date", "func_date", "in_date", "out_date", "appl_s_date", "appl_e_date", "exe_s_date", "exe_e_date", "cure_e_date", "case_date", "close_date", "c_case_date", "c_close_date", "health_date", "valid_s_date", "valid_e_date", "upload_d", "upload_d2", "inoculate_d", "eatdate", "sport_date", "sportdate_date", "sdate", "edate", "cdate", "txt_date", "next_chk_date", "all_sdate", "all_edate":
		return "date"
	}
	return ""
}

func medicationFieldRoles(fields []SupplementField) {
	for i := range fields {
		field := &fields[i]
		switch field.Code {
		case "drug_code", "order_code":
			field.Role = "medication_code"
		case "drug_name", "assay_item_name":
			field.Role = "medication_name"
		case "drug_days", "order_drug_day":
			field.Role = "prescribed_days"
		case "drug_num", "order_qty":
			field.Role = "quantity"
		}
	}
}

func sourceResultMissing(category string, specs []fieldSpec, fields []SupplementField) bool {
	expected := false
	for _, spec := range specs {
		if sourceFieldRole(strings.ToLower(spec.key)) == "result" {
			expected = true
		}
	}
	for _, field := range fields {
		if field.Role == "result" || (strings.HasPrefix(category, "screen_") && slices.Contains([]string{"code_cname", "cytopathic_code", "lung_rads", "diagnosis_code", "hpv_diagnosis_code_rpt"}, field.Code)) {
			return false
		}
	}
	return expected
}

func providerEntryMetadata(entry *SupplementEntry, category string, section providerSection, inherited providerReference) {
	entry.ContractID = sourceContractID(category, section.root)
	entry.Origin = providerOrigin(category, section.root, inherited.kind)
	entry.DateRole = providerDateRole(category, section.date, section.end != "")
	entry.EndDateRole = providerDateRole(category, section.end, false)
	if section.date == "@parent" {
		entry.DateRole = inherited.dateRole
	}
	if entry.Date == "" {
		entry.DateRole = ""
	}
	if entry.EndDate == "" {
		entry.EndDateRole = ""
	}
}

func medicalEntryMetadata(entry *SupplementEntry, category, section string, dispensed bool) {
	entry.ContractID = sourceContractID(category, section)
	entry.Origin = "institutional"
	if entry.Date != "" {
		entry.DateRole = "encounter"
		if category == "medicine" || category == "chronic" {
			entry.DateRole = "unknown" // func_DATE is encounter/dispensing until the explicit flag is present
		}
		if dispensed {
			entry.DateRole = "dispensing"
		}
		if category == "hospital" {
			entry.DateRole = "admission"
		}
	}
	if entry.EndDate != "" {
		entry.EndDateRole = "discharge" // this header/list value is specifically out_DATE
	}
}

func providerDateRole(category, field string, hasEnd bool) string {
	switch strings.ToLower(field) {
	case "real_inspect_date", "c_real_inspect_date", "assay_date":
		return "examination"
	case "sample_date":
		return "specimen_collection"
	case "recv_date":
		return "report"
	case "func_date":
		if strings.HasPrefix(category, "screen_") {
			return "examination"
		}
		return "encounter"
	case "upload_d":
		return "uploaded"
	case "valid_s_date":
		return "valid_from"
	case "valid_e_date":
		return "valid_to"
	case "sdate":
		if category == "menstrual" {
			return "period_start"
		}
	case "edate":
		if category == "menstrual" {
			return "period_end"
		}
	case "case_date", "c_case_date":
		if hasEnd {
			return "period_start" // explicitly paired enrollment/closure, not disease onset
		}
	case "close_date", "c_close_date":
		return "period_end"
	case "health_date", "upload_d2", "inoculate_d", "eatdate", "sport_date", "sportdate_date", "create_date", "cdate":
		return "recorded"
	case "":
		return ""
	}
	return "unknown"
}

func providerOrigin(category, section, kind string) string {
	switch category {
	case "allergies":
		if section == "sP_IHKE3202S04" {
			return "self_entered"
		}
		return "institutional"
	case "vaccinations":
		if section == "sP_IHKE3203S03" {
			return "self_entered"
		}
		return "institutional"
	case "cochlear":
		if section == "maindata03" {
			return "self_entered"
		}
		return "institutional"
	case "diet", "exercise", "menstrual":
		return "self_entered"
	case "device_activity":
		return "device"
	case "physiology":
		if kind == "device_day" || section == "devicelist" {
			return "device"
		}
		return "mixed"
	case "body_measurements", "family_followup", "family_health_followup":
		return "mixed"
	case "e_prescriptions", "self_paid_medicine", "self_paid_material":
		return "institutional"
	case "organ_wishes", "catastrophic", "care_plans", "home_care", "family_doctor":
		return "official_summary"
	}
	return "unknown"
}

func validateSourceMetadata(report SupplementReport) error {
	if report.ContractRevision != "" && !sourceIdentifier.MatchString(report.ContractRevision) {
		return changedError()
	}
	for _, diagnostic := range report.Diagnostics {
		definition, known := sourceDiagnosticMessages[diagnostic.Code]
		if !known || diagnostic.Severity != definition.severity || diagnostic.Message != definition.message || !sourceIdentifier.MatchString(diagnostic.Section) || (diagnostic.Count != nil && (*diagnostic.Count < 0 || *diagnostic.Count > MaxReportRecords)) {
			return changedError()
		}
	}
	dateRoles := []string{"", "examination", "specimen_collection", "report", "encounter", "admission", "discharge", "claim_start", "claim_end", "prescription", "dispensing", "recorded", "uploaded", "valid_from", "valid_to", "period_start", "period_end", "unknown"}
	for _, entry := range report.Entries {
		if (entry.ContractID != "" && !sourceIdentifier.MatchString(entry.ContractID)) || !slices.Contains(dateRoles, entry.DateRole) || !slices.Contains(dateRoles, entry.EndDateRole) || !slices.Contains([]string{"", "institutional", "self_entered", "device", "official_summary", "mixed", "unknown"}, entry.Origin) {
			return changedError()
		}
		for _, field := range entry.Fields {
			if (field.Code != "" && !sourceFieldIdentifier.MatchString(field.Code)) || !slices.Contains([]string{"", "result", "unit", "reference", "method", "specimen", "date", "medication_code", "medication_name", "prescribed_days", "quantity"}, field.Role) || !slices.Contains([]string{"", "source", "public_contract"}, field.UnitBasis) {
				return changedError()
			}
		}
	}
	return nil
}
