// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bestkakkoii/healthbank/internal/config"
	"github.com/bestkakkoii/healthbank/internal/lab"
	"github.com/bestkakkoii/healthbank/internal/nhi"
)

// Starts strictly AFTER synthetic verification. No credentials, official
// authentication or network request is used to exercise this local boundary.
func cacheVerifiedFixture(t *testing.T, browser *browserFixture, identity string) (*session, string) {
	t.Helper()
	browser.refresh(t)
	connector, err := nhi.New()
	if err != nil {
		t.Fatal(err)
	}
	app := browser.app
	app.mu.Lock()
	s := app.sessions[browser.cookie.Value]
	app.mu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	key := config.Subject(app.secret, identity)
	app.bindSubject(s, key)
	s.mode, s.official = "official", connector
	return s, key
}

func cachedReportPayload(key string, revision uint64) map[string]any {
	row := map[string]string{"check_date": "113/02/03", "visit_date": "113/01/19", "order_code": "09026C", "item_name": "ALT",
		"order_name": "合成快取檢驗", "result": "17.125000000000000001", "unit": "U/L", "reference": "[7][52]", "official_flag": "合成旗標", "source": "SYNTHETIC_CACHE_ONLY"}
	return map[string]any{"expected_profile_key": key, "expected_report_revision": revision, "saved_at": "2026-10-28T01:02:03Z", "source_fetched_at": "2025-12-01T00:00:00Z",
		"source_complete": true, "source_query": map[string]any{"mode": "all_history", "start_date": "1912-01-01", "end_date": "2025-12-01", "reported_count": 2},
		"document": map[string]any{"schema": "healthbank.lab.v1", "records": []any{row, row}}}
}

func TestCachedReportsRestorePreservesOfficialIdentityAndHistoricalProvenance(t *testing.T) {
	app, db, _ := testApp(t)
	browser := &browserFixture{app: app}
	s, key := cacheVerifiedFixture(t, browser, "SYNTHETIC_CACHE_BOUNDARY")
	beforeCookie, beforeCSRF, beforeConnector := browser.cookie.Value, browser.csrf, s.official
	response := browser.jsonRequest(t, http.MethodPost, "/api/reports/cache", cachedReportPayload(key, 0))
	report := decodeResponse[reportResponse](t, response, http.StatusOK)
	if len(report.Records) != 2 || report.Complete || report.OfficialQuery != nil || report.FetchedAt != "2025-12-01T00:00:00Z" || report.SessionToken != beforeCSRF {
		t.Fatal("restoration changed records/generation or treated a local snapshot as a new official query")
	}
	for _, row := range report.Records {
		if row.Result != "17.125000000000000001" || row.ExamDate != "2024-02-03" || row.VisitDate != "2024-01-19" || row.RawCheckDate != "113/02/03" || row.OfficialFlag != "合成旗標" {
			t.Fatal("cached re-assessment changed original source dates, duplicate values or flags")
		}
	}
	provenance := report.CacheProvenance
	if provenance == nil || provenance.Kind != "device_cache" || !provenance.SourceComplete || provenance.SourceQuery == nil || provenance.SourceQuery.ReportedCount == nil || *provenance.SourceQuery.ReportedCount != 2 || provenance.SavedAt != "2026-10-28T01:02:03Z" || provenance.SourceFetchedAt != report.FetchedAt {
		t.Fatal("historical client-provided completeness/query was not separately identified")
	}
	if _, err := time.Parse(time.RFC3339Nano, provenance.RestoredAt); err != nil || provenance.RestoredAt == provenance.SourceFetchedAt {
		t.Fatal("restore time must be distinct from the old source fetch time")
	}
	if strings.Contains(response.Body.String(), key) || len(response.Header().Values("Set-Cookie")) != 0 || browser.cookie.Value != beforeCookie {
		t.Fatal("cache response leaked the pseudonymous index or rotated the verified session")
	}
	view := decodeResponse[struct {
		Mode           string `json:"mode"`
		ProfileKey     string `json:"profile_key"`
		ReportRevision uint64 `json:"report_revision"`
	}](t, browser.request(http.MethodGet, "/api/session", nil, "", ""), http.StatusOK)
	if view.Mode != "official" || view.ProfileKey != key || view.ReportRevision != 1 || s.official != beforeConnector {
		t.Fatal("local restoration replaced official identity, connector or revision")
	}
	current := decodeResponse[reportResponse](t, browser.request(http.MethodGet, "/api/reports/current", nil, "", ""), http.StatusOK)
	if current.CacheProvenance == nil || current.Complete || len(current.Records) != 2 {
		t.Fatal("ordinary current-report reads discarded the cache provenance")
	}
	noPersistentProfiles(t, db)
	decodeResponse[sessionResponse](t, browser.jsonRequest(t, http.MethodPost, "/api/auth/logout", map[string]any{}), http.StatusOK)
	cleared := decodeResponse[reportResponse](t, browser.request(http.MethodGet, "/api/reports/current", nil, "", ""), http.StatusOK)
	if len(cleared.Records) != 0 || cleared.CacheProvenance != nil || cleared.FetchedAt != "" {
		t.Fatal("logout retained cached health data in active server memory")
	}
	noPersistentProfiles(t, db)
}

func TestCachedReportsRejectInvalidIdentityAndPayloadWithoutReplacingWorkspace(t *testing.T) {
	app, db, _ := testApp(t)
	browser := &browserFixture{app: app}
	s, key := cacheVerifiedFixture(t, browser, "SYNTHETIC_CACHE_VALIDATION")
	s.mu.Lock()
	s.raw = []lab.RawRecord{{ItemName: "ALT", Result: "SYNTHETIC_PRIOR"}}
	if !s.memory.replaceReports(s.raw) {
		t.Fatal("synthetic prior report admission failed")
	}
	s.mu.Unlock()
	cases := []struct {
		name   string
		mutate func(map[string]any)
		status int
	}{
		{"other_profile", func(v map[string]any) { v["expected_profile_key"] = strings.Repeat("a", 64) }, http.StatusConflict},
		{"stale_report", func(v map[string]any) { v["expected_report_revision"] = 1 }, http.StatusConflict},
		{"missing_revision", func(v map[string]any) { delete(v, "expected_report_revision") }, http.StatusBadRequest},
		{"credential", func(v map[string]any) { v["password"] = "SYNTHETIC_FORBIDDEN" }, http.StatusBadRequest},
		{"case_alias", func(v map[string]any) { v["Saved_at"] = v["saved_at"]; delete(v, "saved_at") }, http.StatusBadRequest},
		{"null_completeness", func(v map[string]any) { v["source_complete"] = nil }, http.StatusBadRequest},
		{"invalid_date", func(v map[string]any) {
			v["source_query"] = map[string]any{"mode": "custom", "start_date": "2026-02-30", "end_date": "2026-03-01"}
		}, http.StatusBadRequest},
		{"unknown_query", func(v map[string]any) {
			v["source_query"] = map[string]any{"mode": "custom", "start_date": "2026-01-01", "end_date": "2026-02-01", "token": "SYNTHETIC_FORBIDDEN"}
		}, http.StatusBadRequest},
		{"unsafe_row", func(v map[string]any) {
			v["document"] = json.RawMessage(`{"schema":"healthbank.lab.v1","records":[{"item_name":"ALT","patient_id":"SYNTHETIC_FORBIDDEN"}]}`)
		}, http.StatusBadRequest},
		{"duplicate_key", func(v map[string]any) {
			v["document"] = json.RawMessage(`{"schema":"healthbank.lab.v1","records":[],"records":[]}`)
		}, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := cachedReportPayload(key, 0)
			tc.mutate(input)
			decodeResponse[map[string]any](t, browser.jsonRequest(t, http.MethodPost, "/api/reports/cache", input), tc.status)
			current := decodeResponse[reportResponse](t, browser.request(http.MethodGet, "/api/reports/current", nil, "", ""), http.StatusOK)
			if len(current.Records) != 1 || current.Records[0].Result != "SYNTHETIC_PRIOR" || current.CacheProvenance != nil || s.reportRevision != 0 {
				t.Fatal("rejected cache body replaced prior workspace or advanced its revision")
			}
		})
	}
	data, err := json.Marshal(cachedReportPayload(key, 0))
	if err != nil {
		t.Fatal(err)
	}
	if response := browser.request(http.MethodPost, "/api/reports/cache", bytes.NewReader(data), "application/json", "wrong-csrf"); response.Code != http.StatusForbidden {
		t.Fatal("local cache restoration bypassed CSRF")
	}
	request := httptest.NewRequest(http.MethodPost, "https://healthbank.test/api/reports/cache", bytes.NewReader(data))
	request.ContentLength = lab.MaxImportBytes + (64 << 10) + 1
	request.AddCookie(browser.cookie)
	request.Header.Set("Origin", "https://healthbank.test")
	request.Header.Set("X-CSRF-Token", browser.csrf)
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	decodeResponse[map[string]any](t, response, http.StatusRequestEntityTooLarge)
	app.budget.mu.Lock()
	app.budget.limits.SessionBytes = s.memory.reports
	app.budget.mu.Unlock()
	decodeResponse[map[string]any](t, browser.jsonRequest(t, http.MethodPost, "/api/reports/cache", cachedReportPayload(key, 0)), http.StatusServiceUnavailable)
	if len(s.raw) != 1 || s.raw[0].Result != "SYNTHETIC_PRIOR" || s.cacheProvenance != nil || s.reportRevision != 0 {
		t.Fatal("capacity refusal changed the existing report or its accounting")
	}
	noPersistentProfiles(t, db)
}

func TestCachedReportsClearRevokesOldRestoreAndPreservesNewOfficialRead(t *testing.T) {
	app, db, _ := testApp(t)
	browser := &browserFixture{app: app}
	s, key := cacheVerifiedFixture(t, browser, "SYNTHETIC_CACHE_CLEAR")
	decodeResponse[reportResponse](t, browser.jsonRequest(t, http.MethodPost, "/api/reports/cache", cachedReportPayload(key, 0)), http.StatusOK)
	clearPayload := map[string]string{"expected_profile_key": key, "scope": "all_reports"}
	decodeResponse[map[string]string](t, browser.jsonRequest(t, http.MethodDelete, "/api/reports/cache", clearPayload), http.StatusOK)
	if len(s.raw) != 0 || s.cacheProvenance != nil || s.reportRevision != 2 || s.mode != "official" || s.subject != key || s.official == nil {
		t.Fatal("refresh did not clear rows/provenance or destroyed the official session")
	}
	decodeResponse[map[string]any](t, browser.jsonRequest(t, http.MethodPost, "/api/reports/cache", cachedReportPayload(key, 1)), http.StatusConflict)
	s.mu.Lock()
	s.setOfficialReport(nhi.Report{Records: []nhi.Record{{ItemName: "ALT", Result: "SYNTHETIC_FRESH"}}, ReportedCount: 1,
		Mode: nhi.QueryModeAllHistory, StartDate: "1912-01-01", EndDate: "2026-10-28"})
	s.mu.Unlock()
	clearPayload["scope"] = "cache_only"
	decodeResponse[map[string]string](t, browser.jsonRequest(t, http.MethodDelete, "/api/reports/cache", clearPayload), http.StatusOK)
	if len(s.raw) != 1 || s.raw[0].Result != "SYNTHETIC_FRESH" || !s.complete || s.cacheProvenance != nil || s.reportRevision != 4 {
		t.Fatal("clearing an old local cache discarded a newer official report")
	}
	decodeResponse[map[string]any](t, browser.jsonRequest(t, http.MethodPost, "/api/reports/cache", cachedReportPayload(key, 2)), http.StatusConflict)
	noPersistentProfiles(t, db)
}

type delayedCacheReader struct {
	reader           io.Reader
	once             sync.Once
	entered, release chan struct{}
}

func (r *delayedCacheReader) Read(data []byte) (int, error) {
	r.once.Do(func() { close(r.entered); <-r.release })
	return r.reader.Read(data)
}

func TestCachedReportsInFlightRestoreCannotReviveRevokedSubject(t *testing.T) {
	app, db, _ := testApp(t)
	first, second := &browserFixture{app: app}, &browserFixture{app: app}
	_, key := cacheVerifiedFixture(t, first, "SYNTHETIC_CACHE_REVOKE")
	cacheVerifiedFixture(t, second, "SYNTHETIC_CACHE_REVOKE")
	data, err := json.Marshal(cachedReportPayload(key, 0))
	if err != nil {
		t.Fatal(err)
	}
	body := &delayedCacheReader{reader: bytes.NewReader(data), entered: make(chan struct{}), release: make(chan struct{})}
	request := httptest.NewRequest(http.MethodPost, "https://healthbank.test/api/reports/cache", body)
	request.AddCookie(second.cookie)
	request.Header.Set("Origin", "https://healthbank.test")
	request.Header.Set("X-CSRF-Token", second.csrf)
	response, finished := httptest.NewRecorder(), make(chan struct{})
	go func() { app.Handler().ServeHTTP(response, request); close(finished) }()
	<-body.entered
	deleted := first.jsonRequest(t, http.MethodDelete, "/api/settings", map[string]int{"revision": 0})
	close(body.release)
	<-finished
	decodeResponse[sessionResponse](t, deleted, http.StatusOK)
	decodeResponse[map[string]any](t, response, http.StatusConflict)
	if len(response.Header().Values("Set-Cookie")) != 0 || strings.Contains(response.Body.String(), "SYNTHETIC_CACHE_ONLY") {
		t.Fatal("revoked delayed restoration revived the old session or disclosed its records")
	}
	noPersistentProfiles(t, db)
}

func TestCachedReportMetadataDoesNotTruncateOriginalIndependentCount(t *testing.T) {
	count := 51_000
	if !validCacheQuery(&officialQueryMetadata{Mode: nhi.QueryModeAllHistory, StartDate: "1912-01-01", EndDate: "2026-10-28", ReportedCount: &count}) {
		t.Fatal("historical independent total was truncated to the number of retained source rows")
	}
	app, db, _ := testApp(t)
	browser := &browserFixture{app: app}
	browser.refresh(t)
	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		payload := cachedReportPayload(strings.Repeat("a", 64), 0)
		if method == http.MethodDelete {
			payload = map[string]any{"expected_profile_key": strings.Repeat("a", 64), "scope": "cache_only"}
		}
		decodeResponse[map[string]any](t, browser.jsonRequest(t, method, "/api/reports/cache", payload), http.StatusUnauthorized)
	}
	noPersistentProfiles(t, db)
}

func TestCachedReportsV2RetainsIndependentUploadDateAndSourceName(t *testing.T) {
	app, db, _ := testApp(t)
	browser := &browserFixture{app: app}
	_, key := cacheVerifiedFixture(t, browser, "SYNTHETIC_CACHE_V2_SOURCE")
	input := cachedReportPayload(key, 0)
	input["document"] = json.RawMessage(`{"schema":"healthbank.lab.v2","records":[{"check_date":"113/02/03","visit_date":"113/01/19","item_name":"ALT","result":"27.000000000000000001","upload_date":"113/03/20 09:41:22","source":"SYNTHETIC_CODE","source_name":"合成上傳來源名稱"},{"check_date":"113/02/03","visit_date":"113/01/19","item_name":"ALT","result":"27.000000000000000001","upload_date":"113/03/20 09:41:22","source":"SYNTHETIC_CODE","source_name":"合成上傳來源名稱"}]}`)
	report := decodeResponse[reportResponse](t, browser.jsonRequest(t, http.MethodPost, "/api/reports/cache", input), http.StatusOK)
	if len(report.Records) != 2 {
		t.Fatal("cached v2 import collapsed duplicated source rows")
	}
	for _, record := range report.Records {
		if record.RawUploadDate != "113/03/20 09:41:22" || record.UploadDate != "2024-03-20" || record.ExamDate != "2024-02-03" || record.VisitDate != "2024-01-19" || record.SourceName != "合成上傳來源名稱" || record.Source != "SYNTHETIC_CODE" || record.Result != "27.000000000000000001" {
			t.Fatal("cached v2 restoration conflated source dates, source code/name or exact result")
		}
	}
	current := decodeResponse[reportResponse](t, browser.request(http.MethodGet, "/api/reports/current", nil, "", ""), http.StatusOK)
	if len(current.Records) != 2 || current.Records[0].RawUploadDate != report.Records[0].RawUploadDate || current.CacheProvenance == nil || current.Complete {
		t.Fatal("v2 cached source metadata or historical provenance was lost on reassessment")
	}
	noPersistentProfiles(t, db)
}
