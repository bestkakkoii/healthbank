// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func personalFieldValues(report SupplementReport, code string) []string {
	values := []string{}
	for _, entry := range report.Entries {
		for _, field := range entry.Fields {
			if field.Code == code {
				values = append(values, field.Value)
			}
		}
	}
	return values
}

func TestPersonalPreventiveArraysPreserveSourceWithoutDateInference(t *testing.T) {
	for _, tc := range []struct{ category, endpoint string }{{"prenatal", "IHKE3104S06"}, {"child_preventive", "IHKE3104S07"}} {
		t.Run(tc.category, func(t *testing.T) {
			s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil || len(body) != 0 || r.Method != http.MethodPost || r.URL.Path != "/api/ihke3000/ihke3104s01/"+tc.endpoint || r.URL.RawQuery != "" {
					t.Error("preventive read acquired invented date parameters or invoked a settings endpoint")
				}
				row := map[string]any{"ID": "PRIVATE_PREVENTIVE_ID", "Title": "合成第2次檢查：111/12/01", "HOSP_ABBR": "合成院所", "HOSP_URL": "https://PRIVATE_LINK.invalid", "CheckItem": "原始檢查文字 <b>未執行標記</b>", "ID_NO": "PRIVATE_IDENTITY"}
				respondJSON(t, w, []any{row, row})
			})
			report, err := s.FetchProvider(context.Background(), tc.category, Query{StartDate: "2026-01-01", EndDate: "2026-02-01"})
			if err != nil {
				t.Fatal(err)
			}
			if len(report.Entries) != 2 || report.Complete || report.StartDate != "" || report.EndDate != "" || report.Entries[0].Date != "" || report.Entries[0].Title != "合成第2次檢查：111/12/01" || report.Entries[0].Key == report.Entries[1].Key {
				t.Fatal("source rows/date labels/window were rewritten or deduplicated")
			}
			if got := personalFieldValues(report, "checkitem"); len(got) != 2 || got[0] != "原始檢查文字 <b>未執行標記</b>" {
				t.Fatal("source text was not retained as literal content")
			}
			encoded, _ := json.Marshal(report)
			if strings.Contains(string(encoded), "PRIVATE_") {
				t.Fatal("source identity, row selector or executable URL escaped the projection")
			}
		})
	}
}

func TestPersonalPreventiveRejectsUnknownAndNullResponses(t *testing.T) {
	for _, response := range []string{"null", "{}", `[null]`, `[{"unknown":"value"}]`, `[{"Title":true}]`} {
		t.Run(response, func(t *testing.T) {
			s := providerFixture(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, response)
			})
			if _, err := s.FetchProvider(context.Background(), "prenatal", Query{}); err == nil {
				t.Fatal("unrecognized/null response silently presented as a complete empty clinical array")
			}
		})
	}
}

func TestPersonalStoredAssessmentsNeverComputeOrConflateInputs(t *testing.T) {
	for _, tc := range []struct {
		category, path, raw, valueCode, expected string
	}{
		{"source_liver_risk", "/api/ihke3000/ihke3502s01/IHKE3502S01", `{"NoData":false,"AskUpdate":true,"Gender":"2","AgeRange":"3","SGPTALT":"12.00000000000001","HBeAg":"Indeterminate","TotalScore":"0","FutureCancerRate":"<1%","ShowPredictionArea":true,"SGPTALTDetail":{"Title":"ALT原始觀察","ASSAY_VALUE":"<0.000000000000001","REAL_INSPECT_DATE":"109/01/01","HOSP_ABBR":"合成院所","HOSP_URL":"PRIVATE_LINK","SRC_NAME":"院所上傳"},"HBeAgDetail":null,"ID_NO":"PRIVATE_IDENTITY"}`, "assay_value", "<0.000000000000001"},
		{"source_kidney_risk", "/api/ihke3000/ihke3503s01/IHKE3503S01", `{"InitPersonalData":{"EGFR":"59.000000000000000001","EGFR_EXAM_DATE":"110/02/03","EGFR_HOSP_ABBR":"合成院所甲","ACR":"<3","ACR_PCR_EXAM_DATE":"109/09/08","ACR_PCR_HOSP_ABBR":"合成院所乙","ACR_SRC_NAME":"來源原文","EGFR_HOSP_URL":"PRIVATE_LINK"},"LastComputeResultData":{"ComputeResultStatus":true,"ComputeResultTitle":"來源分級原文","ComputeResultDesc":"原文 <script>不得執行</script>","EGFR":"70","ACR":"0.0000","PCR":""},"ID_NO":"PRIVATE_IDENTITY"}`, "egfr", "59.000000000000000001"},
		{"source_cvd_risk", "/api/ihke3000/ihke3506s01/IHKE3506S01", `{"InitPersonalData":{"Gender":"F","AgeRange":"5","SBP":"120.0000","TC":"200.000000000001","SMOKER":"N","DIABETES":"N","SBP_EXAM_DATE":"109/01/01","TC_EXAM_DATE":"108/02/02","TC_SRC_NAME":"來源通道","TC_HOSP_URL":"PRIVATE_LINK"},"LastComputeResultData":{"ComputeResultTitle":"來源百分比原文","SEX":"M","AGE_RANGE":"4","SBP":"110","TC":"190","SMOKER":"Y","DIABETES":"N"},"ID_NO":"PRIVATE_IDENTITY"}`, "tc", "200.000000000001"},
	} {
		t.Run(tc.category, func(t *testing.T) {
			var calls atomic.Int32
			s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				body, err := io.ReadAll(r.Body)
				if err != nil || len(body) != 0 || r.Method != http.MethodPost || r.URL.Path != tc.path || r.URL.RawQuery != "" {
					t.Error("stored result read issued a new computation or caller-supplied input")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tc.raw)
			})
			report, err := s.FetchProvider(context.Background(), tc.category, Query{})
			if err != nil {
				t.Fatal(err)
			}
			values := personalFieldValues(report, tc.valueCode)
			if calls.Load() != 1 || len(values) == 0 || values[0] != tc.expected || report.Complete || report.StartDate != "" {
				t.Fatal("existing source raw numeric values/window were changed")
			}
			encoded, _ := json.Marshal(report)
			if strings.Contains(string(encoded), "PRIVATE_") {
				t.Fatal("source-private fields escaped")
			}
			if tc.category != "source_liver_risk" && (len(report.Entries) != 2 || report.Entries[0].Date != "" || report.Entries[1].Date != "" || len(values) != 2 || values[0] == values[1]) {
				t.Fatal("initial source measurements were merged into different stored calculation inputs or assigned an invented joint date")
			}
		})
	}
}

func TestPersonalLiverNoDataDoesNotEraseIndependentlyReturnedFields(t *testing.T) {
	s := providerFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(t, w, map[string]any{"NoData": true, "GenderScore": 0, "TotalScore": 0, "FutureCancerRate": "來源未驗證回傳欄位", "ShowPredictionArea": false, "SGPTALTDetail": map[string]any{"Title": "合成檢驗", "ASSAY_VALUE": "<2"}, "HBeAgDetail": nil})
	})
	report, err := s.FetchProvider(context.Background(), "source_liver_risk", Query{})
	if err != nil {
		t.Fatal(err)
	}
	if values := personalFieldValues(report, "totalscore"); len(values) != 1 || values[0] != "0" {
		t.Fatal("source NoData incorrectly erased a separately returned raw score")
	}
	if len(personalFieldValues(report, "futurecancerrate")) != 1 || len(personalFieldValues(report, "assay_value")) != 1 || !strings.Contains(report.Coverage, "來源預設值") {
		t.Fatal("source raw fields were lost or falsely represented as validated clinical results")
	}
	if values := personalFieldValues(report, "nodata"); len(values) != 1 || values[0] != "true" {
		t.Fatal("source state boolean was not preserved literally")
	}
}

func TestPersonalAssessmentEmptyNestedObjectsRemainExplicit(t *testing.T) {
	s := providerFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(t, w, map[string]any{"InitPersonalData": map[string]any{"EGFR": "", "ACR": nil, "PCR": ""}, "LastComputeResultData": nil})
	})
	report, err := s.FetchProvider(context.Background(), "source_kidney_risk", Query{})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Entries) != 0 || len(report.Diagnostics) != 2 || report.Complete {
		t.Fatal("empty source values/null result were lost or replaced with normal values")
	}
}

func TestPersonalLifestyleInstitutionAndSelfAnswerIndicesRemainSeparate(t *testing.T) {
	var answerCalls atomic.Int32
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if r.Method == http.MethodPost && json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Error("missing source read payload")
		}
		if r.Method == http.MethodPost && body["C_GRP_QUES_CODE"] != "SOURCE_GROUP" {
			t.Error("questionnaire selector was not bound to initial source response")
		}
		switch r.URL.Path {
		case "/api/ihke3000/IHKE3713S01/SP_IHKE3713S01":
			respondJSON(t, w, map[string]any{"codE_CNAME": "SOURCE_GROUP"})
		case "/api/ihke3000/IHKE3713S01/SP_IHKE3713S08":
			respondJSON(t, w, map[string]any{"field_type": []any{map[string]any{"code": "P01", "text": "來源領域"}}})
		case "/api/ihke3000/IHKE3713S01/SP_IHKE3713S07_V02":
			if body["C_LAST_N"] == float64(1) {
				respondJSON(t, w, map[string]any{"date": "115/03/03", "hid": "PRIVATE_HOSP_ID", "hnm": "合成院所", "score_list": []any{map[string]any{"field_name": "P01", "score": "0.00000000000001"}}})
			} else if body["C_LAST_N"] == float64(2) {
				respondJSON(t, w, map[string]any{"date": "115/02/02", "hid": "", "hnm": "", "score_list": []any{map[string]any{"field_name": "P01", "score": 0}}})
			} else {
				t.Error("invented older history index")
			}
		case "/api/ihke3000/IHKE3713S01/SP_IHKE3713S03":
			respondJSON(t, w, map[string]any{"list1": []any{map[string]any{"ques": map[string]any{"no": "Q1", "text": "合成題目", "scoreText": "來源分數說明"}, "ans": []any{map[string]any{"no": "A0", "text": "合成原始選項"}}}}, "list2": []any{}})
		case "/api/ihke3000/IHKE3713S01/SP_IHKE3713S06":
			answerCalls.Add(1)
			if body["C_LAST_N"] != float64(1) {
				t.Error("second mixed-source score was incorrectly mapped to self-answer index2")
			}
			row := map[string]any{"ques": "Q1", "ans": "A0", "ID_NO": "PRIVATE_ANSWER_ID"}
			respondJSON(t, w, map[string]any{"date": "115/02/01", "ans_list": []any{row, row, map[string]any{"ques": "Q_NEW", "ans": "RAW_UNKNOWN_OPTION"}}})
		default:
			t.Error("unexpected read, questionnaire submission or new calculation")
		}
	})
	report, err := s.FetchProvider(context.Background(), "lifestyle_assessment", Query{})
	if err != nil {
		t.Fatal(err)
	}
	if answerCalls.Load() != 1 || len(report.Entries) != 8 || report.Complete || report.Entries[0].Origin != "institutional" || report.Entries[2].Origin != "self_entered" || report.Entries[5].Date != "115/02/01" || report.Entries[5].Key == report.Entries[6].Key {
		t.Fatal("mixed institution/self history, raw-answer date or duplicates were conflated")
	}
	if values := personalFieldValues(report, "score"); len(values) != 2 || values[0] != "0.00000000000001" || values[1] != "0" {
		t.Fatal("source score zero/precision was lost or recomputed")
	}
	if values := personalFieldValues(report, "ans"); len(values) != 3 || values[2] != "RAW_UNKNOWN_OPTION" {
		t.Fatal("unknown original answer was discarded")
	}
	if values := personalFieldValues(report, "answer_text"); len(values) != 2 || values[0] != "合成原始選項" {
		t.Fatal("known original answer label was not source-bound")
	}
	encoded, _ := json.Marshal(report)
	if strings.Contains(string(encoded), "PRIVATE_") {
		t.Fatal("private source selectors escaped")
	}
}

func TestPersonalLifestyleNoDataDoesNotSubmitOrInventAnswers(t *testing.T) {
	var calls atomic.Int32
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch r.URL.Path {
		case "/api/ihke3000/IHKE3713S01/SP_IHKE3713S01":
			respondJSON(t, w, map[string]any{"codE_CNAME": "SOURCE_GROUP"})
		case "/api/ihke3000/IHKE3713S01/SP_IHKE3713S08":
			respondJSON(t, w, map[string]any{"field_type": []any{}})
		case "/api/ihke3000/IHKE3713S01/SP_IHKE3713S07_V02":
			respondJSON(t, w, "NoData")
		default:
			t.Error("empty source history caused an invented answer read or submission")
		}
	})
	report, err := s.FetchProvider(context.Background(), "lifestyle_assessment", Query{})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 4 || len(report.Entries) != 0 || len(report.Diagnostics) != 2 || report.Complete {
		t.Fatal("NoData sentinel was not represented as a bounded source result")
	}
}

func TestPersonalLifestyleEmptyAnswersKeepTheirOwnDate(t *testing.T) {
	for _, tc := range []struct{ raw, diagnostic string }{
		{`{"date":"115/01/02","ans_list":[]}`, "section_empty"},
		{`{"date":"115/01/02","ans_list":null}`, "section_null"},
	} {
		object, err := providerObject([]byte(tc.raw))
		if err != nil {
			t.Fatal(err)
		}
		report := SupplementReport{Category: "lifestyle_assessment", Entries: []SupplementEntry{}}
		section := personalProviderRequests("lifestyle_assessment")[4].sections[0]
		if err := personalLifestyleAnswers(&report, 1, object, section, map[string]personalQuestion{}); err != nil {
			t.Fatal(err)
		}
		if len(report.Entries) != 1 || report.Entries[0].Date != "115/01/02" || report.Entries[0].DateRole != "recorded" || report.Entries[0].Origin != "self_entered" || len(report.Diagnostics) != 1 || report.Diagnostics[0].Code != tc.diagnostic {
			t.Fatal("empty/null answers discarded their own source date or invented answered questions")
		}
	}
}

func TestPersonalChildScheduleBindsSelectedChildAndStoredNotification(t *testing.T) {
	var detailCalls atomic.Int32
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/ihke3000/ihke3703s01/SP_IHKE3703S01":
			respondJSON(t, w, map[string]any{"sP_IHKE3703S01": []any{map[string]any{"roW_ID": "PRIVATE_CHILD_A", "chilD_NAME": "合成兒童甲", "chilD_BIRTH": "112/01/01"}, map[string]any{"roW_ID": "PRIVATE_CHILD_B", "chilD_NAME": "合成兒童乙", "chilD_BIRTH": "113/01/01"}}})
		case "/api/ihke3000/ihke3703s04/SP_IHKE3703S04":
			detailCalls.Add(1)
			var body map[string]string
			if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&body) != nil || len(body) != 1 || body["c_rowid"] != "PRIVATE_CHILD_A" {
				t.Error("child detail was not bound to the selected source reference")
			}
			respondJSON(t, w, map[string]any{"sP_IHKE3703S04": []any{map[string]any{"chilD_NAME": "合成兒童甲", "chilD_BIRTH": "112/01/01", "suggesT_DATE_S": "112/01/01", "suggesT_DATE_E": "112/02/01", "p1_CODE_CNAME": "合成疫苗", "p1_CODE_ENAME": "來源建議年齡", "pusH_SEQ_NO": "PRIVATE_NOTIFICATION_A", "pusH_DATE": "112/01/20"}}})
		case "/api/ihke3000/ihke3703s04/SP_IHKE3104S52":
			var body map[string]string
			if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&body) != nil || len(body) != 1 || body["c_seq_no"] != "PRIVATE_NOTIFICATION_A" {
				t.Error("notification selector was not bound to returned source schedule")
			}
			respondJSON(t, w, map[string]any{"notifyDate": "112/01/20", "notifyMessage": "來源已存提醒 <b>文字</b>", "ID_NO": "PRIVATE_IDENTITY"})
		default:
			t.Error("child read attempted a write or undocumented endpoint")
		}
	})
	list, err := s.FetchProvider(context.Background(), "child_schedule", Query{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Entries) != 2 || len(list.Entries[0].DetailID) != 48 || list.Entries[0].DetailID == list.Entries[1].DetailID || list.Entries[0].Date != "" {
		t.Fatal("child profiles lacked distinct opaque selectors or DOB became a clinical date")
	}
	reference := list.Entries[0].DetailID
	if _, err := s.FetchProviderDetail(context.Background(), "health_coin_clinical", reference); err == nil || detailCalls.Load() != 0 {
		t.Fatal("child selector accepted by a different category")
	}
	report, err := s.FetchProviderDetail(context.Background(), "child_schedule", reference)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Entries) != 2 || report.Entries[0].Date != "" || report.Complete || !strings.Contains(report.Entries[0].Title, "非已接種") {
		t.Fatal("recommended source vaccination window became an observed injection")
	}
	if got := personalFieldValues(report, "notifymessage"); len(got) != 1 || got[0] != "來源已存提醒 <b>文字</b>" {
		t.Fatal("existing notification content was omitted or rewritten")
	}
	for _, report := range []SupplementReport{list, report} {
		encoded, _ := json.Marshal(report)
		if strings.Contains(string(encoded), "PRIVATE_") {
			t.Fatal("private child/reminder selectors escaped")
		}
	}
	if _, err := s.FetchProvider(context.Background(), "child_schedule", Query{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FetchProviderDetail(context.Background(), "child_schedule", reference); err == nil || detailCalls.Load() != 1 {
		t.Fatal("retired child source selector remained usable")
	}
}

func TestPersonalHealthCoinRequiresExistingConsent(t *testing.T) {
	var calls atomic.Int32
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/api/ihke3000/ihke3714s06/pageload" || r.URL.Query().Get("cCat") != "A" || len(r.URL.Query()) != 1 {
			t.Error("acquisition attempted consent, reward claim or a non-source category")
		}
		respondJSON(t, w, map[string]any{"agreePoint": false})
	})
	report, err := s.FetchProvider(context.Background(), "health_coin_clinical", Query{})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || len(report.Entries) != 1 || report.Entries[0].DetailID != "" || report.Complete {
		t.Fatal("absent source consent was bypassed or replaced with clinical zero records")
	}
	if values := personalFieldValues(report, "agreepoint"); len(values) != 1 || values[0] != "false" {
		t.Fatal("original eligibility status was lost")
	}
}

func TestPersonalChildCalendarRequiresStoredReminderDate(t *testing.T) {
	for _, tc := range []struct {
		name    string
		date    any
		present bool
		ready   bool
	}{
		{"absent", nil, false, false},
		{"null", nil, true, false},
		{"empty", "", true, false},
		{"stored", "112/01/02", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/ihke3000/ihke3703s01/SP_IHKE3703S01":
					respondJSON(t, w, map[string]any{"sP_IHKE3703S01": []any{map[string]any{"roW_ID": "PRIVATE_CHILD", "chilD_NAME": "合成兒童"}}})
				case "/api/ihke3000/ihke3703s04/SP_IHKE3703S04":
					row := map[string]any{"p1_CODE_CNAME": "合成疫苗建議", "pusH_SEQ_NO": "PRIVATE_NOTICE"}
					if tc.present {
						row["pusH_DATE"] = tc.date
					}
					respondJSON(t, w, map[string]any{"sP_IHKE3703S04": []any{row}})
				case "/api/ihke3000/ihke3703s04/SP_IHKE3104S52":
					respondJSON(t, w, map[string]any{"notifyDate": "", "notifyMessage": "合成來源文字"})
				default:
					t.Error("calendar readiness check attempted an implicit download or mutation")
					http.NotFound(w, r)
				}
			})
			list, err := s.FetchProvider(context.Background(), "child_schedule", Query{})
			if err != nil || len(list.Entries) != 1 {
				t.Fatal("source child unavailable", err)
			}
			report, err := s.FetchProviderDetail(context.Background(), "child_schedule", list.Entries[0].DetailID)
			if err != nil || len(report.Entries) != 2 {
				t.Fatal("calendar gate discarded the original schedule or source notification", err)
			}
			want := 0
			if tc.ready {
				want = 1
			}
			if len(report.Entries[0].Attachments) != want || len(s.documentReferences) != want || len(report.Entries[0].DocumentActions) != 0 {
				t.Fatal("calendar capability did not follow the source stored reminder date")
			}
		})
	}
}

func TestPersonalChildCalendarsRefreshOnlyTheirSelectedParent(t *testing.T) {
	var downloads atomic.Int32
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/ihke3000/ihke3703s01/SP_IHKE3703S01":
			respondJSON(t, w, map[string]any{"sP_IHKE3703S01": []any{map[string]any{"roW_ID": "PRIVATE_A", "chilD_NAME": "合成兒童甲"}, map[string]any{"roW_ID": "PRIVATE_B", "chilD_NAME": "合成兒童乙"}}})
		case "/api/ihke3000/ihke3703s04/SP_IHKE3703S04":
			var body map[string]string
			if json.NewDecoder(r.Body).Decode(&body) != nil {
				t.Fatal("invalid child selector")
			}
			if body["c_rowid"] != "PRIVATE_A" && body["c_rowid"] != "PRIVATE_B" {
				t.Fatal("unbound child selector")
			}
			respondJSON(t, w, map[string]any{"sP_IHKE3703S04": []any{map[string]any{"p1_CODE_CNAME": "合成疫苗建議", "suggesT_DATE_S": "112/01/01", "pusH_SEQ_NO": body["c_rowid"] + "_NOTICE", "pusH_DATE": "112/01/02"}}})
		case "/api/ihke3000/ihke3703s04/SP_IHKE3104S52":
			respondJSON(t, w, map[string]any{"notifyDate": "112/01/02", "notifyMessage": "合成既有提醒"})
		case "/api/ihke3000/ihke3104s01/IHKE3104S08":
			downloads.Add(1)
			sequence := r.URL.Query().Get("SEQ_NO")
			if r.Method != http.MethodGet || len(r.URL.Query()) != 1 || (sequence != "PRIVATE_A_NOTICE" && sequence != "PRIVATE_B_NOTICE") {
				t.Error("ICS download escaped the stored schedule selector")
			}
			w.Header().Set("Content-Type", "text/calendar;charset=UTF-8")
			_, _ = io.WriteString(w, "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:synthetic-only\r\nSUMMARY:合成既有通知\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")
		default:
			t.Error("unexpected mutation or external document endpoint")
		}
	})
	list, err := s.FetchProvider(context.Background(), "child_schedule", Query{})
	if err != nil {
		t.Fatal(err)
	}
	parentKeys := map[string]string{}
	readDetail := func(reference string) string {
		t.Helper()
		report, err := s.FetchProviderDetail(context.Background(), "child_schedule", reference)
		if err != nil || len(report.Entries) == 0 || len(report.Entries[0].Attachments) != 1 {
			t.Fatalf("missing source child reminder artifact: %v", err)
		}
		for otherReference, key := range parentKeys {
			if otherReference != reference && key == report.Entries[0].Key {
				t.Fatal("different children received the same detail record key")
			}
		}
		parentKeys[reference] = report.Entries[0].Key
		return report.Entries[0].Attachments[0].ID
	}
	first := readDetail(list.Entries[0].DetailID)
	second := readDetail(list.Entries[1].DetailID)
	consume := func(info DocumentInfo, reader io.Reader) error {
		content, err := io.ReadAll(reader)
		if err != nil || info.ContentType != "text/calendar;charset=utf-8" || !strings.Contains(string(content), "SUMMARY:合成既有通知") {
			t.Error("original UTF-8 reminder artifact was not retained")
		}
		return err
	}
	if err := s.DownloadDocument(context.Background(), "child_schedule", first, consume); err != nil {
		t.Fatal("second child detail retired first child artifact", err)
	}
	replacement := readDetail(list.Entries[0].DetailID)
	if replacement == first {
		t.Fatal("child detail refresh reused an old document token")
	}
	before := downloads.Load()
	if err := s.DownloadDocument(context.Background(), "child_schedule", first, consume); err == nil || downloads.Load() != before {
		t.Fatal("retired same-child attachment remained downloadable")
	}
	if err := s.DownloadDocument(context.Background(), "child_schedule", second, consume); err != nil {
		t.Fatal("refreshing first child retired the second child artifact", err)
	}
	if err := s.DownloadDocument(context.Background(), "child_schedule", replacement, consume); err != nil {
		t.Fatal(err)
	}
}

func TestPersonalHealthCoinPreservesClinicalRowsAndLegacySourceText(t *testing.T) {
	var detailCalls atomic.Int32
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Error("health-coin read attempted a mutation")
		}
		switch r.URL.Path {
		case "/api/ihke3000/ihke3714s06/pageload":
			code := r.URL.Query().Get("cCat")
			if (code != "A" && code != "B" && code != "C") || len(r.URL.Query()) != 1 {
				t.Error("invented source category")
			}
			respondJSON(t, w, map[string]any{"agreePoint": true, "pointitemList": []any{map[string]any{"pointitemCode": code + "001", "pointitemName": "合成來源項目"}}})
		case "/api/ihke3000/ihke3714s08/pageload":
			detailCalls.Add(1)
			if len(r.URL.Query()) != 1 {
				t.Error("unsupported detail parameter")
			}
			switch r.URL.Query().Get("pCode") {
			case "A001":
				row := map[string]any{"checkDate": "110/03/04", "hospName": "合成院所", "bloodPressResult": "0", "bloodPressResultDisp": "原始來源結果", "hospUrl": "PRIVATE_URL", "ID_NO": "PRIVATE_IDENTITY"}
				respondJSON(t, w, map[string]any{"agreePoint": true, "pointList": []any{row, row}, "pointValue": "REWARD_MUST_NOT_APPEAR"})
			case "B001":
				respondJSON(t, w, map[string]any{"agreePoint": true, "inoculationDate": "110/01/01", "vaccineName": "合成來源疫苗", "detaildesc": []any{map[string]any{"text": "原始完整來源說明"}}, "adviceList": []any{map[string]any{"codeCname": "來源原文 <script>不得執行</script>"}}, "hospUrl": "PRIVATE_URL"})
			case "C001":
				respondJSON(t, w, map[string]any{"agreePoint": true, "pointList": []any{map[string]any{"createDate": "110/01/02", "sourceNameDisp": "自行記錄", "scale1": 0, "scaleTotal": "0.0000000000001", "scaleDesc": "來源既有說明"}}})
			default:
				t.Error("unbound source detail selector")
			}
		case "/api/ihke3000/ihke3714s07/pageload":
			if r.URL.Query().Get("pCode") != "C001" || len(r.URL.Query()) != 1 {
				t.Error("questionnaire completion date read used an unbound source selector")
			}
			respondJSON(t, w, map[string]any{"agreePoint": true, "lifestyleDate": "110/01/03", "coinAmount": "REWARD_MUST_NOT_APPEAR"})
		default:
			t.Error("health-coin read attempted enrollment, reward claim or an external transfer")
		}
	})
	list, err := s.FetchProvider(context.Background(), "health_coin_clinical", Query{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Entries) != 3 || list.Entries[0].Kind != "source_index" || len(list.Entries[0].DetailID) != 48 {
		t.Fatal("clinical category source indices not retained")
	}
	for index := range list.Entries {
		report, err := s.FetchProviderDetail(context.Background(), "health_coin_clinical", list.Entries[index].DetailID)
		if err != nil {
			t.Fatal(err)
		}
		if report.Complete || report.StartDate != "" {
			t.Fatal("source-only clinical rows falsely declared lifetime complete")
		}
		encoded, _ := json.Marshal(report)
		if strings.Contains(string(encoded), "PRIVATE_") || strings.Contains(string(encoded), "REWARD_MUST_NOT_APPEAR") {
			t.Fatal("identity, remote links or reward-only fields escaped")
		}
		switch index {
		case 0:
			if len(report.Entries) != 2 || report.Entries[0].Date != "110/03/04" || report.Entries[0].Key == report.Entries[1].Key || len(personalFieldValues(report, "bloodpressresultdisp")) != 2 {
				t.Fatal("source checkup details or same-date duplicate rows were lost")
			}
		case 1:
			if len(report.Entries) != 3 || len(personalFieldValues(report, "text")) != 1 || len(personalFieldValues(report, "codecname")) != 1 {
				t.Fatal("legacy source clinical object or unfiltered source explanations were lost")
			}
		case 2:
			if values := personalFieldValues(report, "scale1"); len(values) != 1 || values[0] != "0" {
				t.Fatal("original zero score was replaced with empty text")
			}
			if values := personalFieldValues(report, "lifestyledate"); len(values) != 1 || values[0] != "110/01/03" || report.Entries[0].Date != "110/01/02" {
				t.Fatal("separate completion metadata was omitted or conflated with an assessment date")
			}
		}
	}
	if detailCalls.Load() != 3 {
		t.Fatal("unexpected unbound detail read")
	}
}

func TestPersonalHealthCoinKeepsCompletedDateBeforeResultAvailability(t *testing.T) {
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Error("existing questionnaire metadata read attempted a write")
		}
		switch r.URL.Path {
		case "/api/ihke3000/ihke3714s06/pageload":
			rows := []any{}
			if r.URL.Query().Get("cCat") == "C" {
				rows = append(rows, map[string]any{"pointitemCode": "C001", "pointitemName": "合成生活型態來源"})
			}
			respondJSON(t, w, map[string]any{"agreePoint": true, "pointitemList": rows})
		case "/api/ihke3000/ihke3714s08/pageload":
			respondJSON(t, w, map[string]any{"agreePoint": true, "pointitemName": "合成生活型態來源"})
		case "/api/ihke3000/ihke3714s07/pageload":
			if r.URL.Query().Get("pCode") != "C001" {
				t.Error("completion-date selector not bound")
			}
			respondJSON(t, w, map[string]any{"agreePoint": true, "lifestyleDate": "115/10/02", "gotCoin": false})
		default:
			t.Error("unexpected task submission or crediting operation")
		}
	})
	list, err := s.FetchProvider(context.Background(), "health_coin_clinical", Query{})
	if err != nil || len(list.Entries) != 1 {
		t.Fatalf("source list: %v", err)
	}
	report, err := s.FetchProviderDetail(context.Background(), "health_coin_clinical", list.Entries[0].DetailID)
	if err != nil {
		t.Fatal(err)
	}
	if values := personalFieldValues(report, "lifestyledate"); len(values) != 1 || values[0] != "115/10/02" || len(report.Entries) != 1 {
		t.Fatal("source completed questionnaire date was discarded while results were unavailable")
	}
	if len(personalFieldValues(report, "scaletotal")) != 0 {
		t.Fatal("completion date fabricated a questionnaire result")
	}
}

func TestPersonalCampaignDoesNotExportRegistrationIdentityOrRelabelGlucose(t *testing.T) {
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.RawQuery != "" {
			t.Error("campaign snapshot acquisition attempted registration or changed the source date window")
		}
		switch r.URL.Path {
		case "/api/ihke3000/IHKE3799S01/page_load":
			respondJSON(t, w, map[string]any{"c_NAME": "PRIVATE_NAME", "c_MOBILE": "PRIVATE_PHONE", "c_ADDRESS": "PRIVATE_ADDRESS", "c_WAISTLINE": "82.0000000001", "c_WAISTLINE_D": "110/01/01", "c_WAISTLINE_S": "F", "c_WAISTLINE_R": false, "c_HbA1c": "101", "c_HbA1c_D": "109/03/02", "c_HbA1c_S": "B", "c_HbA1c_R": true})
		case "/api/ihke3000/ihke3205s01/SP_IHKE3205S01":
			respondJSON(t, w, map[string]any{
				"sP_IHKE3205S01":       []any{map[string]any{"code": "105", "value": "120.00000", "uniT_DATA": "mmHg", "abnormal_Status": "N"}},
				"sP_IHKE3205S07":       []any{map[string]any{"code": "104", "codE_NAME": "合成腰圍", "creatE_DATE": "110/01/02", "value": "82.0000000001", "uniT_DATA": "cm"}},
				"sP_IHKE3205S04":       []any{map[string]any{"code": "107", "codE_NAME": "合成血糖", "creatE_DATE": "109/03/03", "value": "101", "uniT_DATA": "mg/dL"}},
				"data_112":             []any{map[string]any{"code": "112", "codE_NAME": "合成三酸甘油脂", "creatE_DATE": "108/02/03", "value": "<0.01", "uniT_DATA": "mg/dL"}},
				"data_113":             []any{map[string]any{"code": "113", "codE_NAME": "合成高密度脂蛋白", "creatE_DATE": "107/02/03", "value": "0.0000", "uniT_DATA": "mg/dL"}},
				"blood_Pressurep_Date": "106/01/02",
			})
		default:
			t.Error("unexpected campaign endpoint")
		}
	})
	report, err := s.FetchProvider(context.Background(), "campaign_measurements", Query{})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Entries) != 7 || report.Entries[0].Date != "" || report.Complete {
		t.Fatal("different measurement dates were collapsed or source scope was overstated")
	}
	if report.Entries[2].Date != "110/01/02" || report.Entries[2].DateRole != "recorded" || report.Entries[2].Title != "合成腰圍" || report.Entries[6].Date != "" {
		t.Fatal("source update dates lost or assigned to unrelated examinations")
	}
	keys := map[string]bool{}
	for _, entry := range report.Entries {
		if keys[entry.Key] {
			t.Fatal("independent root snapshots have colliding source keys")
		}
		keys[entry.Key] = true
	}
	if values := personalFieldValues(report, "blood_pressurep_date"); len(values) != 1 || values[0] != "106/01/02" {
		t.Fatal("source blood-pressure update metadata omitted")
	}
	if values := personalFieldValues(report, "c_hba1c"); len(values) != 1 || values[0] != "101" {
		t.Fatal("original misleading source field identity/value was lost")
	}
	for _, entry := range report.Entries {
		for _, field := range entry.Fields {
			if field.Code == "c_hba1c" && (field.Unit != "" || !strings.Contains(field.Name, "c_HbA1c")) {
				t.Fatal("campaign fasting-glucose display field was falsely standardized as HbA1c")
			}
		}
	}
	encoded, _ := json.Marshal(report)
	if strings.Contains(string(encoded), "PRIVATE_") {
		t.Fatal("campaign registration identity escaped")
	}
}
