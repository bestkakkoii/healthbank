// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bestkakkoii/healthbank/internal/lab"
	"github.com/bestkakkoii/healthbank/internal/nhi"
	"github.com/bestkakkoii/healthbank/internal/store"
)

type reportResponse struct {
	Records       []lab.Record           `json:"records"`
	FetchedAt     string                 `json:"fetched_at"`
	Complete      bool                   `json:"complete"`
	Warnings      []string               `json:"warnings"`
	OfficialQuery *officialQueryMetadata `json:"official_query,omitempty"`
}

// officialQueryMetadata describes the interval returned by NHI, not an inferred
// lifetime history. An unavailable upstream total is explicitly null.
type officialQueryMetadata struct {
	StartDate     string `json:"start_date"`
	EndDate       string `json:"end_date"`
	ReportedCount *int   `json:"reported_count"`
}

func (s *session) setOfficialReport(report nhi.Report) {
	raw := make([]lab.RawRecord, 0, len(report.Records))
	for _, v := range report.Records {
		raw = append(raw, lab.RawRecord{CheckDate: v.CheckDate, OrderCode: v.OrderCode, OrderName: v.OrderName, ItemName: v.ItemName, Result: v.Result, Unit: v.Unit, Reference: v.Reference, Institution: v.Institution, VisitDate: v.VisitDate, Source: v.Source, OfficialFlag: v.OfficialFlag})
	}
	s.raw = raw
	s.officialQuery = &officialQueryMetadata{StartDate: report.StartDate, EndDate: report.EndDate}
	if report.ReportedCount >= 0 {
		count := report.ReportedCount
		s.officialQuery.ReportedCount = &count
	}
	s.fetched = time.Now().UTC().Format(time.RFC3339)
	s.complete = report.ReportedCount >= 0
	s.warnings = []string{}
	if report.ReportedCount < 0 {
		s.warnings = append(s.warnings, "官方未提供總筆數；已讀取查詢回傳的完整陣列，但無法比對總筆數。")
	}
}

func (a *App) persistDiscovered(ctx context.Context, s *session) error {
	if s.subject == "" {
		return nil
	}
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
		changed := false
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
		if r.ItemID == "serum_triglycerides" && r.ExamDate != "" && r.Institution != "" && lab.DefinitelyAbove(r.Result, "400") {
			highTG[r.ExamDate+"\x00"+r.Institution] = true
		}
	}
	for _, r := range records {
		if r.ItemID == "serum_ldl_cholesterol" && highTG[r.ExamDate+"\x00"+r.Institution] {
			warnings = append(warnings, "同日三酸甘油脂超過400 mg/dL：請核對LDL是否為直接量測；常見Friedewald計算法不適用於此濃度。")
			break
		}
	}
	return reportResponse{Records: records, FetchedAt: s.fetched, Complete: s.complete, Warnings: warnings, OfficialQuery: s.officialQuery}
}
func (a *App) fetchReports(w http.ResponseWriter, r *http.Request, s *session) {
	if s.mode != "official" || s.official == nil {
		a.fail(w, r, 401, "login_required", "請先完成官方登入與圖形驗證碼。")
		return
	}
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
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
		a.fail(w, r, 500, "settings_read", "無法讀取個人設定。")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 110*time.Second)
	defer cancel()
	report, err := s.official.Fetch(ctx, nhi.Query{StartDate: from, EndDate: to})
	if err != nil {
		var upstream *nhi.Error
		if errors.As(err, &upstream) && upstream.Code == "SESSION_EXPIRED" {
			s.clear()
			if rotateErr := a.rotate(w, r, s); rotateErr != nil {
				a.fail(w, r, 500, "session_rotate", "連線已結束，請重新整理頁面。")
				return
			}
		}
		a.upstreamError(w, r, err)
		return
	}
	s.setOfficialReport(report)
	_ = a.catalog.Normalize(s.raw, &s.settings)
	if err := a.persistDiscovered(r.Context(), s); err != nil {
		s.warnings = append(s.warnings, "檢驗資料已取得，但新項目設定未能同步；請稍後重新取得資料。")
	}
	a.json(w, 200, a.currentResponse(s))
}
func (a *App) currentReports(w http.ResponseWriter, r *http.Request, s *session) {
	if err := a.refreshSettings(r.Context(), s); err != nil {
		a.fail(w, r, 500, "settings_read", "無法讀取個人設定。")
		return
	}
	a.json(w, 200, a.currentResponse(s))
}
func (a *App) importReports(w http.ResponseWriter, r *http.Request, s *session) {
	const maxBytes = 12 << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+(64<<10))
	reader, err := r.MultipartReader()
	if err != nil {
		a.fail(w, r, 400, "invalid_upload", "請選擇HTML或ZIP檔案。")
		return
	}
	var data []byte
	var filename string
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			a.fail(w, r, 400, "upload_read", "檔案讀取失敗或超過12 MiB。")
			return
		}
		if part.FormName() != "file" || filename != "" {
			_ = part.Close()
			a.fail(w, r, 400, "upload_fields", "一次只能匯入一個檔案。")
			return
		}
		filename = part.FileName()
		data, err = io.ReadAll(io.LimitReader(part, maxBytes+1))
		_ = part.Close()
		if err != nil || len(data) > maxBytes {
			a.fail(w, r, 413, "upload_size", "檔案須小於12 MiB。")
			return
		}
	}
	if filename == "" || len(data) == 0 {
		a.fail(w, r, 400, "empty_upload", "請選擇非空白的HTML或ZIP檔案。")
		return
	}
	rows, warnings, err := lab.ParseImport(filename, data)
	if err != nil {
		a.fail(w, r, 400, "import_format", err.Error())
		return
	}
	// An imported file has no verified ownership. Use an isolated temporary
	// configuration instead of attaching it to the currently signed-in person.
	s.clear()
	s.mode = "import"
	s.raw = rows
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
	a.json(w, 200, out)
}
func (a *App) saveSettings(w http.ResponseWriter, r *http.Request, s *session) {
	// Known definitions are server-owned. The browser cannot manufacture a
	// profile or submit arbitrary JSON for persistent storage.
	var input struct {
		Revision  int64                   `json:"revision"`
		Theme     string                  `json:"theme"`
		FontScale float64                 `json:"font_scale"`
		Favorites []string                `json:"favorites"`
		Overrides map[string]lab.Override `json:"overrides"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		a.fail(w, r, 400, "invalid_settings", "設定格式不正確。")
		return
	}
	if err := a.refreshSettings(r.Context(), s); err != nil {
		a.fail(w, r, 500, "settings_read", "無法讀取個人設定。")
		return
	}
	next := lab.Settings{Revision: input.Revision, Theme: input.Theme, FontScale: input.FontScale, Favorites: input.Favorites, Overrides: input.Overrides, Known: s.settings.Known}
	if next.Favorites == nil {
		next.Favorites = []string{}
	}
	if next.Overrides == nil {
		next.Overrides = map[string]lab.Override{}
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
	d := json.NewDecoder(strings.NewReader(string(data)))
	d.DisallowUnknownFields()
	if err := d.Decode(&s); err != nil {
		return s, err
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
