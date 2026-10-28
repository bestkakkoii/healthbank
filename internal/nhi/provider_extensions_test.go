// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package nhi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestProviderAdditionalCareContracts(t *testing.T) {
	for _, tc := range []struct {
		category, listPath, listRoot, detailPath, detailRoot string
		row                                                  map[string]any
	}{
		{"diabetes_ckd_care", "/api/ihke3000/ihke3516s01/SP_IHKE3516S01", "sP_IHKE3516S01_Data", "/api/ihke3000/ihke3516s02/SP_IHKE3516S02", "sP_IHKE3516S02_Data", map[string]any{"stage": "來源合成階段", "casE_DATE": "112/01/02", "chK_DATE": "111/12/31", "blooD_GA": "9999", "egfr": "62.000000000000000001", "uacr": "<0.000000000000000001"}},
		{"metabolic_care", "/api/ihke3000/ihke3517s01/SP_IHKE3517S01", "sP_IHKE3517S01_Data", "/api/ihke3000/ihke3517s02/SP_IHKE3517S02", "sP_IHKE3517S02_Data", map[string]any{"stage": "來源合成階段", "casE_DATE": "112/01/02", "chK_DATE": "111/12/31", "bmi": "27.000000000000000001", "waist": "90.50", "blooD_TG": "9999"}},
	} {
		t.Run(tc.category, func(t *testing.T) {
			s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case tc.listPath:
					if r.Method != http.MethodGet || r.URL.RawQuery != "" {
						t.Error("source-only care list acquired invented range parameters")
					}
					respondJSON(t, w, map[string]any{tc.listRoot: []any{map[string]any{"roW_ID": "PRIVATE-SYNTHETIC-CASE", "casE_DATE": "112/01/02", "hosP_ABBR": "合成院所"}}})
				case tc.detailPath:
					var payload map[string]string
					if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&payload) != nil || len(payload) != 1 || payload["rid"] != "PRIVATE-SYNTHETIC-CASE" {
						t.Error("care detail did not use the source-bound read selector")
					}
					respondJSON(t, w, map[string]any{tc.detailRoot: []any{tc.row, tc.row}, "id_no": "NEVER-EXPOSE"})
				default:
					t.Error("unexpected care route")
				}
			})
			list, err := s.FetchProvider(context.Background(), tc.category, Query{StartDate: "1912-01-01", EndDate: "2026-10-28"})
			if err != nil || len(list.Entries) != 1 || list.StartDate != "" || list.EndDate != "" {
				t.Fatalf("source coverage was changed: %v", err)
			}
			detail, err := s.FetchProviderDetail(context.Background(), tc.category, list.Entries[0].DetailID)
			if err != nil {
				t.Fatal(err)
			}
			if detail.Complete || detail.StartDate != "" || len(detail.Entries) != 2 || detail.Entries[0].Key == detail.Entries[1].Key || detail.Entries[0].Date != "112/01/02" || detail.Entries[0].Institution != "合成院所" {
				t.Fatal("care duplicates, encounter dates or source coverage were changed")
			}
			values := map[string]bool{}
			for _, field := range detail.Entries[0].Fields {
				values[field.Value] = true
			}
			for _, value := range tc.row {
				if !values[value.(string)] {
					t.Fatal("a known source clinical field was lost or numerically reformatted")
				}
			}
			body, _ := json.Marshal(detail)
			if strings.Contains(string(body), "NEVER-EXPOSE") || strings.Contains(string(body), "PRIVATE-SYNTHETIC-CASE") {
				t.Fatal("care identity fields escaped projection")
			}
		})
	}
}

func TestProviderFamilyHealthPressureIndex(t *testing.T) {
	var calls atomic.Int32
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/ihke3000/ihke3726s01/SP_IHKE3726S01":
			respondJSON(t, w, map[string]any{"hasillCategory": true, "sP_IHKE3726S02": []any{}, "bp_IHKE3726S02": map[string]any{"assaY_NAME": "BP", "assaY_ZH_NAME": "血壓", "nexT_CHK_DATE": "115/11/01", "chk_memo": "來源原文"}})
		case "/api/ihke3000/ihke3726s01/SP_IHKE3726S07":
			calls.Add(1)
			var payload map[string]string
			if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&payload) != nil || len(payload) != 3 || payload["dType"] != "BP" || payload["dStr"] != "1912-01-01" || payload["dEnd"] == "" {
				t.Error("family health pressure lost its full-period source selector")
			}
			respondJSON(t, w, map[string]any{"sP_IHKE3726S07_Data": []any{map[string]any{"assaY_DATE": "99/01/01", "assaY_VALUE": "120.0001", "assay_VALUE_1": "70.0001", "c_ASSAY_VALUE": "120.0001/70.0001", "assaY_SOURCE_NAME": "合成自行登打", "hosp_ABBR": "合成機構"}}})
		default:
			t.Error("unexpected family pressure route")
		}
	})
	list, err := s.FetchProvider(context.Background(), "family_health_followup", Query{})
	if err != nil || len(list.Entries) != 1 || list.Entries[0].Kind != "source_index" || list.Entries[0].Date != "" {
		t.Fatalf("pressure object was lost or its next-check date became an observation: %v", err)
	}
	detail, err := s.FetchProviderDetail(context.Background(), "family_health_followup", list.Entries[0].DetailID)
	if err != nil || calls.Load() != 1 || len(detail.Entries) != 1 || detail.Entries[0].Date != "99/01/01" || detail.Complete {
		t.Fatalf("pressure history unavailable: %v", err)
	}
	values := map[string]bool{}
	for _, field := range detail.Entries[0].Fields {
		values[field.Value] = true
	}
	if !values["120.0001"] || !values["70.0001"] || !values["120.0001/70.0001"] || !values["合成自行登打"] {
		t.Fatal("pressure channel or provenance was collapsed")
	}
}

func TestProviderDeviceActivityDiscoveryAndNestedRead(t *testing.T) {
	var calls atomic.Int32
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var payload map[string]string
		if r.Method == http.MethodPost && json.NewDecoder(r.Body).Decode(&payload) != nil {
			t.Error("invalid device read payload")
		}
		switch r.URL.Path {
		case "/api/ihke3000/IHKEMASTER/SP_IHKE3099S05":
			if r.Method != http.MethodPost || len(payload) != 1 || payload["c_parent_fun_code"] != "IHKE3207" {
				t.Error("device discovery changed to an unobserved menu")
			}
			respondJSON(t, w, map[string]any{"sp_IHKE3207S01": []any{
				map[string]any{"fuN_NAME": "合成行動步數", "url": "/IHKE3207S05?s=SYNTHETIC-STEPS", "TOKEN": "PRIVATE-MENU-TOKEN"},
				map[string]any{"fuN_NAME": "既有自行運動紀錄", "url": "/IHKE3207S02"},
				map[string]any{"fuN_NAME": "不允許的遠端入口", "url": "https://example.invalid/IHKE3207S05?s=SECRET"},
				map[string]any{"fuN_NAME": "不允許的變更入口", "url": "/IHKE3207S06?s=SECRET"},
			}})
		case "/api/ihke3000/ihke3207s05/page_load":
			if r.Method != http.MethodPost || len(payload) != 3 || payload["cCode"] != "SYNTHETIC-STEPS" || payload["cSDate"] != "19120101" || payload["cEDate"] != "20261028" {
				t.Error("device raw query omitted the selected full period")
			}
			respondJSON(t, w, map[string]any{"sP_IHKE3205S03": []any{map[string]any{"codE_NAME": "來源步數", "uniT_DATA": "步"}}, "devicelist": []any{map[string]any{"cdate": "115/10/01", "edate": "2026-10-01", "value": "12345"}}})
		case "/api/ihke3000/ihke3205s02/SP_IHKE3205S08":
			if r.Method != http.MethodPost || len(payload) != 5 || payload["cSDate"] != "19120101" || payload["cEDate"] != "20261028" || payload["cCode1"] != "SYNTHETIC-STEPS" || payload["cCode2"] != "" || payload["cCode3"] != "" {
				t.Error("device series query lost its observed selectors")
			}
			respondJSON(t, w, map[string]any{"sP_IHKE3205S09": []any{map[string]any{"creatE_DATE": "115/10/01", "valuE1": "12345.000000000000001"}}})
		case "/api/ihke3000/ihke3205s04/pageload":
			if r.Method != http.MethodGet || len(r.URL.Query()) != 2 || r.URL.Query().Get("code") != "SYNTHETIC-STEPS" || r.URL.Query().Get("cal_date") != "2026-10-01" {
				t.Error("device day escaped the returned source selector")
			}
			respondJSON(t, w, map[string]any{"devicelist": []any{map[string]any{"cdate": "2026-10-01 08:00", "value": "120.00000000000000001"}}})
		default:
			t.Error("unexpected or mutating device route")
		}
	})
	list, err := s.FetchProvider(context.Background(), "device_activity", Query{StartDate: "1912-01-01", EndDate: "2026-10-28"})
	if err != nil || len(list.Entries) != 1 || list.Entries[0].Title != "合成行動步數" || list.Entries[0].Kind != "source_index" || list.Entries[0].Date != "" {
		t.Fatalf("known source device menu was not isolated: %v", err)
	}
	parent := list.Entries[0].DetailID
	detail, err := s.FetchProviderDetail(context.Background(), "device_activity", parent)
	if err != nil || len(detail.Entries) != 2 || detail.StartDate != "1912-01-01" || detail.EndDate != "2026-10-28" {
		t.Fatalf("device full period/index query failed: %v", err)
	}
	child := detail.Entries[0].DetailID
	if child == "" || detail.Entries[0].Kind != "source_index" || s.providerReferences["device_activity"][child].parentReference != parent {
		t.Fatal("device day was not linked to its source parent")
	}
	if _, err := s.FetchProviderDetail(context.Background(), "physiology", child); err == nil || calls.Load() != 3 {
		t.Fatal("device activity child escaped its category")
	}
	day, err := s.FetchProviderDetail(context.Background(), "device_activity", child)
	if err != nil || len(day.Entries) != 1 || calls.Load() != 4 || day.Entries[0].Date != "2026-10-01 08:00" {
		t.Fatalf("nested activity day failed: %v", err)
	}
	body, _ := json.Marshal([]SupplementReport{list, detail, day})
	if strings.Contains(string(body), "PRIVATE-MENU-TOKEN") || strings.Contains(string(body), "example.invalid") || strings.Contains(string(body), "SECRET") {
		t.Fatal("device discovery exposed arbitrary URLs or private menu fields")
	}
}

func TestProviderDeviceMenuRejectsMalformedKnownSelectors(t *testing.T) {
	for _, route := range []string{"/IHKE3207S05", "/IHKE3207S05?s=", "/IHKE3207S05?s=one&s=two", "/IHKE3207S05?s=../other", "/IHKE3207S05?s=" + strings.Repeat("x", 65)} {
		t.Run(route, func(t *testing.T) {
			var calls atomic.Int32
			s := providerFixture(t, func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				respondJSON(t, w, map[string]any{"sp_IHKE3207S01": []any{map[string]any{"fuN_NAME": "合成入口", "url": route}}})
			})
			if _, err := s.FetchProvider(context.Background(), "device_activity", Query{}); err == nil || calls.Load() != 1 || len(s.providerReferences["device_activity"]) != 0 {
				t.Fatal("invalid known device selector created a callable reference")
			}
		})
	}
}
