// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/bestkakkoii/healthbank/internal/nhi"
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
	category := r.URL.Query().Get("category")
	if reference := r.URL.Query().Get("detail"); reference != "" {
		if len(reference) != 48 {
			a.fail(w, r, 400, "invalid_detail", "請由目前所選類別清單開啟明細。")
			return
		}
		if nhi.MedicalCategory(category) {
			report, err = s.official.FetchMedicalDetail(ctx, category, reference)
		} else if category == "self_paid" {
			report, err = s.official.FetchSelfPaidDetail(ctx, reference)
		} else {
			a.fail(w, r, 400, "invalid_detail", "此資料類別未提供明細，請由原清單開啟。")
			return
		}
	} else if nhi.MedicalCategory(category) {
		report, err = s.official.FetchMedical(ctx, category, nhi.Query{StartDate: r.URL.Query().Get("from"), EndDate: r.URL.Query().Get("to")})
	} else {
		report, err = s.official.FetchSupplement(ctx, category, nhi.Query{StartDate: r.URL.Query().Get("from"), EndDate: r.URL.Query().Get("to")})
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
