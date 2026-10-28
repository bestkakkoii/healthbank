// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bestkakkoii/healthbank/internal/lab"
)

func TestSettingsDeletionDoesNotBlockUnrelatedSessions(t *testing.T) {
	app, _, _ := testApp(t)
	browsers := make([]*browserFixture, 3)
	for i, identity := range []string{"SYNTHETIC_LONG_OPERATION", "SYNTHETIC_DELETION", "SYNTHETIC_UNRELATED"} {
		browsers[i] = &browserFixture{app: app}
		browsers[i].refresh(t)
		attachVerifiedFixture(t, browsers[i], identity, nil)
	}
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	free := func() { once.Do(func() { close(release) }) }
	defer free()
	handler := app.withSession(func(w http.ResponseWriter, r *http.Request, _ *session) {
		close(entered)
		select {
		case <-release:
		case <-r.Context().Done():
			t.Error("unrelated subject's long operation was canceled")
		}
		w.WriteHeader(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodGet, "https://healthbank.test/api/documents", nil)
	request.AddCookie(browsers[0].cookie)
	go func() { handler(httptest.NewRecorder(), request); close(done) }()
	<-entered
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		unauthenticated := (&browserFixture{app: app}).request(http.MethodDelete, "/api/settings", strings.NewReader(`{"revision":0}`), "application/json", "")
		if unauthenticated.Code != http.StatusForbidden && unauthenticated.Code != http.StatusConflict {
			t.Error("unauthenticated deletion did not reject before coordination")
		}
		deleted := browsers[1].jsonRequest(t, http.MethodDelete, "/api/settings", map[string]int{"revision": 0})
		if deleted.Code != http.StatusOK {
			t.Errorf("verified deletion status=%d", deleted.Code)
		}
		if response := browsers[2].request(http.MethodGet, "/api/session", nil, "", ""); response.Code != http.StatusOK {
			t.Error("unrelated session could not read settings")
		}
	}()
	select {
	case <-finished:
	case <-time.After(time.Second):
		free()
		<-finished
		t.Fatal("deletion or unrelated request waited for another person's long operation")
	}
	free()
	<-done
}

func TestSettingsDeletionCancelsSubjectWorkAndRejectsOldWrites(t *testing.T) {
	app, db, _ := testApp(t)
	first, second := &browserFixture{app: app}, &browserFixture{app: app}
	first.refresh(t)
	second.refresh(t)
	subject := attachVerifiedFixture(t, first, "SYNTHETIC_SHARED_DELETE", nil)
	attachVerifiedFixture(t, second, "SYNTHETIC_SHARED_DELETE", nil)
	view := first.refresh(t)
	saved := decodeResponse[lab.Settings](t, first.jsonRequest(t, http.MethodPut, "/api/settings", settingsPayload(view.Settings)), http.StatusOK)
	entered, canceled, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	handler := app.withSession(func(w http.ResponseWriter, r *http.Request, s *session) {
		close(entered)
		select {
		case <-r.Context().Done():
			close(canceled)
		case <-time.After(2 * time.Second):
			t.Error("deleted subject's active operation was not canceled")
		}
		s.settings.Known["late"] = lab.KnownItem{ID: "late", Key: "late", Name: "合成晚到項目", Unit: "unit", Category: "other"}
		if err := app.persistDiscovered(context.Background(), s); !errors.Is(err, errSubjectRevoked) {
			t.Errorf("late discovery was not rejected as revoked: %v", err)
		}
		if err := app.rotate(w, r, s); !errors.Is(err, errSubjectRevoked) {
			t.Errorf("late login rotation could revive a deleted subject: %v", err)
		}
		if app.requireCurrentSubject(w, r, s) {
			t.Error("revoked subject could publish a successful old response")
		}
	})
	request := httptest.NewRequest(http.MethodGet, "https://healthbank.test/api/supplementary", nil)
	request.AddCookie(second.cookie)
	go func() { handler(httptest.NewRecorder(), request); close(finished) }()
	<-entered
	deleted := first.jsonRequest(t, http.MethodDelete, "/api/settings", map[string]int64{"revision": saved.Revision})
	if deleted.Code != http.StatusOK {
		t.Fatalf("deletion status=%d", deleted.Code)
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("active subject operation remained alive after deletion")
	}
	<-finished
	old := second.request(http.MethodPut, "/api/settings", strings.NewReader(`{}`), "application/json", second.csrf)
	if old.Code != http.StatusConflict || len(old.Header().Values("Set-Cookie")) != 0 {
		t.Fatal("retired session wrote settings or replaced the current cookie")
	}
	document, err := db.Get(context.Background(), subject)
	if err != nil || document.Revision != 0 {
		t.Fatal("late operation recreated deleted settings")
	}
}
