// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package nhi

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestOfficialExportPreparationUsesOnlySourceOfferedSelections(t *testing.T) {
	for _, tc := range []struct {
		format string
		body   map[string]any
		want   any
	}{
		{"html", map[string]any{"sp_IHKE3802S01_data": []any{map[string]any{"code_CNAME": "合成就醫", "has_all": true, "all_sdate": "0990101", "all_edate": "1151028", "val_all": "private-synthetic-all"}}}, map[string]any{"row_id": []any{"private-synthetic-all"}}},
		{"html", map[string]any{"sp_IHKE3802S01_data": []any{map[string]any{"code_CNAME": "合成月資料", "has_all": false, "details_items": []any{map[string]any{"details": []any{map[string]any{"text": "115年1月", "value": "private-synthetic-month-1"}, map[string]any{"text": "115年2月", "value": "private-synthetic-month-2"}}}}}}}, map[string]any{"row_id": []any{"private-synthetic-month-1", "private-synthetic-month-2"}}},
		{"xml", map[string]any{"sp_IHKE3803S02_data": []any{map[string]any{"code_CNAME": "合成檢驗"}}, "dat": map[string]any{"offered": "private-synthetic-scope", "version": float64(3)}}, map[string]any{"dat": map[string]any{"offered": "private-synthetic-scope", "version": float64(3)}}},
		{"json", map[string]any{"sp_IHKE3804S02_data": []any{map[string]any{"code_CNAME": "合成檢驗"}}, "dat": "private-synthetic-token"}, map[string]any{"dat": "private-synthetic-token"}},
		{"pdf", map[string]any{"sp_IHKE3805S01_data": []any{map[string]any{"code_ENAME": "合成報告", "sp_IHKE3805S02_data": []any{map[string]any{"rpt_TYPE_NAME": "合成類型", "text": "全部可提供範圍", "row_ID": "private-synthetic-pdf"}}}}}, map[string]any{"row_ID": "private-synthetic-pdf"}},
	} {
		t.Run(tc.format, func(t *testing.T) {
			var posts atomic.Int32
			spec := officialExportSpecs[tc.format]
			s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == spec.path+"/page_load" {
					if r.Method != http.MethodGet || len(r.URL.Query()) != 0 {
						t.Error("invented export range/filter")
					}
					respondJSON(t, w, tc.body)
					return
				}
				if r.URL.Path != spec.path+"/add" || r.Method != http.MethodPost {
					t.Error("wrong export path/method")
				}
				posts.Add(1)
				var input any
				if json.NewDecoder(r.Body).Decode(&input) != nil || !reflect.DeepEqual(input, tc.want) {
					t.Errorf("source selection changed: %#v", input)
				}
				respondJSON(t, w, map[string]any{"status": "Y"})
			})
			s.token, s.tokenObtainedAt = "synthetic", time.Now()
			report, err := s.FetchOfficialExportOptions(context.Background(), tc.format)
			if err != nil || len(report.Entries) != 1 || report.Complete || report.StartDate != "" || posts.Load() != 0 {
				t.Fatalf("export options failed or created a job: %v", err)
			}
			entry := report.Entries[0]
			if len(entry.DocumentActions) != 1 || entry.DocumentActions[0].Kind != "prepare_export" {
				t.Fatal("missing explicit export action")
			}
			data, _ := json.Marshal(report)
			if strings.Contains(string(data), "private-synthetic") {
				t.Fatal("source-private selections sent to browser")
			}
			if _, err := s.PrepareDocument(context.Background(), "imaging", entry.DocumentActions[0].ID); err == nil || posts.Load() != 0 {
				t.Fatal("cross-category action submitted")
			}
			if _, err := s.PrepareDocument(context.Background(), report.Category, entry.DocumentActions[0].ID); err != nil || posts.Load() != 1 {
				t.Fatalf("explicit export did not succeed: %v", err)
			}
			if _, err := s.PrepareDocument(context.Background(), report.Category, entry.DocumentActions[0].ID); err == nil || posts.Load() != 1 {
				t.Fatal("generation action replayed")
			}
		})
	}
}

func TestOfficialExportOptionsInvalidAndEmptyAreDistinct(t *testing.T) {
	for _, tc := range []struct {
		body  string
		valid bool
	}{
		{`{"sp_IHKE3804S02_data":[]}`, true},
		{`{"sp_IHKE3804S02_data":null}`, true},
		{`null`, false},
		{`{"other":[]}`, false},
		{`{"sp_IHKE3804S02_data":[{"code_CNAME":"合成資料"}]}`, false},
		{`{"sp_IHKE3804S02_data":[{"code_CNAME":"合成資料"}],"dat":null}`, false},
	} {
		s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(tc.body)) })
		s.token, s.tokenObtainedAt = "synthetic", time.Now()
		report, err := s.FetchOfficialExportOptions(context.Background(), "json")
		if (err == nil) != tc.valid || (tc.valid && len(report.Entries) != 0) {
			t.Fatalf("malformed or empty response misclassified: %v", err)
		}
	}
}
