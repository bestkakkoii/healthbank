// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// These additional reads are kept separate even where the SPA displays similar
// fields: a matching projection does not establish identical server rowsets.
func extendResidualProviderSpecs(items map[string]providerSpec) {
	menstrual := items["menstrual"]
	menstrual.info.HasDetails = true
	menstrual.info.RangeMode = "date_search"
	menstrual.requests[0].sections[0].detail = true
	menstrual.coverage += "另列所選期間各月份的獨立月曆查詢入口，明細逐月讀取全部回傳起迄；來源期間清單本身沒有日期篩選，與月曆不假設相同。已回傳期間可另讀起日原始紀錄；月份入口不代表該月已有經期紀錄。"
	items["menstrual"] = menstrual

	family := items["family_doctor"]
	family.requests[0].sections[1].fields = append(family.requests[0].sections[1].fields, fieldSpec{"memo", "來源計畫說明（逐項原文）", ""})
	family.requests[0].sections = append(family.requests[0].sections, providerMetadataSection("", "家庭醫師來源說明", "健康存摺／家庭醫師計畫來源說明", []fieldSpec{{"memo", "來源未收案說明（原文）", ""}}))
	area := providerSectionSpec("", "來源院所所屬社區醫療群", "健康存摺／家庭醫師院所群組", "", []fieldSpec{{"area_code", "來源社區醫療群代碼", ""}, {"area_code_name", "來源社區醫療群名稱", ""}})
	family.requests = append(family.requests, providerRequest{path: "/api/ihke3000/ihke3720s01/SQL100", method: http.MethodPost, sections: []providerSection{area}})
	family.coverage += "來源計畫狀態為 1／3 且有院所識別時，另讀取該院所所屬社區醫療群；不推定其他年度收案。"
	items["family_doctor"] = family

	bone := items["osteoporosis"]
	search := bone.requests[0]
	search.path, search.responseArray = "/api/ihke3000/ihke3411s01/search", true
	search.sections = append([]providerSection{}, search.sections...)
	search.sections[0].root = ""
	bone.requests = append(bone.requests, search)
	bone.requests[0].sections = append(bone.requests[0].sections, providerMetadataSection("", "骨質疏鬆來源提供區間", "健康存摺／骨質疏鬆來源提供區間（非個別檢查日期）", []fieldSpec{{"c_sdate", "來源提供區間起日（原文）", ""}, {"c_edate", "來源提供區間迄日（原文）", ""}}))
	bone.coverage = "分別取得來源首頁與全部類型搜尋回應（s_type 為空、s_sort 為 A1），保留各自全部資料列及明細。不假設兩次回應相同；來源未提供任意日期搜尋或獨立總數。"
	items["osteoporosis"] = bone

	covid := items["covid"]
	for index := range covid.requests[0].sections {
		section := &covid.requests[0].sections[index]
		section.fields = append(section.fields, fieldSpec{"source_NAME", "來源名稱（原文）", ""}, fieldSpec{"real_INSPECT_DATE_EN", "來源英文檢查日期", ""}, fieldSpec{"recv_DATE_EN", "來源英文收件日期", ""}, fieldSpec{"inoculate_D_en", "來源英文接種日期", ""}, fieldSpec{"sample_DATE_EN", "來源英文採檢日期", ""}, fieldSpec{"confirm_DATE_EN", "來源英文確認日期", ""}, fieldSpec{"assay_ITEM_NAME_EN", "來源英文檢驗名稱", ""}, fieldSpec{"assay_VALUE_EN", "來源英文結果（原文）", ""})
	}
	timeline := providerSectionSpec("sp_IHKE3410S06", "COVID-19 來源時間軸文字", "健康存摺／COVID-19 來源時間軸摘要", "real_INSPECT_DATE", []fieldSpec{{"real_INSPECT_DATE", "來源日期（原文）", ""}, {"assay_VALUE[]", "來源時間軸逐項中文原文", ""}, {"assay_VALUE_EN[]", "來源時間軸逐項英文原文", ""}})
	status := providerSectionSpec("", "COVID-19 來源接種摘要狀態", "健康存摺／COVID-19 來源摘要", "", []fieldSpec{{"reload", "來源要求等待更新旗標", ""}, {"count_tw", "來源接種數量顯示值", ""}, {"count_tw_all", "來源接種總量顯示值", ""}, {"count_en", "來源英文接種數量顯示值", ""}, {"now_tw", "來源中文查詢日期（非檢查日期）", ""}, {"now_en", "來源英文查詢日期（非檢查日期）", ""}})
	status.optional = true
	covid.requests[0].sections = append(covid.requests[0].sections, status, timeline)
	covid.requests = append(covid.requests, providerRequest{path: "/api/ihke3000/IHKE3410S01/reload_Vaccine", method: http.MethodGet, sections: []providerSection{covid.requests[0].sections[3], timeline}})
	covid.coverage = "保留來源檢驗、接種、個案研判及時間軸逐項原文。只有來源明確回傳 reload=true 時，依官網 12 秒等待後讀取一次接種更新並另存該次回應；不重複輪詢、不產生證明或轉出個人 QR 識別。來源未提供任意日期或完整總數。"
	items["covid"] = covid

	sections := []providerSection{}
	for _, item := range []struct{ root, title string }{{"western_data", "來源近期西醫摘要"}, {"chinese_data", "來源近期中醫摘要"}, {"dentist_data", "來源近期牙醫摘要"}} {
		section := providerSectionSpec(item.root, item.title, "健康存摺／近期就醫摘要", "func_date", []fieldSpec{{"func_date", "來源就醫日期（原文）", ""}, {"icd9cm_name", "來源疾病分類（非診斷確認）", ""}, {"htype", "來源就醫類型", ""}})
		section.detail = true
		sections = append(sections, section)
	}
	items["recent_visits"] = providerSpec{info: ProviderCategoryInfo{ID: "recent_visits", Label: "近期就醫來源摘要", Group: "用藥與就醫", RangeMode: "source", HasDetails: true}, coverage: "分別保留官方近期西醫、中醫與牙醫摘要全部回傳列，並依來源類型開啟原始明細。不假設與歷史清單相同，也不代表全部年度。", requests: []providerRequest{{path: "/api/ihke3000/ihke3301s02/page_load", method: http.MethodGet, sections: sections}}}
}

func (s *Session) providerFamilyDoctorFetch(ctx context.Context, report *SupplementReport) error {
	requests := providerSpecs["family_doctor"].requests
	object, err := s.providerRead(ctx, requests[0])
	if err != nil {
		return err
	}
	if err := s.providerAppend(report, object, requests[0], "family_doctor", nil, providerReference{}); err != nil {
		return err
	}
	statuses, _, err := providerSectionRows(object, requests[0].sections[0])
	if err != nil || len(statuses) != 1 {
		return err
	}
	status, err := rowText(statuses[0], "status")
	if err != nil || (status != "1" && status != "3") {
		return err
	}
	providers, present, err := providerSectionRows(object, requests[0].sections[1])
	if err != nil || !present || len(providers) == 0 {
		return err
	}
	id, err := rowText(providers[0], "hosp_ID")
	if err != nil || id == "" {
		return err
	}
	if !usableDocumentID(id) {
		return changedError()
	}
	request := requests[1]
	request.payload = map[string]string{"HOSP_ID": id}
	area, err := s.providerRead(ctx, request)
	if err != nil {
		return err
	}
	fields, err := appendKnownFields(area, request.sections[0].fields)
	if err != nil {
		return err
	}
	if len(fields) == 0 {
		// An explicitly empty directory match is not a clinical observation.
		if _, code := area["area_code"]; code {
			if _, name := area["area_code_name"]; name {
				return sourceSectionDiagnostic(report, "family_doctor", "community_group", nil, true, false, 0)
			}
		}
		return changedError()
	}
	return s.providerAppend(report, area, request, "family_doctor", nil, providerReference{})
}

func (s *Session) providerBoneFetch(ctx context.Context, report *SupplementReport, refs map[string]providerReference) error {
	requests := providerSpecs["osteoporosis"].requests
	object, err := s.providerRead(ctx, requests[0])
	if err != nil {
		return err
	}
	if err := s.providerAppend(report, object, requests[0], "osteoporosis", refs, providerReference{}); err != nil {
		return err
	}
	request := requests[1]
	rows, err := s.supplementRequest(ctx, http.MethodGet, request.path, request.params, nil, "", report)
	if err != nil {
		return err
	}
	for index, row := range rows {
		// The array root is declared explicitly. The row index stays in the
		// inherited key scope so identical source rows are not collapsed.
		if err := s.providerAppend(report, row, request, "osteoporosis", refs, providerReference{originKey: strconv.Itoa(index)}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Session) providerCOVIDFetch(ctx context.Context, report *SupplementReport) error {
	requests := providerSpecs["covid"].requests
	object, err := s.providerRead(ctx, requests[0])
	if err != nil {
		return err
	}
	if err := s.appendCOVIDSnapshot(report, object, requests[0]); err != nil {
		return err
	}
	raw, present := object["reload"]
	if !present {
		return nil
	}
	switch string(bytes.TrimSpace(raw)) {
	case "false":
		return nil
	case "true":
		// The public page waits 12 seconds once after an explicit reload flag.
		// Cancellation interrupts this wait and cannot trigger a later request.
		timer := time.NewTimer(12 * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	default:
		return changedError()
	}
	updated, err := s.providerRead(ctx, requests[1])
	if err != nil {
		return err
	}
	return s.appendCOVIDSnapshot(report, updated, requests[1])
}

func (s *Session) appendCOVIDSnapshot(report *SupplementReport, object wireRow, request providerRequest) error {
	regular := request
	regular.sections = []providerSection{}
	for _, section := range request.sections[:len(request.sections)-1] {
		if section.root == "" {
			status := make(wireRow, len(section.fields))
			for _, field := range section.fields {
				key := strings.ToLower(field.key)
				if raw, exists := object[key]; exists {
					if key == "reload" {
						switch string(bytes.TrimSpace(raw)) {
						case "true", "false":
							raw = json.RawMessage(strconv.Quote(string(bytes.TrimSpace(raw))))
						default:
							return changedError()
						}
					}
					status[key] = raw
				}
			}
			if len(status) > 0 {
				fields, err := appendKnownFields(status, section.fields)
				if err != nil {
					return err
				}
				report.Entries = append(report.Entries, SupplementEntry{Key: medicalKey("covid", request.path, "snapshot_status", 0), Kind: "source_summary", Origin: "official_summary", ContractID: sourceContractID("covid", "snapshot_status"), Title: section.title, Source: section.source, Fields: fields})
			}
			continue
		}
		regular.sections = append(regular.sections, section)
	}
	section := request.sections[len(request.sections)-1]
	regularPresent := false
	for _, current := range regular.sections {
		_, present := object[strings.ToLower(current.root)]
		regularPresent = regularPresent || present
	}
	if regularPresent {
		if err := s.providerAppend(report, object, regular, "covid", nil, providerReference{}); err != nil {
			return err
		}
	} else {
		if _, present := object[strings.ToLower(section.root)]; !present {
			return changedError()
		}
		for _, current := range regular.sections {
			if err := sourceSectionDiagnostic(report, "covid", current.root, nil, false, current.optional, 0); err != nil {
				return err
			}
		}
	}
	rows, present, err := providerSectionRows(object, section)
	if err != nil {
		return err
	}
	if err := sourceSectionDiagnostic(report, "covid", section.root, object[strings.ToLower(section.root)], present, false, len(rows)); err != nil {
		return err
	}
	for index, row := range rows {
		date, err := rowText(row, "real_INSPECT_DATE")
		if err != nil {
			return err
		}
		entry := SupplementEntry{Key: medicalKey("covid", request.path, section.root, index), Kind: "source_summary", Date: date, Origin: "official_summary", ContractID: sourceContractID("covid", section.root), Title: section.title, Source: section.source, Fields: []SupplementField{}}
		if date != "" {
			entry.DateRole = "unknown" // this combined timeline is not a specimen record
		}
		known := date != ""
		for _, field := range []struct{ key, label string }{{"assay_value", "來源中文時間軸"}, {"assay_value_en", "來源英文時間軸"}} {
			raw, exists := row[field.key]
			known = known || exists
			if !exists || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				continue
			}
			var values []json.RawMessage
			if decodeJSON(raw, &values) != nil || values == nil || len(values) > MaxReportRecords {
				return changedError()
			}
			for item, rawValue := range values {
				var value string
				if bytes.Equal(bytes.TrimSpace(rawValue), []byte("null")) {
					value = "null"
				} else if json.Unmarshal(rawValue, &value) != nil {
					return changedError()
				}
				entry.Fields = append(entry.Fields, SupplementField{Name: field.label + "第 " + strconv.Itoa(item+1) + " 項（原文；null 為未提供）", Value: value, Code: field.key})
			}
		}
		if !known {
			return changedError()
		}
		report.Entries = append(report.Entries, entry)
	}
	return nil
}

func providerMenstrualDetailRequest(ref providerReference) providerRequest {
	section := providerSpecs["menstrual"].requests[0].sections[0]
	section.root, section.detail = "sP_IHKE3208S03", false
	return providerRequest{path: "/api/ihke3000/ihke3208s01/SP_IHKE3208S03", method: http.MethodGet, params: url.Values{"cSDate": {ref.rowID}}, sections: []providerSection{section}}
}

func providerRecentDetailRequests() []providerRequest {
	requests := []providerRequest{}
	for _, category := range []string{"western", "chinese", "dental"} {
		spec := medicalSpecs[category]
		header := providerSectionSpec(spec.detailRoot, spec.title+"明細", "健康存摺／近期就醫原始明細", "func_DATE", medicalHeaderFields)
		sections := []providerSection{header}
		for _, section := range spec.sections {
			sections = append(sections, providerSectionSpec(section.key, section.title, "健康存摺／近期就醫原始明細", "@parent", section.fields))
		}
		requests = append(requests, providerRequest{path: spec.detailPath, method: http.MethodGet, sections: sections})
	}
	return requests
}

// Only actual returned ISO/slash dates become day selectors. Original display
// dates remain untouched in the exported entries.
func providerMenstrualDate(date string) (string, error) {
	date = strings.ReplaceAll(date, "/", "-")
	if date == "" {
		return "", nil
	}
	if len(date) == 8 {
		parsed, err := time.Parse("20060102", date)
		if err != nil {
			return "", changedError()
		}
		date = parsed.Format("2006-01-02")
	}
	if err := validateQuery(Query{StartDate: date, EndDate: date}); err != nil {
		return "", changedError()
	}
	return date, nil
}
