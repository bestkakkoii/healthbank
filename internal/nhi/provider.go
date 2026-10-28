// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// ProviderCategoryInfo describes acquisition, not a promise of upstream retention.
// These routes are fixed contracts observed in public official SPA bundles.
type ProviderCategoryInfo struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	Group      string `json:"group"`
	RangeMode  string `json:"range_mode"`
	HasDetails bool   `json:"has_details"`
}

type providerReference struct {
	rowID, sourceType, date, institution, title, unit, kind, originKey string
	dateRole                                                           string
	parentReference                                                    string
	query                                                              Query
	refillTimes                                                        []string
	children                                                           []string
}

type providerSection struct {
	root, title, source, date, end, institution, titleField string
	fields                                                  []fieldSpec
	object                                                  bool
	keyed                                                   bool
	detail                                                  bool
	optional                                                bool
	metadata                                                bool
}

type providerRequest struct {
	path     string
	method   string
	params   url.Values
	payload  any
	sections []providerSection
	// Custom readers explicitly pin a top-level array without weakening the
	// ordinary providerRead object contract.
	responseArray bool
}

type providerSpec struct {
	info     ProviderCategoryInfo
	coverage string
	requests []providerRequest
}

func ProviderCategory(category string) bool { _, ok := providerSpecs[category]; return ok }

func ProviderCategories() []ProviderCategoryInfo {
	items := make([]ProviderCategoryInfo, 0, len(providerSpecs))
	for _, spec := range providerSpecs {
		items = append(items, spec.info)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items
}

func providerObject(body []byte) (wireRow, error) {
	if len(bytes.TrimSpace(body)) == 0 || bytes.Equal(bytes.TrimSpace(body), []byte("null")) {
		return nil, changedError()
	}
	var object wireRow
	if decodeJSON(body, &object) != nil || object == nil {
		return nil, changedError()
	}
	if err := detectAPIError(object); err != nil {
		return nil, err
	}
	return canonicalRow(object)
}

func providerSectionRows(object wireRow, section providerSection) ([]wireRow, bool, error) {
	if section.root == "" {
		return []wireRow{object}, true, nil
	}
	raw, ok := object[strings.ToLower(section.root)]
	if !ok {
		return nil, false, nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return []wireRow{}, true, nil
	}
	if section.keyed {
		rows, err := providerDietRows(raw)
		return rows, true, err
	}
	if section.object {
		row, err := providerObject(raw)
		if err != nil {
			return nil, true, err
		}
		return []wireRow{row}, true, nil
	}
	rows, err := supplementRows(raw, "")
	return rows, true, err
}

func (s *Session) providerRead(ctx context.Context, request providerRequest) (wireRow, error) {
	var payload []byte
	if request.payload != nil {
		var err error
		payload, err = json.Marshal(request.payload)
		if err != nil {
			return nil, failure("REQUEST_FAILED", "無法建立資料請求。")
		}
		defer clear(payload)
	}
	method := request.method
	if method == "" {
		method = http.MethodGet
	}
	body, _, err := s.request(ctx, method, request.path, request.params, payload, maxJSONBytes, true)
	if err != nil {
		return nil, err
	}
	defer clear(body)
	object, err := providerObject(body)
	if err != nil {
		s.invalidateOnAuthError(err)
		return nil, s.annotateResponseError(err)
	}
	return object, nil
}

func providerEntry(row wireRow, section providerSection, inherited providerReference) (SupplementEntry, error) {
	entry := SupplementEntry{Title: section.title, Source: section.source, Fields: []SupplementField{}}
	var err error
	if section.date != "" {
		entry.Date, err = rowText(row, section.date)
		if err != nil {
			return entry, err
		}
	}
	if section.end != "" {
		entry.EndDate, err = rowText(row, section.end)
		if err != nil {
			return entry, err
		}
	}
	if section.institution != "" {
		entry.Institution, err = rowText(row, section.institution)
	} else {
		entry.Institution, err = rowText(row, "hosp_abbr", "hosp_name")
	}
	if err != nil {
		return entry, err
	}
	if section.titleField != "" {
		title, e := rowText(row, section.titleField)
		if e != nil {
			return entry, e
		}
		if title != "" {
			entry.Title = title
		}
	}
	entry.Fields, err = appendKnownFields(row, section.fields)
	if err != nil {
		return entry, err
	}
	// Parent context is not evidence that this detail row contains source data.
	// Check before inheritance so an empty or changed object cannot become a
	// successful-looking observation merely by copying its parent's institution.
	if entry.Date == "" && entry.Institution == "" && len(entry.Fields) == 0 {
		return entry, changedError()
	}
	// Detail rows inherit their verified parent encounter context only when the
	// source does not supply a separate date/institution. No measurement date is
	// inferred: date-less numerical records keep their date empty.
	if entry.Institution == "" {
		entry.Institution = inherited.institution
	}
	if section.date == "@parent" {
		entry.Date = inherited.date
	}
	return entry, nil
}

func providerNoData(category string, object wireRow) (bool, error) {
	if category == "organ_wishes" {
		status, err := rowText(object, "status")
		if err != nil || status == "" {
			return false, changedError()
		}
		if status == "N" {
			return true, nil
		}
		if status != "Y" {
			return false, changedError()
		}
	}
	if category == "home_care" || category == "source_diabetes_assessment" {
		raw, ok := object["hasdata"]
		if !ok {
			return false, changedError()
		}
		switch string(bytes.TrimSpace(raw)) {
		case "false":
			return true, nil
		case "true":
			return false, nil
		default:
			return false, changedError()
		}
	}
	if category == "family_followup" || category == "family_health_followup" {
		if raw, ok := object["hasillcategory"]; ok {
			var present bool
			if json.Unmarshal(raw, &present) != nil {
				return false, changedError()
			}
			return !present, nil
		}
	}
	return false, nil
}

func (s *Session) providerAppend(report *SupplementReport, object wireRow, request providerRequest, category string, refs map[string]providerReference, inherited providerReference) error {
	empty, err := providerNoData(category, object)
	if err != nil {
		return err
	}
	if empty {
		return nil
	}
	seen := false
	for _, section := range request.sections {
		if section.metadata {
			before := len(report.Entries)
			if err := appendProviderMetadata(report, object, request.path, category, section, inherited); err != nil {
				return err
			}
			_, present := object[strings.ToLower(section.root)]
			seen = seen || len(report.Entries) > before || (section.root != "" && present)
			continue
		}
		rows, present, err := providerSectionRows(object, section)
		if err != nil {
			return err
		}
		seen = seen || present
		if section.root != "" {
			if err := sourceSectionDiagnostic(report, category, section.root, object[strings.ToLower(section.root)], present, section.optional, len(rows)); err != nil {
				return err
			}
		}
		missingResults := 0
		for index, row := range rows {
			if category == "device_activity" && refs != nil {
				code, err := providerDeviceSelector(row)
				if err != nil {
					return err
				}
				if code == "" {
					continue
				}
			}
			if category == "family_doctor" && section.root == "sP_IHKE3720S02" && len(row) == 0 {
				continue // observed optional provider object for a nonmember
			}
			entry, err := providerEntry(row, section, inherited)
			if err != nil {
				return err
			}
			entry.Key = medicalKey(category, request.path+"\x00"+inherited.rowID+"\x00"+inherited.sourceType+"\x00"+inherited.originKey, section.root, index)
			if refs != nil && (category == "family_followup" || category == "family_health_followup" || category == "body_measurements" || category == "device_activity" || category == "physiology" || category == "lipids") {
				entry.Kind = "source_index"
			}
			providerEntryMetadata(&entry, category, section, inherited)
			if category == "recent_visits" {
				entry.Kind = "encounter"
			}
			if err := appendProviderCoverageFields(report, &entry, category, row); err != nil {
				return err
			}
			if entry.Kind != "source_index" && strings.ToLower(section.date) != "inoculate_d" && sourceResultMissing(category, section.fields, entry.Fields) {
				missingResults++
			}
			if section.detail && refs != nil {
				ref, err := providerMakeReference(category, row, entry, inherited.query)
				if err != nil {
					return err
				}
				if ref.rowID != "" {
					key := make([]byte, 24)
					if _, err := rand.Read(key); err != nil {
						return failure("INITIALIZATION_FAILED", "無法建立明細識別。")
					}
					entry.DetailID = hex.EncodeToString(key)
					refs[entry.DetailID] = ref
				}
			}
			if category == "e_prescriptions" {
				if err := providerRefills(&entry, row); err != nil {
					return err
				}
			}
			report.Entries = append(report.Entries, entry)
			if category == "e_prescriptions" {
				events, err := providerDispensingEvents(row, entry)
				if err != nil {
					return err
				}
				report.Entries = append(report.Entries, events...)
			}
			if len(report.Entries) > MaxReportRecords {
				return failure("RESPONSE_TOO_LARGE", "此類別資料過多，未載入不完整清單。")
			}
		}
		if missingResults > 0 {
			if err := addSourceDiagnostic(report, "known_result_missing", sourceContractID(category, section.root), &missingResults); err != nil {
				return err
			}
		}
	}
	if !seen {
		return changedError()
	}
	return nil
}

func providerDispensingEvents(row wireRow, parent SupplementEntry) ([]SupplementEntry, error) {
	events := []SupplementEntry{}
	raw, ok := row["refilllist"]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return events, nil
	}
	rows, err := supplementRows(raw, "")
	if err != nil || len(rows) > 1000 {
		return nil, changedError()
	}
	for i, item := range rows {
		date, err := rowText(item, "rel_DATE_STR")
		if err != nil {
			return nil, err
		}
		if date == "" {
			continue
		}
		institution, err := rowText(item, "hosp_NAME")
		if err != nil {
			return nil, err
		}
		number, err := rowText(item, "recipe_TIMES")
		if err != nil {
			return nil, err
		}
		events = append(events, SupplementEntry{Key: medicalKey("e_prescriptions", parent.Key, "dispensing", i), Kind: "dispensing", Date: date, DateRole: "dispensing", Origin: "institutional", ContractID: sourceContractID("e_prescriptions", "refilllist"), Institution: institution, Title: "第" + number + "聯來源已領紀錄", Source: parent.Source + "／來源已領（非實際服用證明）", Fields: []SupplementField{{Name: "處方就醫日期（原文）", Value: parent.Date, Code: "func_date", Role: "date"}, {Name: "來源已領藥日期（原文）", Value: date, Code: "rel_date_str", Role: "date"}, {Name: "資料關係", Value: "對應此電子處方箋；不以建議領藥區間推定已領。"}}})
	}
	return events, nil
}

// FetchProvider reads every record in the observed upstream arrays. No hidden
// frontend year, source selector, or 50-row display limit narrows these requests.
func (s *Session) FetchProvider(ctx context.Context, category string, query Query) (SupplementReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	report := SupplementReport{ContractRevision: ProviderContractRevision, Category: category, Entries: []SupplementEntry{}, Complete: false}
	spec, ok := providerSpecs[category]
	if !ok {
		return report, failure("INVALID_INPUT", "不支援此資料類別。")
	}
	report.Coverage = spec.coverage
	resolved := query
	if spec.info.RangeMode == "date_search" {
		// A calendar can contain future appointments. An omitted range uses
		// the application's explicit supported bounds, not today's date or
		// the source UI's currently displayed month. Custom bounds stay intact.
		if category == "calendar_events" && query.StartDate == "" && query.EndDate == "" {
			resolved = Query{StartDate: AllHistoryStartDate, EndDate: "2200-12-31"}
		}
		var err error
		resolved, _, err = resolveQuery(resolved, time.Now())
		if err != nil {
			return report, err
		}
		report.StartDate, report.EndDate = resolved.StartDate, resolved.EndDate
	}
	if err := s.supplementReady(ctx); err != nil {
		return report, err
	}
	refs := map[string]providerReference{}
	documents := map[string]documentReference{}
	handled, err := s.providerPersonalFetch(ctx, &report, category, refs)
	if err != nil {
		return report, err
	}
	if !handled {
		handled, err = s.providerCommunicationsFetch(ctx, &report, category, resolved, refs, documents)
		if err != nil {
			return report, err
		}
	}
	if !handled {
		handled, err = s.providerFamilyFollowupCoverageFetch(ctx, &report, category, resolved, refs)
		if err != nil {
			return report, err
		}
	}
	if !handled {
		handled, err = s.providerCoverageFetch(ctx, &report, category, resolved, refs)
		if err != nil {
			return report, err
		}
	}
	requests := spec.requests
	if handled {
		requests = nil
	}
	for _, request := range requests {
		object, err := s.providerRead(ctx, request)
		if err != nil {
			return report, err
		}
		if category == "glucose" {
			// The homepage table and date-search chart are independent source
			// responses. Preserve both, even when either returns no rows.
			if _, present := object["sp_ihke3406s01_data"]; !present {
				return report, changedError()
			}
			if err := s.providerAppend(&report, object, request, category, nil, providerReference{}); err != nil {
				return report, s.annotateResponseError(err)
			}
			request, err = providerDetailRequest(category, providerReference{rowID: "09006C", title: "糖化血色素（HbA1c）", query: resolved})
			if err != nil {
				return report, err
			}
			object, err = s.providerRead(ctx, request)
			if err != nil {
				return report, err
			}
		}
		if err := s.providerAppend(&report, object, request, category, refs, providerReference{query: resolved}); err != nil {
			return report, s.annotateResponseError(err)
		}
	}
	if category == "physiology" {
		// The official home always exposes this combined source selector, even
		// when its latest-value summary is empty. This is an index, not a result.
		key := make([]byte, 24)
		if _, err := rand.Read(key); err != nil {
			return report, failure("INITIALIZATION_FAILED", "無法建立明細識別。")
		}
		reference := hex.EncodeToString(key)
		refs[reference] = providerReference{rowID: "10568", title: "血壓與心率", query: resolved}
		report.Entries = append(report.Entries, SupplementEntry{Key: medicalKey(category, "10568", "source_index", 0), Kind: "source_index", ContractID: sourceContractID(category, "source_index"), Origin: "mixed", Title: "血壓與心率資料索引", Source: "健康存摺／生理量測資料入口", Fields: []SupplementField{{Name: "資料索引", Value: "取得明細後才包含實際量測紀錄。"}}, DetailID: reference})
	}
	if category == "menstrual" {
		if err := appendProviderMenstrualMonths(&report, resolved, refs); err != nil {
			return report, err
		}
	}
	if err := validateSupplementReport(report); err != nil {
		return report, err
	}
	if err := s.commitProviderListReferences(category, refs, documents); err != nil {
		return report, err
	}
	report.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	return report, nil
}

// A list may expose both detail and download handles. Validate and admit the
// complete replacement before retiring either previous capability group.
func (s *Session) commitProviderListReferences(category string, refs map[string]providerReference, documents map[string]documentReference) error {
	count, size := s.remainingDocumentReferences(category, "", true, documents)
	if count > MaxReportRecords {
		return failure("RESPONSE_TOO_LARGE", "本次文件識別數量已達上限，請重新取得所需類別。")
	}
	changes := map[string]int64{"provider/" + category: referenceMapSize(refs, providerReferenceSize), "documents": size}
	if category == "annual_overview" || category == "recent_visits" {
		changes["medical/"+category] = 0
	}
	if err := s.reserveReferenceGroups(changes); err != nil {
		return err
	}
	s.clearDocumentCategory(category)
	if category == "annual_overview" || category == "recent_visits" {
		delete(s.medicalReferences, category)
	}
	if len(documents) > 0 && s.documentReferences == nil {
		s.documentReferences = make(map[string]documentReference)
	}
	for id, ref := range documents {
		s.documentReferences[id] = ref
	}
	if s.providerReferences == nil {
		s.providerReferences = map[string]map[string]providerReference{}
	}
	s.providerReferences[category] = refs
	return nil
}

func providerMakeReference(category string, row wireRow, entry SupplementEntry, query Query) (providerReference, error) {
	ref := providerReference{date: entry.Date, dateRole: entry.DateRole, institution: entry.Institution, title: entry.Title, originKey: entry.Key, query: query}
	var err error
	switch category {
	case "menstrual":
		ref.rowID, err = providerMenstrualDate(entry.Date)
	case "recent_visits":
		ref.rowID, err = rowText(row, "row_id")
		if err == nil {
			ref.sourceType, err = rowText(row, "ori_type")
		}
		var sourceKind string
		if err == nil {
			sourceKind, err = rowText(row, "htype")
		}
		switch sourceKind {
		case "西醫":
			ref.kind = "western"
		case "中醫":
			ref.kind = "chinese"
		case "牙醫":
			ref.kind = "dental"
		default:
			ref.rowID = "" // the source supplies no detail route for this label
		}
		if ref.sourceType == "" {
			ref.rowID = ""
		}
	case "annual_overview":
		ref.rowID, err = rowText(row, "year")
		ref.kind = "annual_year"
	case "e_prescriptions":
		ref.rowID, err = rowText(row, "treat_NUM")
		if err == nil {
			ref.sourceType, err = rowText(row, "recipe_TYPE")
		}
		if ref.rowID != "" && ref.sourceType == "" {
			return ref, changedError()
		}
		if err == nil && ref.sourceType == "C" {
			ref.refillTimes, err = providerPrescriptionCalendarTimes(row)
		}
	case "physiology":
		ref.rowID, err = rowText(row, "code")
		if err == nil {
			ref.unit, err = rowText(row, "uniT_DATA")
		}
	case "lipids":
		ref.rowID, err = rowText(row, "ORDER_CODE")
		if err == nil {
			ref.unit, err = rowText(row, "UNIT_DATA")
		}
	case "family_followup", "family_health_followup":
		ref.rowID, err = rowText(row, "assay_NAME")
	case "body_measurements":
		ref.rowID, err = rowText(row, "code")
	case "device_activity":
		ref.rowID, err = providerDeviceSelector(row)
	case "diet":
		ref.rowID, err = rowText(row, providerDietKey)
		if raw := bytes.TrimSpace(row[providerDietKey]); len(raw) > 0 && raw[0] != '"' {
			ref.kind = "array_index"
		}
	case "glucose":
		ref.rowID = "09006C" // fixed HbA1c source selector in IHKE3406S01
	case "osteoporosis":
		ref.rowID, err = rowText(row, "row_ID")
		if err == nil {
			ref.sourceType, err = rowText(row, "ori_TYPE")
		}
	default:
		ref.rowID, err = rowText(row, "row_ID")
	}
	if err != nil || len(ref.rowID) > 4096 || len(ref.sourceType) > 128 {
		return ref, changedError()
	}
	return ref, nil
}

// Source-confirmed dispensing and suggested windows retain different labels.
// Their strings are evidence, never an assertion of actual medication use.
func providerRefills(entry *SupplementEntry, row wireRow) error {
	raw, ok := row["refilllist"]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	rows, err := supplementRows(raw, "")
	if err != nil || len(rows) > 1000 {
		return changedError()
	}
	for _, item := range rows {
		number, err := rowText(item, "recipe_TIMES")
		if err != nil {
			return err
		}
		date, err := rowText(item, "rel_DATE_STR")
		if err != nil {
			return err
		}
		label := "第" + number + "聯"
		if number != "" {
			entry.Fields = append(entry.Fields, SupplementField{Name: label + "來源聯次（原文）", Value: number, Code: "recipe_times"})
		}
		window, err := rowText(item, "recipe_MEMO")
		if err != nil {
			return err
		}
		institution, err := rowText(item, "hosp_NAME")
		if err != nil {
			return err
		}
		if number == "" && date == "" && window == "" && institution == "" {
			return changedError()
		}
		if date != "" {
			entry.Fields = append(entry.Fields, SupplementField{Name: label + "來源已領藥日期", Value: date, Code: "rel_date_str", Role: "date"})
		}
		if institution != "" {
			entry.Fields = append(entry.Fields, SupplementField{Name: label + "來源領藥院所（未推定完成）", Value: institution, Code: "hosp_name"})
		}
		if window != "" {
			entry.Fields = append(entry.Fields, SupplementField{Name: label + "來源建議領藥期間（非已領）", Value: window, Code: "recipe_memo"})
		}
		if date == "" && window == "" {
			entry.Fields = append(entry.Fields, SupplementField{Name: label + "來源欄位狀態", Value: "來源未提供領藥日期或建議區間；未推定已領藥或實際服用。"})
		}
	}
	return nil
}

func (s *Session) FetchProviderDetail(ctx context.Context, category, reference string) (SupplementReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	report := SupplementReport{ContractRevision: ProviderContractRevision, Category: category + "_detail", Entries: []SupplementEntry{}, Complete: false}
	if err := s.supplementReady(ctx); err != nil {
		return report, err
	}
	ref, ok := s.providerReferences[category][reference]
	if !ok {
		if category == "annual_overview" || category == "recent_visits" {
			if child, exists := s.medicalReferences[category][reference]; exists && child.kind != "annual_year" {
				return s.fetchMedicalDetail(ctx, category, reference, child)
			}
		}
		return report, failure("INVALID_INPUT", "明細識別已失效，請重新取得目前類別清單。")
	}
	if category == "annual_overview" {
		return s.fetchAnnualProviderDetail(ctx, reference, ref, report)
	}
	if category == "recent_visits" {
		return s.fetchMedicalDetail(ctx, category, reference, medicalReference{category: ref.kind, rowID: ref.rowID, sourceType: ref.sourceType, date: ref.date, institution: ref.institution})
	}
	if category == "menstrual" && ref.kind == "menstrual_month" {
		return s.fetchProviderMenstrualMonth(ctx, reference, ref, report)
	}
	if handled, err := s.providerPersonalDetail(ctx, &report, category, reference, ref); handled {
		if err != nil {
			return report, err
		}
		if err := validateSupplementReport(report); err != nil {
			return report, err
		}
		report.FetchedAt = time.Now().UTC().Format(time.RFC3339Nano)
		return report, nil
	}
	if handled, err := s.providerCommunicationsDetail(ctx, &report, category, reference, ref); handled {
		if err != nil {
			return report, err
		}
		if err := validateSupplementReport(report); err != nil {
			return report, err
		}
		report.FetchedAt = time.Now().UTC().Format(time.RFC3339Nano)
		return report, nil
	}
	request, err := providerDetailRequest(category, ref)
	if err != nil {
		return report, err
	}
	// Only source indices can produce child references. Ordinary clinical
	// details do not copy the full category map for every historical row.
	var refs map[string]providerReference
	refreshChildren := (category == "physiology" || category == "device_activity") && ref.kind != "device_day"
	if refreshChildren {
		refs = providerStageChildRefresh(s.providerReferences[category], reference)
		if err := s.providerPhysiologyRaw(ctx, &report, category, reference, ref, refs); err != nil {
			return report, err
		}
	}
	object, err := s.providerRead(ctx, request)
	if err != nil {
		return report, err
	}
	if err := providerVerifyDetailIdentity(category, object, ref); err != nil {
		return report, err
	}
	report.Coverage = "此清單項目的官方明細；保留來源值、日期與類型，不推定實際服用、診斷或終身資料完整。"
	if ((category == "physiology" || category == "device_activity") && ref.kind != "device_day") || category == "glucose" || category == "lipids" || category == "family_followup" || category == "family_health_followup" || category == "body_measurements" {
		report.StartDate, report.EndDate = ref.query.StartDate, ref.query.EndDate
		report.Coverage = "已送出所選完整日期區間並保留來源搜尋陣列；無獨立總數，不保證官方保有全部歷史。不同來源／方法未強制併入檢驗序列。"
	}
	if err := s.providerAppend(&report, object, request, category, nil, ref); err != nil {
		return report, s.annotateResponseError(err)
	}
	if ref.kind == "care_chart" {
		report.Coverage = "來源照護圖表的全部回傳資料列，保留每筆原始日期與數值；不假定與明細表相同，不轉成浮點數或推定缺少單位。來源未提供日期搜尋或獨立總數。"
		for index := range report.Entries {
			report.Entries[index].Kind, report.Entries[index].Origin = "source_summary", "official_summary"
		}
	}
	careChildren, err := appendProviderCareChartIndices(&report, category, reference, ref)
	if err != nil {
		return report, err
	}
	if category == "e_prescriptions" || category == "self_paid_medicine" {
		for i := range report.Entries {
			entry := &report.Entries[i]
			if entry.Kind == "medicine" || entry.Kind == "dispensing" {
				continue
			}
			if request.sections[0].root == "treatData" && i == 0 {
				continue
			}
			// Source dosage/frequency are kept in fields; only explicitly named
			// duration/quantity fields feed the existing prescribed-days graph.
			order := MedicineOrder{}
			medicationFieldRoles(entry.Fields)
			for _, field := range entry.Fields {
				switch field.Role {
				case "medication_code":
					order.Code = field.Value
				case "medication_name":
					order.Name = field.Value
				case "prescribed_days":
					order.Days = field.Value
				case "quantity":
					order.Quantity = field.Value
				}
			}
			if order.Code != "" || order.Name != "" {
				entry.Kind, entry.Medicine = "medicine", &order
			}
		}
	}
	if err := validateSupplementReport(report); err != nil {
		return report, err
	}
	if err := s.attachProviderDocuments(category, reference, ref, object, &report); err != nil {
		return report, err
	}
	if err := s.commitProviderCareChartReferences(category, reference, ref, careChildren); err != nil {
		return report, err
	}
	if refreshChildren {
		if err := s.reserveReferenceGroups(map[string]int64{"provider/" + category: referenceMapSize(refs, providerReferenceSize)}); err != nil {
			return report, err
		}
		s.providerReferences[category] = refs
	}
	report.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	return report, nil
}

// Stage the replacement without touching the successful live reference map.
// Descendants are indexed once so a large source history remains linear in the
// number of retained references rather than walking every ancestry repeatedly.
func providerStageChildRefresh(current map[string]providerReference, parent string) map[string]providerReference {
	children := make(map[string][]string)
	for id, ref := range current {
		if ref.parentReference != "" {
			children[ref.parentReference] = append(children[ref.parentReference], id)
		}
	}
	retired := map[string]bool{parent: true}
	queue := []string{parent}
	for i := 0; i < len(queue); i++ {
		for _, id := range children[queue[i]] {
			if !retired[id] {
				retired[id] = true
				queue = append(queue, id)
			}
		}
	}
	staged := make(map[string]providerReference, len(current))
	for id, ref := range current {
		if id == parent || !retired[id] {
			staged[id] = ref
		}
	}
	return staged
}

func providerPhysiologyRawRequest(category string, ref providerReference) providerRequest {
	fields := []fieldSpec{{"code", "來源量測項目代碼", ""}, {"create_DATE", "量測日期（原文）", ""}, {"time", "時段（原文）", ""}, {"value", "來源量測值（原文）", ""}, {"unit_DATA", "單位（原文）", ""}, {"txt_DATE", "來源編輯時間（原文，非量測日期）", ""}, {"abnormal_Status", "來源旗標（原文）", ""}}
	payload := map[string]string{"cCode": ref.rowID}
	request := providerRequest{path: "/api/ihke3000/ihke3205s02/page_load", method: http.MethodPost, payload: payload, sections: []providerSection{providerSectionSpec("sP_IHKE3205S04", ref.title+"原始紀錄", "健康存摺／生理量測原始紀錄（可含自行記錄）", "create_DATE", fields)}}
	if category == "device_activity" {
		request.path = "/api/ihke3000/ihke3207s05/page_load"
		payload["cSDate"] = strings.ReplaceAll(ref.query.StartDate, "-", "")
		payload["cEDate"] = strings.ReplaceAll(ref.query.EndDate, "-", "")
		request.sections[0].source = "健康存摺／行動裝置運動原始紀錄"
	}
	device := providerSectionSpec("devicelist", ref.title+"行動裝置日期索引", "健康存摺／行動裝置資料來源", "cdate", []fieldSpec{{"cdate", "來源日期（原文）", ""}, {"value", "來源摘要（原文）", ""}, {"act_os", "來源裝置平台代碼（未推定平台）", ""}})
	metadata := providerSectionSpec("sP_IHKE3205S03", ref.title+"來源項目與單位說明", "健康存摺／量測項目來源定義（非量測結果）", "", []fieldSpec{{"codE_NAME", "來源量測項目名稱", ""}, {"uniT_DATA", "來源項目單位（未自動套用其他紀錄）", ""}})
	request.sections[0].optional, device.optional = true, true
	metadata.optional = true
	platformFields := []fieldSpec{{"act_os", "來源裝置平台代碼（未推定平台）", ""}}
	if category == "physiology" {
		platformFields = append(platformFields, fieldSpec{"act_os1", "來源第一裝置可用性旗標", ""}, fieldSpec{"act_os2", "來源第二裝置可用性旗標", ""}, fieldSpec{"act_os3", "來源第三裝置可用性旗標", ""})
	}
	platform := providerMetadataSection("", ref.title+"來源裝置平台資訊", "健康存摺／裝置來源原始代碼與可用性旗標", platformFields)
	request.sections = append(request.sections, device, metadata, platform)
	return request
}

func (s *Session) providerPhysiologyRaw(ctx context.Context, report *SupplementReport, category, parent string, ref providerReference, refs map[string]providerReference) error {
	request := providerPhysiologyRawRequest(category, ref)
	object, err := s.providerRead(ctx, request)
	if err != nil {
		return err
	}
	if _, present := object["sp_ihke3205s04"]; present {
		rawRequest := request
		rawRequest.sections = request.sections[:1]
		if err := s.providerAppend(report, object, rawRequest, category, nil, ref); err != nil {
			return err
		}
	} else {
		if err := sourceSectionDiagnostic(report, category, "sP_IHKE3205S04", nil, false, true, 0); err != nil {
			return err
		}
		_, metadata, err := providerSectionRows(object, providerSection{root: "sP_IHKE3205S03"})
		if err != nil {
			return err
		}
		if _, devices := object["devicelist"]; !metadata && !devices {
			return changedError()
		}
	}
	device := request.sections[1]
	rows, present, err := providerSectionRows(object, device)
	if err != nil {
		return err
	}
	if err := sourceSectionDiagnostic(report, category, device.root, object["devicelist"], present, true, len(rows)); err != nil {
		return err
	}
	for index, row := range rows {
		entry, err := providerEntry(row, device, ref)
		if err != nil {
			return err
		}
		entry.Kind = "source_index"
		providerEntryMetadata(&entry, category, device, ref)
		entry.Key = medicalKey(category, ref.rowID+"\x00"+ref.originKey, "device_day", index)
		day, err := rowText(row, "edate")
		if err != nil || len(day) > 128 {
			return changedError()
		}
		if day != "" {
			if len(refs) >= MaxReportRecords {
				return failure("RESPONSE_TOO_LARGE", "行動量測明細索引過多，未回傳不完整明細。")
			}
			key := make([]byte, 24)
			if _, err := rand.Read(key); err != nil {
				return failure("INITIALIZATION_FAILED", "無法建立明細識別。")
			}
			entry.DetailID = hex.EncodeToString(key)
			child := ref
			child.kind, child.sourceType, child.originKey = "device_day", day, entry.Key
			child.parentReference = parent
			refs[entry.DetailID] = child
		}
		report.Entries = append(report.Entries, entry)
		if len(report.Entries) > MaxReportRecords {
			return failure("RESPONSE_TOO_LARGE", "量測資料過多，未回傳不完整明細。")
		}
	}
	metadata := request
	metadata.sections = request.sections[2:3]
	if _, present := object["sp_ihke3205s03"]; present {
		start := len(report.Entries)
		if err := s.providerAppend(report, object, metadata, category, nil, ref); err != nil {
			return err
		}
		for index := start; index < len(report.Entries); index++ {
			report.Entries[index].Kind = "source_metadata"
		}
	} else if err := sourceSectionDiagnostic(report, category, "sP_IHKE3205S03", nil, false, true, 0); err != nil {
		return err
	}
	return appendProviderMetadata(report, object, request.path, category, request.sections[3], ref)
}

func providerVerifyDetailIdentity(category string, object wireRow, ref providerReference) error {
	var rows []wireRow
	switch category {
	case "diet", "exercise":
		var success bool
		if raw, present := object["resultstatus"]; !present || json.Unmarshal(raw, &success) != nil {
			return changedError()
		}
		if !success {
			return failure("REQUEST_FAILED", "官方未提供此筆自行記錄明細，請重新取得來源清單後再試。")
		}
		if category == "diet" {
			if _, present := object["isselectfoodlist"]; !present {
				return changedError()
			}
			return nil
		}
		rows, present, err := providerSectionRows(object, providerSection{root: "oneRecordList"})
		if err != nil || !present || len(rows) == 0 {
			return changedError()
		}
		for _, row := range rows {
			id, err := rowText(row, "ROW_ID")
			if err != nil || id != "" && id != ref.rowID {
				return changedError()
			}
		}
		return nil
	case "e_prescriptions":
		raw, ok := object["treatdata"]
		if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return changedError()
		}
		row, err := providerObject(raw)
		if err != nil {
			return err
		}
		rows = []wireRow{row}
	case "self_paid_medicine", "self_paid_material":
		section := providerSpecs[category].requests[0].sections[0]
		var present bool
		var err error
		rows, present, err = providerSectionRows(object, section)
		if err != nil || !present {
			return changedError()
		}
	default:
		return nil
	}
	for _, row := range rows {
		field := "row_ID"
		if category == "e_prescriptions" {
			field = "treat_NUM"
		}
		id, err := rowText(row, field)
		if err != nil || id != "" && id != ref.rowID {
			return changedError()
		}
		if category == "e_prescriptions" {
			kind, err := rowText(row, "recipe_TYPE")
			if err != nil || kind != "" && kind != ref.sourceType {
				return changedError()
			}
		}
	}
	return nil
}
