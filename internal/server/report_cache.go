// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package server

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/bestkakkoii/healthbank/internal/lab"
	"github.com/bestkakkoii/healthbank/internal/nhi"
	"github.com/bestkakkoii/healthbank/internal/strictjson"
)

var cacheProfilePattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// This provenance describes a client-supplied local snapshot, never a new
// independent official count or ownership proof. No profile key enters it.
type cacheReportProvenance struct {
	Kind            string                 `json:"kind"`
	SavedAt         string                 `json:"saved_at"`
	RestoredAt      string                 `json:"restored_at"`
	SourceFetchedAt string                 `json:"source_fetched_at"`
	SourceComplete  bool                   `json:"source_complete"`
	SourceQuery     *officialQueryMetadata `json:"source_query,omitempty"`
}

type cacheReportInput struct {
	ExpectedProfileKey     string                 `json:"expected_profile_key"`
	ExpectedReportRevision *uint64                `json:"expected_report_revision"`
	SavedAt                string                 `json:"saved_at"`
	SourceFetchedAt        string                 `json:"source_fetched_at"`
	SourceComplete         bool                   `json:"source_complete"`
	SourceQuery            *officialQueryMetadata `json:"source_query,omitempty"`
	Document               json.RawMessage        `json:"document"`
}

func (a *App) requireCacheIdentity(w http.ResponseWriter, r *http.Request, s *session, expected string) bool {
	if s.mode != "official" || s.official == nil || s.scope == nil || !cacheProfilePattern.MatchString(s.subject) {
		a.fail(w, r, http.StatusUnauthorized, "login_required", "請先完成官方身分驗證，再開啟這位使用者的本機快取。")
		return false
	}
	if !cacheProfilePattern.MatchString(expected) || subtle.ConstantTimeCompare([]byte(expected), []byte(s.subject)) != 1 {
		a.fail(w, r, http.StatusConflict, "cache_profile_changed", "目前官方驗證身分與快取使用者不符，沒有載入其他人的資料。")
		return false
	}
	return a.requireCurrentSubject(w, r, s)
}

func validCacheQuery(query *officialQueryMetadata) bool {
	if query == nil {
		return true
	}
	if query.Mode != nhi.QueryModeAllHistory && query.Mode != nhi.QueryModeCustom || query.StartDate > query.EndDate ||
		query.ReportedCount != nil && (*query.ReportedCount < 0 || *query.ReportedCount > 9_007_199_254_740_991) {
		return false
	}
	for _, value := range []string{query.StartDate, query.EndDate} {
		date, err := time.Parse("2006-01-02", value)
		if err != nil || date.Year() < 1912 || date.Year() > 2200 {
			return false
		}
	}
	return true
}

func validCacheTimestamp(value string, allowEmpty bool) bool {
	if value == "" {
		return allowEmpty
	}
	if len(value) > 100 {
		return false
	}
	_, err := time.Parse(time.RFC3339Nano, value)
	return err == nil
}

func decodeCacheReport(w http.ResponseWriter, r *http.Request, input *cacheReportInput) error {
	const maximum = lab.MaxImportBytes + (64 << 10)
	if r.ContentLength > maximum {
		return &http.MaxBytesError{Limit: maximum}
	}
	r.Body = http.MaxBytesReader(w, r.Body, maximum)
	data, err := io.ReadAll(r.Body)
	defer clear(data)
	if err != nil {
		return err
	}
	if err := strictjson.Validate(data, strictjson.Options{MaxDepth: 32, FoldNames: true}); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return errors.New("one cache report object required")
	}
	for name := range fields {
		switch name {
		case "expected_profile_key", "expected_report_revision", "saved_at", "source_fetched_at", "source_complete", "source_query", "document":
		default:
			return errors.New("unknown cache report field")
		}
	}
	for _, name := range []string{"expected_profile_key", "expected_report_revision", "saved_at", "source_fetched_at", "source_complete", "document"} {
		if len(fields[name]) == 0 || bytes.Equal(fields[name], []byte("null")) {
			return errors.New("missing cache report field")
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(input); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("one cache report object required")
	}
	if input.ExpectedReportRevision == nil || len(input.Document) == 0 || len(input.Document) > lab.MaxImportBytes || !validCacheTimestamp(input.SavedAt, false) ||
		!validCacheTimestamp(input.SourceFetchedAt, true) || !validCacheQuery(input.SourceQuery) {
		return errors.New("invalid bounded cache report")
	}
	return nil
}

// restoreCachedReports never performs an upstream request, rotates the official
// identity into an import session, or writes observations/settings into SQL.
// withSession provides CSRF, cookie-generation, session-mutex and memory gates.
func (a *App) restoreCachedReports(w http.ResponseWriter, r *http.Request, s *session) {
	// Reject anonymous/import/demo callers before reading a potentially large body.
	if s.mode != "official" || s.official == nil || s.scope == nil {
		a.fail(w, r, http.StatusUnauthorized, "login_required", "請先完成官方身分驗證，再開啟本機快取。")
		return
	}
	var input cacheReportInput
	if err := decodeCacheReport(w, r, &input); err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			a.fail(w, r, http.StatusRequestEntityTooLarge, "cache_report_size", "快取檢驗 JSON 不可超過 32 MiB，包裝資料另限 64 KiB。")
		} else {
			a.fail(w, r, http.StatusBadRequest, "cache_report_format", "快取檢驗的格式或來源描述不符，先前資料未被取代。")
		}
		return
	}
	defer clear(input.Document)
	if !a.requireCacheIdentity(w, r, s, input.ExpectedProfileKey) {
		return
	}
	if *input.ExpectedReportRevision != s.reportRevision {
		a.fail(w, r, http.StatusConflict, "cache_report_changed", "檢驗工作區已重新取得或清除，較早的快取沒有覆蓋目前資料。")
		return
	}
	rows, warnings, err := lab.ParseImport("healthbank-cached-labs.json", input.Document)
	if err != nil {
		a.fail(w, r, http.StatusBadRequest, "cache_report_format", "快取檢驗內容未通過可攜資料驗證，先前資料未被取代。")
		return
	}
	if r.Context().Err() != nil {
		a.subjectAdmissionError(w, r, s)
		return
	}
	unlock, err := a.lockSubject(r.Context(), s)
	if err != nil {
		a.subjectAdmissionError(w, r, s)
		return
	}
	defer unlock()
	if r.Context().Err() != nil {
		a.subjectAdmissionError(w, r, s)
		return
	}
	if !a.requireCacheIdentity(w, r, s, input.ExpectedProfileKey) || !a.admitReport(w, r, s, rows) {
		return
	}
	// The session and subject locks remain held. There is no asynchronous work
	// between a successful memory admission and installing the replacement.
	s.raw, s.fetched, s.complete, s.officialQuery = rows, input.SourceFetchedAt, false, nil
	s.reportRevision++
	s.cacheProvenance = &cacheReportProvenance{Kind: "device_cache", SavedAt: input.SavedAt,
		RestoredAt: time.Now().UTC().Format(time.RFC3339Nano), SourceFetchedAt: input.SourceFetchedAt,
		SourceComplete: input.SourceComplete, SourceQuery: input.SourceQuery}
	s.warnings = append(warnings, "這些檢驗來自您同意保存在此裝置的快取；本次僅重新判讀，未重新向官方拉取或核對完整性。")
	a.protectWorkspace(s)
	a.json(w, http.StatusOK, a.currentResponse(s))
}

// clearCachedReports is explicit and idempotent. 'all_reports' is the first
// step of a user-requested refresh; 'cache_only' leaves newer official reads.
func (a *App) clearCachedReports(w http.ResponseWriter, r *http.Request, s *session) {
	var input struct {
		ExpectedProfileKey string `json:"expected_profile_key"`
		Scope              string `json:"scope"`
	}
	if err := decodeJSON(w, r, &input); err != nil || input.Scope != "cache_only" && input.Scope != "all_reports" {
		a.fail(w, r, http.StatusBadRequest, "cache_clear_format", "請明確指定要清除本次快取檢驗或重新取得前的全部檢驗。")
		return
	}
	if !a.requireCacheIdentity(w, r, s, input.ExpectedProfileKey) {
		return
	}
	unlock, err := a.lockSubject(r.Context(), s)
	if err != nil {
		a.subjectAdmissionError(w, r, s)
		return
	}
	defer unlock()
	if r.Context().Err() != nil {
		a.subjectAdmissionError(w, r, s)
		return
	}
	// Revoke previously observed restore revisions even if a newer official
	// report remains in place for the cache-only clear.
	s.reportRevision++
	if input.Scope == "all_reports" || s.cacheProvenance != nil {
		if s.memory != nil {
			s.memory.releaseReports()
		}
		s.raw, s.officialQuery, s.cacheProvenance = nil, nil, nil
		s.fetched, s.complete, s.warnings = "", false, []string{}
	}
	a.json(w, http.StatusOK, map[string]string{"status": "cleared"})
}
