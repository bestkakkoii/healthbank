// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// These adapters read existing calendar/reminder/notification data. They do not
// create events, supply the SPA's new-record defaults, change notification
// settings, or follow a provider-controlled navigation URL.
func extendCommunicationsProviderSpecs(items map[string]providerSpec) {
	for _, item := range []struct {
		id, label, mode, coverage string
		details                   bool
	}{
		{"calendar_events", "來源行事曆與既有記事", "date_search", "保留來源行事曆的每個回傳日期、所有類別、同日多筆內容及當日明細；不只讀目前月份，也不把記事當成已完成就醫或服用。未指定日期時查詢應用程式接受的 1912-01-01 至 2200-12-31，包含未來已排定內容；此範圍不是官方保存年限或完整性保證。", true},
		{"stored_reminders", "來源已存提醒與用藥通知", "source", "讀取來源一般、自訂及慢連箋提醒清單，並按來源已存識別取得原始通知內容與時段。提醒不代表已就醫、已領藥或實際服用；沒有已存識別時不補造日期、預設訊息或新提醒。來源沒有任意日期查詢或獨立總數。", true},
		{"notifications", "來源個別化推播訊息", "source", "分別保留來源個別化推播清單與首頁提示欄位；兩者不假定相同。原始標記以文字呈現，不執行其中的連結或程式。推播日期不是就醫日期；首頁提示沒有來源日期，不代填取得時間。未提供任意日期查詢或獨立總數。", false},
	} {
		items[item.id] = providerSpec{info: ProviderCategoryInfo{ID: item.id, Label: item.label, Group: "記事與來源通知", RangeMode: item.mode, HasDetails: item.details}, coverage: item.coverage, requests: communicationsProviderRequests(item.id)}
	}
}

var communicationsCalendarFields = []fieldSpec{{"REC_TYPE", "來源記事類別代碼", ""}, {"REC_TYPE_NAME", "來源記事類別名稱", ""}, {"REC_CON", "來源記事內容（原文）", ""}, {"ORI_TYPE", "來源就醫資料類型代碼（原文）", ""}}
var communicationsCalendarTypeFields = []fieldSpec{{"REC_TYPE", "來源記事類別代碼", ""}, {"REC_TYPE_NAME", "來源目前類別名稱", ""}, {"TypeName", "來源原類別名稱", ""}, {"Editable", "來源類別可編輯旗標（僅讀取）", ""}}
var communicationsCalendarDayFields = []fieldSpec{{"year", "來源日期年欄位（原文）", ""}, {"month", "來源日期月欄位（原文）", ""}, {"day", "來源日期日欄位（原文）", ""}, {"weekName", "來源星期文字（未重新計算）", ""}}
var communicationsReminderFields = []fieldSpec{{"PUSH_TYPE", "來源提醒類型代碼", ""}, {"PUSH_TYPE_CH", "來源提醒類型名稱", ""}, {"PUSH_MSG", "來源提醒內容（原文）", ""}, {"PUSH_MSG2", "來源補充提醒（原文）", ""}, {"PUSH_DATE", "來源設定通知日期（非就醫或服用日期）", ""}, {"HOSP_ABBR", "來源醫事機構", ""}, {"ICD9CM_CODE", "來源疾病分類代碼（非診斷確認）", ""}, {"DISPLAY_CARD_STATUS", "來源清單顯示旗標", ""}, {"DISPLAY_ALERT_STATUS", "來源通知控制顯示旗標", ""}, {"PUSH_STATUS", "來源已設通知旗標", ""}}
var communicationsChronicFields = []fieldSpec{{"icd9cm_CODE_CNAME", "來源疾病分類名稱（非診斷確認）", ""}, {"drug_CNAME", "來源提醒藥品名稱", ""}, {"order_CODE", "來源提醒藥品醫令代碼", ""}, {"sdate", "來源通知起日（非開始服用日期）", ""}, {"edate", "來源通知迄日（非停藥日期）", ""}}
var communicationsStoredDetailFields = []fieldSpec{{"notifyDate", "來源已存通知日期（非就醫或服用日期）", ""}, {"notifyMessage", "來源已存通知內容（原文）", ""}}
var communicationsChronicDetailFields = []fieldSpec{{"sdate", "來源通知起日（非開始服用日期）", ""}, {"edate", "來源通知迄日（非停藥日期）", ""}, {"hh1", "來源第 1 筆提醒小時", ""}, {"mm1", "來源第 1 筆提醒分鐘", ""}, {"hh2", "來源第 2 筆提醒小時", ""}, {"mm2", "來源第 2 筆提醒分鐘", ""}, {"hh3", "來源第 3 筆提醒小時", ""}, {"mm3", "來源第 3 筆提醒分鐘", ""}, {"hh4", "來源第 4 筆提醒小時", ""}, {"mm4", "來源第 4 筆提醒分鐘", ""}, {"push_msg", "來源已存用藥提醒內容（非實際服用紀錄）", ""}}

// Custom shapes are declared here as well as implemented below, so the checked
// registry pins every read path and projected source field. Private selectors,
// source colors and arbitrary navigation URLs are not projected clinical data.
func communicationsProviderRequests(category string) []providerRequest {
	post := func(module string, sections ...providerSection) providerRequest {
		return providerRequest{path: "/api/ihke3000/" + module, method: http.MethodPost, sections: sections}
	}
	get := func(module string, sections ...providerSection) providerRequest {
		return providerRequest{path: "/api/ihke3000/" + module, method: http.MethodGet, sections: sections}
	}
	switch category {
	case "calendar_events":
		fields := []fieldSpec{{"[date]", "來源日期鍵（原文）", ""}}
		for _, field := range communicationsCalendarDayFields {
			fields = append(fields, fieldSpec{"[date]." + field.key, field.label, field.unit})
		}
		for _, field := range communicationsCalendarFields {
			fields = append(fields, fieldSpec{"[date].data[]." + field.key, field.label, field.unit})
		}
		return []providerRequest{
			post("ihke3103s01/IHKE3103S01", personalSection("RecordData", "來源行事曆日期與全部記事", fields, true), personalSection("RecordTypeData", "來源記事類別定義", communicationsCalendarTypeFields, false)),
			post("ihke3103s01/IHKE3103S03", personalSection("", "來源當日查詢日期", []fieldSpec{{"SearchDate", "來源當日查詢日期（原文）", ""}}, true), personalSection("RecordData", "來源當日記事", communicationsCalendarFields, false), personalSection("RecordTypeData", "來源當日記事類別定義", communicationsCalendarTypeFields, false)),
		}
	case "stored_reminders":
		general := personalSection("IntimateReminder", "來源一般提醒", communicationsReminderFields, false)
		general.date, general.institution, general.titleField = "PUSH_DATE", "HOSP_ABBR", "PUSH_TYPE_CH"
		custom := personalSection("SelfDetermination", "來源自訂提醒", communicationsReminderFields, false)
		custom.date, custom.titleField = "PUSH_DATE", "PUSH_TYPE_CH"
		chronic := personalSection("slowUseMedicineList", "來源慢連箋用藥提醒（非服用紀錄）", communicationsChronicFields, false)
		chronic.date, chronic.end = "sdate", "edate"
		state := personalSection("", "來源個人化通知設定狀態（僅讀取）", []fieldSpec{{"sys_NOTIFY", "來源系統個人化通知設定代碼", ""}}, true)
		state.optional = true
		stored := personalSection("", "來源已存提醒內容", communicationsStoredDetailFields, true)
		stored.date = "notifyDate"
		times := personalSection("", "來源已存用藥提醒時段（非服用紀錄）", communicationsChronicDetailFields, true)
		times.date, times.end = "sdate", "edate"
		return []providerRequest{post("ihke3104s01/page_load", general, custom, chronic, state), post("ihke3104s01/IHKE3104S02", stored), get("IHKE3306S02/getpushdata", times)}
	case "notifications":
		history := personalSection("sp_IHKE3114S0501_list", "來源個別化推播訊息", []fieldSpec{{"push_date", "來源推播日期（非就醫日期）", ""}, {"full_msg", "來源完整訊息（含原始標記，僅文字呈現）", ""}, {"show_Url", "來源眷屬管理連結提示旗標（僅狀態）", ""}}, false)
		history.date = "push_date"
		home := personalSection("", "來源首頁推播提示快照", []fieldSpec{{"push_msg", "來源首頁提示欄位（原文）", ""}}, true)
		return []providerRequest{get("ihke3114s01/page_load", history), get("ihke3101s01/page_load", home)}
	}
	return nil
}

func communicationsFlagKey(key string) bool {
	switch key {
	case "editable", "display_card_status", "display_alert_status", "push_status", "show_url":
		return true
	}
	return false
}

func communicationsFields(row wireRow, specs []fieldSpec) ([]SupplementField, bool, error) {
	projected := make(wireRow, len(specs))
	known := false
	for _, field := range specs {
		key := strings.ToLower(field.key)
		raw, present := row[key]
		if !present {
			continue
		}
		known = true
		if communicationsFlagKey(key) {
			value := bytes.TrimSpace(raw)
			if !bytes.Equal(value, []byte("null")) {
				if !bytes.Equal(value, []byte("true")) && !bytes.Equal(value, []byte("false")) {
					return nil, false, changedError()
				}
				raw = json.RawMessage(strconv.Quote(string(value)))
			}
		}
		projected[key] = raw
	}
	fields, err := appendKnownFields(projected, specs)
	return fields, known, err
}

func communicationsEntry(row wireRow, section providerSection, category, scope, parent, origin string, index int) (SupplementEntry, error) {
	fields, known, err := communicationsFields(row, section.fields)
	if err != nil || !known {
		return SupplementEntry{}, changedError()
	}
	entry := SupplementEntry{Key: medicalKey(category, parent, scope, index), Kind: "source_message", Title: section.title, Source: section.source, Origin: origin, ContractID: sourceContractID(category, scope), Fields: fields}
	for _, item := range []struct {
		key string
		out *string
	}{{section.date, &entry.Date}, {section.end, &entry.EndDate}, {section.institution, &entry.Institution}, {section.titleField, &entry.Title}} {
		if item.key == "" {
			continue
		}
		value, readErr := rowText(row, item.key)
		if readErr != nil {
			return SupplementEntry{}, readErr
		}
		if value != "" {
			*item.out = value
		}
	}
	if entry.Date != "" {
		entry.DateRole = "unknown" // A source calendar/notice date is not an encounter or measurement.
	}
	if entry.EndDate != "" {
		entry.EndDateRole = "unknown"
	}
	return entry, nil
}

func appendCommunicationsEntry(report *SupplementReport, entry SupplementEntry) error {
	if len(report.Entries) >= MaxReportRecords {
		return failure("RESPONSE_TOO_LARGE", "來源記事或通知超過單次讀取上限，未回傳截斷資料。")
	}
	report.Entries = append(report.Entries, entry)
	return nil
}

func communicationsAppendRows(report *SupplementReport, object wireRow, section providerSection, category, scope, parent, origin string) error {
	rows, present, err := providerSectionRows(object, section)
	if err != nil {
		return err
	}
	if err := sourceSectionDiagnostic(report, category, scope, object[strings.ToLower(section.root)], present, section.optional, len(rows)); err != nil {
		return err
	}
	for index, row := range rows {
		entry, err := communicationsEntry(row, section, category, scope, parent, origin, index)
		if err != nil {
			return err
		}
		if scope == "recordtypedata" {
			entry.Kind = "source_metadata"
		}
		if err := appendCommunicationsEntry(report, entry); err != nil {
			return err
		}
	}
	return nil
}

func (s *Session) providerCommunicationsFetch(ctx context.Context, report *SupplementReport, category string, query Query, refs map[string]providerReference, documents map[string]documentReference) (bool, error) {
	switch category {
	case "calendar_events":
		return true, s.communicationsCalendar(ctx, report, query, refs)
	case "stored_reminders":
		return true, s.communicationsReminders(ctx, report, refs, documents)
	case "notifications":
		return true, s.communicationsNotifications(ctx, report)
	}
	return false, nil
}

func (s *Session) communicationsCalendar(ctx context.Context, report *SupplementReport, query Query, refs map[string]providerReference) error {
	const category = "calendar_events"
	request := communicationsProviderRequests(category)[0]
	request.payload = map[string]string{"REC_TYPE": "0", "KeyWord": "", "StartDate": strings.ReplaceAll(query.StartDate, "-", ""), "EndDate": strings.ReplaceAll(query.EndDate, "-", "")}
	object, err := s.providerRead(ctx, request)
	if err != nil {
		return err
	}
	raw, present := object["recorddata"]
	var days map[string]json.RawMessage
	if present && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if decodeJSON(raw, &days) != nil || days == nil || len(days) > MaxReportRecords {
			return changedError()
		}
	}
	if err := sourceSectionDiagnostic(report, category, "RecordData", raw, present, false, len(days)); err != nil {
		return err
	}
	if _, types := object["recordtypedata"]; !present && !types {
		return changedError()
	}
	if err := communicationsAppendRows(report, object, request.sections[1], category, "recordtypedata", "list", "mixed"); err != nil {
		return err
	}
	dates := make([]string, 0, len(days))
	for date := range days {
		dates = append(dates, date)
	}
	sort.Strings(dates)
	for _, date := range dates {
		if validateQuery(Query{StartDate: date, EndDate: date}) != nil {
			return changedError()
		}
		day, err := providerObject(days[date])
		if err != nil {
			return err
		}
		fields, _, err := communicationsFields(day, communicationsCalendarDayFields)
		if err != nil {
			return err
		}
		fields = append([]SupplementField{{Name: "來源行事曆日期鍵（非完成就醫證明）", Value: date, Role: "date"}}, fields...)
		header := SupplementEntry{Key: medicalKey(category, date, "day", 0), Kind: "source_index", Date: date, DateRole: "unknown", Title: "來源行事曆當日資料", Source: "健康存摺／來源行事曆日期索引", Origin: "mixed", ContractID: sourceContractID(category, "RecordData"), Fields: fields}
		if err := personalAddReference(refs, &header, providerReference{rowID: date, kind: "calendar_day", date: date, dateRole: "unknown", query: query}); err != nil {
			return err
		}
		if err := appendCommunicationsEntry(report, header); err != nil {
			return err
		}
		section := personalSection("data", "來源行事曆記事（非完成證明）", communicationsCalendarFields, false)
		rows, present, err := providerSectionRows(day, section)
		if err != nil {
			return err
		}
		if err := sourceSectionDiagnostic(report, category, "RecordData.data", day["data"], present, false, len(rows)); err != nil {
			return err
		}
		for index, row := range rows {
			entry, err := communicationsEntry(row, section, category, "RecordData.data", date, "mixed", index)
			if err != nil {
				return err
			}
			entry.Kind, entry.Date, entry.DateRole = "source_event", date, "unknown"
			if err := appendCommunicationsEntry(report, entry); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Session) communicationsReminders(ctx context.Context, report *SupplementReport, refs map[string]providerReference, documents map[string]documentReference) error {
	const category = "stored_reminders"
	request := communicationsProviderRequests(category)[0]
	object, err := s.providerRead(ctx, request)
	if err != nil {
		return err
	}
	seen := false
	for sectionIndex, section := range request.sections[:3] {
		rows, present, err := providerSectionRows(object, section)
		seen = seen || present
		if err != nil {
			return err
		}
		if err := sourceSectionDiagnostic(report, category, section.root, object[strings.ToLower(section.root)], present, false, len(rows)); err != nil {
			return err
		}
		for index, row := range rows {
			origin := "official_summary"
			if sectionIndex == 1 {
				origin = "self_entered"
			} else if sectionIndex == 2 {
				origin = "mixed"
			}
			entry, err := communicationsEntry(row, section, category, section.root, "list", origin, index)
			if err != nil {
				return err
			}
			entry.Kind = "source_reminder"
			idKey, kind, calendarKey := "SEQ_NO", "stored_reminder", "SEQ_NO"
			if sectionIndex == 1 {
				kind = "custom_reminder"
			} else if sectionIndex == 2 {
				idKey, kind, calendarKey = "push_ROWID", "chronic_reminder", "rec_SEQ"
			}
			id, err := rowText(row, idKey)
			if err != nil || id != "" && !usableDocumentID(id) {
				return changedError()
			}
			if id != "" {
				if err := personalAddReference(refs, &entry, providerReference{rowID: id, kind: kind}); err != nil {
					return err
				}
			}
			ready := bytes.Equal(bytes.TrimSpace(row["push_status"]), []byte("true"))
			if sectionIndex == 2 {
				// The source exposes each chronic row's rec_SEQ calendar without
				// a date gate; absent dates do not invalidate that returned file.
				ready = true
			}
			if ready {
				parent := entry.DetailID
				if parent == "" {
					parent = entry.Key
				}
				if err := appendNotificationCalendar(&entry, row, calendarKey, category, parent, documents); err != nil {
					return err
				}
			}
			if err := appendCommunicationsEntry(report, entry); err != nil {
				return err
			}
		}
	}
	if !seen {
		return changedError()
	}
	if _, present := object["sys_notify"]; present {
		entry, err := communicationsEntry(object, request.sections[3], category, "system_notification_state", "list", "self_entered", 0)
		if err != nil {
			return err
		}
		entry.Kind = "source_metadata"
		return appendCommunicationsEntry(report, entry)
	}
	return sourceSectionDiagnostic(report, category, "system_notification_state", nil, false, true, 0)
}

func (s *Session) communicationsNotifications(ctx context.Context, report *SupplementReport) error {
	const category = "notifications"
	requests := communicationsProviderRequests(category)
	for index, request := range requests {
		object, err := s.providerRead(ctx, request)
		if err != nil {
			return err
		}
		if index == 0 {
			if _, present := object["sp_ihke3114s0501_list"]; !present {
				return changedError()
			}
			if err := communicationsAppendRows(report, object, request.sections[0], category, "sp_IHKE3114S0501_list", "history", "official_summary"); err != nil {
				return err
			}
			continue
		}
		raw, present := object["push_msg"]
		if !present {
			if err := sourceSectionDiagnostic(report, category, "homepage_push_msg", nil, false, false, 0); err != nil {
				return err
			}
			continue
		}
		value, err := rowText(object, "push_msg")
		if err != nil {
			return err
		}
		count := 0
		if value != "" {
			count = 1
		}
		if err := sourceSectionDiagnostic(report, category, "homepage_push_msg", raw, true, false, count); err != nil {
			return err
		}
		entry := SupplementEntry{Key: medicalKey(category, "homepage", "push_msg", 0), Kind: "source_summary", Title: request.sections[0].title, Source: request.sections[0].source, Origin: "official_summary", ContractID: sourceContractID(category, "homepage_push_msg"), Fields: []SupplementField{{Name: "來源首頁提示欄位（原文，未推定與歷史訊息相同）", Code: "push_msg", Value: value}}}
		if err := appendCommunicationsEntry(report, entry); err != nil {
			return err
		}
	}
	return nil
}

func communicationsProviderDetailRequest(category string, ref providerReference) (providerRequest, error) {
	requests := communicationsProviderRequests(category)
	switch {
	case category == "calendar_events" && ref.kind == "calendar_day":
		if validateQuery(Query{StartDate: ref.rowID, EndDate: ref.rowID}) != nil {
			return providerRequest{}, changedError()
		}
		request := requests[1]
		request.payload = map[string]string{"SearchDate": ref.rowID}
		return request, nil
	case category == "stored_reminders" && (ref.kind == "stored_reminder" || ref.kind == "custom_reminder"):
		request := requests[1]
		request.payload = map[string]string{"SEQ_NO": ref.rowID}
		return request, nil
	case category == "stored_reminders" && ref.kind == "chronic_reminder":
		request := requests[2]
		request.params = url.Values{"push_id": {ref.rowID}}
		return request, nil
	}
	return providerRequest{}, failure("INVALID_INPUT", "來源記事或通知明細識別已失效，請重新取得清單。")
}

func (s *Session) providerCommunicationsDetail(ctx context.Context, report *SupplementReport, category, reference string, ref providerReference) (bool, error) {
	if category != "calendar_events" && category != "stored_reminders" {
		return false, nil
	}
	request, err := communicationsProviderDetailRequest(category, ref)
	if err != nil {
		return true, err
	}
	object, err := s.providerRead(ctx, request)
	if err != nil {
		return true, err
	}
	report.Coverage = providerSpecs[category].coverage
	if category == "stored_reminders" {
		origin := "official_summary"
		if ref.kind == "custom_reminder" {
			origin = "self_entered"
		} else if ref.kind == "chronic_reminder" {
			origin = "mixed"
		}
		entry, err := communicationsEntry(object, request.sections[0], category, ref.kind, ref.originKey, origin, 0)
		if err != nil {
			return true, err
		}
		entry.Kind = "source_reminder"
		return true, appendCommunicationsEntry(report, entry)
	}
	date, err := rowText(object, "SearchDate")
	if err != nil || date != ref.rowID {
		return true, changedError()
	}
	report.StartDate, report.EndDate = ref.query.StartDate, ref.query.EndDate
	header := SupplementEntry{Key: medicalKey(category, ref.originKey, "day_header", 0), Kind: "source_index", Date: date, DateRole: "unknown", Title: "來源當日行事曆明細", Source: "健康存摺／當日記事與就醫摘要", Origin: "mixed", ContractID: sourceContractID(category, "day_header"), Fields: []SupplementField{{Name: "來源當日查詢日期（非完成就醫證明）", Code: "searchdate", Role: "date", Value: date}}}
	if err := appendCommunicationsEntry(report, header); err != nil {
		return true, err
	}
	rows, present, err := providerSectionRows(object, request.sections[1])
	if err != nil {
		return true, err
	}
	if err := sourceSectionDiagnostic(report, category, "day_RecordData", object["recorddata"], present, false, len(rows)); err != nil {
		return true, err
	}
	for index, row := range rows {
		entry, err := communicationsEntry(row, request.sections[1], category, "day_RecordData", ref.originKey, "mixed", index)
		if err != nil {
			return true, err
		}
		entry.Kind, entry.Date, entry.DateRole = "source_event", date, "unknown"
		if err := appendCommunicationsEntry(report, entry); err != nil {
			return true, err
		}
	}
	return true, communicationsAppendRows(report, object, request.sections[2], category, "recordtypedata", ref.originKey, "mixed")
}
