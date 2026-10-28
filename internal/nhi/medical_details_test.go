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

func TestMedicalNestedLaboratoryHistoryAndImagingRemainBound(t *testing.T) {
	var requests atomic.Int32
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodGet {
			t.Error("read-only medical chain performed a mutation")
		}
		switch r.URL.Path {
		case "/api/ihke3000/ihke3303s02/page_load":
			if r.URL.Query().Get("crid") != "PRIVATE-ENCOUNTER" || r.URL.Query().Get("ctype") != "A" {
				t.Error("encounter reference escaped its parent binding")
			}
			respondJSON(t, w, map[string]any{"ihke3303S02_main_data": []any{map[string]any{"rowid": "PRIVATE-ENCOUNTER", "part_AMT": "0001", "sp_IHKE3302S07_data": []any{map[string]any{"order_CODE": "SYNTHETIC-ORDER", "cure_CNAME": "合成檢驗", "show_link": true}}, "icdcode_data": []any{map[string]any{"icd_tit": "合成次分類", "icd_code_name": "來源分類原文"}, map[string]any{"icd_tit": "合成次分類", "icd_code_name": "來源分類原文"}}, "opcode_data": []any{map[string]any{"op_tit": "合成次處置", "op_code_name": "來源處置原文"}}}}})
		case "/api/ihke3000/ihke3303s03/pageload":
			if len(r.URL.Query()) != 3 || r.URL.Query().Get("crid") != "PRIVATE-ENCOUNTER" || r.URL.Query().Get("ctype") != "A" || r.URL.Query().Get("ocode") != "SYNTHETIC-ORDER" {
				t.Error("laboratory selectors were guessed or replaced")
			}
			respondJSON(t, w, map[string]any{"sp_IHKE3302S20_data_list": []any{map[string]any{"creal_INSPECT_DATE": "113/02/03", "ereal_INSPECT_DATE": "2024/02/03", "order_CODE": "SYNTHETIC-ORDER", "assay_ITEM_NAME": "合成項目", "assay_VALUE": "<1.000000000000000001", "consult_VALUE": "來源參考"}, map[string]any{"creal_INSPECT_DATE": "113/02/03", "order_CODE": "SYNTHETIC-ORDER", "assay_ITEM_NAME": "合成項目", "assay_VALUE": "<1.000000000000000001"}}, "sp_IHKE3302S21_data_list": []any{map[string]any{"row_ID": "PRIVATE-IMAGE", "ori_TYPE": "B", "creal_INSPECT_DATE": "113/02/04", "cure_CNAME": "合成影像", "hosp_ABBR": "合成影像院所"}}})
		case "/api/ihke3000/ihke3303s04/pageload":
			if len(r.URL.Query()) != 1 || r.URL.Query().Get("ocode") != "SYNTHETIC-ORDER" {
				t.Error("source history invented dates or lost its observed code")
			}
			respondJSON(t, w, map[string]any{"sp_IHKE3302S20_data_list": []any{map[string]any{"order_CODE": "SYNTHETIC-ORDER", "assay_VALUE": "0.000000000000000001"}}})
		case "/api/ihke3000/ihke3303s05/pageload":
			if len(r.URL.Query()) != 1 || r.URL.Query().Get("ocode") != "SYNTHETIC-ORDER" {
				t.Error("image history selector was not source bound")
			}
			respondJSON(t, w, map[string]any{"sp_IHKE3302S21_data_list": []any{}})
		case "/api/ihke3000/IHKE3408S02/page_load":
			if r.URL.Query().Get("crid") != "PRIVATE-IMAGE" || r.URL.Query().Get("ctype") != "B" {
				t.Error("nested imaging selector was not source bound")
			}
			respondJSON(t, w, map[string]any{"ihke3408S02_main_data": []any{map[string]any{"desc": "合成影像文字", "jpg_STATUS": "1", "ipl_CASE_SEQ_NO": "PRIVATE-IMAGE-FILE", "img_ORI_SIZE": "1 MB"}}})
		default:
			t.Errorf("unexpected source route: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	s.medicalReferences = map[string]map[string]medicalReference{"western": {"parent": {category: "western", rowID: "PRIVATE-ENCOUNTER", sourceType: "A", date: "115/10/01", institution: "合成就醫院所"}}}
	parent, err := s.FetchMedicalDetail(context.Background(), "western", "parent")
	if err != nil {
		t.Fatal(err)
	}
	if len(parent.Entries) != 5 || parent.Entries[2].Key == parent.Entries[3].Key {
		t.Fatal("secondary classification/procedure rows were omitted or duplicates collapsed")
	}
	var laboratory string
	for _, entry := range parent.Entries {
		if entry.DetailID != "" {
			laboratory = entry.DetailID
		}
	}
	if laboratory == "" {
		t.Fatal("observed laboratory link was discarded")
	}
	report, err := s.FetchMedicalDetail(context.Background(), "western", laboratory)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Entries) != 5 || report.Entries[0].Date != "113/02/03" || report.Entries[0].DateRole != "examination" || report.Entries[0].Key == report.Entries[1].Key {
		t.Fatal("lab result precision, duplication or examination date relation changed")
	}
	encoded, _ := json.Marshal(report)
	if strings.Contains(string(encoded), "PRIVATE-") || !strings.Contains(string(encoded), "1.000000000000000001") {
		t.Fatal("private selector exposed or exact laboratory value rounded")
	}
	var imageReference, historyReference, imageHistory string
	for id, ref := range s.medicalReferences["western"] {
		switch ref.kind {
		case "imaging":
			imageReference = id
		case "laboratory_history":
			historyReference = id
		case "imaging_history":
			imageHistory = id
		}
	}
	history, err := s.FetchMedicalDetail(context.Background(), "western", historyReference)
	if err != nil || len(history.Entries) != 1 || history.Entries[0].Date != "" || history.Entries[0].Institution != "" {
		t.Fatal("source history fabricated parent date/institution", err)
	}
	if _, err := s.FetchMedicalDetail(context.Background(), "western", imageHistory); err != nil {
		t.Fatal(err)
	}
	image, err := s.FetchMedicalDetail(context.Background(), "western", imageReference)
	if err != nil || len(image.Entries) != 1 || len(image.Entries[0].Attachments) != 1 {
		t.Fatal("nested source image documents were lost", err)
	}
	attachment := image.Entries[0].Attachments[0].ID
	if s.documentReferences[attachment].category != "western" {
		t.Fatal("nested imaging document escaped owning category")
	}
	if _, err := s.FetchMedicalDetail(context.Background(), "western", "parent"); err != nil {
		t.Fatal(err)
	}
	before := requests.Load()
	for _, old := range []string{laboratory, imageReference, historyReference, imageHistory} {
		if _, err := s.FetchMedicalDetail(context.Background(), "western", old); err == nil {
			t.Fatal("refresh retained a retired descendant")
		}
	}
	if _, exists := s.documentReferences[attachment]; exists || requests.Load() != before {
		t.Fatal("refresh retained descendant documents or requested a retired selector")
	}
}

func TestMedicalChildAdmissionFailurePreservesPreviousTree(t *testing.T) {
	large := false
	s := providerFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		code := "SYNTHETIC"
		if large {
			code = strings.Repeat("S", 4096)
		}
		respondJSON(t, w, map[string]any{"ihke3303S02_main_data": []any{map[string]any{"rowid": "synthetic-parent", "sp_IHKE3302S07_data": []any{map[string]any{"order_CODE": code, "show_link": true}}}}})
	})
	s.medicalReferences = map[string]map[string]medicalReference{"western": {"parent": {category: "western", rowID: "synthetic-parent", sourceType: "A"}}}
	first, err := s.FetchMedicalDetail(context.Background(), "western", "parent")
	if err != nil {
		t.Fatal(err)
	}
	child := first.Entries[1].DetailID
	previousBytes := s.referenceBytes
	s.referenceBudget = func(previous, next int64) bool { return next <= previous }
	large = true
	if _, err := s.FetchMedicalDetail(context.Background(), "western", "parent"); err == nil {
		t.Fatal("over-budget new child state admitted")
	}
	if _, present := s.medicalReferences["western"][child]; !present || len(s.medicalReferences["western"]["parent"].children) != 1 || s.medicalReferences["western"]["parent"].children[0] != child || s.referenceBytes != previousBytes {
		t.Fatal("rejected detail refresh damaged the successful old tree/budget")
	}
}

func TestMedicalNestedResultsKeepMissingValuesExplicit(t *testing.T) {
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/ihke3000/ihke3303s04/pageload" {
			t.Error("unexpected source history route")
		}
		respondJSON(t, w, map[string]any{"sp_IHKE3302S20_data_list": []any{
			map[string]any{"order_CODE": "synthetic-order", "assay_ITEM_NAME": "合成缺值項目", "assay_VALUE": nil},
			map[string]any{"order_CODE": "synthetic-order", "assay_ITEM_NAME": "合成缺值項目"},
			map[string]any{"order_CODE": "synthetic-order", "assay_ITEM_NAME": "合成零值項目", "assay_VALUE": "0"},
		}})
	})
	s.medicalReferences = map[string]map[string]medicalReference{"western": {"history": {category: "western", kind: "laboratory_history", orderCode: "synthetic-order"}}}
	report, err := s.FetchMedicalDetail(context.Background(), "western", "history")
	if err != nil || len(report.Entries) != 3 {
		t.Fatal("source rows with missing values were dropped", err)
	}
	if report.Entries[0].Key == report.Entries[1].Key || report.Entries[0].Date != "" {
		t.Fatal("missing-result source rows collapsed or gained invented dates")
	}
	missing := 0
	for _, diagnostic := range report.Diagnostics {
		if diagnostic.Code == "known_result_missing" && diagnostic.Section == "nhi.western.sp_ihke3302s20_data_list" && diagnostic.Count != nil {
			missing += *diagnostic.Count
		}
	}
	if missing != 2 {
		t.Fatal("absent/null results did not retain the explicit missing-result diagnostic or zero was treated as absent")
	}
}
