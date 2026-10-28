// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestResidualMenstrualDayIsBoundToReturnedStartAndPreservesRows(t *testing.T) {
	var calls atomic.Int32
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch r.URL.Path {
		case "/api/ihke3000/ihke3208s01/SP_IHKE3208S06":
			respondJSON(t, w, map[string]any{"sP_IHKE3208S06": []any{map[string]any{"sdate": "2020/03/01", "edate": "2020/03/05", "diffDay": "5.00", "seQ_NO": "PRIVATE_SEQUENCE"}}})
		case "/api/ihke3000/ihke3208s01/SP_IHKE3208S03":
			if r.Method != http.MethodGet || len(r.URL.Query()) != 1 || r.URL.Query().Get("cSDate") != "2020-03-01" {
				t.Error("day read did not use the exact returned date")
			}
			row := map[string]any{"sdate": "2020-03-01", "edate": "2020-03-05", "diffDay": "5.000", "seQ_NO": "PRIVATE_SEQUENCE"}
			respondJSON(t, w, map[string]any{"sP_IHKE3208S03": []any{row, row}})
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
	})
	list, err := s.FetchProvider(context.Background(), "menstrual", Query{StartDate: "2020-03-01", EndDate: "2020-03-31"})
	if err != nil || len(list.Entries) != 2 || list.Entries[0].DetailID == "" || list.Entries[0].Date != "2020/03/01" {
		t.Fatalf("period list or detail index lost: %v", err)
	}
	detail, err := s.FetchProviderDetail(context.Background(), "menstrual", list.Entries[0].DetailID)
	if err != nil || len(detail.Entries) != 2 || detail.Entries[0].Key == detail.Entries[1].Key || detail.Entries[0].Fields[2].Value != "5.000" || calls.Load() != 2 {
		t.Fatalf("day response lost original rows: %v", err)
	}
	encoded, _ := json.Marshal(detail)
	if strings.Contains(string(encoded), "PRIVATE_SEQUENCE") {
		t.Fatal("private edit selector escaped")
	}
}

func TestResidualFamilyDoctorDirectoryUsesObservedStatusAndBoundInstitution(t *testing.T) {
	for _, status := range []string{"1", "3", "11", "13", "N"} {
		t.Run(status, func(t *testing.T) {
			var lookups atomic.Int32
			s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/SP_IHKE3720S01") {
					respondJSON(t, w, map[string]any{"sP_IHKE3720S01": map[string]any{"status": status}, "sP_IHKE3720S02": map[string]any{"hosP_ID": "PRIVATE_HOSPITAL_ID", "hosP_ABBR": "合成院所"}})
					return
				}
				lookups.Add(1)
				var body map[string]string
				if r.Method != http.MethodPost || r.URL.Path != "/api/ihke3000/ihke3720s01/SQL100" || json.NewDecoder(r.Body).Decode(&body) != nil || len(body) != 1 || body["HOSP_ID"] != "PRIVATE_HOSPITAL_ID" {
					t.Error("unbound community group lookup")
				}
				respondJSON(t, w, map[string]any{"area_code": "SYNTHETIC_GROUP", "area_code_name": "合成社區醫療群", "private": "PRIVATE_RESPONSE"})
			})
			report, err := s.FetchProvider(context.Background(), "family_doctor", Query{})
			if err != nil {
				t.Fatal(err)
			}
			want := int32(0)
			if status == "1" || status == "3" {
				want = 1
			}
			if lookups.Load() != want || len(report.Entries) != 2+int(want) {
				t.Fatal("source eligibility or group record lost")
			}
			encoded, _ := json.Marshal(report)
			if strings.Contains(string(encoded), "PRIVATE_") {
				t.Fatal("private source selectors forwarded")
			}
		})
	}
}

func TestResidualBoneAllTypeSearchRetainsBothSourceSnapshots(t *testing.T) {
	var calls atomic.Int32
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || len(r.URL.Query()) != 2 || r.URL.Query().Get("s_type") != "" || r.URL.Query().Get("s_sort") != "A1" {
			t.Error("bone retrieval narrowed the all-type selector")
		}
		row := map[string]any{"real_INSPECT_DATE": "115/01/01", "assay_VALUE": "-1.000000000000000001", "row_ID": "SOURCE_ROW", "ori_TYPE": "A"}
		switch r.URL.Path {
		case "/api/ihke3000/ihke3411s01/page_load":
			respondJSON(t, w, map[string]any{"sp_IHKE3411S01_data": []any{row}})
		case "/api/ihke3000/ihke3411s01/search":
			respondJSON(t, w, []any{row, row})
		default:
			t.Errorf("unexpected read: %s", r.URL.Path)
		}
	})
	report, err := s.FetchProvider(context.Background(), "osteoporosis", Query{})
	if err != nil || len(report.Entries) != 3 || calls.Load() != 2 {
		t.Fatalf("all-type source response omitted: %v", err)
	}
	keys := map[string]bool{}
	for _, entry := range report.Entries {
		if entry.DetailID == "" || keys[entry.Key] {
			t.Fatal("snapshot rows collapsed or lost their detail binding")
		}
		keys[entry.Key] = true
	}
}

func TestResidualRecentVisitUsesActualRootsAndExactTypeRoutes(t *testing.T) {
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/ihke3000/ihke3301s02/page_load" {
			row := func(kind string) map[string]any {
				return map[string]any{"func_date": "115/01/01", "hosp_abbr": "合成院所", "icd9cm_name": "合成分類", "htype": kind, "row_id": "PRIVATE_" + kind, "ori_type": "A"}
			}
			respondJSON(t, w, map[string]any{"western_data": []any{row("西醫"), row("其他")}, "chinese_data": []any{row("中醫")}, "dentist_data": []any{row("牙醫")}})
			return
		}
		id := r.URL.Query().Get("crid")
		root, path := "ihke3303S02_main_data", "/api/ihke3000/ihke3303s02/page_load"
		if id == "PRIVATE_牙醫" {
			root, path = "ihke3305S02_main_data", "/api/ihke3000/ihke3305s02/page_load"
		}
		if r.Method != http.MethodGet || r.URL.Path != path || r.URL.Query().Get("ctype") != "A" || id == "PRIVATE_其他" {
			t.Error("recent detail route invented or not source-bound")
		}
		respondJSON(t, w, map[string]any{root: []any{map[string]any{"rowid": id, "func_DATE": "115/01/01"}}})
	})
	list, err := s.FetchProvider(context.Background(), "recent_visits", Query{})
	if err != nil || len(list.Entries) != 4 || list.StartDate != "" || list.Complete {
		t.Fatalf("recent source arrays missing: %v", err)
	}
	for i, entry := range list.Entries {
		if i == 1 {
			if entry.DetailID != "" {
				t.Fatal("unknown source type gained an invented detail route")
			}
			continue
		}
		detail, err := s.FetchProviderDetail(context.Background(), "recent_visits", entry.DetailID)
		if err != nil || len(detail.Entries) != 1 || detail.Category != "recent_visits_detail" {
			t.Fatalf("recent detail missing: %v", err)
		}
	}
	if _, err := s.FetchProvider(context.Background(), "recent_visits", Query{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FetchProviderDetail(context.Background(), "recent_visits", list.Entries[0].DetailID); err == nil {
		t.Fatal("old recent detail survived category refresh")
	}
}

func TestResidualCOVIDTimelineAndOneSourceGatedRefresh(t *testing.T) {
	var calls atomic.Int32
	started := time.Now()
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.URL.RawQuery != "" {
			t.Error("COVID source read added an unsupported selector")
		}
		switch r.URL.Path {
		case "/api/ihke3000/IHKE3410S01/page_load":
			respondJSON(t, w, map[string]any{"reload": true, "count_tw_all": "2", "sp_IHKE3410S01": []any{}, "sp_IHKE3410S05": []any{}, "sp_IHKE3410S06": []any{map[string]any{"real_INSPECT_DATE": "115/01/01", "assay_VALUE": []any{"<b>合成原文</b>", "重複", "重複", nil}, "assay_VALUE_EN": []any{"Synthetic original"}}}, "hid_qrcode": "PRIVATE_QR"})
		case "/api/ihke3000/IHKE3410S01/reload_Vaccine":
			if time.Since(started) < 12*time.Second {
				t.Error("source refresh happened before its observed wait")
			}
			respondJSON(t, w, map[string]any{"sp_IHKE3410S05": []any{map[string]any{"inoculate_D": "115/01/01", "inoculate_D_en": "2026/01/01", "code_cname": "合成疫苗"}}, "sp_IHKE3410S06": []any{map[string]any{"real_INSPECT_DATE": "115/01/01", "assay_VALUE": []any{"更新原文"}, "assay_VALUE_EN": []any{"Updated original"}}}})
		default:
			t.Errorf("unexpected read: %s", r.URL.Path)
		}
	})
	report, err := s.FetchProvider(context.Background(), "covid", Query{})
	if err != nil || calls.Load() != 2 || len(report.Entries) != 4 || report.Complete {
		t.Fatalf("source timeline/reload omitted: %v", err)
	}
	var timelines []SupplementEntry
	for _, entry := range report.Entries {
		if entry.ContractID == "nhi.covid.sp_ihke3410s06" {
			timelines = append(timelines, entry)
		}
	}
	if len(timelines) != 2 || timelines[0].Key == timelines[1].Key || len(timelines[0].Fields) != 5 || timelines[0].Fields[0].Value != "<b>合成原文</b>" || timelines[0].Fields[3].Value != "null" || timelines[0].DateRole != "unknown" {
		t.Fatal("source strings, duplicate items, nulls or independent snapshots were lost")
	}
	encoded, _ := json.Marshal(report)
	if strings.Contains(string(encoded), "PRIVATE_QR") {
		t.Fatal("personal QR identity forwarded")
	}
}

func TestResidualCOVIDRefreshCancellationAndInvalidFlags(t *testing.T) {
	for _, flag := range []any{false, "true", nil, true} {
		var calls atomic.Int32
		s := providerFixture(t, func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			respondJSON(t, w, map[string]any{"reload": flag, "sp_IHKE3410S01": []any{}, "sp_IHKE3410S06": []any{}})
		})
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		_, err := s.FetchProvider(ctx, "covid", Query{})
		cancel()
		if calls.Load() != 1 || ((flag != false) != (err != nil)) {
			t.Fatalf("invalid/canceled readiness launched a refresh or passed: %v", err)
		}
	}
}

func TestResidualCOVIDTimelineSurvivesMissingOtherKnownSections(t *testing.T) {
	s := providerFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(t, w, map[string]any{"sp_IHKE3410S06": []any{map[string]any{"real_INSPECT_DATE": "115/01/01", "assay_VALUE": []any{"合成時間軸原文"}}}})
	})
	report, err := s.FetchProvider(context.Background(), "covid", Query{})
	if err != nil || len(report.Entries) != 1 || report.Entries[0].Fields[0].Value != "合成時間軸原文" || len(report.Diagnostics) != 5 {
		t.Fatalf("partial source response discarded a valid timeline: %v", err)
	}
	for _, diagnostic := range report.Diagnostics {
		if diagnostic.Code != "section_missing" {
			t.Fatal("missing sections were falsely classified empty")
		}
	}
}

func TestResidualEncounterContextRetainsSequenceToothAndPriceFlags(t *testing.T) {
	for _, category := range []string{"western", "dental", "hospital"} {
		t.Run(category, func(t *testing.T) {
			spec := medicalSpecs[category]
			s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
				row := map[string]any{"row_ID": "SYNTHETIC_ENCOUNTER", "ori_TYPE": "A", "func_DATE": "115/01/01", "in_DATE": "115/01/01", "func_SEQ_NO": "0007", "hosp_ABBR": "合成院所"}
				if category == "dental" {
					row["cure_path"] = []any{"11", "12", "12"}
				}
				switch r.URL.Path {
				case spec.listPath:
					respondJSON(t, w, map[string]any{spec.listRoot: []any{row}})
				case spec.searchPath:
					respondJSON(t, w, []any{row, row})
				case spec.detailPath:
					header := map[string]any{"rowid": "SYNTHETIC_ENCOUNTER", "func_SEQ_NO": "0007", "pay_PRICE": "0.000", "show_pay_PRICE": false}
					if category == "western" {
						header["sp_IHKE3302S05"] = []any{map[string]any{"order_code": "SYNTHETIC_ORDER", "pay_PRICE": "123.000", "show_PRICE": false}}
					}
					respondJSON(t, w, map[string]any{spec.detailRoot: []any{header}})
				default:
					t.Errorf("unexpected read %s", r.URL.Path)
				}
			})
			list, err := s.FetchMedical(context.Background(), category, Query{})
			if err != nil || len(list.Entries) != 3 || list.Entries[1].Key == list.Entries[2].Key || list.Entries[0].ContractID == list.Entries[1].ContractID {
				t.Fatalf("encounter sequence incorrectly joined records: %v", err)
			}
			fields := map[string]string{}
			for _, field := range list.Entries[0].Fields {
				fields[field.Code] = field.Value
			}
			if fields["func_seq_no"] != "0007" || category == "dental" && fields["cure_path"] != `["11","12","12"]` {
				t.Fatal("source sequence or raw tooth filter context was lost")
			}
			detail, err := s.FetchMedicalDetail(context.Background(), category, list.Entries[0].DetailID)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range detail.Entries {
				values := map[string]string{}
				for _, field := range entry.Fields {
					values[field.Code] = field.Value
				}
				if entry.Kind == "encounter" && (values["func_seq_no"] != "0007" || values["show_pay_price"] != "false" || values["pay_price"] != "0.000") {
					t.Fatal("header display state or raw source value changed")
				}
				if entry.Kind == "order" && (values["show_price"] != "false" || values["pay_price"] != "123.000") {
					t.Fatal("order display state or raw source amount lost")
				}
			}
		})
	}
	if _, err := appendKnownFields(wireRow{"assay_value": json.RawMessage(`false`)}, []fieldSpec{{"assay_VALUE", "來源結果", ""}}); err == nil {
		t.Fatal("display-flag handling incorrectly allowed a boolean clinical result")
	}
}
