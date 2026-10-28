// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package server

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bestkakkoii/healthbank/internal/nhi"
)

func validDocumentReference(reference string) bool {
	if len(reference) != 48 {
		return false
	}
	_, err := hex.DecodeString(reference)
	return err == nil
}

func (a *App) documents(w http.ResponseWriter, r *http.Request, s *session) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		a.fail(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "文件下載請使用 GET。")
		return
	}
	if s.mode != "official" || s.official == nil || s.subject == "" {
		a.fail(w, r, 401, "login_required", "請先完成官方登入，再取得個人文件。")
		return
	}
	category, reference := r.URL.Query().Get("category"), r.URL.Query().Get("id")
	if !validDocumentReference(reference) {
		a.fail(w, r, 400, "invalid_document", "請由目前資料明細中的文件按鈕下載。")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), nhi.DocumentDownloadBudget)
	defer cancel()
	started := false
	err := s.official.DownloadDocument(ctx, category, reference, func(info nhi.DocumentInfo, reader io.Reader) error {
		// A long source archive must not inherit the shorter JSON write deadline.
		// The whole transfer still has a bounded context and size, and cancellation
		// closes the upstream response without creating a spool file.
		if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(nhi.DocumentDownloadBudget + 5*time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
			return err
		}
		w.Header().Set("Content-Type", info.ContentType)
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": info.Name}))
		w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; frame-ancestors 'none'")
		if info.Size >= 0 {
			w.Header().Set("Content-Length", strconv.FormatInt(info.Size, 10))
		}
		if len(info.Warnings) != 0 {
			w.Header().Set("X-Healthbank-Document-Warning", strings.Join(info.Warnings, ","))
		}
		started = true
		_, err := io.CopyBuffer(w, reader, make([]byte, 32768))
		return err
	})
	if err == nil {
		return
	}
	if started {
		// Do not append JSON to a partially written patient document or report a
		// successful truncated download. Log only a bounded safe error code.
		code := "document_transfer_failed"
		var upstream *nhi.Error
		if errors.As(err, &upstream) {
			code = upstream.Code
		}
		slog.Warn("document transfer interrupted", "code", code)
		panic(http.ErrAbortHandler)
	}
	a.documentError(w, r, s, err)
}

func (a *App) prepareDocument(w http.ResponseWriter, r *http.Request, s *session) {
	if s.mode != "official" || s.official == nil || s.subject == "" {
		a.fail(w, r, 401, "login_required", "請先完成官方登入，再要求官方準備文件。")
		return
	}
	var input struct {
		Category string `json:"category"`
		ID       string `json:"id"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		a.fail(w, r, 400, "invalid_document", "文件產製請求格式不正確。")
		return
	}
	_, exportCategory := nhi.OfficialExportCategory(input.Category)
	if !validDocumentReference(input.ID) || (input.Category != "imaging" && input.Category != "osteoporosis" && !exportCategory) {
		a.fail(w, r, 400, "invalid_document", "請由目前資料明細提出檔案產製請求。")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	result, err := s.official.PrepareDocument(ctx, input.Category, input.ID)
	if err != nil {
		a.documentError(w, r, s, err)
		return
	}
	a.json(w, http.StatusAccepted, result)
}

func (a *App) documentError(w http.ResponseWriter, r *http.Request, s *session, err error) {
	var upstream *nhi.Error
	if errors.As(err, &upstream) && upstream.Code == "SESSION_EXPIRED" {
		s.clear()
		if rotateErr := a.rotate(w, r, s); rotateErr != nil {
			a.fail(w, r, 500, "session_rotate", "連線已結束，請重新整理頁面。")
			return
		}
	}
	a.upstreamError(w, r, err)
}
