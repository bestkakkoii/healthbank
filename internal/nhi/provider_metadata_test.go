// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func fieldValues(entries []SupplementEntry, code string) []string {
	values := []string{}
	for _, entry := range entries {
		for _, field := range entry.Fields {
			if field.Code == code {
				values = append(values, field.Value)
			}
		}
	}
	return values
}

func TestProviderSourceNotesAndPeriodsRemainSeparateMetadata(t *testing.T) {
	for _, tc := range []struct {
		category, code string
		object         map[string]any
		want           []string
	}{
		{"family_doctor", "memo", map[string]any{"sP_IHKE3720S01": map[string]any{"status": "11"}, "sP_IHKE3720S02": map[string]any{"hosp_ABBR": "合成院所", "memo": []any{"<b>來源原文</b>", "重複說明", "重複說明"}}, "memo": "來源未收案說明"}, []string{"<b>來源原文</b>", "重複說明", "重複說明", "來源未收案說明"}},
		{"osteoporosis", "c_sdate", map[string]any{"sp_IHKE3411S01_data": []any{}, "c_sdate": "合成來源起日", "c_edate": "合成來源迄日"}, []string{"合成來源起日"}},
		{"covid", "now_tw", map[string]any{"sp_IHKE3410S01": []any{}, "sp_IHKE3410S06": []any{}, "reload": false, "now_tw": "合成查詢日期", "now_en": "Synthetic query date"}, []string{"合成查詢日期"}},
		{"lipids", "sdate", map[string]any{"RecordList": []any{}, "Remark_Date": []any{map[string]any{"sdate": "合成來源起日", "edate": "合成來源迄日"}, map[string]any{"sdate": "合成來源起日", "edate": "合成來源迄日"}}}, []string{"合成來源起日", "合成來源起日"}},
		{"physiology", "blood_pressurep_date", map[string]any{"maindata": []any{}, "maindata_10568": []any{}, "blood_Pressurep_Date": "合成摘要更新時間"}, []string{"合成摘要更新時間"}},
	} {
		t.Run(tc.category, func(t *testing.T) {
			s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/ihke3000/ihke3407s01/getyears" {
					respondJSON(t, w, map[string]any{"years": []any{}})
					return
				}
				if r.URL.Path == "/api/ihke3000/ihke3411s01/search" {
					respondJSON(t, w, []any{})
					return
				}
				if strings.HasSuffix(r.URL.Path, "/SQL100") {
					t.Error("noneligible family status triggered directory lookup")
				}
				respondJSON(t, w, tc.object)
			})
			report, err := s.FetchProvider(context.Background(), tc.category, Query{})
			if err != nil {
				t.Fatal(err)
			}
			values := fieldValues(report.Entries, tc.code)
			if strings.Join(values, "\x00") != strings.Join(tc.want, "\x00") {
				t.Fatalf("source notes, duplicate periods or query labels lost: %#v", values)
			}
			for _, entry := range report.Entries {
				if entry.Date != "" || entry.EndDate != "" || entry.DateRole != "" || entry.EndDateRole != "" {
					t.Fatal("source availability/query metadata became a clinical date")
				}
			}
		})
	}
}

func TestProviderGlucoseRetainsIndependentlyReturnedHomepageRows(t *testing.T) {
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/ihke3000/ihke3406s01/SP_IHKE3406S01":
			row := map[string]any{"real_INSPECT_DATE": "103/01/01", "assay_VALUE": "6.000000000000000001", "hosp_ABBR": "合成院所"}
			respondJSON(t, w, map[string]any{"sP_IHKE3406S01_Data": []any{row, row}, "remark_SDate": "來源首頁起日", "remark_EDate": "來源首頁迄日"})
		case "/api/ihke3000/ihke3406s02/SP_IHKE3406S02":
			var payload map[string]string
			if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&payload) != nil || payload["dStr"] != "2018-01-01" || payload["dEnd"] != "2019-12-31" {
				t.Error("homepage metadata overwrote explicit search dates")
			}
			respondJSON(t, w, map[string]any{"sP_IHKE3406S02_Data": []any{map[string]any{"real_INSPECT_DATE": "108/01/01", "assay_VALUE": "5.000000000000000001"}}})
		default:
			t.Error("unexpected read")
		}
	})
	report, err := s.FetchProvider(context.Background(), "glucose", Query{StartDate: "2018-01-01", EndDate: "2019-12-31"})
	if err != nil || len(report.Entries) != 4 || report.Complete {
		t.Fatalf("independent homepage table was discarded: %v", err)
	}
	if report.Entries[0].Key == report.Entries[1].Key || report.Entries[0].ContractID == report.Entries[3].ContractID || report.Entries[2].Kind != "source_metadata" || report.Entries[2].Date != "" {
		t.Fatal("duplicate multiplicity or source query provenance collapsed")
	}
	values := fieldValues(report.Entries, "assay_value")
	if len(values) != 3 || values[0] != "6.000000000000000001" || values[1] != values[0] || values[2] != "5.000000000000000001" {
		t.Fatalf("exact result values changed: %#v", values)
	}
}

func TestProviderClinicalSourceProvenanceAndFlagsRetained(t *testing.T) {
	t.Run("self entered cochlear", func(t *testing.T) {
		s := providerFixture(t, func(w http.ResponseWriter, _ *http.Request) {
			respondJSON(t, w, map[string]any{"maindata01": []any{}, "maindata02": []any{}, "maindata03": []any{map[string]any{"c_exe_S_DATE": "104/02/03", "c_ear_MARK": "來源左右耳碼"}}})
		})
		report, err := s.FetchProvider(context.Background(), "cochlear", Query{})
		if err != nil || len(report.Entries) != 1 || report.Entries[0].Date != "104/02/03" || report.Entries[0].Origin != "self_entered" || len(fieldValues(report.Entries, "c_ear_mark")) != 1 {
			t.Fatalf("self-entered surgery date/laterality lost: %v", err)
		}
	})
	t.Run("allergy source", func(t *testing.T) {
		s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
			root := "sP_IHKE3202S01"
			if strings.HasSuffix(r.URL.Path, "S04") {
				root = "sP_IHKE3202S04"
			}
			respondJSON(t, w, map[string]any{root: []any{map[string]any{"drug_NAME": "合成過敏內容", "source": "原始登錄來源"}}})
		})
		report, err := s.FetchProvider(context.Background(), "allergies", Query{})
		if err != nil || len(fieldValues(report.Entries, "source")) != 2 || report.Entries[0].Origin == report.Entries[1].Origin {
			t.Fatalf("allergy source/provenance lost: %v", err)
		}
	})
	t.Run("screening authoritative booleans", func(t *testing.T) {
		for _, flag := range []any{false, true, "true", nil} {
			s := providerFixture(t, func(w http.ResponseWriter, _ *http.Request) {
				respondJSON(t, w, map[string]any{"sP_IHKE3404S04_Data": []any{map[string]any{"func_DATE": "104/02/03", "code_CNAME": "來源結果原文", "code_ENAME": "SOURCE_CODE"}}, "hasSaveData": flag, "hasWarningData": false, "hasInvalidData": true})
			})
			report, err := s.FetchProvider(context.Background(), "screen_breast", Query{})
			if err != nil || len(report.Entries) != 2 || report.Entries[1].Date != "" || fieldValues(report.Entries, "haswarningdata")[0] != "false" || fieldValues(report.Entries, "hasinvaliddata")[0] != "true" {
				t.Fatalf("returned flags were recomputed or became observations: %v", err)
			}
			_, boolean := flag.(bool)
			if (len(fieldValues(report.Entries, "hassavedata")) == 1) != boolean {
				t.Fatal("nonboolean source hint treated as authoritative flag")
			}
		}
	})
}

func TestProviderPrescriptionIncompleteRefillOccurrenceIsNotDropped(t *testing.T) {
	s := providerFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(t, w, map[string]any{"main_data": []any{map[string]any{"func_DATE": "104/02/03", "prsn_NAME": "合成處方醫師", "refillList": []any{map[string]any{"recipe_TIMES": "02", "rel_DATE_STR": "", "recipe_MEMO": ""}, map[string]any{"recipe_TIMES": "02", "rel_DATE_STR": nil, "recipe_MEMO": nil}}}}, "hist_list": []any{}})
	})
	report, err := s.FetchProvider(context.Background(), "e_prescriptions", Query{})
	if err != nil || len(report.Entries) != 1 || len(fieldValues(report.Entries, "prsn_name")) != 1 {
		t.Fatalf("physician or incomplete refill omitted: %v", err)
	}
	values := fieldValues(report.Entries, "recipe_times")
	if len(values) != 2 || values[0] != "02" || values[1] != "02" || report.Entries[0].Kind == "dispensing" {
		t.Fatal("refill occurrences collapsed or inferred as completed dispensing")
	}
}

func TestProviderAdultAndKinmenRawInterpretationCodes(t *testing.T) {
	codes := []string{"bloD_PRESS_RESULT", "bA_DIAG_RESULT", "s_09005C_DIAG_RESULT", "rF_DIAG_RESULT", "urinE_UA_DIAG_RESULT", "metA_SYNDR_RESULT", "lF_DIAG_RESULT", "hbsag", "hbV_RESULT", "antI_HCV", "hcV_RESULT"}
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		row := map[string]any{"firsT_DIAG_DATE": "104/02/03", "tel": "合成院所電話"}
		for _, code := range codes {
			row[code] = "009"
		}
		if r.URL.Path == "/api/ihke3000/ihke3402s01/SP_IHKE3402S01" {
			respondJSON(t, w, map[string]any{"sP_IHKE3402S01_Data": []any{row}})
			return
		}
		respondJSON(t, w, map[string]any{"sP_IHKE3405S01_Data": []any{map[string]any{"funC_DATE": "104/02/03", "tel": "合成院所電話", "assaY_MARK": "SOURCE_FLAG", "assaY_VALUE": "4.000000000000000001"}}})
	})
	adult, err := s.FetchSupplement(context.Background(), "adult", Query{})
	if err != nil || len(adult.Entries) != 1 || len(adult.Entries[0].Fields) != len(codes)+1 {
		t.Fatalf("adult source interpretations dropped: %v", err)
	}
	for _, code := range codes {
		if got := fieldValues(adult.Entries, strings.ToLower(code)); len(got) != 1 || got[0] != "009" {
			t.Fatal("source code normalized or replaced with a local judgment")
		}
	}
	kinmen, err := s.FetchProvider(context.Background(), "kinmen", Query{})
	if err != nil || len(kinmen.Entries) != 1 || kinmen.Entries[0].Date != "104/02/03" || kinmen.Entries[0].DateRole != "examination" || len(fieldValues(kinmen.Entries, "assay_mark")) != 1 || len(fieldValues(kinmen.Entries, "tel")) != 1 {
		t.Fatalf("source check date or interpretation flag lost: %v", err)
	}
}

func TestProviderPhysiologyAvailabilityDoesNotInventDeviceObservations(t *testing.T) {
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/ihke3000/ihke3205s01/page_load":
			respondJSON(t, w, map[string]any{"maindata": []any{map[string]any{"code": "105", "code_NAME": "合成量測"}}, "maindata_10568": []any{}})
		case "/api/ihke3000/ihke3205s02/page_load":
			respondJSON(t, w, map[string]any{"sP_IHKE3205S04": []any{}, "sP_IHKE3205S03": []any{}, "devicelist": []any{}, "act_os": "SOURCE_UNKNOWN", "act_os1": false, "act_os2": "0", "act_os3": nil})
		case "/api/ihke3000/ihke3205s02/SP_IHKE3205S08":
			respondJSON(t, w, map[string]any{"sP_IHKE3205S09": []any{}})
		default:
			t.Error("unexpected source query")
		}
	})
	list, err := s.FetchProvider(context.Background(), "physiology", Query{})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := s.FetchProviderDetail(context.Background(), "physiology", list.Entries[0].DetailID)
	if err != nil || len(detail.Entries) != 1 || detail.Entries[0].Kind != "source_metadata" || detail.Entries[0].Date != "" {
		t.Fatalf("availability flags fabricated a measurement: %v", err)
	}
	if fieldValues(detail.Entries, "act_os")[0] != "SOURCE_UNKNOWN" || fieldValues(detail.Entries, "act_os1")[0] != "false" || fieldValues(detail.Entries, "act_os2")[0] != "0" || len(fieldValues(detail.Entries, "act_os3")) != 0 {
		t.Fatal("source device code, false, zero or missing state was normalized")
	}
}

func TestProviderLipidPeriodMetadataDoesNotNarrowHistory(t *testing.T) {
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/ihke3000/ihke3407s01/SP_IHKE3407S01":
			respondJSON(t, w, map[string]any{"RecordList": []any{map[string]any{"ORDER_CODE": "09004C", "CURE_CNAME": "合成項目"}}, "Remark_Date": []any{}})
		case "/api/ihke3000/ihke3407s01/getyears":
			if r.Method != http.MethodGet || r.URL.RawQuery != "" {
				t.Error("unexpected source period selector")
			}
			option := map[string]any{"c": "來源期間文字", "d": "來源期間起日", "e": "002"}
			respondJSON(t, w, map[string]any{"years": []any{option, option}, "remark_SDate": "來源 A/B 起日", "remark_EDate": "來源 A/B 迄日", "ID_NO": "PRIVATE_ID"})
		case "/api/ihke3000/ihke3407s02/get_point":
			var payload map[string]string
			if json.NewDecoder(r.Body).Decode(&payload) != nil || payload["dCode"] != "09004C" || payload["dStr"] != "2001-01-01" || payload["dEnd"] != "2002-12-31" {
				t.Error("source date options replaced explicit history query")
			}
			respondJSON(t, w, map[string]any{"sP_IHKE3406S02_Data": []any{}})
		default:
			t.Error("unexpected lipid read")
		}
	})
	list, err := s.FetchProvider(context.Background(), "lipids", Query{StartDate: "2001-01-01", EndDate: "2002-12-31"})
	if err != nil || len(list.Entries) != 4 || list.StartDate != "2001-01-01" || list.EndDate != "2002-12-31" {
		t.Fatalf("source period metadata omitted: %v", err)
	}
	if list.Entries[1].Key == list.Entries[2].Key || len(fieldValues(list.Entries, "e")) != 2 || fieldValues(list.Entries, "e")[0] != "002" {
		t.Fatal("date-option duplicates or original selector collapsed")
	}
	for _, entry := range list.Entries[1:] {
		if entry.Kind != "source_metadata" || entry.Date != "" || entry.EndDate != "" {
			t.Fatal("source period became an examination")
		}
	}
	if _, err := s.FetchProviderDetail(context.Background(), "lipids", list.Entries[0].DetailID); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(list)
	if strings.Contains(string(encoded), "PRIVATE_ID") {
		t.Fatal("source identity escaped metadata projection")
	}
}
