// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bestkakkoii/healthbank/internal/nhi"
)

func TestOfficialReportMetadataSurvivesAPIProjectionAndLogout(t *testing.T) {
	for _, tc := range []struct {
		name          string
		reportedCount int
		rows          int
		mode          nhi.QueryMode
	}{
		{name: "known_total", reportedCount: 2, rows: 2, mode: nhi.QueryModeCustom},
		{name: "zero_total", reportedCount: 0, rows: 0, mode: nhi.QueryModeCustom},
		{name: "unavailable_total", reportedCount: -1, rows: 2, mode: nhi.QueryModeCustom},
		{name: "all_history_unavailable_total", reportedCount: -1, rows: 2, mode: nhi.QueryModeAllHistory},
		{name: "new_rows_after_metadata", reportedCount: 1, rows: 2, mode: nhi.QueryModeCustom},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, db, _ := testApp(t)
			browser := &browserFixture{app: app}
			browser.refresh(t)
			report := nhi.Report{
				Mode: tc.mode, StartDate: "2023-01-01", EndDate: "2026-10-27", ReportedCount: tc.reportedCount,
				Records: make([]nhi.Record, tc.rows),
			}
			if tc.mode == nhi.QueryModeAllHistory {
				report.StartDate = "1912-01-01"
			}
			for i := range report.Records {
				report.Records[i] = nhi.Record{
					CheckDate: "112/03/20", VisitDate: "112/03/02", OrderCode: "09026C", ItemName: "ALT",
					Result: "17.125", Unit: "U/L", Reference: "[7][52]", OfficialFlag: "1",
				}
			}
			app.mu.Lock()
			s := app.sessions[browser.cookie.Value]
			app.mu.Unlock()
			// This fixture starts after successful verification. It neither
			// authenticates an identity nor makes an official network request.
			s.mu.Lock()
			s.mode = "official"
			s.setOfficialReport(report)
			s.mu.Unlock()

			response := browser.request(http.MethodGet, "/api/reports/current", nil, "", "")
			got := decodeResponse[reportResponse](t, response, http.StatusOK)
			if len(got.Records) != tc.rows || got.OfficialQuery == nil || got.OfficialQuery.Mode != report.Mode || got.OfficialQuery.StartDate != report.StartDate || got.OfficialQuery.EndDate != report.EndDate {
				t.Fatal("API projection lost records, query mode or submitted historical interval")
			}
			if got.Complete != (tc.reportedCount >= 0 && tc.reportedCount == tc.rows) {
				t.Fatal("completeness requires the independent count to match every returned record")
			}
			var payload struct {
				OfficialQuery map[string]json.RawMessage `json:"official_query"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			countJSON, exists := payload.OfficialQuery["reported_count"]
			if !exists || len(payload.OfficialQuery) != 4 {
				t.Fatal("official query metadata omitted its count or exposed unexpected fields")
			}
			if tc.reportedCount < 0 {
				if got.OfficialQuery.ReportedCount != nil || !bytes.Equal(countJSON, []byte("null")) || len(got.Warnings) == 0 {
					t.Fatal("unavailable upstream total was not explicit null with a warning")
				}
			} else if got.OfficialQuery.ReportedCount == nil || *got.OfficialQuery.ReportedCount != tc.reportedCount {
				t.Fatal("official total was omitted, changed or inferred from the array length")
			}
			if tc.reportedCount >= 0 && tc.reportedCount != tc.rows && len(got.Warnings) == 0 {
				t.Fatal("contradictory independent count was not disclosed")
			}
			if tc.rows > 0 {
				row := got.Records[0]
				if row.Result != "17.125" || row.ExamDate != "2023-03-20" || row.VisitDate != "2023-03-02" || row.OfficialFlag != "1" {
					t.Fatal("official report conversion changed the original observation")
				}
			}
			loggedOut := decodeResponse[sessionResponse](t, browser.jsonRequest(t, http.MethodPost, "/api/auth/logout", map[string]any{}), http.StatusOK)
			browser.csrf = loggedOut.CSRFToken
			clearedResponse := browser.request(http.MethodGet, "/api/reports/current", nil, "", "")
			cleared := decodeResponse[map[string]json.RawMessage](t, clearedResponse, http.StatusOK)
			if _, exists := cleared["official_query"]; exists {
				t.Fatal("logout retained the previous official query metadata")
			}
			noPersistentProfiles(t, db)
		})
	}
}

func TestIncompleteResponseExposesOnlyValidatedCountDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name     string
		code     string
		expected int
		received int
		included bool
	}{
		{name: "partial", code: "INCOMPLETE_RESPONSE", expected: 52, received: 50, included: true},
		{name: "zero_received", code: "INCOMPLETE_RESPONSE", expected: 52, received: 0, included: true},
		{name: "maximum", code: "INCOMPLETE_RESPONSE", expected: nhi.MaxReportRecords, received: nhi.MaxReportRecords - 1, included: true},
		{name: "negative_expected", code: "INCOMPLETE_RESPONSE", expected: -1, received: 0},
		{name: "negative_received", code: "INCOMPLETE_RESPONSE", expected: 52, received: -1},
		{name: "zero_expected", code: "INCOMPLETE_RESPONSE", expected: 0, received: 0},
		{name: "equal_counts", code: "INCOMPLETE_RESPONSE", expected: 52, received: 52},
		{name: "received_larger", code: "INCOMPLETE_RESPONSE", expected: 52, received: 53},
		{name: "above_limit", code: "INCOMPLETE_RESPONSE", expected: nhi.MaxReportRecords + 1, received: 1},
		{name: "unrelated_code", code: "UPSTREAM_CHANGED", expected: 52, received: 50},
		{name: "unknown_code", code: "UNRECOGNIZED", expected: 52, received: 50},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "https://healthbank.test/api/reports", nil)
			response := httptest.NewRecorder()
			response.Header().Set("X-Request-ID", "synthetic-count-test")
			app := &App{}
			app.upstreamError(response, request, &nhi.Error{
				Code: tc.code, Message: "合成資料筆數不足。", ExpectedCount: tc.expected, ReceivedCount: tc.received,
			})
			if response.Code != http.StatusFailedDependency {
				t.Fatal("incomplete report failure became a gateway error or success")
			}
			var payload struct {
				Error map[string]json.RawMessage `json:"error"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			for key, want := range map[string]int{"expected_count": tc.expected, "received_count": tc.received} {
				value, present := payload.Error[key]
				if present != tc.included {
					t.Fatalf("%s presence does not match the safe incomplete-response contract", key)
				}
				if present {
					var got int
					if err := json.Unmarshal(value, &got); err != nil || got != want {
						t.Fatalf("%s did not preserve a numeric count", key)
					}
				}
			}
		})
	}
}
