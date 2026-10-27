// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestResolveQueryUsesTaiwanDateWithoutChangingCustomBounds(t *testing.T) {
	for _, tc := range []struct {
		name  string
		now   time.Time
		query Query
		want  Query
		mode  QueryMode
	}{
		{
			name: "before_taiwan_midnight", now: time.Date(2026, 10, 26, 15, 59, 59, 0, time.UTC),
			want: Query{StartDate: "1912-01-01", EndDate: "2026-10-26"}, mode: QueryModeAllHistory,
		},
		{
			name: "at_taiwan_midnight", now: time.Date(2026, 10, 26, 16, 0, 0, 0, time.UTC),
			want: Query{StartDate: "1912-01-01", EndDate: "2026-10-27"}, mode: QueryModeAllHistory,
		},
		{
			name: "custom_history", now: time.Date(2026, 10, 26, 16, 0, 0, 0, time.UTC),
			query: Query{StartDate: "2023-01-01", EndDate: "2024-12-31"},
			want:  Query{StartDate: "2023-01-01", EndDate: "2024-12-31"}, mode: QueryModeCustom,
		},
		{
			name: "application_year_bounds", now: time.Date(2026, 10, 26, 16, 0, 0, 0, time.UTC),
			query: Query{StartDate: "1912-01-01", EndDate: "2200-12-31"},
			want:  Query{StartDate: "1912-01-01", EndDate: "2200-12-31"}, mode: QueryModeCustom,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, mode, err := resolveQuery(tc.query, tc.now)
			if err != nil || got != tc.want || mode != tc.mode {
				t.Fatalf("resolved query=%+v mode=%s error=%v; want %+v mode=%s", got, mode, err, tc.want, tc.mode)
			}
		})
	}
}

func TestInvalidQueryStopsBeforeRefreshOrReportRequests(t *testing.T) {
	for _, query := range []Query{
		{StartDate: "2023-01-01"},
		{EndDate: "2025-12-31"},
		{StartDate: "2024-02-30", EndDate: "2025-12-31"},
		{StartDate: "2025-01-01", EndDate: "2024-12-31"},
		{StartDate: "1911-12-31", EndDate: "2025-12-31"},
		{StartDate: "2023-01-01", EndDate: "2201-01-01"},
		{StartDate: "112/01/01", EndDate: "2025-12-31"},
	} {
		t.Run(query.StartDate+"_"+query.EndDate, func(t *testing.T) {
			var calls atomic.Int32
			session := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				http.Error(w, "unexpected synthetic request", http.StatusInternalServerError)
			})
			session.token, session.tokenObtainedAt = "synthetic.token", time.Now().Add(-9*time.Minute)
			report, err := session.Fetch(context.Background(), query)
			var upstream *Error
			if !errors.As(err, &upstream) || upstream.Code != "INVALID_INPUT" || calls.Load() != 0 || len(report.Records) != 0 {
				t.Fatalf("invalid query was not rejected before network work: calls=%d error=%v", calls.Load(), err)
			}
		})
	}
}

func TestFetchAllHistoryPreservesOldRowsAndIdenticalObservations(t *testing.T) {
	rows := make([]map[string]any, 60)
	for i := range rows {
		date := "112/03/20"
		if i%2 != 0 {
			date = "113/04/21"
		}
		rows[i] = map[string]any{
			"reaL_INSPECT_DATE": date, "funC_DATE": "112/02/10", "assaY_ITEM_NAME": "Synthetic historical item",
			"assaY_VALUE": "17.125", "uniT_DATA": "test-unit", "orI_TYPE": "A",
		}
	}
	var pageCalls, searchCalls atomic.Int32
	requestedQueries := make(chan Query, 1)
	session := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case pageLoadPath:
			pageCalls.Add(1)
			q := r.URL.Query()
			if q.Get("s_date") != "" || q.Get("e_date") != "" || q.Get("nowPage") != "1" || q.Get("pageDataCount") != "50" {
				t.Error("metadata bootstrap no longer matches the official initial request")
			}
			respondJSON(t, w, map[string]any{"sdate": "2025-01-01", "edate": "2025-12-31", "counts": 0, "sP_IHKE3409S02_Data": []any{}})
		case searchPath:
			searchCalls.Add(1)
			q := r.URL.Query()
			select {
			case requestedQueries <- Query{StartDate: q.Get("s_date"), EndDate: q.Get("e_date")}:
			default:
				t.Error("historical search was repeated")
			}
			if len(q) != 4 || q.Get("s_type") != "" || q.Get("s_sort") != "A1" {
				t.Error("historical search gained a source or pagination filter")
			}
			respondJSON(t, w, rows)
		default:
			t.Error("unexpected synthetic endpoint")
			http.NotFound(w, r)
		}
	})
	session.token, session.tokenObtainedAt = "synthetic.token", time.Now()
	before, _, err := resolveQuery(Query{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	report, err := session.Fetch(context.Background(), Query{})
	if err != nil {
		t.Fatal(err)
	}
	after, _, err := resolveQuery(Query{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var requested Query
	select {
	case requested = <-requestedQueries:
	default:
		t.Fatal("historical search was not requested")
	}
	if requested.StartDate != "1912-01-01" || requested.EndDate < before.EndDate || requested.EndDate > after.EndDate || report.StartDate != requested.StartDate || report.EndDate != requested.EndDate || report.Mode != QueryModeAllHistory {
		t.Fatal("all-history search did not retain its explicit application-floor/Taiwan-date bounds")
	}
	if report.ReportedCount != -1 || len(report.Records) != 60 || pageCalls.Load() != 1 || searchCalls.Load() != 1 {
		t.Fatal("default count was misapplied, historical rows were lost, or extra pagination was requested")
	}
	for i, row := range report.Records {
		wantDate := "112/03/20"
		if i%2 != 0 {
			wantDate = "113/04/21"
		}
		if row.CheckDate != wantDate || row.VisitDate != "112/02/10" || row.Result != "17.125" {
			t.Fatal("historical observations or their original date semantics changed")
		}
	}
	if report.Records[0] != report.Records[2] {
		t.Fatal("identical observations were rewritten")
	}
}

func TestFetchCountChecksRespectTheDefaultInterval(t *testing.T) {
	for _, tc := range []struct {
		name          string
		query         Query
		defaultCount  any
		rows          int
		reportedCount int
		code          string
		searchCalls   int32
		minimum       bool
	}{
		{name: "matching_total", query: Query{"2025-01-01", "2025-12-31"}, defaultCount: 2, rows: 2, reportedCount: 2, searchCalls: 1},
		{name: "matching_new_rows", query: Query{"2025-01-01", "2025-12-31"}, defaultCount: 1, rows: 2, reportedCount: 1, searchCalls: 1},
		{name: "matching_unknown_total", query: Query{"2025-01-01", "2025-12-31"}, rows: 2, reportedCount: -1, searchCalls: 1},
		{name: "wider_range", query: Query{"2023-01-01", "2026-12-31"}, defaultCount: 1, rows: 2, reportedCount: -1, searchCalls: 1},
		{name: "disjoint_oversized_default", query: Query{"2023-01-01", "2024-12-31"}, defaultCount: MaxReportRecords + 1, rows: 2, reportedCount: -1, searchCalls: 1},
		{name: "narrower_oversized_default", query: Query{"2025-03-01", "2025-04-30"}, defaultCount: MaxReportRecords + 1, rows: 2, reportedCount: -1, searchCalls: 1},
		{name: "overlapping_oversized_default", query: Query{"2024-01-01", "2025-04-30"}, defaultCount: MaxReportRecords + 1, rows: 2, reportedCount: -1, searchCalls: 1},
		{name: "matching_missing_rows", query: Query{"2025-01-01", "2025-12-31"}, defaultCount: 3, rows: 2, reportedCount: 3, code: "INCOMPLETE_RESPONSE", searchCalls: 1},
		{name: "wider_below_known_minimum", query: Query{"2023-01-01", "2026-12-31"}, defaultCount: 3, rows: 2, reportedCount: -1, code: "INCOMPLETE_RESPONSE", searchCalls: 1, minimum: true},
		{name: "matching_oversized", query: Query{"2025-01-01", "2025-12-31"}, defaultCount: MaxReportRecords + 1, code: "RESPONSE_TOO_LARGE"},
		{name: "wider_oversized_minimum", query: Query{"2023-01-01", "2026-12-31"}, defaultCount: MaxReportRecords + 1, code: "RESPONSE_TOO_LARGE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var searchCalls atomic.Int32
			session := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case pageLoadPath:
					respondJSON(t, w, map[string]any{"sdate": "2025-01-01", "edate": "2025-12-31", "counts": tc.defaultCount, "sP_IHKE3409S02_Data": []any{}})
				case searchPath:
					searchCalls.Add(1)
					q := r.URL.Query()
					if q.Get("s_date") != tc.query.StartDate || q.Get("e_date") != tc.query.EndDate {
						t.Error("default metadata narrowed or replaced explicit query bounds")
					}
					rows := make([]map[string]string, tc.rows)
					for i := range rows {
						rows[i] = map[string]string{"assaY_ITEM_NAME": "Synthetic count fixture", "assaY_VALUE": "1"}
					}
					respondJSON(t, w, rows)
				default:
					t.Error("unexpected synthetic endpoint")
					http.NotFound(w, r)
				}
			})
			session.token, session.tokenObtainedAt = "synthetic.token", time.Now()
			report, err := session.Fetch(context.Background(), tc.query)
			if searchCalls.Load() != tc.searchCalls {
				t.Fatalf("search calls=%d, want %d", searchCalls.Load(), tc.searchCalls)
			}
			if tc.code != "" {
				var upstream *Error
				if !errors.As(err, &upstream) || upstream.Code != tc.code || len(report.Records) != 0 {
					t.Fatalf("expected closed failure %s without observations, got %v", tc.code, err)
				}
				if tc.code == "INCOMPLETE_RESPONSE" {
					if upstream.ExpectedCount != 3 || upstream.ReceivedCount != 2 || report.ReportedCount != tc.reportedCount {
						t.Fatal("count diagnostic lost its independent-total provenance")
					}
					if tc.minimum && !strings.Contains(upstream.Message, "筆數下限") {
						t.Fatal("wider-range failure presented a minimum as the historical total")
					}
				}
				return
			}
			if err != nil || len(report.Records) != tc.rows || report.ReportedCount != tc.reportedCount || report.Mode != QueryModeCustom || report.StartDate != tc.query.StartDate || report.EndDate != tc.query.EndDate {
				t.Fatalf("query/count provenance changed: report=%+v error=%v", report, err)
			}
		})
	}
}

func TestFetchRejectsMalformedDefaultMetadata(t *testing.T) {
	for _, tc := range []struct {
		name  string
		start any
		end   any
	}{
		{name: "missing_start", end: "2025-12-31"},
		{name: "empty_start", start: "", end: "2025-12-31"},
		{name: "invalid_date", start: "2025-02-30", end: "2025-12-31"},
		{name: "reversed", start: "2026-01-01", end: "2025-12-31"},
		{name: "year_before_floor", start: "1911-12-31", end: "2025-12-31"},
		{name: "year_after_ceiling", start: "2025-01-01", end: "2201-01-01"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var searchCalls atomic.Int32
			session := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != pageLoadPath {
					searchCalls.Add(1)
					http.NotFound(w, r)
					return
				}
				respondJSON(t, w, map[string]any{"sdate": tc.start, "edate": tc.end, "counts": 0, "sP_IHKE3409S02_Data": []any{}})
			})
			session.token, session.tokenObtainedAt = "synthetic.token", time.Now()
			report, err := session.Fetch(context.Background(), Query{})
			var upstream *Error
			if !errors.As(err, &upstream) || upstream.Code != "UPSTREAM_CHANGED" || len(report.Records) != 0 || searchCalls.Load() != 0 {
				t.Fatalf("malformed defaults became an empty/default success: %v", err)
			}
		})
	}
}
