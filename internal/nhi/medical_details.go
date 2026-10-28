// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

func medicalDetailSections(category string, spec medicalSpec) []medicalSection {
	if category != "western" && category != "chinese" && category != "dental" && category != "hospital" {
		return spec.sections
	}
	sections := append([]medicalSection(nil), spec.sections...)
	return append(sections,
		medicalSection{"icdcode_data", "source_classification", "來源疾病次分類（非本站診斷）", []fieldSpec{{"icd_tit", "來源分類項目", ""}, {"icd_code_name", "來源疾病次分類（原文）", ""}}},
		medicalSection{"opcode_data", "source_classification", "來源次處置", []fieldSpec{{"op_tit", "來源處置項目", ""}, {"op_code_name", "來源次處置名稱（原文）", ""}}},
	)
}

func addMedicalChild(entry *SupplementEntry, ref medicalReference, children map[string]medicalReference) error {
	if len(children) >= MaxReportRecords {
		return failure("RESPONSE_TOO_LARGE", "來源醫療明細索引過多，未回傳不完整資料。")
	}
	id, err := documentToken()
	if err != nil {
		return err
	}
	ref.children = nil
	children[id], entry.DetailID = ref, id
	return nil
}

func bindMedicalLaboratory(entry *SupplementEntry, row wireRow, flag, orderField string, parent medicalReference, children map[string]medicalReference) error {
	raw, present := row[flag]
	if !present || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || bytes.Equal(bytes.TrimSpace(raw), []byte("false")) {
		return nil
	}
	if !bytes.Equal(bytes.TrimSpace(raw), []byte("true")) {
		return changedError()
	}
	order := ""
	if orderField != "" {
		var err error
		order, err = rowText(row, orderField)
		if err != nil || !usableDocumentID(order) {
			return changedError()
		}
	}
	ref := parent
	ref.kind, ref.orderCode = "laboratory", order
	return addMedicalChild(entry, ref, children)
}

var medicalLaboratoryFields = []fieldSpec{
	{"creal_INSPECT_DATE", "來源中文檢查日期（原文）", ""}, {"ereal_INSPECT_DATE", "來源英文檢查日期（原文）", ""},
	{"order_CODE", "來源醫令代碼", ""}, {"cure_CNAME", "來源醫令名稱（原文）", ""}, {"assay_ITEM_NAME", "來源檢驗項目名稱", ""},
	{"assay_VALUE", "來源檢驗結果（原文，未推定單位）", ""}, {"consult_VALUE", "來源參考值（原文）", ""},
}

func medicalSubdetailRequest(ref medicalReference) (providerRequest, error) {
	numeric := providerSectionSpec("sp_IHKE3302S20_data_list", "來源檢驗結果", "健康存摺／醫囑連結檢驗結果（未推定與其他來源同次採檢）", "creal_INSPECT_DATE", medicalLaboratoryFields)
	numeric.titleField = "assay_ITEM_NAME"
	images := providerSectionSpec("sp_IHKE3302S21_data_list", "來源影像／病理報告索引", "健康存摺／醫囑連結影像與病理", "creal_INSPECT_DATE", []fieldSpec{{"creal_INSPECT_DATE", "來源中文檢查日期（原文）", ""}, {"ereal_INSPECT_DATE", "來源英文檢查日期（原文）", ""}, {"cure_CNAME", "來源醫令名稱（原文）", ""}})
	images.titleField = "cure_CNAME"
	request := providerRequest{method: http.MethodGet}
	switch ref.kind {
	case "laboratory":
		request.path = "/api/ihke3000/ihke3303s03/pageload"
		request.params = url.Values{"crid": {ref.rowID}, "ctype": {ref.sourceType}, "ocode": {ref.orderCode}}
		request.sections = []providerSection{numeric, images}
	case "laboratory_history":
		request.path = "/api/ihke3000/ihke3303s04/pageload"
		request.params = url.Values{"ocode": {ref.orderCode}}
		request.sections = []providerSection{numeric}
	case "imaging_history":
		request.path = "/api/ihke3000/ihke3303s05/pageload"
		request.params = url.Values{"ocode": {ref.orderCode}}
		request.sections = []providerSection{images}
	default:
		return request, failure("INVALID_INPUT", "不支援此來源檢驗明細。")
	}
	return request, nil
}

func (s *Session) fetchMedicalSubdetail(ctx context.Context, owner, reference string, ref medicalReference, report SupplementReport) (SupplementReport, error) {
	if ref.kind == "imaging" {
		return s.fetchImagingDetail(ctx, owner, reference, westernReference{rowID: ref.rowID, sourceType: ref.sourceType, date: ref.date, institution: ref.institution})
	}
	request, err := medicalSubdetailRequest(ref)
	if err != nil {
		return report, err
	}
	object, err := s.providerRead(ctx, request)
	if err != nil {
		return report, err
	}
	report.Coverage = "讀取來源醫囑連結的檢驗／影像資料與原有歷次紀錄入口；來源未提供日期搜尋或獨立總數，不保證全部終身歷史。不把就醫日期當成檢查日期。"
	children := make(map[string]medicalReference)
	history := make(map[string]bool)
	seen := false
	for _, section := range request.sections {
		rows, present, err := providerSectionRows(object, section)
		if err != nil {
			return report, err
		}
		seen = seen || present
		if err := sourceSectionDiagnostic(&report, owner, section.root, object[strings.ToLower(section.root)], present, false, len(rows)); err != nil {
			return report, err
		}
		missingResults := 0
		for index, row := range rows {
			entry, err := providerEntry(row, section, providerReference{})
			if err != nil {
				return report, err
			}
			entry.Key = medicalKey(ref.sourceType, ref.rowID+"\x00"+ref.orderCode, ref.kind+"\x00"+section.root, index)
			entry.Origin, entry.ContractID = "mixed", sourceContractID(owner, section.root)
			if entry.Date != "" {
				entry.DateRole = "examination"
			}
			if sourceResultMissing(owner, section.fields, entry.Fields) {
				missingResults++
			}
			if section.root == "sp_IHKE3302S21_data_list" {
				rowID, e1 := rowText(row, "row_ID")
				sourceType, e2 := rowText(row, "ori_TYPE")
				if e1 != nil || e2 != nil || !usableDocumentID(rowID) || !usableDocumentID(sourceType) {
					return report, changedError()
				}
				child := medicalReference{category: ref.category, kind: "imaging", rowID: rowID, sourceType: sourceType, date: entry.Date, institution: entry.Institution}
				if err := addMedicalChild(&entry, child, children); err != nil {
					return report, err
				}
				entry.Kind = "source_index"
				if ref.kind == "laboratory" && ref.orderCode != "" {
					history["imaging_history\x00"+ref.orderCode] = true
				}
			} else if ref.kind == "laboratory" && ref.orderCode != "" {
				code, err := rowText(row, "order_CODE")
				if err != nil || !usableDocumentID(code) {
					return report, changedError()
				}
				history["laboratory_history\x00"+code] = true
			}
			report.Entries = append(report.Entries, entry)
			if len(report.Entries) > MaxReportRecords {
				return report, failure("RESPONSE_TOO_LARGE", "來源檢驗資料過多，未回傳不完整明細。")
			}
		}
		if missingResults > 0 {
			if err := addSourceDiagnostic(&report, "known_result_missing", sourceContractID(owner, section.root), &missingResults); err != nil {
				return report, err
			}
		}
	}
	if !seen {
		return report, changedError()
	}
	keys := make([]string, 0, len(history))
	for key := range history {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for index, key := range keys {
		kind, code, _ := strings.Cut(key, "\x00")
		entry := SupplementEntry{Key: medicalKey(ref.sourceType, ref.rowID, key, index), Kind: "source_index", Origin: "mixed", ContractID: sourceContractID(owner, "order_history_index"), Title: "來源醫令歷次紀錄", Source: "健康存摺／來源醫令歷次紀錄入口", Fields: []SupplementField{{Name: "來源醫令代碼", Code: "order_code", Value: code}}}
		child := ref
		child.kind, child.orderCode, child.date, child.endDate, child.institution = kind, code, "", "", ""
		if err := addMedicalChild(&entry, child, children); err != nil {
			return report, err
		}
		report.Entries = append(report.Entries, entry)
	}
	if err := validateSupplementReport(report); err != nil {
		return report, err
	}
	if err := s.commitMedicalDetailReferences(owner, reference, ref, children, nil); err != nil {
		return report, err
	}
	report.FetchedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return report, nil
}

// The ancestry is stored on each reference. Refreshing one encounter therefore
// retires only its descendants, without copying or scanning every medical row.
// Both child and document accounting are admitted before any live handle changes.
func (s *Session) commitMedicalDetailReferences(category, parent string, ref medicalReference, children map[string]medicalReference, documents map[string]documentReference) error {
	s.initializeReferenceAccounting()
	current := s.medicalReferences[category]
	previous, parentExists := current[parent]
	retired := make(map[string]bool)
	queue := append([]string(nil), previous.children...)
	for index := 0; index < len(queue); index++ {
		id := queue[index]
		if id == parent || retired[id] {
			continue
		}
		child, exists := current[id]
		if !exists {
			continue
		}
		retired[id] = true
		queue = append(queue, child.children...)
	}
	count := len(current) - len(retired) + len(children)
	if !parentExists {
		count++
	}
	if count > MaxReportRecords {
		return failure("RESPONSE_TOO_LARGE", "醫療明細索引數量過多，先前取得資料已保留。")
	}
	ref.children = make([]string, 0, len(children))
	for id := range children {
		ref.children = append(ref.children, id)
	}
	sort.Strings(ref.children)
	size := s.referenceGroups["medical/"+category]
	if len(current) == 0 {
		size += referenceMapOverhead
	}
	for id := range retired {
		size -= referenceEntryOverhead + int64(len(id)) + medicalReferenceSize(current[id])
	}
	if parentExists {
		size -= referenceEntryOverhead + int64(len(parent)) + medicalReferenceSize(previous)
	}
	size += referenceEntryOverhead + int64(len(parent)) + medicalReferenceSize(ref)
	for id, child := range children {
		size += referenceEntryOverhead + int64(len(id)) + medicalReferenceSize(child)
	}
	documentSize, documentCount := int64(0), 0
	retireDocument := func(document documentReference) bool {
		return document.category == category && (document.parent == parent || retired[document.parent])
	}
	for id, document := range s.documentReferences {
		if !retireDocument(document) {
			documentCount++
			documentSize += referenceEntryOverhead + int64(len(id)) + documentReferenceSize(document)
		}
	}
	for id, document := range documents {
		document.category = category
		documents[id] = document
		documentCount++
		documentSize += referenceEntryOverhead + int64(len(id)) + documentReferenceSize(document)
	}
	if documentCount > MaxReportRecords {
		return failure("RESPONSE_TOO_LARGE", "醫療附件索引數量過多，先前取得資料已保留。")
	}
	if documentCount > 0 {
		documentSize += referenceMapOverhead
	}
	if err := s.reserveReferenceGroups(map[string]int64{"medical/" + category: size, "documents": documentSize}); err != nil {
		return err
	}
	if s.medicalReferences == nil {
		s.medicalReferences = make(map[string]map[string]medicalReference)
	}
	if current == nil {
		current = make(map[string]medicalReference)
		s.medicalReferences[category] = current
	}
	for id := range retired {
		delete(current, id)
	}
	current[parent] = ref
	for id, child := range children {
		current[id] = child
	}
	for id, document := range s.documentReferences {
		if retireDocument(document) {
			clear(document.payload)
			delete(s.documentReferences, id)
		}
	}
	if len(documents) > 0 && s.documentReferences == nil {
		s.documentReferences = make(map[string]documentReference)
	}
	for id, document := range documents {
		s.documentReferences[id] = document
	}
	return nil
}
