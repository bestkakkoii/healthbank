// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package server

import (
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/bestkakkoii/healthbank/internal/store"
)

// Environment values describe the actual operator, not the open-source author.
// Missing deployment details remain visibly missing instead of invented assurances.
func (a *App) privacyInfo(w http.ResponseWriter, r *http.Request) {
	value := func(name string) string { return strings.TrimSpace(os.Getenv(name)) }
	a.json(w, http.StatusOK, map[string]any{
		"operator": value("HEALTHBANK_OPERATOR_NAME"), "contact": value("HEALTHBANK_PRIVACY_CONTACT"),
		"processing_region": value("HEALTHBANK_PROCESSING_REGION"), "backup_retention": value("HEALTHBANK_BACKUP_RETENTION"),
		"session_idle_minutes": int(sessionLifetime.Minutes()), "cleanup_interval_seconds": 60,
		"privacy_revision": "2026-10-28", "settings_retention": "保留至使用者刪除或營運者依已告知政策刪除；備份期限另列。",
	})
}

// Only this verified subject's configuration mutations wait for deletion.
// Revocation cancels/removes old sessions without blocking unrelated people.
func (a *App) deleteSettings(w http.ResponseWriter, r *http.Request, current *session) {
	if current.subject == "" || current.mode != "official" {
		a.fail(w, r, 401, "login_required", "刪除同步設定需要先完成官方身分驗證。")
		return
	}
	var input struct {
		Revision int64 `json:"revision"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		a.fail(w, r, 400, "invalid_settings", "刪除要求格式不正確。")
		return
	}
	unlock, err := a.lockSubject(r.Context(), current)
	if err != nil {
		a.subjectAdmissionError(w, r, current)
		return
	}
	defer unlock()
	if err := a.refreshSettings(r.Context(), current); err != nil {
		a.fail(w, r, 500, "settings_read", "無法讀取設定以確認刪除範圍。")
		return
	}
	if input.Revision != current.settings.Revision {
		a.fail(w, r, 409, "settings_conflict", "設定已由另一裝置更新，請重新讀取後再刪除。")
		return
	}
	subject := current.subject
	if input.Revision > 0 {
		if err := a.store.Delete(r.Context(), subject, input.Revision); err != nil {
			if errors.Is(err, store.ErrConflict) {
				a.fail(w, r, 409, "settings_conflict", "設定已更新，未執行刪除。")
			} else {
				a.fail(w, r, 500, "settings_delete", "無法刪除設定；請聯絡營運者並提供查詢代碼。")
			}
			return
		}
	}
	a.revokeSubject(current)
	a.clearSession(current)
	if err := a.rotate(w, r, current); err != nil {
		a.fail(w, r, 500, "session_rotate", "設定已刪除，但無法建立新的匿名使用階段；請重新載入。")
		return
	}
	a.json(w, http.StatusOK, a.sessionView(current))
}
