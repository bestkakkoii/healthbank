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
	parentReference                                                    string
	query                                                              Query
}

type providerSection struct {
	root, title, source, date, end, institution, titleField string
	fields                                                  []fieldSpec
	object                                                  bool
	keyed                                                   bool
	detail                                                  bool
}

type providerRequest struct {
	path     string
	method   string
	params   url.Values
	payload  any
	sections []providerSection
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
	// Detail rows inherit their verified parent encounter context only when the
	// source does not supply a separate date/institution. No measurement date is
	// inferred: date-less numerical records keep their date empty.
	if entry.Institution == "" {
		entry.Institution = inherited.institution
	}
	if section.date == "@parent" {
		entry.Date = inherited.date
	}
	if entry.Date == "" && entry.Institution == "" && len(entry.Fields) == 0 {
		return entry, changedError()
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
	if category == "home_care" {
		raw, ok := object["hasdata"]
		var present bool
		if !ok || json.Unmarshal(raw, &present) != nil {
			return false, changedError()
		}
		return !present, nil
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
		rows, present, err := providerSectionRows(object, section)
		if err != nil {
			return err
		}
		seen = seen || present
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
		events = append(events, SupplementEntry{Key: medicalKey("e_prescriptions", parent.Key, "dispensing", i), Kind: "dispensing", Date: date, Institution: institution, Title: "第" + number + "聯來源已領紀錄", Source: parent.Source + "／來源已領（非實際服用證明）", Fields: []SupplementField{{Name: "處方就醫日期（原文）", Value: parent.Date}, {Name: "來源已領藥日期（原文）", Value: date}, {Name: "資料關係", Value: "對應此電子處方箋；不以建議領藥區間推定已領。"}}})
	}
	return events, nil
}

// FetchProvider reads every record in the observed upstream arrays. No hidden
// frontend year, source selector, or 50-row display limit narrows these requests.
func (s *Session) FetchProvider(ctx context.Context, category string, query Query) (SupplementReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	report := SupplementReport{Category: category, Entries: []SupplementEntry{}, Complete: false}
	spec, ok := providerSpecs[category]
	if !ok {
		return report, failure("INVALID_INPUT", "不支援此資料類別。")
	}
	if err := s.supplementReady(ctx); err != nil {
		return report, err
	}
	report.Coverage = spec.coverage
	resolved := query
	if spec.info.RangeMode == "date_search" {
		var err error
		resolved, _, err = resolveQuery(query, time.Now())
		if err != nil {
			return report, err
		}
		report.StartDate, report.EndDate = resolved.StartDate, resolved.EndDate
	}
	refs := map[string]providerReference{}
	for _, request := range spec.requests {
		object, err := s.providerRead(ctx, request)
		if err != nil {
			return report, err
		}
		if category == "glucose" {
			// The initial page is discovery/default data only. Query even if it
			// returns no recent rows, so older observations remain reachable.
			if _, present := object["sp_ihke3406s01_data"]; !present {
				return report, changedError()
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
		report.Entries = append(report.Entries, SupplementEntry{Key: medicalKey(category, "10568", "source_index", 0), Kind: "source_index", Title: "血壓與心率資料索引", Source: "健康存摺／生理量測資料入口", Fields: []SupplementField{{Name: "資料索引", Value: "取得明細後才包含實際量測紀錄。"}}, DetailID: reference})
	}
	if s.providerReferences == nil {
		s.providerReferences = map[string]map[string]providerReference{}
	}
	s.providerReferences[category] = refs
	s.clearDocumentCategory(category)
	report.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	return report, nil
}

func providerMakeReference(category string, row wireRow, entry SupplementEntry, query Query) (providerReference, error) {
	ref := providerReference{date: entry.Date, institution: entry.Institution, title: entry.Title, originKey: entry.Key, query: query}
	var err error
	switch category {
	case "e_prescriptions":
		ref.rowID, err = rowText(row, "treat_NUM")
		if err == nil {
			ref.sourceType, err = rowText(row, "recipe_TYPE")
		}
		if ref.rowID != "" && ref.sourceType == "" {
			return ref, changedError()
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
		if date != "" {
			entry.Fields = append(entry.Fields, SupplementField{Name: label + "來源已領藥日期", Value: date})
			institution, err := rowText(item, "hosp_NAME")
			if err != nil {
				return err
			}
			if institution != "" {
				entry.Fields = append(entry.Fields, SupplementField{Name: label + "領藥院所", Value: institution})
			}
		} else {
			window, err := rowText(item, "recipe_MEMO")
			if err != nil {
				return err
			}
			if window != "" {
				entry.Fields = append(entry.Fields, SupplementField{Name: label + "來源建議領藥期間（非已領）", Value: window})
			}
		}
	}
	return nil
}

func (s *Session) FetchProviderDetail(ctx context.Context, category, reference string) (SupplementReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	report := SupplementReport{Category: category + "_detail", Entries: []SupplementEntry{}, Complete: false}
	if err := s.supplementReady(ctx); err != nil {
		return report, err
	}
	ref, ok := s.providerReferences[category][reference]
	if !ok {
		return report, failure("INVALID_INPUT", "明細識別已失效，請重新取得目前類別清單。")
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
			for _, field := range entry.Fields {
				switch field.Name {
				case "藥品代碼", "醫令代碼":
					order.Code = field.Value
				case "藥品名稱", "藥品成分":
					order.Name = field.Value
				case "給藥日數":
					order.Days = field.Value
				case "給藥總量":
					order.Quantity = field.Value
				}
			}
			if order.Code != "" || order.Name != "" {
				entry.Kind, entry.Medicine = "medicine", &order
			}
		}
	}
	if err := s.attachProviderDocuments(category, reference, ref, object, &report); err != nil {
		return report, err
	}
	if refreshChildren {
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

func (s *Session) providerPhysiologyRaw(ctx context.Context, report *SupplementReport, category, parent string, ref providerReference, refs map[string]providerReference) error {
	fields := []fieldSpec{{"code", "來源量測項目代碼", ""}, {"create_DATE", "量測日期（原文）", ""}, {"time", "時段（原文）", ""}, {"value", "來源量測值（原文）", ""}, {"unit_DATA", "單位（原文）", ""}, {"txt_DATE", "來源編輯時間（原文，非量測日期）", ""}, {"abnormal_Status", "來源旗標（原文）", ""}}
	payload := map[string]string{"cCode": ref.rowID}
	request := providerRequest{path: "/api/ihke3000/ihke3205s02/page_load", method: http.MethodPost, payload: payload, sections: []providerSection{providerSectionSpec("sP_IHKE3205S04", ref.title+"原始紀錄", "健康存摺／生理量測原始紀錄（可含自行記錄）", "create_DATE", fields)}}
	if category == "device_activity" {
		request.path = "/api/ihke3000/ihke3207s05/page_load"
		payload["cSDate"] = strings.ReplaceAll(ref.query.StartDate, "-", "")
		payload["cEDate"] = strings.ReplaceAll(ref.query.EndDate, "-", "")
		request.sections[0].source = "健康存摺／行動裝置運動原始紀錄"
	}
	object, err := s.providerRead(ctx, request)
	if err != nil {
		return err
	}
	if _, present := object["sp_ihke3205s04"]; present {
		if err := s.providerAppend(report, object, request, category, nil, ref); err != nil {
			return err
		}
	} else {
		_, metadata, err := providerSectionRows(object, providerSection{root: "sP_IHKE3205S03"})
		if err != nil {
			return err
		}
		if _, devices := object["devicelist"]; !metadata && !devices {
			return changedError()
		}
	}
	device := providerSectionSpec("devicelist", ref.title+"行動裝置日期索引", "健康存摺／行動裝置資料來源", "cdate", []fieldSpec{{"cdate", "來源日期（原文）", ""}, {"value", "來源摘要（原文）", ""}})
	rows, _, err := providerSectionRows(object, device)
	if err != nil {
		return err
	}
	for index, row := range rows {
		entry, err := providerEntry(row, device, ref)
		if err != nil {
			return err
		}
		entry.Kind = "source_index"
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
	return nil
}

func providerVerifyDetailIdentity(category string, object wireRow, ref providerReference) error {
	var rows []wireRow
	switch category {
	case "diet":
		var success bool
		if raw, present := object["resultstatus"]; !present || json.Unmarshal(raw, &success) != nil {
			return changedError()
		}
		if !success {
			return failure("REQUEST_FAILED", "官方未提供此筆飲食明細，請重新取得來源清單後再試。")
		}
		if _, present := object["isselectfoodlist"]; !present {
			return changedError()
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
