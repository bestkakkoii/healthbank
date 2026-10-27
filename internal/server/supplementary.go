// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package server

import (
	"context"
	"errors"
	"github.com/bestkakkoii/healthbank/internal/nhi"
	"net/http"
	"time"
)

func (a *App) supplementary(w http.ResponseWriter, r *http.Request, s *session) {
	if s.mode != "official" || s.official == nil || s.subject == "" {
		a.fail(w, r, 401, "login_required", "請先完成官方登入；匯入與示範資料不能驗證身分。")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 110*time.Second)
	defer cancel()
	var report nhi.SupplementReport
	var err error
	if reference := r.URL.Query().Get("detail"); reference != "" {
		if len(reference) != 48 {
			a.fail(w, r, 400, "invalid_detail", "請由目前自費健檢清單開啟明細。")
			return
		}
		if r.URL.Query().Get("category") == "western" {
			report, err = s.official.FetchWesternDetail(ctx, reference)
		} else {
			report, err = s.official.FetchSelfPaidDetail(ctx, reference)
		}
	} else {
		report, err = s.official.FetchSupplement(ctx, r.URL.Query().Get("category"), nhi.Query{StartDate: r.URL.Query().Get("from"), EndDate: r.URL.Query().Get("to")})
	}
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
	// Results are returned to this browser only. They never enter report/settings
	// storage, discovery, logs, or another category's successful result.
	a.json(w, http.StatusOK, report)
}
