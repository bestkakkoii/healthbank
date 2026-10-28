// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCommunicationsCalendarReadsFutureDatesDuplicatesAndBoundDays(t *testing.T) {
	queries := make(chan map[string]string, 4)
	var dayCalls atomic.Int32
	var wrongDay atomic.Bool
	const content = "合成原始記事 <a href=\"https://example.invalid\">僅文字</a>"
	types := []any{map[string]any{"REC_TYPE": "9", "REC_TYPE_NAME": "來源就醫分類", "TypeName": "來源固定類別", "Editable": false}, map[string]any{"REC_TYPE": "A", "REC_TYPE_NAME": "自訂項目", "TypeName": "自訂類別", "Editable": true}}
	event := map[string]any{"REC_TYPE": "9", "REC_TYPE_NAME": "來源就醫分類", "REC_CON": content, "ORI_TYPE": "A", "ROW_ID": "PRIVATE_MEDICAL_LINK"}
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Error("calendar acquisition used an unobserved method")
		}
		var body map[string]string
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Error("calendar read payload missing")
		}
		switch r.URL.Path {
		case "/api/ihke3000/ihke3103s01/IHKE3103S01":
			if len(body) != 4 || body["REC_TYPE"] != "0" || body["KeyWord"] != "" {
				t.Error("calendar silently narrowed source types or keywords")
			}
			queries <- body
			respondJSON(t, w, map[string]any{"RecordTypeData": types, "RecordData": map[string]any{
				"2027-03-10": map[string]any{"year": 2027, "month": 3, "day": 10, "weekName": "來源星期標記", "data": []any{event, event}},
				"2027-03-11": map[string]any{"weekName": "另一來源標記", "data": []any{}},
			}})
		case "/api/ihke3000/ihke3103s01/IHKE3103S03":
			dayCalls.Add(1)
			day := body["SearchDate"]
			if len(body) != 1 || (day != "2027-03-10" && day != "2027-03-11") {
				t.Error("day detail escaped its returned calendar date")
			}
			if wrongDay.Load() {
				day = "2027-04-01"
			}
			respondJSON(t, w, map[string]any{"SearchDate": day, "RecordData": []any{event, event, map[string]any{"REC_TYPE": "A", "REC_TYPE_NAME": "自訂項目", "REC_CON": "只在當日明細回傳"}}, "RecordTypeData": types})
		default:
			t.Error("calendar acquisition followed a redirect or invoked a source edit")
			http.NotFound(w, r)
		}
	})
	report, err := s.FetchProvider(context.Background(), "calendar_events", Query{})
	if err != nil {
		t.Fatal(err)
	}
	query := <-queries
	if query["StartDate"] != "19120101" || query["EndDate"] != "22001231" || report.StartDate != "1912-01-01" || report.EndDate != "2200-12-31" || report.Complete {
		t.Fatal("default calendar range lost future saved events or claimed lifetime completeness")
	}
	if len(report.Entries) != 6 || len(s.providerReferences["calendar_events"]) != 2 {
		t.Fatal("calendar date index, duplicate source events or type metadata lost")
	}
	dates := map[string]string{}
	keys := map[string]bool{}
	for _, entry := range report.Entries {
		if entry.Key == "" || keys[entry.Key] || entry.Medicine != nil {
			t.Fatal("calendar duplicates collapsed or became medication records")
		}
		keys[entry.Key] = true
		if entry.Date != "" && entry.DateRole != "unknown" {
			t.Fatal("calendar date became an observed encounter or examination date")
		}
		if entry.DetailID != "" {
			dates[entry.Date] = entry.DetailID
		}
	}
	if values := personalFieldValues(report, "rec_con"); len(values) != 2 || values[0] != content || values[1] != content {
		t.Fatal("same-day raw source contents were changed or merged")
	}
	if values := personalFieldValues(report, "weekname"); len(values) != 2 || values[0] != "來源星期標記" {
		t.Fatal("source day text was replaced by a local calendar calculation")
	}
	if _, err := s.FetchProviderDetail(context.Background(), "stored_reminders", dates["2027-03-10"]); err == nil || dayCalls.Load() != 0 {
		t.Fatal("calendar reference was accepted by another category")
	}
	var previousKeys map[string]bool
	for _, date := range []string{"2027-03-10", "2027-03-11"} {
		detail, err := s.FetchProviderDetail(context.Background(), "calendar_events", dates[date])
		if err != nil || len(detail.Entries) != 6 || detail.Complete {
			t.Fatal("bound date detail lost source-only records", err)
		}
		if values := personalFieldValues(detail, "rec_con"); len(values) != 3 || values[2] != "只在當日明細回傳" {
			t.Fatal("date detail was replaced by the source list's smaller projection")
		}
		current := map[string]bool{}
		for _, entry := range detail.Entries {
			if previousKeys[entry.Key] {
				t.Fatal("different calendar days shared a projected record key")
			}
			current[entry.Key] = true
		}
		previousKeys = current
	}
	wrongDay.Store(true)
	if _, err := s.FetchProviderDetail(context.Background(), "calendar_events", dates["2027-03-10"]); err == nil {
		t.Fatal("a different returned day was attached to the selected date")
	}
	custom, err := s.FetchProvider(context.Background(), "calendar_events", Query{StartDate: "2027-03-01", EndDate: "2027-03-31"})
	if err != nil {
		t.Fatal(err)
	}
	query = <-queries
	if query["StartDate"] != "20270301" || query["EndDate"] != "20270331" || custom.StartDate != "2027-03-01" || custom.EndDate != "2027-03-31" {
		t.Fatal("explicit calendar dates were replaced by default bounds")
	}
	before := dayCalls.Load()
	if _, err := s.FetchProviderDetail(context.Background(), "calendar_events", dates["2027-03-10"]); err == nil || dayCalls.Load() != before {
		t.Fatal("retired calendar date reference remained usable")
	}
	if _, err := s.FetchProvider(context.Background(), "calendar_events", Query{StartDate: "2027-03-01"}); err == nil || len(queries) != 0 {
		t.Fatal("partial explicit calendar query became an invented range")
	}
	encoded, _ := json.Marshal(report)
	if strings.Contains(string(encoded), "PRIVATE_MEDICAL_LINK") {
		t.Fatal("calendar medical navigation selector escaped")
	}
}

func TestCommunicationsRemindersPreserveStoredFactsWithoutNewDefaults(t *testing.T) {
	var downloads atomic.Int32
	const calendar = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nSUMMARY:合成來源提醒\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/ihke3000/ihke3104s01/page_load":
			body, _ := io.ReadAll(r.Body)
			if r.Method != http.MethodPost || len(body) != 0 {
				t.Error("reminder list deviated from its source read contract")
			}
			respondJSON(t, w, map[string]any{
				"IntimateReminder": []any{
					map[string]any{"SEQ_NO": "PRIVATE_GENERAL", "PUSH_TYPE": "01", "PUSH_TYPE_CH": "來源一般提醒", "PUSH_MSG": "合成來源提醒 <b>原文</b>", "PUSH_MSG2": "來源補充", "PUSH_DATE": "115/11/01", "HOSP_ABBR": "合成院所", "PUSH_STATUS": true, "DISPLAY_CARD_STATUS": false, "DISPLAY_ALERT_STATUS": true, "DEFAULT_NOTIFY_NOTE": "PRIVATE_UNSAVED_DEFAULT"},
					map[string]any{"SEQ_NO": "PRIVATE_DISABLED", "PUSH_TYPE_CH": "來源未啟用提醒", "PUSH_DATE": "", "PUSH_STATUS": false},
					map[string]any{"SEQ_NO": nil, "PUSH_TYPE_CH": "尚未設定", "PUSH_DATE": nil, "PUSH_STATUS": false, "DEFAULT_NOTIFY_NOTE": "PRIVATE_NEW_DEFAULT"},
				},
				"SelfDetermination": []any{map[string]any{"SEQ_NO": "PRIVATE_CUSTOM", "PUSH_TYPE_CH": "來源自訂提醒", "PUSH_DATE": "115/12/01", "PUSH_STATUS": true}},
				"slowUseMedicineList": []any{
					map[string]any{"push_ROWID": "PRIVATE_CHRONIC", "rec_SEQ": "PRIVATE_CHRONIC_ICS", "sdate": "115/11/01", "edate": "115/11/30", "drug_CNAME": "合成藥品", "icd9cm_CODE_CNAME": "來源分類", "order_CODE": "SYNTHETIC_DRUG"},
					map[string]any{"push_ROWID": "", "rec_SEQ": "PRIVATE_UNDATED_ICS", "sdate": "", "edate": "", "drug_CNAME": "來源未提供日期的藥品提醒"},
				}, "sys_NOTIFY": "Y", "sName": "PRIVATE_PERSON_NAME", "token": "PRIVATE_TOKEN",
			})
		case "/api/ihke3000/ihke3104s01/IHKE3104S02":
			var body map[string]string
			if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&body) != nil || len(body) != 1 {
				t.Error("stored notification detail used a source write or unbound payload")
			}
			switch body["SEQ_NO"] {
			case "PRIVATE_GENERAL", "PRIVATE_CUSTOM":
				respondJSON(t, w, map[string]any{"notifyDate": "115/11/02", "notifyMessage": "來源已存 <script>不可執行</script>"})
			case "PRIVATE_DISABLED":
				respondJSON(t, w, map[string]any{"notifyDate": "", "notifyMessage": ""})
			default:
				t.Error("notification detail accepted an unobserved private selector")
			}
		case "/api/ihke3000/IHKE3306S02/getpushdata":
			if r.Method != http.MethodGet || len(r.URL.Query()) != 1 || r.URL.Query().Get("push_id") != "PRIVATE_CHRONIC" {
				t.Error("chronic reminder detail was not source-bound")
			}
			respondJSON(t, w, map[string]any{"sdate": "115/11/02", "edate": "115/11/29", "hh1": "00", "mm1": "05", "hh2": "23", "mm2": "59", "hh3": "", "mm3": nil, "hh4": "12", "mm4": "30", "push_msg": "來源已存用藥提醒"})
		case "/api/ihke3000/ihke3104s01/IHKE3104S08":
			downloads.Add(1)
			id := r.URL.Query().Get("SEQ_NO")
			if r.Method != http.MethodGet || len(r.URL.Query()) != 1 || (id != "PRIVATE_GENERAL" && id != "PRIVATE_CUSTOM" && id != "PRIVATE_CHRONIC_ICS" && id != "PRIVATE_UNDATED_ICS") {
				t.Error("calendar used an unreturned sequence or disabled general reminder")
			}
			w.Header().Set("Content-Type", "text/calendar;charset=UTF-8")
			_, _ = io.WriteString(w, calendar)
		default:
			t.Error("reminder acquisition attempted a mutation, fallback default or mobile handoff")
			http.NotFound(w, r)
		}
	})
	report, err := s.FetchProvider(context.Background(), "stored_reminders", Query{})
	if err != nil || len(report.Entries) != 7 || downloads.Load() != 0 || report.Complete {
		t.Fatal("source reminder list lost records or downloaded artifacts implicitly", err)
	}
	if len(s.providerReferences["stored_reminders"]) != 4 || len(s.documentReferences) != 4 {
		t.Fatal("existing details or calendar readiness did not match the source rows")
	}
	for _, entry := range report.Entries {
		if entry.Medicine != nil || entry.Kind == "dispensing" || len(entry.DocumentActions) != 0 {
			t.Fatal("reminder became medication adherence or a new source action")
		}
		if entry.Title == "尚未設定" && (entry.Date != "" || entry.DetailID != "" || len(entry.Attachments) != 0) {
			t.Fatal("missing stored reminder acquired the source UI's new-record defaults")
		}
		for _, field := range entry.Fields {
			if field.Code == "drug_cname" && field.Value == "來源未提供日期的藥品提醒" && (entry.Date != "" || entry.EndDate != "" || len(entry.Attachments) != 1) {
				t.Fatal("source-provided chronic calendar was suppressed or acquired invented dates")
			}
		}
		if entry.DetailID != "" {
			detail, err := s.FetchProviderDetail(context.Background(), "stored_reminders", entry.DetailID)
			if err != nil || len(detail.Entries) != 1 {
				t.Fatal("existing reminder detail unavailable", err)
			}
			if detail.Entries[0].Origin != entry.Origin {
				t.Fatal("general, custom and mixed reminder detail origins were conflated")
			}
			if entry.Title == "來源未啟用提醒" && (detail.Entries[0].Date != "" || len(detail.Entries[0].Fields) != 0) {
				t.Fatal("empty stored fields were replaced by default message or tomorrow")
			}
			if strings.Contains(entry.Title, "慢連箋") {
				if detail.Entries[0].Date != "115/11/02" || detail.Entries[0].EndDate != "115/11/29" {
					t.Fatal("stored reminder detail dates were overwritten by list context")
				}
				if values := personalFieldValues(detail, "hh1"); len(values) != 1 || values[0] != "00" {
					t.Fatal("midnight reminder value was treated as absent or normalized")
				}
			}
		}
		for _, attachment := range entry.Attachments {
			var info DocumentInfo
			var output bytes.Buffer
			if err := s.DownloadDocument(context.Background(), "stored_reminders", attachment.ID, collectDocument(&info, &output)); err != nil || output.String() != calendar {
				t.Fatal("bound original reminder calendar changed", err)
			}
		}
	}
	if downloads.Load() != 4 {
		t.Fatal("not every ready existing source calendar was independently downloadable")
	}
	encoded, _ := json.Marshal(report)
	if strings.Contains(string(encoded), "PRIVATE_") {
		t.Fatal("private identity/selector/default template leaked into reminder data")
	}
}

func TestCommunicationsReminderAdmissionPreservesOldDetailsAndCalendars(t *testing.T) {
	var grow atomic.Bool
	var malformed atomic.Bool
	var downloads atomic.Int32
	s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/ihke3000/ihke3104s01/page_load":
			rows := []any{map[string]any{"SEQ_NO": "PRIVATE_OLD", "PUSH_TYPE_CH": "合成已存提醒", "PUSH_DATE": "115/11/01", "PUSH_STATUS": true}}
			if grow.Load() {
				rows = append(rows, map[string]any{"SEQ_NO": "PRIVATE_NEW", "PUSH_TYPE_CH": strings.Repeat("來源新增提醒", 40), "PUSH_DATE": "115/11/02", "PUSH_STATUS": true})
			}
			if malformed.Load() {
				rows = []any{map[string]any{"SEQ_NO": "PRIVATE_NEW", "PUSH_TYPE_CH": "不接受字串布林", "PUSH_STATUS": "false"}}
			}
			respondJSON(t, w, map[string]any{"IntimateReminder": rows, "SelfDetermination": []any{}, "slowUseMedicineList": []any{}})
		case "/api/ihke3000/ihke3104s01/IHKE3104S02":
			var body map[string]string
			if json.NewDecoder(r.Body).Decode(&body) != nil || body["SEQ_NO"] != "PRIVATE_OLD" {
				t.Error("unadmitted reminder reference reached the source")
			}
			respondJSON(t, w, map[string]any{"notifyDate": "115/11/01", "notifyMessage": "合成已存內容"})
		case "/api/ihke3000/ihke3104s01/IHKE3104S08":
			downloads.Add(1)
			if r.URL.Query().Get("SEQ_NO") != "PRIVATE_OLD" {
				t.Error("unadmitted document reference reached the source")
			}
			_, _ = io.WriteString(w, "BEGIN:VCALENDAR\nVERSION:2.0\nEND:VCALENDAR\n")
		default:
			t.Error("unexpected reminder operation")
		}
	})
	var held int64
	limit := int64(8192)
	if err := s.SetReferenceBudget(func(previous, next int64) bool {
		if previous != held {
			t.Fatal("reference replacement lost its previous reservation")
		}
		if next > limit && next > previous {
			return false
		}
		held = next
		return true
	}); err != nil {
		t.Fatal(err)
	}
	s.token, s.tokenObtainedAt = "synthetic-provider-token", time.Now()
	first, err := s.FetchProvider(context.Background(), "stored_reminders", Query{})
	if err != nil || len(first.Entries) != 1 || len(first.Entries[0].Attachments) != 1 || held == 0 {
		t.Fatal("initial staged reminder references unavailable", err)
	}
	detailID, attachment := first.Entries[0].DetailID, first.Entries[0].Attachments[0].ID
	limit = held
	grow.Store(true)
	_, err = s.FetchProvider(context.Background(), "stored_reminders", Query{})
	var upstream *Error
	if !errors.As(err, &upstream) || upstream.Code != "RESOURCE_LIMIT" || held != limit || s.referenceBytes != limit || len(s.providerReferences["stored_reminders"]) != 1 || len(s.documentReferences) != 1 {
		t.Fatal("denied combined list/document replacement changed live references", err)
	}
	malformed.Store(true)
	if _, err := s.FetchProvider(context.Background(), "stored_reminders", Query{}); err == nil {
		t.Fatal("string false became a ready source notification")
	}
	if _, err := s.FetchProviderDetail(context.Background(), "stored_reminders", detailID); err != nil {
		t.Fatal("failed replacement retired the old stored detail", err)
	}
	var info DocumentInfo
	var output bytes.Buffer
	if err := s.DownloadDocument(context.Background(), "stored_reminders", attachment, collectDocument(&info, &output)); err != nil || downloads.Load() != 1 {
		t.Fatal("failed replacement retired the old calendar", err)
	}
	grow.Store(false)
	malformed.Store(false)
	if _, err := s.FetchProvider(context.Background(), "stored_reminders", Query{}); err != nil {
		t.Fatal(err)
	}
	if err := s.DownloadDocument(context.Background(), "stored_reminders", attachment, collectDocument(&info, &output)); err == nil || downloads.Load() != 1 {
		t.Fatal("successful category refresh retained the old calendar handle")
	}
	s.Close()
	if held != 0 {
		t.Fatal("closed communications session retained reference accounting")
	}
}

func TestCommunicationsNotificationsRetainRawHistoryAndSeparateUndatedPrompt(t *testing.T) {
	const rawMessage = "來源原始訊息 <script>不可執行</script><a href=\"https://example.invalid\">原始連結文字</a>"
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.RawQuery != "" {
			t.Error("notification retrieval added an unobserved selector")
		}
		switch r.URL.Path {
		case "/api/ihke3000/ihke3114s01/page_load":
			row := map[string]any{"push_date": "115/10/28 23:30", "full_msg": rawMessage, "show_Url": true, "ID_NO": "PRIVATE_IDENTITY"}
			respondJSON(t, w, map[string]any{"sp_IHKE3114S0501_list": []any{row, row, map[string]any{"push_date": "", "full_msg": nil, "show_Url": false}}})
		case "/api/ihke3000/ihke3101s01/page_load":
			respondJSON(t, w, map[string]any{"push_msg": "與歷史清單不同的來源首頁欄位", "sP_IHKE3101S02": []any{map[string]any{"name": "PRIVATE_DEPENDENT"}}, "token": "PRIVATE_TOKEN"})
		default:
			t.Error("notification markup or source navigation was followed")
		}
	})
	report, err := s.FetchProvider(context.Background(), "notifications", Query{})
	if err != nil || len(report.Entries) != 4 || report.Complete || len(s.providerReferences["notifications"]) != 0 {
		t.Fatal("source notifications were collapsed or treated as complete", err)
	}
	if report.Entries[0].Key == report.Entries[1].Key || report.Entries[0].Date != "115/10/28 23:30" || report.Entries[2].Date != "" || report.Entries[3].Date != "" {
		t.Fatal("duplicate notification identity or absent source dates changed")
	}
	if values := personalFieldValues(report, "full_msg"); len(values) != 2 || values[0] != rawMessage || values[1] != rawMessage {
		t.Fatal("original notification text was rewritten or discarded")
	}
	if values := personalFieldValues(report, "push_msg"); len(values) != 1 || values[0] != "與歷史清單不同的來源首頁欄位" {
		t.Fatal("source homepage notification prompt was assumed identical to history")
	}
	for _, entry := range report.Entries {
		if entry.DetailID != "" || len(entry.Attachments) != 0 || len(entry.DocumentActions) != 0 {
			t.Fatal("notification content acquired an executable action")
		}
	}
	encoded, _ := json.Marshal(report)
	if strings.Contains(string(encoded), "PRIVATE_") {
		t.Fatal("unrelated homepage identity or token data escaped")
	}
}

func TestCommunicationsEmptyNullAndChangedSourceShapesRemainDistinct(t *testing.T) {
	for _, tc := range []struct {
		name, category, body string
		wantCode             string
	}{
		{"calendar_empty", "calendar_events", `{"RecordData":{},"RecordTypeData":[]}`, "section_empty"},
		{"calendar_null", "calendar_events", `{"RecordData":null,"RecordTypeData":[]}`, "section_null"},
		{"calendar_missing", "calendar_events", `{"RecordTypeData":[]}`, "section_missing"},
		{"calendar_bad_date", "calendar_events", `{"RecordData":{"not-a-date":{"data":[]}},"RecordTypeData":[]}`, "error"},
		{"calendar_changed_row", "calendar_events", `{"RecordData":{"2027-03-10":{"data":[{"unreviewed":"value"}]}},"RecordTypeData":[]}`, "error"},
		{"reminders_null", "stored_reminders", `{"IntimateReminder":null,"SelfDetermination":[],"slowUseMedicineList":[]}`, "section_null"},
		{"reminders_changed_row", "stored_reminders", `{"IntimateReminder":[{"SEQ_NO":"PRIVATE_ONLY"}],"SelfDetermination":[],"slowUseMedicineList":[]}`, "error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := providerFixture(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tc.body)
			})
			report, err := s.FetchProvider(context.Background(), tc.category, Query{})
			if tc.wantCode == "error" {
				if err == nil || len(s.providerReferences[tc.category]) != 0 || len(s.documentReferences) != 0 {
					t.Fatal("changed source shape committed a successful-looking result")
				}
				return
			}
			if err != nil || report.Complete {
				t.Fatal("known empty/null source shape failed or became a completeness guarantee", err)
			}
			found := false
			for _, diagnostic := range report.Diagnostics {
				found = found || diagnostic.Code == tc.wantCode
			}
			if !found {
				t.Fatal("source empty/null/missing distinction was lost")
			}
		})
	}
}
