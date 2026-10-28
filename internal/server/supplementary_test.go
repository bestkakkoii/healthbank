// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/bestkakkoii/healthbank/internal/nhi"
)

func TestSupplementaryRequiresOfficialIdentityAndKnownDetailCategory(t *testing.T) {
	app, db, _ := testApp(t)
	browser := &browserFixture{app: app}
	browser.refresh(t)
	for _, category := range []string{"medicine", "chronic", "western", "chinese", "dental", "procedure", "hospital", "recent_medicine", "recent_chronic", "recent_procedure", "recent_hospital", "self_paid", "imaging", "official_files", "official_export_html", "official_export_json", "official_export_xml", "official_export_pdf"} {
		response := browser.request(http.MethodGet, "/api/supplementary?category="+category, nil, "", "")
		if response.Code != http.StatusUnauthorized {
			t.Fatal("anonymous session accessed official supplementary data")
		}
	}
	// A synthetic application identity with a fresh, unauthenticated connector
	// allows checking routing without making any real network/authentication call.
	app.mu.Lock()
	s := app.sessions[browser.cookie.Value]
	app.mu.Unlock()
	if s == nil {
		t.Fatal("missing synthetic application session")
	}
	connector, err := nhi.New()
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	app.bindSubject(s, "synthetic-subject")
	s.mode, s.official = "official", connector
	s.mu.Unlock()
	for _, category := range []string{"", "adult", "official_export_json", "unknown"} {
		response := browser.request(http.MethodGet, "/api/supplementary?category="+category+"&detail="+strings.Repeat("a", 48), nil, "", "")
		var envelope struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if response.Code != http.StatusBadRequest || envelope.Error.Code != "invalid_detail" {
			t.Fatalf("unsupported detail category %q entered a connector: status=%d code=%q", category, response.Code, envelope.Error.Code)
		}
		if response.Header().Get("Cache-Control") != "no-store, private" || response.Header().Get("Pragma") != "no-cache" {
			t.Fatal("medical error response lost no-cache policy")
		}
	}
	noPersistentProfiles(t, db)
}

func TestRecentSupplementaryDetailRoutesRequireConnectorAuthentication(t *testing.T) {
	for _, category := range []string{"recent_medicine", "recent_chronic", "recent_procedure", "recent_hospital"} {
		t.Run(category, func(t *testing.T) {
			app, db, _ := testApp(t)
			browser := &browserFixture{app: app}
			browser.refresh(t)
			connector, err := nhi.New()
			if err != nil {
				t.Fatal(err)
			}
			app.mu.Lock()
			s := app.sessions[browser.cookie.Value]
			app.mu.Unlock()
			if s == nil {
				t.Fatal("missing synthetic application session")
			}
			s.mu.Lock()
			app.bindSubject(s, "synthetic-subject")
			s.mode, s.official = "official", connector
			s.mu.Unlock()
			// This category now supports source-bound details. The fresh connector
			// has no upstream token, so dispatch must stop locally at authentication,
			// without pretending that a recent summary has no detail capability.
			response := browser.request(http.MethodGet, "/api/supplementary?category="+category+"&detail="+strings.Repeat("a", 48), nil, "", "")
			envelope := decodeResponse[struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}](t, response, http.StatusUnauthorized)
			if envelope.Error.Code != "SESSION_EXPIRED" {
				t.Fatalf("supported recent detail failed to reach the authentication boundary: %q", envelope.Error.Code)
			}
			if response.Header().Get("Cache-Control") != "no-store, private" || response.Header().Get("Pragma") != "no-cache" {
				t.Fatal("recent detail authentication response lost no-cache policy")
			}
			noPersistentProfiles(t, db)
		})
	}
}
