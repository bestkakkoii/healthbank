// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package server

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/bestkakkoii/healthbank/internal/lab"
)

func TestSessionCookieUsesServerEnforcedIdleLifetime(t *testing.T) {
	app, db, _ := testApp(t)
	browser := &browserFixture{app: app}
	browser.refresh(t)
	if browser.cookie.MaxAge != 0 || !browser.cookie.Expires.IsZero() {
		t.Fatal("active sessions must not expire at a fixed browser-cookie age")
	}
	subject := attachVerifiedFixture(t, browser, "SYNTHETIC_IDLE_SUBJECT", []lab.RawRecord{{ItemName: "ALT", Result: "17.125"}})
	before, err := db.Get(context.Background(), subject)
	if err != nil {
		t.Fatal(err)
	}
	oldCookie := *browser.cookie
	app.mu.Lock()
	s := app.sessions[oldCookie.Value]
	s.touched = time.Now().Add(-sessionLifetime + time.Minute)
	app.mu.Unlock()
	active := browser.request(http.MethodGet, "/api/reports/current", nil, "", "")
	if got := decodeResponse[reportResponse](t, active, http.StatusOK); len(got.Records) != 1 {
		t.Fatal("unexpired active session lost its report")
	}
	if len(active.Header().Values("Set-Cookie")) != 0 {
		t.Fatal("ordinary response can reissue a retired cookie generation")
	}
	app.mu.Lock()
	if time.Since(s.touched) > time.Minute {
		app.mu.Unlock()
		t.Fatal("activity failed to renew server idle time")
	}
	s.touched = time.Now().Add(-sessionLifetime - time.Second)
	app.mu.Unlock()
	expired := decodeResponse[reportResponse](t, browser.request(http.MethodGet, "/api/reports/current", nil, "", ""), http.StatusOK)
	if len(expired.Records) != 0 || browser.cookie.Value == oldCookie.Value || browser.cookie.MaxAge != 0 {
		t.Fatal("idle expiry retained old medical data or its session identifier")
	}
	if view := browser.refresh(t); view.Authenticated || view.Mode != "anonymous" {
		t.Fatal("idle expiry retained verified identity")
	}
	after, err := db.Get(context.Background(), subject)
	if err != nil || before.Revision != after.Revision || string(before.Settings) != string(after.Settings) {
		t.Fatal("idle expiry changed the independently retained configuration")
	}
}
