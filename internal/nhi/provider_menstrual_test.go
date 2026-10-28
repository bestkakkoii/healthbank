// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestMenstrualIndependentMonthRecordsAndBoundDayRefresh(t *testing.T) {
	var calls atomic.Int32
	malformed := false
	s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch r.URL.Path {
		case "/api/ihke3000/ihke3208s01/SP_IHKE3208S06":
			respondJSON(t, w, map[string]any{"sP_IHKE3208S06": []any{}})
		case "/api/ihke3000/ihke3208s01/SP_IHKE3208S01":
			if r.Method != http.MethodGet || len(r.URL.Query()) != 2 || r.URL.Query().Get("ym") != "202012" || r.URL.Query().Get("ymNext") != "202101" {
				t.Error("source month selector or year rollover changed")
			}
			if malformed {
				respondJSON(t, w, map[string]any{"sP_IHKE3208S01": []any{map[string]any{"unknown": "not a period"}}})
				return
			}
			row := map[string]any{"sdate": "20201231", "edate": "20210104"}
			respondJSON(t, w, map[string]any{"sP_IHKE3208S01": []any{row, row}})
		case "/api/ihke3000/ihke3208s01/SP_IHKE3208S03":
			if len(r.URL.Query()) != 1 || r.URL.Query().Get("cSDate") != "2020-12-31" {
				t.Error("compact source date not converted only for the bound day selector")
			}
			respondJSON(t, w, map[string]any{"sP_IHKE3208S03": []any{map[string]any{"sdate": "2020-12-31", "edate": "2021-01-04", "diffDay": "5"}}})
		default:
			t.Error("unexpected source read")
		}
	})
	list, err := s.FetchProvider(context.Background(), "menstrual", Query{StartDate: "2020-11-15", EndDate: "2020-12-20"})
	if err != nil || len(list.Entries) != 2 || calls.Load() != 1 || list.Entries[0].Kind != "source_index" || list.Entries[0].Date != "" {
		t.Fatalf("month indexing fabricated records or launched implicit reads: %v", err)
	}
	month := list.Entries[1].DetailID
	detail, err := s.FetchProviderDetail(context.Background(), "menstrual", month)
	if err != nil || len(detail.Entries) != 2 || detail.Entries[0].Date != "20201231" || detail.Entries[0].EndDate != "20210104" || detail.Entries[0].DateRole != "period_start" || detail.Entries[0].Key == detail.Entries[1].Key || detail.Complete {
		t.Fatalf("independent month rows clipped, merged or date-normalized: %v", err)
	}
	oldDay := detail.Entries[0].DetailID
	malformed = true
	if _, err := s.FetchProviderDetail(context.Background(), "menstrual", month); err == nil {
		t.Fatal("unknown month response accepted")
	}
	if _, err := s.FetchProviderDetail(context.Background(), "menstrual", oldDay); err != nil {
		t.Fatal("failed month refresh retired the successful day handle", err)
	}
	malformed = false
	if _, err := s.FetchProviderDetail(context.Background(), "menstrual", month); err != nil {
		t.Fatal(err)
	}
	before := calls.Load()
	if _, err := s.FetchProviderDetail(context.Background(), "menstrual", oldDay); err == nil || calls.Load() != before {
		t.Fatal("old day handle survived successful month refresh")
	}
}

func TestMenstrualMonthBoundsAndBudgetKeepPriorHandles(t *testing.T) {
	s := providerFixture(t, func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(t, w, map[string]any{"sP_IHKE3208S06": []any{}})
	})
	first, err := s.FetchProvider(context.Background(), "menstrual", Query{StartDate: "2200-12-01", EndDate: "2200-12-31"})
	if err != nil || len(first.Entries) != 1 || fieldValues(first.Entries, "ym_next")[0] != "220101" {
		t.Fatalf("supported upper month boundary lost: %v", err)
	}
	old := first.Entries[0].DetailID
	before := s.referenceBytes
	s.referenceBudget = func(previous, next int64) bool { return next <= previous }
	if _, err := s.FetchProvider(context.Background(), "menstrual", Query{}); err == nil || !strings.Contains(err.Error(), "容量") {
		t.Fatal("full-range month handles bypassed memory admission")
	}
	if _, present := s.providerReferences["menstrual"][old]; !present || s.referenceBytes != before {
		t.Fatal("rejected month index refresh erased the previous query")
	}
}
