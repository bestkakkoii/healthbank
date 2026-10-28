// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/bestkakkoii/healthbank/internal/lab"
	"github.com/bestkakkoii/healthbank/internal/nhi"
	"github.com/bestkakkoii/healthbank/internal/store"
	"github.com/bestkakkoii/healthbank/internal/strictjson"
)

type reportResponse struct {
	Records         []lab.Record           `json:"records"`
	SessionToken    string                 `json:"session_token"`
	FetchedAt       string                 `json:"fetched_at"`
	Complete        bool                   `json:"complete"`
	Warnings        []string               `json:"warnings"`
	OfficialQuery   *officialQueryMetadata `json:"official_query,omitempty"`
	CacheProvenance *cacheReportProvenance `json:"cache_provenance,omitempty"`
}

// officialQueryMetadata describes the exact interval submitted to search. It
// does not assert NHI retention or lifetime coverage. Only a total for that
// same interval is exposed; the default-page count is not a history total.
type officialQueryMetadata struct {
	Mode          nhi.QueryMode `json:"mode"`
	StartDate     string        `json:"start_date"`
	EndDate       string        `json:"end_date"`
	ReportedCount *int          `json:"reported_count"`
}

func (s *session) setOfficialReport(report nhi.Report) {
	s.cacheProvenance = nil
	s.reportRevision++
	raw := make([]lab.RawRecord, 0, len(report.Records))
	for _, v := range report.Records {
		raw = append(raw, lab.RawRecord{CheckDate: v.CheckDate, OrderCode: v.OrderCode, OrderName: v.OrderName, ItemName: v.ItemName, Result: v.Result, Unit: v.Unit, Reference: v.Reference, Institution: v.Institution, VisitDate: v.VisitDate, Source: v.Source, OfficialFlag: v.OfficialFlag, UploadDate: v.UploadDate, SourceName: v.SourceName})
	}
	s.raw = raw
	s.officialQuery = &officialQueryMetadata{Mode: report.Mode, StartDate: report.StartDate, EndDate: report.EndDate}
	if report.ReportedCount >= 0 {
		count := report.ReportedCount
		s.officialQuery.ReportedCount = &count
	}
	s.fetched = time.Now().UTC().Format(time.RFC3339)
	s.complete = report.ReportedCount >= 0 && report.ReportedCount == len(report.Records)
	s.warnings = []string{}
	if report.ReportedCount < 0 {
		s.warnings = append(s.warnings, "已讀取本次查詢回傳的完整資料陣列；官方未提供相同期間的獨立總筆數，無法另外核對。")
	} else if report.ReportedCount != len(report.Records) {
		s.warnings = append(s.warnings, "已保留本次查詢回傳的全部資料；官方相同期間的獨立總筆數與實際回傳筆數不一致，資料可能在兩次讀取間更新，尚未核對完整性。")
	}
}

func (a *App) persistDiscovered(ctx context.Context, s *session) error {
	if s.subject == "" {
		return nil
	}
	unlock, err := a.lockSubject(ctx, s)
	if err != nil {
		return err
	}
	defer unlock()
	discovered := s.settings.Known
	for attempt := 0; attempt < 3; attempt++ {
		doc, err := a.store.Get(ctx, s.subject)
		if err != nil {
			return err
		}
		latest, err := lab.DecodeSettings(doc.Settings, doc.Revision)
		if err != nil {
			return err
		}
		before, err := latest.Document()
		if err != nil {
			return err
		}
		a.catalog.ReconcileSettings(&latest)
		reconciled, err := latest.Document()
		if err != nil {
			return err
		}
		changed := !bytes.Equal(before, reconciled)
		for k, v := range discovered {
			if _, exists := latest.Known[k]; !exists {
				latest.Known[k] = v
				changed = true
			}
		}
		if !changed {
			s.settings = latest
			return nil
		}
		// Source-provided item labels are observations, not already validated
		// persistent configuration. Preserve the report even when discovery exceeds
		// the portable settings schema or capacity; never write an un-restorable row.
		if err := lab.ValidateKnown(latest.Known); err != nil {
			return fmt.Errorf("%w: discovered item definition exceeds settings constraints", store.ErrInvalid)
		}
		if err := lab.ValidateSettings(latest, a.catalog.ConfigurableItems(latest.Known)); err != nil {
			return fmt.Errorf("%w: discovered settings exceed schema or capacity", store.ErrInvalid)
		}
		data, err := latest.Document()
		if err != nil {
			return err
		}
		saved, err := a.store.Put(ctx, s.subject, doc.Revision, data)
		if errors.Is(err, store.ErrConflict) {
			continue
		}
		if err != nil {
			return err
		}
		latest.Revision = saved.Revision
		s.settings = latest
		return nil
	}
	return store.ErrConflict
}
func (a *App) currentResponse(s *session) reportResponse {
	records := a.catalog.Normalize(s.raw, &s.settings)
	warnings := append([]string{}, s.warnings...)
	highTG := map[string]bool{}
	for _, r := range records {
		if r.ItemID == "serum_triglycerides" && r.ExamDate != "" && r.Institution != "" && r.Unit == "mg/dL" && lab.DefinitelyAtLeast(r.Result, "400") {
			highTG[r.ExamDate+"\x00"+r.Institution] = true
		}
	}
	for _, r := range records {
		if r.ItemID == "serum_ldl_cholesterol" && highTG[r.ExamDate+"\x00"+r.Institution] {
			warnings = append(warnings, "同日同院有TG≥400 mg/dL：須核對是否同次採檢及LDL方法；Friedewald不適用，直接量測亦須核對脂血干擾。保留原院結果，不自動判成失真或正常；可對照同次non-HDL／已量測apoB。")
			break
		}
	}
	return reportResponse{Records: records, SessionToken: s.csrf, FetchedAt: s.fetched, Complete: s.complete, Warnings: warnings, OfficialQuery: s.officialQuery, CacheProvenance: s.cacheProvenance}
}
func (a *App) fetchReports(w http.ResponseWriter, r *http.Request, s *session) {
	if s.mode != "official" || s.official == nil {
		a.fail(w, r, 401, "login_required", "請先完成官方登入與圖形驗證碼。")
		return
	}
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	if (from == "") != (to == "") {
		a.fail(w, r, 400, "invalid_date", "自訂查詢須同時填寫開始與結束日期；全部歷史請使用取得全部歷史。")
		return
	}
	for _, v := range []string{from, to} {
		if v != "" {
			if t, err := time.Parse("2006-01-02", v); err != nil || t.Year() < 1912 || t.Year() > 2200 {
				a.fail(w, r, 400, "invalid_date", "日期須使用西元YYYY-MM-DD格式。")
				return
			}
		}
	}
	if from != "" && to != "" && from > to {
		a.fail(w, r, 400, "date_order", "起始日期不可晚於結束日期。")
		return
	}
	if err := a.refreshSettings(r.Context(), s); err != nil {
		if !a.requireCurrentSubject(w, r, s) {
			return
		}
		a.fail(w, r, 500, "settings_read", "無法讀取個人設定。")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 110*time.Second)
	defer cancel()
	report, err := s.official.Fetch(ctx, nhi.Query{StartDate: from, EndDate: to})
	if err != nil {
		if !a.requireCurrentSubject(w, r, s) {
			return
		}
		var upstream *nhi.Error
		if errors.As(err, &upstream) && upstream.Code == "SESSION_EXPIRED" {
			a.clearSession(s)
			if rotateErr := a.rotate(w, r, s); rotateErr != nil {
				a.fail(w, r, 500, "session_rotate", "連線已結束，請重新整理頁面。")
				return
			}
		}
		a.upstreamError(w, r, err)
		return
	}
	a.publishOfficialReport(w, r, s, report)
}

// The caller holds the session lock. Settings discovery can wait behind another
// session's mutation; recheck subject revocation after that asynchronous step
// before publishing any retrieved medical content.
func (a *App) publishOfficialReport(w http.ResponseWriter, r *http.Request, s *session, report nhi.Report) {
	if !a.requireCurrentSubject(w, r, s) {
		return
	}
	next := &session{}
	next.setOfficialReport(report)
	if !a.admitReport(w, r, s, next.raw) {
		return
	}
	s.raw, s.officialQuery, s.fetched, s.complete, s.warnings = next.raw, next.officialQuery, next.fetched, next.complete, next.warnings
	s.cacheProvenance = nil
	s.reportRevision++
	_ = a.catalog.Normalize(s.raw, &s.settings)
	if err := a.persistDiscovered(r.Context(), s); err != nil {
		if errors.Is(err, store.ErrInvalid) {
			s.warnings = append(s.warnings, "本次回傳的檢驗原文均已保留；新項目定義超出同步設定的格式或容量上限，尚未同步。仍可查看與匯出原始結果。")
		} else {
			s.warnings = append(s.warnings, "檢驗資料已取得，但新項目設定未能同步；請稍後重新取得資料。")
		}
	}
	if !a.requireCurrentSubject(w, r, s) {
		return
	}
	a.json(w, 200, a.currentResponse(s))
}
func (a *App) currentReports(w http.ResponseWriter, r *http.Request, s *session) {
	if err := a.refreshSettings(r.Context(), s); err != nil {
		if !a.requireCurrentSubject(w, r, s) {
			return
		}
		a.fail(w, r, 500, "settings_read", "無法讀取個人設定。")
		return
	}
	a.json(w, 200, a.currentResponse(s))
}
func (a *App) importReports(w http.ResponseWriter, r *http.Request, s *session) {
	const maxEnvelopeBytes = lab.MaxImportBytes + (64 << 10)
	if r.ContentLength > maxEnvelopeBytes {
		a.fail(w, r, http.StatusRequestEntityTooLarge, "upload_size", "上傳超過 32 MiB 檔案與 64 KiB 表單包裝的總上限。")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxEnvelopeBytes)
	reader, err := r.MultipartReader()
	if err != nil {
		a.fail(w, r, 400, "invalid_upload", "請選擇HTML、ZIP或本站可攜JSON／XML檔案。")
		return
	}
	var data []byte
	defer func() { clear(data) }()
	var filename string
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				a.fail(w, r, http.StatusRequestEntityTooLarge, "upload_size", "上傳超過 32 MiB 檔案與 64 KiB 表單包裝的總上限。")
			} else {
				a.fail(w, r, 400, "upload_read", "檔案或表單包裝讀取失敗。")
			}
			return
		}
		if part.FormName() != "file" || part.FileName() == "" || filename != "" {
			_ = part.Close()
			a.fail(w, r, 400, "upload_fields", "一次只能匯入一個具有檔名的檔案。")
			return
		}
		filename = part.FileName()
		data, err = io.ReadAll(io.LimitReader(part, lab.MaxImportBytes+1))
		_ = part.Close()
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) || len(data) > lab.MaxImportBytes {
			a.fail(w, r, http.StatusRequestEntityTooLarge, "upload_size", "檔案不可超過 32 MiB；另存 HTML／ZIP 仍依較小的格式上限檢查。")
			return
		}
		if err != nil {
			a.fail(w, r, 400, "upload_read", "檔案讀取失敗。")
			return
		}
	}
	// A multipart closing boundary can precede an epilogue. Drain only through
	// the bounded reader so chunked uploads cannot evade the envelope ceiling.
	if _, err := io.Copy(io.Discard, r.Body); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			a.fail(w, r, http.StatusRequestEntityTooLarge, "upload_size", "上傳超過 32 MiB 檔案與 64 KiB 表單包裝的總上限。")
		} else {
			a.fail(w, r, 400, "upload_read", "檔案表單未完整讀取。")
		}
		return
	}
	if filename == "" || len(data) == 0 {
		a.fail(w, r, 400, "empty_upload", "請選擇非空白的HTML、ZIP或本站可攜JSON／XML檔案。")
		return
	}
	rows, warnings, err := lab.ParseImport(filename, data)
	if err != nil {
		a.fail(w, r, 400, "import_format", err.Error())
		return
	}
	// An imported file has no verified ownership. Use an isolated temporary
	// configuration instead of attaching it to the currently signed-in person.
	if !a.admitReport(w, r, s, rows) {
		return
	}
	a.clearSessionState(s)
	s.mode = "import"
	s.raw = rows
	a.protectWorkspace(s)
	s.fetched = time.Now().UTC().Format(time.RFC3339)
	s.complete = false
	s.warnings = warnings
	if err := a.rotate(w, r, s); err != nil {
		a.fail(w, r, 500, "session_rotate", "無法更新匯入連線。")
		return
	}
	a.json(w, 200, a.currentResponse(s))
}
func (a *App) catalogResponse(w http.ResponseWriter, r *http.Request, s *session) {
	if err := a.refreshSettings(r.Context(), s); err != nil {
		if !a.requireCurrentSubject(w, r, s) {
			return
		}
		a.fail(w, r, 500, "settings_read", "無法讀取個人設定。")
		return
	}
	rows := a.catalog.Normalize(s.raw, &s.settings)
	out := a.catalog.ForSettings(s.settings)
	for i := range out.Items {
		var newest *lab.Record
		for j := range rows {
			if rows[j].ConfigurationKey == out.Items[i].Key && (newest == nil || rows[j].ExamDate > newest.ExamDate) {
				newest = &rows[j]
			}
		}
		if newest != nil {
			out.Items[i].Low = newest.ReferenceLow
			out.Items[i].High = newest.ReferenceHigh
		}
	}
	a.json(w, 200, struct {
		lab.CatalogResponse
		Provenance lab.CatalogProvenance `json:"provenance"`
	}{out, a.catalog.Provenance()})
}
func (a *App) saveSettings(w http.ResponseWriter, r *http.Request, s *session) {
	unlock, err := a.lockSubject(r.Context(), s)
	if err != nil {
		a.subjectAdmissionError(w, r, s)
		return
	}
	defer unlock()
	// Known definitions are server-owned. The browser cannot manufacture a
	// profile or submit arbitrary JSON for persistent storage.
	var input struct {
		Analysis  *lab.AnalysisPreferences `json:"analysis"`
		Revision  int64                    `json:"revision"`
		Theme     string                   `json:"theme"`
		FontScale float64                  `json:"font_scale"`
		Favorites []string                 `json:"favorites"`
		Overrides map[string]lab.Override  `json:"overrides"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		a.fail(w, r, 400, "invalid_settings", "設定格式不正確。")
		return
	}
	if err := a.refreshSettings(r.Context(), s); err != nil {
		a.fail(w, r, 500, "settings_read", "無法讀取個人設定。")
		return
	}
	next := lab.Settings{Analysis: input.Analysis, Revision: input.Revision, Theme: input.Theme, FontScale: input.FontScale, Favorites: input.Favorites, Overrides: input.Overrides, Known: s.settings.Known}
	if next.Favorites == nil {
		next.Favorites = []string{}
	}
	if next.Overrides == nil {
		next.Overrides = map[string]lab.Override{}
	}
	if err := lab.ValidateKnown(next.Known); err != nil {
		a.fail(w, r, 400, "invalid_settings", "既有項目定義不符合設定格式；未變更個人同步設定。")
		return
	}
	if err := lab.ValidateSettings(next, a.catalog.ConfigurableItems(s.settings.Known)); err != nil {
		a.fail(w, r, 400, "invalid_settings", err.Error())
		return
	}
	if input.Revision != s.settings.Revision {
		a.fail(w, r, 409, "settings_conflict", "另一個裝置已更新設定，請重新整理後再修改。")
		return
	}
	if s.subject != "" {
		data, err := next.Document()
		if err != nil {
			a.fail(w, r, 500, "settings_encode", "無法處理設定。")
			return
		}
		doc, err := a.store.Put(r.Context(), s.subject, input.Revision, data)
		if errors.Is(err, store.ErrConflict) {
			a.fail(w, r, 409, "settings_conflict", "另一個裝置已更新設定，請重新整理後再修改。")
			return
		}
		if err != nil {
			a.fail(w, r, 500, "settings_save", "無法儲存設定，請稍後重試。")
			return
		}
		next.Revision = doc.Revision
	} else {
		next.Revision++
	}
	s.settings = next
	a.json(w, 200, next)
}

// ValidatePortableSettings is shared by CLI import and API persistence rules.
// Unknown nested fields are rejected, including credentials and result arrays.
func ValidatePortableSettings(data []byte) (lab.Settings, error) {
	var s lab.Settings
	if len(data) > store.MaxSettingsBytes {
		return s, errors.New("configuration document exceeds its size limit")
	}
	if err := strictjson.Validate(data, strictjson.Options{MaxDepth: 32, FoldNames: true}); err != nil {
		return s, err
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&s); err != nil {
		return s, errors.New("configuration JSON does not match the settings schema")
	}
	if err := d.Decode(&struct{}{}); err != io.EOF {
		return s, errors.New("one settings object required")
	}
	if err := lab.ValidateKnown(s.Known); err != nil {
		return s, err
	}
	catalog, err := lab.LoadCatalog()
	if err != nil {
		return s, err
	}
	catalog.ReconcileSettings(&s)
	return s, lab.ValidateSettings(s, catalog.ConfigurableItems(s.Known))
}
