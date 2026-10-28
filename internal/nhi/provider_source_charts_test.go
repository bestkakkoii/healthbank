// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestSourceCalorieSeriesUseFullRequestedRangeAndExactIndexPairs(t *testing.T) {
	for _, category := range []string{"diet_source_totals", "exercise_source_totals"} {
		t.Run(category, func(t *testing.T) {
			s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
				var payload map[string]string
				if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&payload) != nil || len(payload) != 2 {
					t.Error("source series lost its explicit read-only POST range")
				}
				if category == "diet_source_totals" {
					if r.URL.Path != "/api/ihke3000/ihke3206s01/SP_IHKE3206S06" || payload["SDATE"] != "19120101" || payload["EDATE"] != "20261001" {
						t.Error("diet series used a recent-default window")
					}
				} else if r.URL.Path != "/api/ihke3000/ihke3207s02/SP_IHKE3207S07" || payload["c_sdate"] != "19120101" || payload["c_edate"] != "20261001" {
					t.Error("exercise series used a recent-default window")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"ResultStatus":true,"DateList":["115/01/01","115/01/01","115/01/02"],"CalList":[123.000000000000000001,0,null],"token":"PRIVATE_MUST_NOT_LEAK"}`))
			})
			report, err := s.FetchProvider(context.Background(), category, Query{StartDate: "1912-01-01", EndDate: "2026-10-01"})
			if err != nil {
				t.Fatal(err)
			}
			if len(report.Entries) != 3 || report.Entries[0].Key == report.Entries[1].Key || report.StartDate != "1912-01-01" || report.EndDate != "2026-10-01" || report.Complete {
				t.Fatal("source index, duplicates or coverage lost")
			}
			for index, expected := range []string{"123.000000000000000001", "0", "null"} {
				entry := report.Entries[index]
				if entry.Origin != "official_summary" || entry.Fields[1].Value != expected || entry.Fields[1].Unit != "kcal" || entry.Fields[1].Role != "" {
					t.Fatal("source estimate became rounded, zero-filled or a clinical measurement")
				}
			}
			encoded, _ := json.Marshal(report)
			if strings.Contains(string(encoded), "PRIVATE_MUST_NOT_LEAK") {
				t.Fatal("unallowlisted series field leaked")
			}
		})
	}
}

func TestSourceCalorieSeriesRejectMismatchedOrChangedArrays(t *testing.T) {
	for _, body := range []string{
		`{"ResultStatus":true,"DateList":["115/01/01"],"CalList":[]}`,
		`{"ResultStatus":true,"DateList":null,"CalList":null}`,
		`{"ResultStatus":false,"DateList":[],"CalList":[]}`,
		`{"ResultStatus":true,"DateList":["115/01/01"],"CalList":[true]}`,
	} {
		s := providerFixture(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		})
		if _, err := s.FetchProvider(context.Background(), "diet_source_totals", Query{}); err == nil {
			t.Fatal("source chart changed shape was treated as valid complete data")
		}
	}
}

func TestFamilyDoctorConsentSnapshotDoesNotCreateConsentOrExposeIdentity(t *testing.T) {
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/ihke3000/IHKE3709S01/page_load" || r.URL.RawQuery != "" {
			t.Error("consent snapshot attempted a change or arbitrary selector")
		}
		respondJSON(t, w, map[string]any{"hosp_abbr": "合成院所", "prsn_name": "合成家庭醫師", "hosp_tel": "合成電話", "isshowDR": false, "showmain": true, "showcontent": false, "user_name": "PRIVATE_MUST_NOT_LEAK", "user_bday": "PRIVATE_MUST_NOT_LEAK", "content": "PRIVATE_MUST_NOT_LEAK"})
	})
	report, err := s.FetchProvider(context.Background(), "family_doctor_consent_snapshot", Query{})
	if err != nil || len(report.Entries) != 1 {
		t.Fatal("missing source provider snapshot", err)
	}
	encoded, _ := json.Marshal(report)
	if strings.Contains(string(encoded), "PRIVATE_MUST_NOT_LEAK") || !strings.Contains(string(encoded), "非已同意") || !strings.Contains(string(encoded), "false") {
		t.Fatal("source consent or identity semantics changed")
	}
}
