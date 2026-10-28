// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"strings"
)

// The care-plan page contains its own source snapshots as well as enrollment.
// They have no source examination dates and must not inherit an enrollment date
// or the page's client-side color thresholds.
func extendCareCoverageSpecs(items map[string]providerSpec) {
	spec := items["care_plans"]
	physiology := providerSectionSpec("physiology", "照護計畫內嵌生理量測摘要", "健康存摺／照護計畫內嵌生理量測摘要", "", []fieldSpec{{"code_name", "來源量測名稱", ""}, {"value", "來源量測值（原文）", ""}, {"unit_data", "來源單位", ""}, {"consult_min_value", "來源參考下限（原文）", ""}, {"consult_max_value", "來源參考上限（原文）", ""}})
	physiology.titleField = "code_name"
	bloodsugar := providerSectionSpec("bloodsugar", "照護計畫內嵌糖化血色素（HbA1c）摘要", "健康存摺／照護計畫內嵌 HbA1c 摘要", "", []fieldSpec{{"assaY_VALUE", "來源 HbA1c 結果（含原文單位）", ""}})
	lipids := providerSectionSpec("booldlipid", "照護計畫內嵌血脂摘要", "健康存摺／照護計畫內嵌血脂摘要", "", []fieldSpec{{"curE_CNAME", "來源檢驗項目", ""}, {"assaY_VALUE", "來源檢驗值（原文）", ""}, {"uniT_DATA", "來源單位", ""}, {"consult", "來源參考值（原文）", ""}, {"ordeR_CODE", "來源檢驗代碼", ""}})
	lipids.titleField = "curE_CNAME"
	spec.requests[0].sections = append(spec.requests[0].sections, physiology, bloodsugar, lipids)
	spec.coverage += "另保留照護計畫頁內嵌的生理量測、HbA1c 與血脂摘要及來源參考值；沒有原始採檢日期的摘要不推造日期，也不套用官網前端自行計算的顏色判讀。"
	items["care_plans"] = spec

	home := items["home_care"]
	home.requests[0].sections[0].fields = append(home.requests[0].sections[0].fields, fieldSpec{"c_MAIN_PRSN_NAME", "來源主治醫師姓名", ""}, fieldSpec{"c_NURSE_NAME", "來源主責護理師姓名", ""}, fieldSpec{"c_BRE_PRSN_HOSP_NAME", "來源主治呼吸治療師姓名", ""})
	items["home_care"] = home
	copd := items["copd_care"]
	copd.requests[0].sections = append(copd.requests[0].sections, providerMetadataSection("", "肺阻塞照護來源提供區間", "健康存摺／肺阻塞照護來源提供區間（非個別檢查日期）", []fieldSpec{{"sdate", "來源提供區間起日（原文）", ""}, {"edate", "來源提供區間迄日（原文）", ""}}))
	items["copd_care"] = copd

	for _, category := range []string{"family_followup", "family_health_followup"} {
		family := items[category]
		nestedRoot, referenceRoot := familyFollowupRoots(category)
		for index := range family.requests[0].sections {
			section := &family.requests[0].sections[index]
			// These paths declare the custom reader's nested projection. They
			// are not flattened by appendKnownFields or joined to S03 by index.
			section.fields = append([]fieldSpec{}, section.fields...)
			for _, field := range familyFollowupResultFields(category) {
				section.fields = append(section.fields, fieldSpec{nestedRoot + "[]." + field.key, field.label, field.unit})
			}
		}
		reference := providerMetadataSection(referenceRoot, "家庭醫師來源分類與參考資料", "健康存摺／家庭醫師來源分類與參考資料（非 Healthbank 判讀）", familyFollowupReferenceFields)
		family.requests[0].sections = append(family.requests[0].sections, reference)
		if category == "family_health_followup" {
			pressure := reference
			pressure.root, pressure.title, pressure.object = "bp_IHKE3726S03", "家庭醫師來源血壓分類與參考資料", true
			family.requests[0].sections = append(family.requests[0].sections, pressure)
		}
		contextFields := append(append([]fieldSpec{}, familyFollowupContextFields...), fieldSpec{"memo[]", "來源追蹤註記（逐項原文）", ""}, fieldSpec{"health_edu[]", "來源附帶說明（逐項原文，非 Healthbank 建議）", ""})
		family.requests[0].sections = append(family.requests[0].sections, providerMetadataSection("", "家庭醫師來源追蹤狀態與說明", "健康存摺／家庭醫師來源追蹤狀態與說明", contextFields))
		family.coverage += "清單內嵌結果、來源分類與參考值、狀態及逐項說明另行保留；不假設與日期搜尋回應相同，不按清單位置推定同次採檢或套用來源前端的顏色判讀。"
		if category == "family_health_followup" {
			family.coverage += "血壓歷史使用來源固定 EBP 選項。"
		}
		items[category] = family
	}
}

var familyFollowupReferenceFields = []fieldSpec{{"diseasE_STAGE_NAME", "來源分期／分類名稱（非 Healthbank 診斷）", ""}, {"assaY_NAME", "來源檢驗名稱", ""}, {"assaY_ZH_NAME", "來源中文名稱", ""}, {"consulT_VALUE_MAX", "來源參考上限（原文）", ""}, {"consulT_VALUE_LOWER", "來源參考下限（原文）", ""}, {"uniT_DATA", "來源參考資料單位（未推定套用至其他列）", ""}}
var familyFollowupContextFields = []fieldSpec{{"hasillCategory", "來源追蹤適用狀態（原文）", ""}, {"title", "來源追蹤對象說明（原文）", ""}, {"non_case_desc", "來源未適用說明（原文）", ""}}

func familyFollowupRoots(category string) (string, string) {
	if category == "family_health_followup" {
		return "sP_IHKE3726S04", "sP_IHKE3726S03"
	}
	return "sP_IHKE3721S04", "sP_IHKE3721S03"
}

func familyFollowupResultFields(category string) []fieldSpec {
	fields := []fieldSpec{{"assaY_DATE", "來源檢驗日期（原文）", ""}, {"assaY_NAME", "來源檢驗名稱", ""}, {"assaY_VALUE", "來源檢驗值（原文）", ""}, {"hosP_ABBR", "來源醫事機構", ""}, {"assaY_SOURCE_NAME", "資料來源（原文）", ""}}
	if category == "family_health_followup" {
		fields = append(fields, fieldSpec{"c_ASSAY_VALUE", "來源組合顯示值（原文）", ""})
	}
	return fields
}

// The default page contains results that the source can return independently
// from S07's requested-date history. Retain both channels and all duplicate rows.
func (s *Session) providerFamilyFollowupCoverageFetch(ctx context.Context, report *SupplementReport, category string, query Query, refs map[string]providerReference) (bool, error) {
	if category != "family_followup" && category != "family_health_followup" {
		return false, nil
	}
	request := providerSpecs[category].requests[0]
	object, err := s.providerRead(ctx, request)
	if err != nil {
		return true, err
	}
	nestedRoot, _ := familyFollowupRoots(category)
	seen := false
	for _, section := range request.sections {
		if section.root == "" {
			continue
		}
		rows, present, err := providerSectionRows(object, section)
		if err != nil {
			return true, err
		}
		seen = seen || present
		if err := sourceSectionDiagnostic(report, category, section.root, object[strings.ToLower(section.root)], present, section.optional, len(rows)); err != nil {
			return true, err
		}
		for index, row := range rows {
			plain := section
			plain.fields = nil
			for _, field := range section.fields {
				if !strings.Contains(field.key, "[].") {
					plain.fields = append(plain.fields, field)
				}
			}
			entry, err := familyFollowupEntry(row, plain)
			if err != nil {
				return true, err
			}
			entry.Key = medicalKey(category, request.path, section.root, index)
			providerEntryMetadata(&entry, category, section, providerReference{})
			if section.detail {
				entry.Kind = "source_index"
				ref, err := providerMakeReference(category, row, entry, query)
				if err != nil {
					return true, err
				}
				if section.root == "bp_IHKE3726S02" {
					ref.rowID = "EBP" // the source's More Records selector, not its display name
				}
				if ref.rowID != "" {
					if err := personalAddReference(refs, &entry, ref); err != nil {
						return true, err
					}
				}
			} else {
				entry.Kind, entry.Origin = "source_metadata", "official_summary"
			}
			report.Entries = append(report.Entries, entry)
			if section.detail {
				if err := appendFamilyFollowupResults(report, row, category, request.path, nestedRoot, entry); err != nil {
					return true, err
				}
			}
			if len(report.Entries) > MaxReportRecords {
				return true, failure("RESPONSE_TOO_LARGE", "家庭醫師追蹤資料合併後超過上限，未回傳截斷資料。")
			}
		}
	}
	contextSeen, err := appendFamilyFollowupContext(report, object, category, request.path)
	if err != nil {
		return true, err
	}
	if !seen && !contextSeen {
		return true, changedError()
	}
	return true, nil
}

// Known null/empty values remain a source row. Unknown-only objects fail closed;
// no parent date, unit, institution or reference threshold is synthesized.
func familyFollowupEntry(row wireRow, section providerSection) (SupplementEntry, error) {
	known := false
	for _, field := range section.fields {
		_, present := row[strings.ToLower(field.key)]
		known = known || present
	}
	if !known {
		return SupplementEntry{}, changedError()
	}
	fields, err := appendKnownFields(row, section.fields)
	if err != nil {
		return SupplementEntry{}, err
	}
	entry := SupplementEntry{Title: section.title, Source: section.source, Fields: fields}
	if section.date != "" {
		entry.Date, err = rowText(row, section.date)
		if err != nil {
			return SupplementEntry{}, err
		}
	}
	entry.Institution, err = rowText(row, "hosp_abbr")
	if err != nil {
		return SupplementEntry{}, err
	}
	if section.titleField != "" {
		title, err := rowText(row, section.titleField)
		if err != nil {
			return SupplementEntry{}, err
		}
		if title != "" {
			entry.Title = title
		}
	}
	return entry, nil
}

func appendFamilyFollowupResults(report *SupplementReport, parent wireRow, category, path, root string, indexEntry SupplementEntry) error {
	section := providerSectionSpec(root, "家庭醫師清單內嵌檢驗結果", "健康存摺／家庭醫師清單內嵌結果", "assaY_DATE", familyFollowupResultFields(category))
	rows, present, err := providerSectionRows(parent, section)
	if err != nil {
		return err
	}
	if err := sourceSectionDiagnostic(report, category, root, parent[strings.ToLower(root)], present, false, len(rows)); err != nil {
		return err
	}
	missing := 0
	for index, row := range rows {
		entry, err := familyFollowupEntry(row, section)
		if err != nil {
			return err
		}
		entry.Key = medicalKey(category, path+"\x00"+indexEntry.Key, root, index)
		providerEntryMetadata(&entry, category, section, providerReference{})
		if sourceResultMissing(category, section.fields, entry.Fields) {
			missing++
		}
		entry.Fields = append(entry.Fields, SupplementField{Name: "所屬來源追蹤項目", Value: indexEntry.Title})
		report.Entries = append(report.Entries, entry)
		if len(report.Entries) > MaxReportRecords {
			return failure("RESPONSE_TOO_LARGE", "家庭醫師追蹤資料合併後超過上限，未回傳截斷資料。")
		}
	}
	if missing > 0 {
		return addSourceDiagnostic(report, "known_result_missing", sourceContractID(category, root), &missing)
	}
	return nil
}

func appendFamilyFollowupContext(report *SupplementReport, object wireRow, category, path string) (bool, error) {
	projected, seen := wireRow{}, false
	for _, field := range familyFollowupContextFields {
		key := strings.ToLower(field.key)
		raw, present := object[key]
		if !present {
			continue
		}
		seen = true
		if key == "hasillcategory" {
			switch string(bytes.TrimSpace(raw)) {
			case "true", "false":
				raw = json.RawMessage(strconv.Quote(string(bytes.TrimSpace(raw))))
			case "null":
				if err := addSourceDiagnostic(report, "section_null", sourceContractID(category, key), nil); err != nil {
					return false, err
				}
			default:
				return false, changedError()
			}
		}
		projected[key] = raw
	}
	if len(projected) > 0 {
		section := providerSectionSpec("", "家庭醫師來源追蹤狀態與說明", "健康存摺／家庭醫師來源狀態（非 Healthbank 判讀）", "", familyFollowupContextFields)
		entry, err := familyFollowupEntry(projected, section)
		if err != nil {
			return false, err
		}
		entry.Key, entry.Kind, entry.Origin, entry.ContractID = medicalKey(category, path, "source_context", 0), "source_metadata", "official_summary", sourceContractID(category, "source_context")
		report.Entries = append(report.Entries, entry)
	}
	for _, item := range []struct{ key, title string }{{"memo", "來源追蹤註記（原文）"}, {"health_edu", "來源附帶說明（原文，非 Healthbank 建議）"}} {
		raw, present := object[item.key]
		seen = seen || present
		var notes []json.RawMessage
		if present && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) && (decodeJSON(raw, &notes) != nil || notes == nil || len(notes) > MaxReportRecords) {
			return false, changedError()
		}
		if err := sourceSectionDiagnostic(report, category, item.key, raw, present, true, len(notes)); err != nil {
			return false, err
		}
		for index, rawNote := range notes {
			var note string
			part := bytes.TrimSpace(rawNote)
			if len(part) == 0 || part[0] != '"' || json.Unmarshal(part, &note) != nil {
				return false, changedError()
			}
			report.Entries = append(report.Entries, SupplementEntry{Key: medicalKey(category, path, item.key, index), Kind: "source_metadata", ContractID: sourceContractID(category, item.key), Origin: "official_summary", Title: item.title, Source: "健康存摺／家庭醫師來源附帶說明", Fields: []SupplementField{{Code: item.key, Name: item.title, Value: note}}})
		}
	}
	return seen, nil
}

func (s *Session) providerCarePlanFetch(ctx context.Context, report *SupplementReport) error {
	const category = "care_plans"
	request := providerSpecs[category].requests[0]
	object, err := s.providerRead(ctx, request)
	if err != nil {
		return err
	}
	seen := false
	for _, section := range request.sections {
		rows, present, err := providerSectionRows(object, section)
		if err != nil {
			return err
		}
		seen = seen || present
		if err := sourceSectionDiagnostic(report, category, section.root, object[strings.ToLower(section.root)], present, section.optional, len(rows)); err != nil {
			return err
		}
		missing := 0
		for index, row := range rows {
			// The public page explicitly handles an absent/empty HbA1c result.
			// A known null result is a retained source row with a missing-result
			// diagnostic, not a fabricated zero or an unknown schema success.
			entry, err := providerEntry(row, section, providerReference{})
			if err != nil {
				_, known := row["assay_value"]
				value, valueErr := rowText(row, "assay_value")
				if section.root != "bloodsugar" || !known || valueErr != nil || value != "" {
					return err
				}
				entry = SupplementEntry{Title: section.title, Source: section.source, Fields: []SupplementField{}}
			}
			entry.Key = medicalKey(category, request.path, section.root, index)
			providerEntryMetadata(&entry, category, section, providerReference{})
			if sourceResultMissing(category, section.fields, entry.Fields) {
				missing++
			}
			report.Entries = append(report.Entries, entry)
			if len(report.Entries) > MaxReportRecords {
				return failure("RESPONSE_TOO_LARGE", "照護計畫與摘要超過上限，未回傳截斷資料。")
			}
		}
		if missing > 0 {
			if err := addSourceDiagnostic(report, "known_result_missing", sourceContractID(category, section.root), &missing); err != nil {
				return err
			}
		}
	}
	if !seen {
		return changedError()
	}
	return nil
}
