// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAnnualOverviewAcquiresBothSourceListsAndBoundMedicalDetails(t *testing.T) {
	var requests atomic.Int32
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodGet {
			t.Error("annual clinical read became a mutation")
		}
		switch r.URL.Path {
		case "/api/ihke3000/ihke3302s01/page_load":
			respondJSON(t, w, map[string]any{"maindata": []any{map[string]any{"year": "112", "cnt": "2", "o_cnt": "1", "radi_MSV": "0.001"}}})
		case "/api/ihke3000/ihke3302s02/SP_IHKE3302S02":
			if len(r.URL.Query()) != 1 || r.URL.Query().Get("c_year") != "112" {
				t.Error("annual outpatient request invented a year or date range")
			}
			respondJSON(t, w, map[string]any{"c_SDATE": "112/01/01", "c_EDATE": "112/12/31", "sP_IHKE3302S01_Data": []any{map[string]any{"roW_ID": "PRIVATE-DENTAL", "orI_TYPE": "A", "hosP_DATA_TYPE_NAME": "牙醫", "funC_DATE": "112/02/03", "hosP_ABBR": "合成院所"}, map[string]any{"roW_ID": "PRIVATE-DENTAL", "orI_TYPE": "A", "hosP_DATA_TYPE_NAME": "牙醫", "funC_DATE": "112/02/03", "hosP_ABBR": "合成院所"}}})
		case "/api/ihke3000/ihke3302s03/SP_IHKE3302S03":
			if len(r.URL.Query()) != 1 || r.URL.Query().Get("c_year") != "112" {
				t.Error("annual inpatient selector lost")
			}
			respondJSON(t, w, map[string]any{"sP_IHKE3302S02_Data": []any{map[string]any{"roW_ID": "PRIVATE-HOSPITAL", "orI_TYPE": "B", "iN_DATE": "112/04/01", "ouT_DATE": "112/04/05"}}})
		case "/api/ihke3000/ihke3305s02/page_load":
			if r.URL.Query().Get("crid") != "PRIVATE-DENTAL" || r.URL.Query().Get("ctype") != "A" {
				t.Error("annual outpatient source category or record selector changed")
			}
			respondJSON(t, w, map[string]any{"ihke3305S02_main_data": []any{map[string]any{"rowid": "PRIVATE-DENTAL", "icd9cm_CODE_CNAME": "來源牙醫分類"}}})
		case "/api/ihke3000/IHKE3309S02/page_load":
			if r.URL.Query().Get("crid") != "PRIVATE-HOSPITAL" || r.URL.Query().Get("ctype") != "B" {
				t.Error("annual inpatient source record selector changed")
			}
			respondJSON(t, w, map[string]any{"ihke3309S02_main_data": []any{map[string]any{"rowid": "PRIVATE-HOSPITAL", "main_tit": "合成住院", "has_PDF": "Y"}}})
		default:
			t.Error("annual chain used an unobserved route")
			http.NotFound(w, r)
		}
	})
	list, err := s.FetchProvider(context.Background(), "annual_overview", Query{})
	if err != nil || len(list.Entries) != 1 || list.Entries[0].DetailID == "" {
		t.Fatal("missing source year index", err)
	}
	year := list.Entries[0].DetailID
	detail, err := s.FetchProviderDetail(context.Background(), "annual_overview", year)
	if err != nil || len(detail.Entries) != 4 {
		t.Fatal("annual full child arrays or source period missing", err)
	}
	first, duplicate, hospital := detail.Entries[1], detail.Entries[2], detail.Entries[3]
	if first.Key == duplicate.Key || first.DetailID == duplicate.DetailID || first.DateRole != "encounter" || hospital.DateRole != "admission" || hospital.EndDateRole != "discharge" {
		t.Fatal("annual duplicates or date meanings changed")
	}
	if _, err := s.FetchProviderDetail(context.Background(), "annual_overview", first.DetailID); err != nil {
		t.Fatal(err)
	}
	inpatient, err := s.FetchProviderDetail(context.Background(), "annual_overview", hospital.DetailID)
	if err != nil || len(inpatient.Entries[0].Attachments) != 1 {
		t.Fatal("annual linked inpatient document omitted", err)
	}
	file := inpatient.Entries[0].Attachments[0].ID
	if s.documentReferences[file].category != "annual_overview" {
		t.Fatal("annual nested hospital file escaped its category")
	}
	if _, err := s.FetchProvider(context.Background(), "annual_overview", Query{}); err != nil {
		t.Fatal(err)
	}
	before := requests.Load()
	for _, retired := range []string{year, first.DetailID, duplicate.DetailID, hospital.DetailID} {
		if _, err := s.FetchProviderDetail(context.Background(), "annual_overview", retired); err == nil {
			t.Fatal("annual list refresh retained an old child reference")
		}
	}
	if _, exists := s.documentReferences[file]; exists || requests.Load() != before {
		t.Fatal("annual refresh retained medical child documents or fetched retired selectors")
	}
}

func TestAnnualOverviewUsesObservedDefaultOutpatientRouteWithoutGuessingType(t *testing.T) {
	for _, label := range []string{"", "合成未分類來源"} {
		s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/ihke3000/ihke3302s02/SP_IHKE3302S02":
				respondJSON(t, w, map[string]any{"sP_IHKE3302S01_Data": []any{map[string]any{"roW_ID": "synthetic-outpatient", "orI_TYPE": "A", "hosP_DATA_TYPE_NAME": label, "funC_DATE": "112/02/03"}}})
			case "/api/ihke3000/ihke3302s03/SP_IHKE3302S03":
				respondJSON(t, w, map[string]any{"sP_IHKE3302S02_Data": []any{}})
			case "/api/ihke3000/ihke3303s02/page_load":
				if r.URL.Query().Get("crid") != "synthetic-outpatient" || r.URL.Query().Get("ctype") != "A" {
					t.Error("default outpatient route lost its source binding")
				}
				respondJSON(t, w, map[string]any{"ihke3303S02_main_data": []any{map[string]any{"rowid": "synthetic-outpatient", "icd9cm_CODE_CNAME": "來源分類原文"}}})
			default:
				t.Error("unobserved annual fallback route")
				http.NotFound(w, r)
			}
		})
		s.providerReferences = map[string]map[string]providerReference{"annual_overview": {"year": {rowID: "112", kind: "annual_year"}}}
		list, err := s.FetchProviderDetail(context.Background(), "annual_overview", "year")
		if err != nil || len(list.Entries) != 1 {
			t.Fatal("source-supported default outpatient label rejected", err)
		}
		if label != "" {
			found := false
			for _, field := range list.Entries[0].Fields {
				found = found || field.Code == "hosp_data_type_name" && field.Value == label
			}
			if !found {
				t.Fatal("unrecognized source type label was discarded")
			}
		}
		detail, err := s.FetchProviderDetail(context.Background(), "annual_overview", list.Entries[0].DetailID)
		if err != nil || len(detail.Entries) != 1 || strings.Contains(detail.Entries[0].Title, "西醫") || strings.Contains(detail.Entries[0].Source, "西醫") {
			t.Fatal("default route invented a clinical type or lost its detail", err)
		}
	}
}
