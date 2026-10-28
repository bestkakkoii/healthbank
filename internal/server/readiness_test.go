// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bestkakkoii/healthbank/internal/config"
	"github.com/bestkakkoii/healthbank/internal/lab"
	"github.com/bestkakkoii/healthbank/internal/store"
)

func TestReadinessRequiresDirectLocalOperatorAndExposesNoProfile(t *testing.T) {
	a, db, _ := testApp(t)
	b := &browserFixture{app: a}
	b.refresh(t)
	subject := attachVerifiedFixture(t, b, "SYNTHETIC_READINESS_PRIVATE", []lab.RawRecord{{ItemName: "ALT", Result: "17.125", Unit: "U/L"}})
	for _, tc := range []struct {
		name, host, peer, header, value string
		status                          int
	}{
		{"local", "127.0.0.1:8787", "127.0.0.1:4321", "", "", 200},
		{"ipv6", "[::1]:8787", "[::1]:4321", "", "", 200},
		{"remote", "127.0.0.1:8787", "192.0.2.1:4321", "", "", 404},
		{"public_host", "healthbank.example", "127.0.0.1:4321", "", "", 404},
		{"forwarded", "localhost:8787", "127.0.0.1:4321", "X-Forwarded-Proto", "https", 404},
		{"proxy", "localhost:8787", "127.0.0.1:4321", "Forwarded", "for=192.0.2.1", 404},
		{"real_ip", "localhost:8787", "127.0.0.1:4321", "X-Real-IP", "192.0.2.1", 404},
		{"browser_origin", "localhost:8787", "127.0.0.1:4321", "Origin", "https://healthbank.example", 404},
		{"cross_site", "localhost:8787", "127.0.0.1:4321", "Sec-Fetch-Site", "cross-site", 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "http://"+tc.host+"/api/ready", nil)
			r.RemoteAddr = tc.peer
			if tc.header != "" {
				r.Header.Set(tc.header, tc.value)
			}
			w := httptest.NewRecorder()
			a.Handler().ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("unexpected readiness status %d", w.Code)
			}
			if len(w.Result().Cookies()) != 0 {
				t.Fatal("operator probe created a session")
			}
			for _, private := range []string{subject, config.KeyCheck(a.secret), "SYNTHETIC_READINESS_PRIVATE", "profile_key", "key_check"} {
				if strings.Contains(w.Body.String(), private) {
					t.Fatal("readiness exposed identifying metadata")
				}
			}
			if tc.status == 200 {
				var body map[string]json.RawMessage
				if json.Unmarshal(w.Body.Bytes(), &body) != nil || string(body["identity_key_origin"]) != `"legacy_unverified"` || string(body["identity_key_status"]) != `"unverified"` {
					t.Fatal("legacy provenance was falsely verified")
				}
			}
		})
	}
	if err := db.BindIdentityKey(context.Background(), config.KeyCheck(a.secret), true); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "http://localhost:8787/api/ready", nil)
	r.RemoteAddr = "127.0.0.1:4321"
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	if !strings.Contains(w.Body.String(), store.IdentityKeyLegacyConfirmed) {
		t.Fatal("readiness lost operator-confirmed provenance")
	}
}

func TestReadinessDetectsUnavailableDatabaseWhileLivenessRemains(t *testing.T) {
	a, db, _ := testApp(t)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "http://localhost:8787/api/ready", nil)
	r.RemoteAddr = "127.0.0.1:4321"
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "settings_unavailable") || strings.Contains(w.Body.String(), "sql:") {
		t.Fatal("readiness hid database failure or leaked raw driver diagnostics")
	}
	health := httptest.NewRecorder()
	a.Handler().ServeHTTP(health, httptest.NewRequest(http.MethodGet, "http://localhost:8787/api/health", nil))
	if health.Code != http.StatusOK {
		t.Fatal("liveness incorrectly became a database check")
	}
}
