// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestFetchReconcilesObservedInitialRowsAsExactMultiset(t *testing.T) {
	a := map[string]string{"assaY_ITEM_NAME": "SYNTHETIC_OBSERVED_A", "assaY_VALUE": "17.000000000000000001", "reaL_INSPECT_DATE": "113/06/07", "funC_DATE": "113/06/01", "assaY_UPLOAD_DATE": "113/07/02 09:15", "orI_TYPE": "A", "orI_TYPE_NAME": "合成來源原文"}
	b := map[string]string{"assaY_ITEM_NAME": "SYNTHETIC_OTHER_B", "assaY_VALUE": "23.000000000000000002"}
	changedSource := make(map[string]string, len(a))
	for key, value := range a {
		changedSource[key] = value
	}
	changedSource["orI_TYPE_NAME"] = "合成來源已變更"
	year := Query{StartDate: "2024-01-01", EndDate: "2024-12-31"}
	for _, tc := range []struct {
		name     string
		query    Query
		first    []map[string]string
		search   []map[string]string
		count    any
		missing  bool
		reported int
	}{
		{name: "same_count_substitution", query: year, first: []map[string]string{a, b}, search: []map[string]string{b, b}, count: 2, missing: true, reported: 2},
		{name: "duplicate_multiplicity", query: year, first: []map[string]string{a, a}, search: []map[string]string{a, b}, count: 2, missing: true, reported: 2},
		{name: "changed_raw_source_name", query: year, first: []map[string]string{a}, search: []map[string]string{changedSource}, count: 1, missing: true, reported: 1},
		{name: "unknown_total_still_has_observed_rows", query: year, first: []map[string]string{a}, search: []map[string]string{b}, missing: true, reported: -1},
		{name: "wider_interval", query: Query{StartDate: "2023-01-01", EndDate: "2025-12-31"}, first: []map[string]string{a}, search: []map[string]string{b, b}, count: 1, missing: true, reported: -1},
		{name: "reordered_exact_duplicates", query: year, first: []map[string]string{a, b, b}, search: []map[string]string{b, b, a}, count: 3, reported: 3},
		{name: "additional_search_rows", query: year, first: []map[string]string{a}, search: []map[string]string{b, a, a}, count: 1, reported: 1},
		{name: "disjoint_interval", query: Query{StartDate: "2023-01-01", EndDate: "2023-12-31"}, first: []map[string]string{a}, search: []map[string]string{b}, count: 1, reported: -1},
		{name: "narrower_interval", query: Query{StartDate: "2024-05-01", EndDate: "2024-06-30"}, first: []map[string]string{a}, search: []map[string]string{b}, count: 1, reported: -1},
		{name: "partially_overlapping_interval", query: Query{StartDate: "2023-01-01", EndDate: "2024-06-30"}, first: []map[string]string{a}, search: []map[string]string{b}, count: 1, reported: -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case pageLoadPath:
					respondJSON(t, w, map[string]any{"sdate": year.StartDate, "edate": year.EndDate, "counts": tc.count, "sP_IHKE3409S02_Data": tc.first})
				case searchPath:
					if r.URL.Query().Get("s_date") != tc.query.StartDate || r.URL.Query().Get("e_date") != tc.query.EndDate {
						t.Error("initial interval changed the requested search")
					}
					respondJSON(t, w, tc.search)
				default:
					t.Error("unexpected synthetic endpoint")
					http.NotFound(w, r)
				}
			})
			s.token, s.tokenObtainedAt = "synthetic.token", time.Now()
			report, err := s.Fetch(context.Background(), tc.query)
			if tc.missing {
				var upstream *Error
				if !errors.As(err, &upstream) || upstream.Code != "INCOMPLETE_RESPONSE" || upstream.ExpectedCount != 0 || upstream.ReceivedCount != 0 || len(report.Records) != 0 {
					t.Fatalf("observed source-row mismatch did not fail closed: %v", err)
				}
			} else {
				if err != nil || len(report.Records) != len(tc.search) {
					t.Fatalf("valid search was rejected or initial rows were appended: %v", err)
				}
				for index, record := range report.Records {
					if record.ItemName != tc.search[index]["assaY_ITEM_NAME"] || record.Result != tc.search[index]["assaY_VALUE"] || record.UploadDate != tc.search[index]["assaY_UPLOAD_DATE"] || record.SourceName != tc.search[index]["orI_TYPE_NAME"] {
						t.Fatal("source order, exact values, upload date or source name changed")
					}
				}
			}
			if report.ReportedCount != tc.reported || report.StartDate != tc.query.StartDate || report.EndDate != tc.query.EndDate {
				t.Fatal("initial-row consistency check changed independent-count provenance or query")
			}
		})
	}
}

func TestFetchStrictlyParsesInitialRowsBeforeSearch(t *testing.T) {
	for name, raw := range map[string]string{
		"unknown_row":            `[{}]`,
		"null_row":               `[null]`,
		"non_object":             `["synthetic"]`,
		"wrong_source_name_type": `[{"assaY_ITEM_NAME":"synthetic","orI_TYPE_NAME":true}]`,
		"ambiguous_raw_field":    `[{"assaY_ITEM_NAME":"synthetic","assaY_VALUE":"1","ASSAY_VALUE":"2"}]`,
	} {
		t.Run(name, func(t *testing.T) {
			var searches atomic.Int32
			s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == pageLoadPath {
					respondJSON(t, w, map[string]any{"sdate": "2024-01-01", "edate": "2024-12-31", "counts": 1, "sP_IHKE3409S02_Data": json.RawMessage(raw)})
					return
				}
				searches.Add(1)
				respondJSON(t, w, []any{})
			})
			s.token, s.tokenObtainedAt = "synthetic.token", time.Now()
			report, err := s.Fetch(context.Background(), Query{StartDate: "2024-01-01", EndDate: "2024-12-31"})
			var upstream *Error
			if !errors.As(err, &upstream) || upstream.Code != "UPSTREAM_CHANGED" || searches.Load() != 0 || len(report.Records) != 0 {
				t.Fatalf("malformed initial rows were silently ignored: %v", err)
			}
		})
	}
}
