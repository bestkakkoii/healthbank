// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package nhi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func providerFixture(t *testing.T, handler http.HandlerFunc) *Session {
	t.Helper()
	s := fixtureSession(t, handler)
	s.token, s.tokenObtainedAt = "synthetic-provider-token", time.Now()
	return s
}

func TestProviderIndependentClinicalSources(t *testing.T) {
	// Independent public-contract examples, not snapshots of registry internals.
	for _, tc := range []struct{ category, path, root, dateField, valueField, value string }{
		{"screen_colorectal", "/api/ihke3000/ihke3404s02/SP_IHKE3404S02", "sP_IHKE3404S02_Data", "funC_DATE", "codE_CNAME", "合成陰性"},
		{"screen_oral", "/api/ihke3000/ihke3404s03/SP_IHKE3404S03", "sP_IHKE3404S03_Data", "funC_DATE", "codE_CNAME", "合成口腔結果"},
		{"screen_breast", "/api/ihke3000/ihke3404s04/page_load", "sP_IHKE3404S04_Data", "funC_DATE", "codE_CNAME", "合成乳房結果"},
		{"screen_cervical", "/api/ihke3000/ihke3404S05/SP_IHKE3404S05", "sP_IHKE3404S05_Data", "funC_DATE", "cytopathiC_CODE", "合成抹片分類"},
		{"screen_lung", "/api/ihke3000/ihke3404s06/page_load", "sp_IHKE3404S06_Data", "funC_DATE", "codE_CNAME", "合成肺部結果"},
		{"screen_hpv", "/api/ihke3000/ihke3404s07/page_load", "sp_IHKE3404S07_Data", "funC_DATE", "codE_CNAME", "合成 HPV 結果"},
		{"screen_hpsa", "/api/ihke3000/ihke3404s08/page_load", "sp_IHKE3404S08_Data", "funC_DATE", "codE_CNAME", "合成 HPSA 結果"},
		{"catastrophic", "/api/ihke3000/ihke3209s01/SP_IHKE3209S01", "sP_IHKE3209S01", "valiD_S_DATE", "icD10CM_CNAME", "合成證明項目"},
		{"care_plans", "/api/ihke3000/IHKE3213S01/page_load", "myplan", "case_date", "mhbt_name", "合成照護計畫"},
		{"hepatitis_test", "/api/ihke3000/ihke3514s01/page_load", "maindata", "real_INSPECT_DATE", "assay_VALUE", "<0.000000000000000001"},
	} {
		t.Run(tc.category, func(t *testing.T) {
			s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != tc.path || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "Bearer synthetic-provider-token" {
					t.Error("unexpected endpoint, mutation or invented date filter")
				}
				row := map[string]any{tc.dateField: "112/01/02", tc.valueField: tc.value, "hosP_ABBR": "合成院所", "ID_NO": "PRIVATE_ID_MUST_NOT_LEAK", "jwt_token": "PRIVATE_TOKEN_MUST_NOT_LEAK", "unknown_clinical_field": "UNKNOWN_FIELD_MUST_NOT_LEAK"}
				respondJSON(t, w, map[string]any{tc.root: []any{row, row}})
			})
			report, err := s.FetchProvider(context.Background(), tc.category, Query{StartDate: "2025-01-01", EndDate: "2026-01-01"})
			if err != nil {
				t.Fatal(err)
			}
			if len(report.Entries) != 2 || report.Complete || report.StartDate != "" || report.EndDate != "" || report.Entries[0].Date != "112/01/02" || report.Entries[0].Key == report.Entries[1].Key {
				t.Fatal("source coverage, duplicate rows or original dates were changed")
			}
			encoded, _ := json.Marshal(report)
			foundValue := false
			for _, field := range report.Entries[0].Fields {
				foundValue = foundValue || field.Value == tc.value
			}
			if !foundValue || strings.Contains(string(encoded), "MUST_NOT_LEAK") {
				t.Fatal("raw clinical value lost or private/unknown field forwarded")
			}
		})
	}
}

func TestProviderSeparateInstitutionAndSelfEnteredRecords(t *testing.T) {
	for _, category := range []string{"allergies", "vaccinations"} {
		t.Run(category, func(t *testing.T) {
			var calls atomic.Int32
			s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodGet || r.URL.RawQuery != "" {
					t.Fatal("read-only source request changed")
				}
				switch r.URL.Path {
				case "/api/ihke3000/ihke3202s01/SP_IHKE3202S01":
					respondJSON(t, w, map[string]any{"sP_IHKE3202S01": []any{map[string]any{"druG_NAME": "合成過敏內容", "uploaD_D": "115/01/01", "allerG_SEVERITY_LEVEL": "來源嚴重度"}}})
				case "/api/ihke3000/ihke3202s01/SP_IHKE3202S04":
					respondJSON(t, w, map[string]any{"sP_IHKE3202S04": []any{map[string]any{"druG_NAME": "合成自行記錄", "uploaD_D2": "115/02/01"}}})
				case "/api/ihke3000/ihke3203s01/SP_IHKE3203S01":
					respondJSON(t, w, map[string]any{"sP_IHKE3203S01": []any{map[string]any{"codE_CNAME": "合成疫苗", "inoculatE_D": "115/01/01"}}})
				case "/api/ihke3000/ihke3203s01/SP_IHKE3203S03":
					respondJSON(t, w, map[string]any{"sP_IHKE3203S03": []any{map[string]any{"codE_CNAME": "合成自行接種紀錄", "inoculatE_D": "115/02/01"}}})
				default:
					t.Error("unexpected official route")
				}
			})
			report, err := s.FetchProvider(context.Background(), category, Query{})
			if err != nil {
				t.Fatal(err)
			}
			if calls.Load() != 2 || len(report.Entries) != 2 || report.Entries[0].Source == report.Entries[1].Source || !strings.Contains(report.Entries[1].Source, "自行記錄") {
				t.Fatal("provider and self-entered provenance collapsed")
			}
		})
	}
}

func TestProviderElectronicPrescriptionsAndReferenceIsolation(t *testing.T) {
	var detailCalls atomic.Int32
	row := map[string]any{"treat_NUM": "PRIVATE_TREAT_REFERENCE", "recipe_TYPE": "C", "recipe_TYPE_name": "合成慢連箋", "func_DATE": "115/09/01", "hosp_ABBR": "合成院所", "drug_CNT": 1, "refillList": []any{map[string]any{"recipE_TIMES": 1, "reL_DATE_STR": "115/09/01", "hosP_NAME": "合成藥局"}, map[string]any{"recipE_TIMES": 2, "recipE_MEMO": "115/09/22～115/10/01"}}, "qrcode_64": "QR_MUST_NOT_LEAK"}
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/ihke3000/IHKE3212S01/page_load":
			respondJSON(t, w, map[string]any{"main_data": []any{row}, "hist_list": []any{row}})
		case "/api/ihke3000/IHKE3212S02/page_load":
			detailCalls.Add(1)
			if r.Method != http.MethodGet || r.URL.Query().Get("tid") != "PRIVATE_TREAT_REFERENCE" || r.URL.Query().Get("recipeType") != "C" || len(r.URL.Query()) != 2 {
				t.Error("detail parameters escaped session binding")
			}
			respondJSON(t, w, map[string]any{"treatData": row, "list": []any{map[string]any{"druG_CODE": "SYNTHETIC-EP", "druG_NAME": "合成藥品", "generiC_NAME": "合成學名", "druG_DAYS": "28", "dose": "0.5000", "dosE_UNIT": "來源單位", "druG_NUM": "56.0000", "useR_TIME_NAME": "來源頻次"}}})
		default:
			t.Error("unexpected route")
		}
	})
	list, err := s.FetchProvider(context.Background(), "e_prescriptions", Query{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Entries) != 4 || list.Entries[0].DetailID == list.Entries[2].DetailID || len(list.Entries[0].DetailID) != 48 {
		t.Fatal("recent/history or per-row detail refs lost")
	}
	data, _ := json.Marshal(list)
	if !strings.Contains(string(data), "來源已領藥日期") || !strings.Contains(string(data), "來源建議領藥期間（非已領）") || strings.Contains(string(data), "PRIVATE_TREAT_REFERENCE") || strings.Contains(string(data), "QR_MUST_NOT_LEAK") {
		t.Fatal("dispensing distinction/privacy violated")
	}
	ref := list.Entries[0].DetailID
	if _, err := s.FetchProviderDetail(context.Background(), "self_paid_medicine", ref); err == nil || detailCalls.Load() != 0 {
		t.Fatal("cross-category reference accepted")
	}
	detail, err := s.FetchProviderDetail(context.Background(), "e_prescriptions", ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Entries) != 3 || detail.Entries[2].Medicine == nil || detail.Entries[2].Medicine.Days != "28" || detail.Entries[2].Medicine.Quantity != "56.0000" || detail.Entries[2].Date != "115/09/01" {
		t.Fatal("source dose/day context lost or inferred")
	}
	if _, err := s.FetchProvider(context.Background(), "e_prescriptions", Query{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FetchProviderDetail(context.Background(), "e_prescriptions", ref); err == nil || detailCalls.Load() != 1 {
		t.Fatal("retired reference reused")
	}
	s.clearAuth()
	if len(s.providerReferences) != 0 {
		t.Fatal("auth reset retained provider references")
	}
}

func TestProviderGlucoseFullRangeWhenDefaultIsEmpty(t *testing.T) {
	var searches atomic.Int32
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/ihke3000/ihke3406s01/SP_IHKE3406S01":
			respondJSON(t, w, map[string]any{"sP_IHKE3406S01_Data": []any{}, "years": []any{}})
		case "/api/ihke3000/ihke3406s02/SP_IHKE3406S02":
			searches.Add(1)
			var payload map[string]string
			if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&payload) != nil || payload["dCode"] != "09006C" || payload["dStr"] != "1912-01-01" || payload["dEnd"] == "" {
				t.Error("full history was replaced with initial date/year")
			}
			respondJSON(t, w, map[string]any{"sP_IHKE3406S02_Data": []any{map[string]any{"reaL_INSPECT_DATE": "108/01/01", "assaY_VALUE": "5.000000000000000001", "orI_TYPE": "D", "orI_TYPE_NAME": "自行登錄"}}})
		default:
			t.Error("unexpected route")
		}
	})
	report, err := s.FetchProvider(context.Background(), "glucose", Query{})
	if err != nil {
		t.Fatal(err)
	}
	if searches.Load() != 1 || len(report.Entries) != 1 || report.Entries[0].Date != "108/01/01" || report.Complete {
		t.Fatal("empty recent default hid old data or asserted completeness")
	}
}

func TestProviderCareDetailsPreserveSourceDatesAndSentinels(t *testing.T) {
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/ihke3000/ihke3504s01/SP_IHKE3504S01":
			respondJSON(t, w, map[string]any{"sP_IHKE3504S01_Data": []any{map[string]any{"roW_ID": "SYNTHETIC-CARE", "casE_DATE": "114/02/03", "hosP_ABBR": "合成院所"}}})
		case "/api/ihke3000/ihke3504s02/SP_IHKE3504S02":
			var payload map[string]string
			if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&payload) != nil || payload["rid"] != "SYNTHETIC-CARE" {
				t.Error("wrong bound care read")
			}
			respondJSON(t, w, map[string]any{"sP_IHKE3504S02_Data": []any{map[string]any{"casE_DATE": "114/02/03", "blooD_CHK_DATE": "114/02/01", "urinE_CHK_DATE": "114/01/31", "blooD_ALB": "90.01000000000000001", "blooD_CREAT": 9999, "stage": "來源追蹤階段"}}})
		default:
			t.Error("unexpected route")
		}
	})
	list, err := s.FetchProvider(context.Background(), "diabetes_care", Query{})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := s.FetchProviderDetail(context.Background(), "diabetes_care", list.Entries[0].DetailID)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(detail)
	for _, value := range []string{"114/02/03", "114/02/01", "114/01/31", "90.01000000000000001", "9999", "來源腎絲球率值"} {
		if !strings.Contains(string(data), value) {
			t.Fatalf("source detail lost %s", value)
		}
	}
	if detail.Entries[0].Medicine != nil || detail.Complete {
		t.Fatal("care data became medication or guaranteed complete")
	}
}

func TestProviderMalformedEmptyAndAuthResponses(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		wantCode   string
	}{
		{"empty", "{\"sP_IHKE3404S03_Data\":[]}", ""},
		{"nested_null", "{\"sP_IHKE3404S03_Data\":null}", ""},
		{"top_null", "null", "UPSTREAM_CHANGED"},
		{"unknown_schema", "{\"another_array\":[]}", "UPSTREAM_CHANGED"},
		{"null_row", "{\"sP_IHKE3404S03_Data\":[null]}", "UPSTREAM_CHANGED"},
		{"wrong_scalar", "{\"sP_IHKE3404S03_Data\":[{\"codE_CNAME\":{\"x\":1}}]}", "UPSTREAM_CHANGED"},
		{"case_collision", "{\"sP_IHKE3404S03_Data\":[{\"codE_CNAME\":\"合成\",\"CODE_CNAME\":\"矛盾\"}]}", "UPSTREAM_CHANGED"},
		{"session_expired", "{\"error_code\":\"9999\"}", "SESSION_EXPIRED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := providerFixture(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			})
			report, err := s.FetchProvider(context.Background(), "screen_oral", Query{})
			if tc.wantCode == "" {
				if err != nil || len(report.Entries) != 0 || report.Complete {
					t.Fatal("known empty source mishandled")
				}
				return
			}
			var upstream *Error
			if !errors.As(err, &upstream) || upstream.Code != tc.wantCode {
				t.Fatalf("unexpected safe error: %v", err)
			}
			if tc.wantCode == "SESSION_EXPIRED" && s.token != "" {
				t.Fatal("expired upstream session retained")
			}
		})
	}
}

func TestProviderMetadataAndNoDataStatus(t *testing.T) {
	seen := map[string]bool{}
	for _, category := range ProviderCategories() {
		if category.ID == "" || category.Label == "" || category.Group == "" || seen[category.ID] || !ProviderCategory(category.ID) || (category.RangeMode != "source" && category.RangeMode != "date_search") {
			t.Fatal("invalid category metadata")
		}
		seen[category.ID] = true
	}
	for _, category := range []string{"organ_wishes", "home_care"} {
		t.Run(category, func(t *testing.T) {
			s := providerFixture(t, func(w http.ResponseWriter, _ *http.Request) {
				if category == "organ_wishes" {
					respondJSON(t, w, map[string]any{"status": "N"})
				} else {
					respondJSON(t, w, map[string]any{"hasdata": false})
				}
			})
			report, err := s.FetchProvider(context.Background(), category, Query{})
			if err != nil || len(report.Entries) != 0 || report.Complete {
				t.Fatal("known no-record status became a fabricated record")
			}
		})
	}
}

func TestProviderPhysiologyRawSeriesAndNestedDeviceDay(t *testing.T) {
	var dayCalls atomic.Int32
	var failSeries atomic.Bool
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]string
		if r.Method == http.MethodPost && json.NewDecoder(r.Body).Decode(&payload) != nil {
			t.Fatal("invalid read payload")
		}
		switch r.URL.Path {
		case "/api/ihke3000/ihke3205s01/page_load":
			respondJSON(t, w, map[string]any{"maindata": []any{map[string]any{"code": "102", "codE_NAME": "體重", "value": "70.0000", "uniT_DATA": "kg", "creatE_DATE": "115/10/01"}}, "maindata_10568": []any{}})
		case "/api/ihke3000/ihke3205s02/page_load":
			if payload["cCode"] != "102" || len(payload) != 1 {
				t.Error("unexpected physiology selector")
			}
			respondJSON(t, w, map[string]any{"sP_IHKE3205S04": []any{map[string]any{"code": "102", "creatE_DATE": "114/01/01", "value": "71.000000000000000001", "txT_DATE": "114/01/03", "time": "2"}}, "devicelist": []any{map[string]any{"cdate": "114/01/02", "edate": "2025-01-02", "value": "來源裝置摘要"}}})
		case "/api/ihke3000/ihke3205s02/SP_IHKE3205S08":
			if payload["cSDate"] != "20200101" || payload["cEDate"] != "20261028" || payload["cCode1"] != "102" || payload["cCode2"] != "" || payload["cCode3"] != "" {
				t.Error("source chart range defaulted or source selector changed")
			}
			if failSeries.Load() {
				respondJSON(t, w, map[string]any{"changed_schema": []any{}})
			} else {
				respondJSON(t, w, map[string]any{"sP_IHKE3205S09": []any{map[string]any{"creatE_DATE": "114/01/01", "valuE1": "71.000000000000000001", "valuE2": "70.50", "valuE3": "71.50", "time": "2"}}})
			}
		case "/api/ihke3000/ihke3205s04/pageload":
			dayCalls.Add(1)
			if r.Method != http.MethodGet || r.URL.Query().Get("code") != "102" || r.URL.Query().Get("cal_date") != "2025-01-02" {
				t.Error("nested day escaped source binding")
			}
			respondJSON(t, w, map[string]any{"devicelist": []any{map[string]any{"cdate": "2025-01-02 08:30", "value": "70.1234567890123456789", "TOKEN": "NO_FORWARD"}}})
		default:
			t.Error("unexpected upstream route")
		}
	})
	list, err := s.FetchProvider(context.Background(), "physiology", Query{StartDate: "2020-01-01", EndDate: "2026-10-28"})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Entries) != 2 || list.Entries[1].Kind != "source_index" || list.Entries[1].Date != "" {
		t.Fatal("pressure source entry was fabricated as a measurement")
	}
	report, err := s.FetchProviderDetail(context.Background(), "physiology", list.Entries[0].DetailID)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Entries) != 3 || report.StartDate != "2020-01-01" || report.EndDate != "2026-10-28" {
		t.Fatal("raw/source series/device index not all retained")
	}
	ref := report.Entries[1].DetailID
	if ref == "" || report.Entries[1].Kind != "source_index" {
		t.Fatal("device day unavailable to bounded retrieval queue")
	}
	nested, err := s.FetchProviderDetail(context.Background(), "physiology", ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(nested.Entries) != 1 || nested.Entries[0].Date != "2025-01-02 08:30" || dayCalls.Load() != 1 {
		t.Fatal("nested device record date lost")
	}
	data, _ := json.Marshal(nested)
	if strings.Contains(string(data), "NO_FORWARD") || !strings.Contains(string(data), "70.1234567890123456789") {
		t.Fatal("nested precision/privacy violation")
	}
	refreshed, err := s.FetchProviderDetail(context.Background(), "physiology", list.Entries[0].DetailID)
	if err != nil {
		t.Fatal(err)
	}
	newRef := refreshed.Entries[1].DetailID
	if newRef == ref || newRef == "" || len(s.providerReferences["physiology"]) != 3 || s.providerReferences["physiology"][newRef].parentReference != list.Entries[0].DetailID {
		t.Fatal("successful parent refresh retained stale children or lost parent binding")
	}
	if _, err := s.FetchProviderDetail(context.Background(), "physiology", ref); err == nil || dayCalls.Load() != 1 {
		t.Fatal("retired child reference remained callable")
	}
	failSeries.Store(true)
	if _, err := s.FetchProviderDetail(context.Background(), "physiology", list.Entries[0].DetailID); err == nil || len(s.providerReferences["physiology"]) != 3 {
		t.Fatal("failed parent refresh committed partial child references")
	}
	if _, err := s.FetchProviderDetail(context.Background(), "physiology", newRef); err != nil || dayCalls.Load() != 2 {
		t.Fatal("failed parent refresh retired the earlier successful child")
	}
	if _, err := s.FetchProvider(context.Background(), "physiology", Query{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FetchProviderDetail(context.Background(), "physiology", newRef); err == nil || dayCalls.Load() != 2 {
		t.Fatal("nested ref survived list refresh")
	}
}

func TestProviderClinicalReadBoundsAndWrongDetail(t *testing.T) {
	t.Run("oversize_field", func(t *testing.T) {
		s := providerFixture(t, func(w http.ResponseWriter, _ *http.Request) {
			respondJSON(t, w, map[string]any{"sP_IHKE3404S03_Data": []any{map[string]any{"codE_CNAME": strings.Repeat("x", 65537)}}})
		})
		_, err := s.FetchProvider(context.Background(), "screen_oral", Query{})
		var upstream *Error
		if !errors.As(err, &upstream) || upstream.Code != "UPSTREAM_CHANGED" {
			t.Fatal("oversize clinical text accepted")
		}
	})
	t.Run("oversize_array", func(t *testing.T) {
		rows := make([]any, MaxReportRecords+1)
		for i := range rows {
			rows[i] = map[string]string{"codE_CNAME": "合成"}
		}
		s := providerFixture(t, func(w http.ResponseWriter, _ *http.Request) {
			respondJSON(t, w, map[string]any{"sP_IHKE3404S03_Data": rows})
		})
		if _, err := s.FetchProvider(context.Background(), "screen_oral", Query{}); err == nil {
			t.Fatal("oversize record array accepted")
		}
	})
	t.Run("wrong_detail", func(t *testing.T) {
		s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
			id := "SYNTHETIC-A"
			if r.URL.Query().Has("rid") {
				id = "SYNTHETIC-B"
			}
			respondJSON(t, w, map[string]any{"sp_IHKE3220S01_data": []any{map[string]any{"row_ID": id, "reaL_INSPECT_DATE": "115/01/01", "assaY_ITEM_NAME": "合成自費藥品"}}})
		})
		list, err := s.FetchProvider(context.Background(), "self_paid_medicine", Query{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.FetchProviderDetail(context.Background(), "self_paid_medicine", list.Entries[0].DetailID); err == nil {
			t.Fatal("mismatching source detail accepted")
		}
	})
}

func TestProviderFailedRefreshKeepsSuccessfulReferences(t *testing.T) {
	var lists atomic.Int32
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("rid") {
			respondJSON(t, w, map[string]any{"sp_IHKE3221S01_data": []any{map[string]any{"row_ID": "SYNTHETIC-MATERIAL", "assay_ITEM_NAME": "合成醫材", "assay_VALUE": "合成規格"}}})
			return
		}
		if lists.Add(1) == 1 {
			respondJSON(t, w, map[string]any{"sp_IHKE3221S01_data": []any{map[string]any{"row_ID": "SYNTHETIC-MATERIAL", "assay_ITEM_NAME": "合成醫材"}}})
			return
		}
		respondJSON(t, w, map[string]any{"unexpected": []any{}})
	})
	list, err := s.FetchProvider(context.Background(), "self_paid_material", Query{})
	if err != nil {
		t.Fatal(err)
	}
	ref := list.Entries[0].DetailID
	if _, err := s.FetchProvider(context.Background(), "self_paid_material", Query{}); err == nil {
		t.Fatal("malformed refresh accepted")
	}
	if _, err := s.FetchProviderDetail(context.Background(), "self_paid_material", ref); err != nil {
		t.Fatal("failed refresh retired previous successful refs")
	}
}

func TestProviderObservedDetailPayloads(t *testing.T) {
	for _, tc := range []struct {
		category, listPath, listRoot, detailPath, detailRoot string
		listRow, detailRow                                   map[string]any
		payload                                              map[string]string
	}{
		{"lipids", "/api/ihke3000/ihke3407s01/SP_IHKE3407S01", "RecordList", "/api/ihke3000/ihke3407s02/get_point", "sP_IHKE3406S02_Data", map[string]any{"ORDER_CODE": "09004C", "CURE_CNAME": "合成三酸甘油脂", "ASSAY_VALUE": "400.0000", "UNIT_DATA": "mg/dL"}, map[string]any{"reaL_INSPECT_DATE": "112/02/03", "assaY_VALUE": "400.000000000000000001"}, map[string]string{"dCode": "09004C", "dStr": "2010-01-01", "dEnd": "2026-10-28"}},
		{"family_followup", "/api/ihke3000/ihke3721s01/SP_IHKE3721S01", "sP_IHKE3721S02", "/api/ihke3000/ihke3721s01/SP_IHKE3721S07", "sP_IHKE3721S07_Data", map[string]any{"assaY_NAME": "SYNTHETIC-TEST", "assaY_ZH_NAME": "合成追蹤項目", "nexT_CHK_DATE": "115/12/01"}, map[string]any{"assaY_DATE": "112/02/03", "assaY_VALUE": "70.000000000000000001"}, map[string]string{"dType": "SYNTHETIC-TEST", "dStr": "2010-01-01", "dEnd": "2026-10-28"}},
		{"exercise", "/api/ihke3000/ihke3207s02/page_load", "sportsRecordList", "/api/ihke3000/ihke3207s02/SP_IHKE3207S04", "oneRecordList", map[string]any{"ROW_ID": "SYNTHETIC-EXERCISE", "SPORT_DATE": "115/10/01", "CODE_NAME": "合成運動", "TOT_CAL": "120"}, map[string]any{"CODE": "SYNTHETIC-CODE", "CODE_LEVEL": "2", "TOT_HR": "1", "TOT_MIN": "30"}, map[string]string{"c_rowid": "SYNTHETIC-EXERCISE"}},
		{"body_measurements", "/api/ihke3000/IHKE3512S01/page_load", "maindata", "/api/ihke3000/IHKE3512S02/search", "maindata", map[string]any{"code": "BP", "c_ORDER_CODE": "WRONG-SELECTOR", "c_CURE_CNAME": "血壓", "c_REAL_INSPECT_DATE": "115/10/01", "c_ASSAY_VALUE": "130/80", "c_ORI_TYPE_NAME": "合成院所來源"}, map[string]any{"reaL_INSPECT_DATE": "112/02/03", "assaY_VALUE": "120.000000000000000001", "assaY_VALUE_1": "75.000000000000000001", "orI_TYPE_NAME": "合成自行記錄來源"}, map[string]string{"dCode": "BP", "dStr": "2010-01-01", "dEnd": "2026-10-28"}},
		{"family_health_followup", "/api/ihke3000/ihke3726s01/SP_IHKE3726S01", "sP_IHKE3726S02", "/api/ihke3000/ihke3726s01/SP_IHKE3726S07", "sP_IHKE3726S07_Data", map[string]any{"assaY_NAME": "SYNTHETIC-TEST", "assaY_ZH_NAME": "合成健康追蹤項目", "nexT_CHK_DATE": "115/12/01"}, map[string]any{"assaY_DATE": "112/02/03", "assaY_VALUE": "70.000000000000000001", "assaY_VALUE_1": "71.000000000000000001", "c_ASSAY_VALUE": "來源複合結果"}, map[string]string{"dType": "SYNTHETIC-TEST", "dStr": "2010-01-01", "dEnd": "2026-10-28"}},
	} {
		t.Run(tc.category, func(t *testing.T) {
			s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case tc.listPath:
					respondJSON(t, w, map[string]any{tc.listRoot: []any{tc.listRow}})
				case tc.detailPath:
					var payload map[string]string
					if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&payload) != nil || len(payload) != len(tc.payload) {
						t.Error("unexpected source read payload")
					}
					for key, value := range tc.payload {
						if payload[key] != value {
							t.Errorf("source parameter %s changed", key)
						}
					}
					body := map[string]any{tc.detailRoot: []any{tc.detailRow, tc.detailRow}}
					if tc.category == "exercise" {
						body["ResultStatus"], body["SPORTDATE_DATE"], body["SPORTDATE_HOUR"], body["SPORTDATE_MINUTE"] = true, "2026-10-01", "08", "05"
					}
					respondJSON(t, w, body)
				default:
					t.Error("unexpected route")
				}
			})
			list, err := s.FetchProvider(context.Background(), tc.category, Query{StartDate: "2010-01-01", EndDate: "2026-10-28"})
			if err != nil {
				t.Fatal(err)
			}
			detail, err := s.FetchProviderDetail(context.Background(), tc.category, list.Entries[0].DetailID)
			if err != nil {
				t.Fatal(err)
			}
			expectedEntries, firstItem := 2, 0
			if tc.category == "exercise" {
				expectedEntries, firstItem = 3, 1
			}
			if len(detail.Entries) != expectedEntries || detail.Entries[firstItem].Key == detail.Entries[firstItem+1].Key || detail.Complete {
				t.Fatal("duplicate detail rows collapsed or completeness invented")
			}
			if tc.category == "body_measurements" || tc.category == "family_health_followup" {
				if list.StartDate != "2010-01-01" || detail.StartDate != list.StartDate || detail.EndDate != "2026-10-28" || list.Entries[0].Kind != "source_index" {
					t.Fatal("full-period query was not declared or source index became a measurement")
				}
				for _, original := range []string{"75.000000000000000001", "71.000000000000000001"} {
					if tc.detailRow["assaY_VALUE_1"] == original {
						found := false
						for _, field := range detail.Entries[0].Fields {
							found = found || field.Value == original
						}
						if !found {
							t.Fatal("second raw measurement channel lost")
						}
					}
				}
			}
		})
	}
}
