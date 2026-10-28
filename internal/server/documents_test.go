// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/bestkakkoii/healthbank/internal/nhi"
)

func TestDocumentEndpointsRequireIdentityCSRFAndOpaqueReferences(t *testing.T) {
	app, db, _ := testApp(t)
	browser := &browserFixture{app: app}
	browser.refresh(t)
	response := browser.request(http.MethodGet, "/api/documents?category=imaging&id="+strings.Repeat("a", 48), nil, "", "")
	if response.Code != http.StatusUnauthorized || response.Header().Get("Cache-Control") != "no-store, private" {
		t.Fatal("anonymous document access or missing privacy headers")
	}
	response = browser.jsonRequest(t, http.MethodPost, "/api/documents/prepare", map[string]string{"category": "imaging", "id": strings.Repeat("a", 48)})
	if response.Code != http.StatusUnauthorized {
		t.Fatal("anonymous user prepared official file")
	}
	connector, err := nhi.New()
	if err != nil {
		t.Fatal(err)
	}
	app.mu.Lock()
	s := app.sessions[browser.cookie.Value]
	app.mu.Unlock()
	s.mu.Lock()
	app.bindSubject(s, "synthetic-subject")
	s.mode, s.official = "official", connector
	s.mu.Unlock()
	response = browser.request(http.MethodPost, "/api/documents/prepare", strings.NewReader(`{"category":"imaging","id":"`+strings.Repeat("a", 48)+`"}`), "application/json", "")
	if response.Code != http.StatusForbidden {
		t.Fatal("document generation bypassed CSRF")
	}
	for _, id := range []string{"https://example.invalid/patient.pdf", strings.Repeat("z", 48), "short"} {
		response = browser.request(http.MethodGet, "/api/documents?category=imaging&id="+id, nil, "", "")
		if response.Code != http.StatusBadRequest {
			t.Fatal("invalid document reference accepted")
		}
	}
	response = browser.request(http.MethodGet, "/api/documents?category=imaging&id="+strings.Repeat("a", 48), nil, "", "")
	if response.Code != http.StatusBadRequest || strings.Contains(response.Body.String(), "synthetic-subject") {
		t.Fatal("unlisted document reference reached official network or leaked subject")
	}
	response = browser.jsonRequest(t, http.MethodPost, "/api/documents/prepare", map[string]string{"category": "imaging", "id": strings.Repeat("a", 48), "url": "https://example.invalid"})
	if response.Code != http.StatusBadRequest {
		t.Fatal("caller supplied an upstream document URL")
	}
	for _, category := range []string{"western", "annual_overview"} {
		response = browser.jsonRequest(t, http.MethodPost, "/api/documents/prepare", map[string]string{"category": category, "id": strings.Repeat("a", 48)})
		body := decodeResponse[struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}](t, response, http.StatusBadRequest)
		if body.Error.Code != "INVALID_INPUT" {
			t.Fatal("nested imaging owner was rejected before its exact action capability could be verified")
		}
	}
	response = browser.request(http.MethodHead, "/api/documents?category=hospital&id="+strings.Repeat("a", 48), nil, "", "")
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatal("HEAD probe initiated source artifact retrieval")
	}
	noPersistentProfiles(t, db)
}
