// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"context"
	"net/http"
	"strings"
	"time"
)

var recentMedicalCategories = map[string]string{
	"recent_medicine": "medicine", "recent_chronic": "chronic",
	"recent_procedure": "procedure", "recent_hospital": "hospital",
}

func RecentMedicalCategory(category string) bool {
	_, known := recentMedicalCategories[category]
	return known
}

func recentMedicalRequest(category string) providerRequest {
	spec := supplements[category]
	fields := append([]fieldSpec{}, medicalHeaderFields...)
	fields = append(fields, fieldSpec{"icd9cm_name", "來源疾病分類（非診斷確認）", ""}, fieldSpec{"icD9CM_CODE", "來源疾病分類代碼", ""}, fieldSpec{"order_NAME", "來源檢查名稱", ""}, fieldSpec{"order_CODE_2Word", "來源檢查類別", ""})
	date := "func_DATE"
	if category == "recent_hospital" {
		date = "in_DATE"
	}
	section := providerSectionSpec("main_data", spec.title, "健康存摺／近期來源摘要（非全部歷史）", date, fields)
	section.end = "out_DATE"
	sections := []providerSection{section}
	if category == "recent_medicine" {
		chinese := section
		chinese.root, chinese.title, chinese.optional = "chinese_data", "近期中藥來源摘要", true
		sections = append(sections, chinese)
	}
	return providerRequest{path: "/api/ihke3000/" + spec.module + "/page_load", method: http.MethodGet, sections: sections}
}

// Called under the connector mutex. Recent summaries have independent source
// rows, so their handles never borrow or replace a date-search category's tree.
func (s *Session) fetchRecentSupplement(ctx context.Context, category string) (SupplementReport, error) {
	report := SupplementReport{Category: category, ContractRevision: ProviderContractRevision, Entries: []SupplementEntry{}, Coverage: supplements[category].coverage + "保留每個回傳區塊與來源起迄，並提供來源原有明細；不假設與歷史搜尋資料相同。", Complete: false}
	request := recentMedicalRequest(category)
	object, err := s.providerRead(ctx, request)
	if err != nil {
		return report, err
	}
	target := recentMedicalCategories[category]
	refs := map[string]medicalReference{}
	seen := false
	for _, section := range request.sections {
		rows, present, err := providerSectionRows(object, section)
		if err != nil {
			return report, err
		}
		seen = seen || present
		if err := sourceSectionDiagnostic(&report, category, section.root, object[strings.ToLower(section.root)], present, section.optional, len(rows)); err != nil {
			return report, err
		}
		for index, row := range rows {
			entry, err := providerEntry(row, section, providerReference{})
			if err != nil {
				return report, err
			}
			entry.Kind, entry.Key = "encounter", medicalKey(category, request.path, section.root, index)
			medicalEntryMetadata(&entry, target, section.root, false)
			entry.ContractID = sourceContractID(category, section.root)
			rowID, e1 := rowText(row, "row_id")
			sourceType, e2 := rowText(row, "ori_type")
			if e1 != nil || e2 != nil || (rowID != "" && !usableDocumentID(rowID)) || (sourceType != "" && !usableDocumentID(sourceType)) {
				return report, changedError()
			}
			if rowID != "" && sourceType != "" {
				if err := addMedicalChild(&entry, medicalReference{category: target, rowID: rowID, sourceType: sourceType, date: entry.Date, endDate: entry.EndDate, institution: entry.Institution}, refs); err != nil {
					return report, err
				}
			}
			report.Entries = append(report.Entries, entry)
		}
	}
	if !seen {
		return report, changedError()
	}
	if err := validateSupplementReport(report); err != nil {
		return report, err
	}
	if err := s.reserveCategoryReferences("medical/"+category, referenceMapSize(refs, medicalReferenceSize), category); err != nil {
		return report, err
	}
	if s.medicalReferences == nil {
		s.medicalReferences = map[string]map[string]medicalReference{}
	}
	s.medicalReferences[category] = refs
	report.FetchedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return report, nil
}

func (s *Session) FetchRecentMedicalDetail(ctx context.Context, category, reference string) (SupplementReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	report := SupplementReport{Category: category + "_detail", ContractRevision: ProviderContractRevision, Entries: []SupplementEntry{}, Complete: false}
	if !RecentMedicalCategory(category) {
		return report, failure("INVALID_INPUT", "不支援此近期摘要明細。")
	}
	if err := s.supplementReady(ctx); err != nil {
		return report, err
	}
	ref, present := s.medicalReferences[category][reference]
	if !present || ref.category != recentMedicalCategories[category] {
		return report, failure("INVALID_INPUT", "近期摘要明細識別已失效或不屬於此類別，請重新取得清單。")
	}
	return s.fetchMedicalDetail(ctx, category, reference, ref)
}
