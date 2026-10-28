// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

// Homepage and explicit-search arrays can differ. Each retains its own source
// section and row multiplicity; matching dates or IDs do not merge snapshots.
func appendMedicalListRows(report *SupplementReport, category, scope string, spec medicalSpec, rows []wireRow, refs map[string]medicalReference) error {
	dateField := "func_DATE"
	if category == "hospital" {
		dateField = "in_DATE"
	}
	section := providerSectionSpec(scope, spec.title, "官方"+spec.title+"／"+scope, dateField, medicalHeaderFields)
	section.end = "out_DATE"
	for index, row := range rows {
		entry, err := providerEntry(row, section, providerReference{})
		if err != nil {
			return err
		}
		rowID, e1 := rowText(row, "row_ID")
		sourceType, e2 := rowText(row, "ori_TYPE")
		if e1 != nil || e2 != nil || (rowID != "" && !usableDocumentID(rowID)) || (sourceType != "" && !usableDocumentID(sourceType)) {
			return changedError()
		}
		entry.Kind, entry.Key = "encounter", medicalKey(category, sourceType+"\x00"+rowID, scope, index)
		medicalEntryMetadata(&entry, category, scope, false)
		if rowID != "" && sourceType != "" {
			if err := addMedicalChild(&entry, medicalReference{category: category, rowID: rowID, sourceType: sourceType, date: entry.Date, endDate: entry.EndDate, institution: entry.Institution}, refs); err != nil {
				return err
			}
		}
		report.Entries = append(report.Entries, entry)
		if len(report.Entries) > MaxReportRecords {
			return failure("RESPONSE_TOO_LARGE", "來源首頁與搜尋資料合併後超過上限；未回傳截斷資料。")
		}
	}
	return nil
}

func medicalAvailabilitySection(category string) (providerSection, bool) {
	label := "來源可提供資料起日（非個別就醫日期）"
	switch category {
	case "western", "chinese", "dental", "medicine", "chronic":
	case "imaging":
		label = "來源 A／B 類型可提供資料起日（非個別檢查日期）"
	default:
		return providerSection{}, false
	}
	return providerMetadataSection("", "來源可提供資料起日", "健康存摺／來源可提供區間說明", []fieldSpec{{"c_sdate", label, ""}}), true
}
