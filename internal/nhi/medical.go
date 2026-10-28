// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Medical record contracts are observed in the public SPA. Optional document
// retrieval uses its separate, explicit temporary-artifact lifecycle.
type medicalSpec struct {
	listPath, listRoot, searchPath, detailPath, detailRoot, title string
	sections                                                      []medicalSection
}
type medicalSection struct {
	key, kind, title string
	fields           []fieldSpec
}
type medicalReference struct{ category, rowID, sourceType, date, endDate, institution string }

type MedicineOrder struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Quantity string `json:"quantity"`
	Days     string `json:"days"`
}

var medicationFields = []fieldSpec{{"order_code", "藥品代碼", ""}, {"drug_name", "藥品名稱（原文）", ""}, {"act", "藥品藥理分類（原文）", ""}, {"order_qty", "藥品用量（原文，單位未確認）", ""}, {"order_drug_day", "給藥日數", "日"}}
var orderFields = []fieldSpec{{"order_code", "醫令代碼", ""}, {"cure_CNAME", "醫令名稱", ""}, {"cure_ENAME", "醫令英文名稱", ""}, {"order_qty", "醫囑總量（原文）", ""}}
var medicalSpecs = map[string]medicalSpec{
	"medicine": {"/api/ihke3000/ihke3306s01/page_load", "sp_IHKE3306S01_data", "/api/ihke3000/ihke3306s01/search", "/api/ihke3000/IHKE3306S02/page_load", "ihke3306S02_main_data", "用藥紀錄", []medicalSection{{"sp_IHKE3306S03_data_list", "medicine", "藥品醫囑", medicationFields}}},
	// The chronic list has no date-search endpoint in the observed official SPA.
	"chronic":   {"/api/ihke3000/ihke3307s01/page_load", "sp_IHKE3307S01_data", "", "/api/ihke3000/IHKE3306S02/page_load", "ihke3306S02_main_data", "慢連箋用藥資料", []medicalSection{{"sp_IHKE3306S03_data_list", "medicine", "慢連箋藥品醫囑", medicationFields}}},
	"procedure": {"/api/ihke3000/ihke3308s01/page_load", "sp_IHKE3308S01_data", "/api/ihke3000/IHKE3308s01/search", "/api/ihke3000/IHKE3308S02/page_load", "ihke3308S02_main_data", "手術紀錄", []medicalSection{{"sp_IHKE3308S04_data_list", "procedure", "手術明細", []fieldSpec{{"order_CODE", "醫令代碼", ""}, {"order_CODE_NAME", "手術名稱", ""}, {"exe_S_DATE", "手術日期（原文）", ""}, {"code_CNAME", "診療部位", ""}, {"order_QTY", "醫囑數量（原文）", ""}, {"assay_VALUE", "來源結果（原文）", ""}}}}},
	"hospital":  {"/api/ihke3000/ihke3309s01/page_load", "sp_IHKE3309S01_data", "/api/ihke3000/ihke3309s01/search", "/api/ihke3000/IHKE3309S02/page_load", "ihke3309S02_main_data", "住院紀錄", []medicalSection{{"sp_IHKE3302S11_data", "medicine", "住院藥品醫囑", []fieldSpec{{"order_code", "藥品代碼", ""}, {"drug_NAME", "藥品名稱（原文）", ""}, {"exe_S_DATE", "執行起日（原文）", ""}, {"exe_E_DATE", "執行迄日（原文）", ""}}}, {"sp_IHKE3302S10_data", "order", "住院檢驗檢查醫囑", []fieldSpec{{"order_code", "醫令代碼", ""}, {"order_code_name", "醫令名稱", ""}}}}},
	"western":   {"/api/ihke3000/ihke3303s01/page_load", "sp_IHKE3303S01_data", "/api/ihke3000/ihke3303s01/search", "/api/ihke3000/ihke3303s02/page_load", "ihke3303S02_main_data", "西醫就醫", []medicalSection{{"sp_IHKE3302S04_data", "medicine", "藥品醫囑", medicationFields}, {"sp_IHKE3302S05", "order", "非藥品醫囑", orderFields}, {"sp_IHKE3302S07_data", "order", "檢驗檢查醫囑", orderFields}}},
	"chinese":   {"/api/ihke3000/ihke3304s01/page_load", "sp_IHKE3304S01_data", "/api/ihke3000/ihke3304s01/search", "/api/ihke3000/ihke3303s02/page_load", "ihke3303S02_main_data", "中醫就醫", []medicalSection{{"sp_IHKE3302S04_data", "medicine", "中醫藥品醫囑", medicationFields}, {"sp_IHKE3302S05", "order", "非藥品醫囑", orderFields}}},
	"dental":    {"/api/ihke3000/ihke3305s01/page_load", "sp_IHKE3305S01_data", "/api/ihke3000/ihke3305s01/search", "/api/ihke3000/ihke3305s02/page_load", "ihke3305S02_main_data", "牙醫就醫", []medicalSection{{"sp_IHKE3305S06_data", "medicine", "牙醫藥品醫囑", medicationFields}, {"sp_IHKE3305S07", "order", "牙醫非藥品醫囑", append(append([]fieldSpec{}, orderFields...), fieldSpec{"teethname", "牙位", ""})}}},
}

func MedicalCategory(category string) bool { _, ok := medicalSpecs[category]; return ok }

var medicalHeaderFields = []fieldSpec{
	{"main_tit", "官方明細標題（原文）", ""},
	{"icd9cm_CODE_CNAME", "疾病分類（非診斷確認）", ""}, {"op_CODE_CNAME", "手術／處置名稱（原文）", ""},
	{"ori_type_cname", "資料來源類型（原文）", ""}, {"ori_TYPE_NAME", "就醫／調劑類別（原文）", ""},
	{"in_DATE", "入院日期（原文）", ""}, {"out_DATE", "出院日期（原文）", ""},
	{"func_DATE", "就醫／調劑日期（原文，依來源類別）", ""}, {"cure_E_DATE", "來源另列就醫日期（非給藥迄日）", ""},
	{"appl_S_DATE", "申報起日（原文，非入院日期）", ""}, {"appl_E_DATE", "申報迄日（原文，非出院日期）", ""},
}

// The key is only for linking rows already returned in this transient session.
// It is not authentication, a cross-device account key or persisted metadata.
func medicalKey(sourceType, rowID, section string, index int) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("medical-v1\x00%s\x00%s\x00%s\x00%d", sourceType, rowID, section, index)))
	return hex.EncodeToString(digest[:])
}

func (s *Session) FetchMedical(ctx context.Context, category string, query Query) (SupplementReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	report := SupplementReport{ContractRevision: ProviderContractRevision, Category: category, Entries: []SupplementEntry{}, Complete: false}
	spec, ok := medicalSpecs[category]
	if !ok {
		return report, failure("INVALID_INPUT", "不支援此醫療資料類別。")
	}
	var params url.Values
	if spec.searchPath != "" {
		resolved, _, err := resolveQuery(query, time.Now())
		if err != nil {
			return report, err
		}
		report.StartDate, report.EndDate = resolved.StartDate, resolved.EndDate
		params = url.Values{"s_date": {resolved.StartDate}, "e_date": {resolved.EndDate}, "s_sort": {"A1"}}
		if category == "medicine" {
			params.Set("s_type", "A")
		}
		report.Coverage = "已明確送出所選期間並讀取全部搜尋回應；無同期間獨立總數，不宣稱完整終身歷史。明細需另行取得。"
	} else {
		report.Coverage = "官方慢連箋清單未提供日期搜尋；僅呈現官方回傳範圍，不冒充全部歷史。明細需另行取得。"
	}
	if err := s.supplementReady(ctx); err != nil {
		return report, err
	}
	var sourceReport []*SupplementReport
	if spec.searchPath == "" {
		sourceReport = append(sourceReport, &report)
	}
	rows, err := s.supplementRequest(ctx, http.MethodGet, spec.listPath, params, nil, spec.listRoot, sourceReport...)
	if err != nil {
		return report, err
	}
	if spec.searchPath != "" {
		rows, err = s.supplementRequest(ctx, http.MethodGet, spec.searchPath, params, nil, "", &report)
		if err != nil {
			return report, err
		}
	}
	refs := make(map[string]medicalReference, len(rows))
	for index, row := range rows {
		rowID, e := rowText(row, "row_ID")
		if e != nil {
			return report, e
		}
		sourceType, e := rowText(row, "ori_TYPE")
		if e != nil {
			return report, e
		}
		dateField := "func_DATE"
		if category == "hospital" {
			dateField = "in_DATE"
		}
		date, e := rowText(row, dateField)
		if e != nil {
			return report, e
		}
		end, e := rowText(row, "out_DATE")
		if e != nil {
			return report, e
		}
		institution, e := rowText(row, "hosp_ABBR")
		if e != nil {
			return report, e
		}
		fields, e := appendKnownFields(row, medicalHeaderFields)
		if e != nil {
			return report, e
		}
		if date == "" && institution == "" && len(fields) == 0 {
			return report, changedError()
		}
		entry := SupplementEntry{Date: date, EndDate: end, Institution: institution, Title: spec.title, Source: "官方" + spec.title, Kind: "encounter", Fields: fields}
		medicalEntryMetadata(&entry, category, "list", false)
		if rowID != "" && sourceType != "" {
			entry.Key = medicalKey(sourceType, rowID, category, index)
			random := make([]byte, 24)
			if _, err := rand.Read(random); err != nil {
				return report, failure("INITIALIZATION_FAILED", "無法建立明細識別。")
			}
			entry.DetailID = hex.EncodeToString(random)
			refs[entry.DetailID] = medicalReference{category, rowID, sourceType, date, end, institution}
		}
		report.Entries = append(report.Entries, entry)
	}
	if err := validateSupplementReport(report); err != nil {
		return report, err
	}
	if err := s.reserveCategoryReferences("medical/"+category, referenceMapSize(refs, medicalReferenceSize), category); err != nil {
		return report, err
	}
	if s.medicalReferences == nil {
		s.medicalReferences = make(map[string]map[string]medicalReference)
	}
	s.medicalReferences[category] = refs
	report.FetchedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return report, nil
}

func (s *Session) FetchMedicalDetail(ctx context.Context, category, reference string) (SupplementReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	report := SupplementReport{ContractRevision: ProviderContractRevision, Category: category + "_detail", Entries: []SupplementEntry{}, Complete: false,
		Coverage: "官方醫囑原文，不等同實際服用。未提供成分、單位、頻次時不推算日劑量；疾病分類不等於確診。"}
	spec, ok := medicalSpecs[category]
	if !ok {
		return report, failure("INVALID_INPUT", "不支援此醫療資料類別。")
	}
	if err := s.supplementReady(ctx); err != nil {
		return report, err
	}
	ref, ok := s.medicalReferences[category][reference]
	if !ok || ref.category != category {
		return report, failure("INVALID_INPUT", "明細識別已失效或不屬於此類別，請重新取得清單。")
	}
	rows, err := s.supplementRequest(ctx, http.MethodGet, spec.detailPath, url.Values{"crid": {ref.rowID}, "ctype": {ref.sourceType}}, nil, spec.detailRoot)
	if err != nil {
		return report, err
	}
	if len(rows) == 0 {
		return report, failure("UPSTREAM_CHANGED", "官方未回傳此筆明細；未將空回應當成成功。")
	}
	documents := make(map[string]documentReference)
	for rowIndex, row := range rows {
		returned, e := rowText(row, "rowid")
		if e != nil || returned != ref.rowID {
			return report, changedError()
		}
		fields, e := appendKnownFields(row, medicalHeaderFields)
		if e != nil {
			return report, e
		}
		source := "官方" + spec.title
		// The public medicine/chronic detail view calls func_DATE the dispensing
		// date only when type_name_isr is true. A missing or unrecognized flag
		// does not establish dispensing, actual use, or the treatment start date.
		var dispensed bool
		if (category == "medicine" || category == "chronic") && json.Unmarshal(row["type_name_isr"], &dispensed) == nil && dispensed {
			source += "／調劑"
			for index := range fields {
				if fields[index].Code == "func_date" {
					fields[index].Name = "調劑日期（原文）"
				}
			}
		}
		if len(report.Entries) >= MaxReportRecords {
			return report, failure("RESPONSE_TOO_LARGE", "明細數量過多，未回傳不完整資料。")
		}
		header := SupplementEntry{Key: medicalKey(ref.sourceType, ref.rowID, "header", rowIndex), Date: ref.date, EndDate: ref.endDate, Institution: ref.institution, Title: spec.title + "明細", Source: source, Kind: "encounter", Fields: fields}
		medicalEntryMetadata(&header, category, "header", dispensed)
		if category == "hospital" {
			header.Attachments, e = hospitalDocuments(row, ref, reference, documents)
			if e != nil {
				return report, e
			}
		}
		report.Entries = append(report.Entries, header)
		for _, section := range spec.sections {
			raw, exists := row[strings.ToLower(section.key)]
			if !exists {
				if err := sourceSectionDiagnostic(&report, category, section.key, nil, false, false, 0); err != nil {
					return report, err
				}
				continue
			}
			items, e := supplementRows(raw, "")
			if e != nil {
				return report, e
			}
			if err := sourceSectionDiagnostic(&report, category, section.key, raw, true, false, len(items)); err != nil {
				return report, err
			}
			for index, item := range items {
				fields, e := appendKnownFields(item, section.fields)
				if e != nil {
					return report, e
				}
				if len(fields) == 0 {
					return report, changedError()
				}
				entry := SupplementEntry{Key: medicalKey(ref.sourceType, ref.rowID, section.key, index), Date: ref.date, Institution: ref.institution, Title: section.title, Source: source, Kind: section.kind, Fields: fields}
				medicalEntryMetadata(&entry, category, section.key, dispensed)
				if category == "procedure" || category == "hospital" {
					entry.Date, e = rowText(item, "exe_S_DATE")
					if e != nil {
						return report, e
					}
					entry.EndDate, e = rowText(item, "exe_E_DATE")
					if e != nil {
						return report, e
					}
					entry.DateRole, entry.EndDateRole = "", ""
					if entry.Date != "" {
						entry.DateRole = "unknown" // execution is not admission, prescription or dispensing
					}
					if entry.EndDate != "" {
						entry.EndDateRole = "period_end"
						if entry.Date != "" {
							entry.DateRole = "period_start"
						}
					}
				}
				if section.kind == "medicine" {
					medicationFieldRoles(entry.Fields)
					code, e := rowText(item, "order_code")
					if e != nil {
						return report, e
					}
					name, e := rowText(item, "drug_name")
					if e != nil {
						return report, e
					}
					quantity, days := "", ""
					if category != "hospital" {
						quantity, e = rowText(item, "order_qty")
						if e != nil {
							return report, e
						}
						days, e = rowText(item, "order_drug_day")
						if e != nil {
							return report, e
						}
					}
					entry.Medicine = &MedicineOrder{Code: code, Name: name, Quantity: quantity, Days: days}
				}
				report.Entries = append(report.Entries, entry)
				if len(report.Entries) > MaxReportRecords {
					return report, failure("RESPONSE_TOO_LARGE", "明細數量過多，未回傳不完整資料。")
				}
			}
		}
	}
	if err := validateSupplementReport(report); err != nil {
		return report, err
	}
	if err := s.replaceDocumentReferences(category, reference, documents); err != nil {
		return report, err
	}
	report.FetchedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return report, nil
}
