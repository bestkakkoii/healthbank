// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package server

import (
	"context"
	"github.com/bestkakkoii/healthbank/internal/lab"
	"net/http"
	"strings"
	"testing"
)

func TestAnalysisSettingsSynchronizeWithoutBirthdays(t *testing.T) {
	app, _, _ := testApp(t)
	first := &browserFixture{app: app}
	first.refresh(t)
	attachVerifiedFixture(t, first, "SYNTHETIC_PROFILE_ANALYSIS", nil)
	view := first.refresh(t)
	payload := settingsPayload(view.Settings)
	payload["analysis"] = map[string]any{"mode": "primary", "primary": map[string]any{"age": 70, "sex": "male"}, "comparison": map[string]any{"age": 60, "sex": "female"}}
	saved := decodeResponse[lab.Settings](t, first.jsonRequest(t, http.MethodPut, "/api/settings", payload), 200)
	second := &browserFixture{app: app}
	second.refresh(t)
	attachVerifiedFixture(t, second, "SYNTHETIC_PROFILE_ANALYSIS", nil)
	other := second.refresh(t)
	if other.Settings.Analysis == nil || *other.Settings.Analysis.Primary.Age != 70 {
		t.Fatal("cross-device scenario preference lost")
	}
	payload = settingsPayload(saved)
	payload["analysis"] = map[string]any{"mode": "primary", "primary": map[string]any{"age": 70, "sex": "male", "birth_date": "1956-01-01"}, "comparison": map[string]any{"age": 60, "sex": "female"}}
	if response := first.jsonRequest(t, http.MethodPut, "/api/settings", payload); response.Code != 400 {
		t.Fatal("birthday was accepted in persistent settings")
	}
	payload["analysis"] = map[string]any{"mode": "primary", "primary": map[string]any{"age": 131, "sex": "male"}, "comparison": map[string]any{"age": 60, "sex": "female"}}
	if response := first.jsonRequest(t, http.MethodPut, "/api/settings", payload); response.Code != 400 {
		t.Fatal("out-of-bound age accepted")
	}
}
func TestDeleteSettingsInvalidatesAllSubjectSessions(t *testing.T) {
	app, db, _ := testApp(t)
	first := &browserFixture{app: app}
	first.refresh(t)
	subject := attachVerifiedFixture(t, first, "SYNTHETIC_DELETE", nil)
	second := &browserFixture{app: app}
	second.refresh(t)
	attachVerifiedFixture(t, second, "SYNTHETIC_DELETE", nil)
	unrelated := &browserFixture{app: app}
	unrelated.refresh(t)
	attachVerifiedFixture(t, unrelated, "SYNTHETIC_OTHER", nil)
	view := first.refresh(t)
	payload := settingsPayload(view.Settings)
	payload["theme"] = "dark"
	saved := decodeResponse[lab.Settings](t, first.jsonRequest(t, http.MethodPut, "/api/settings", payload), 200)
	oldCookie, oldCSRF := *second.cookie, second.csrf
	if response := first.jsonRequest(t, http.MethodDelete, "/api/settings", map[string]any{"revision": saved.Revision - 1}); response.Code != 409 {
		t.Fatal("stale delete did not conflict")
	}
	deleted := decodeResponse[sessionResponse](t, first.jsonRequest(t, http.MethodDelete, "/api/settings", map[string]any{"revision": saved.Revision}), 200)
	if deleted.Authenticated || deleted.Mode != "anonymous" {
		t.Fatal("deleting session remains authenticated")
	}
	stale := &browserFixture{app: app, cookie: &oldCookie, csrf: oldCSRF}
	if response := stale.request(http.MethodPut, "/api/settings", strings.NewReader(`{}`), "application/json", oldCSRF); response.Code == 200 {
		t.Fatal("old session can write after deletion")
	}
	snapshot, err := db.Export(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range snapshot.Entries {
		if entry.SubjectKey == subject {
			t.Fatal("deleted subject resurrected")
		}
	}
	if !unrelated.refresh(t).Authenticated {
		t.Fatal("unrelated profile logged out")
	}
}
