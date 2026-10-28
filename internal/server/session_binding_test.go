// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bestkakkoii/healthbank/internal/lab"
)

func TestStatefulGETRejectsAnotherTabsStaleGeneration(t *testing.T) {
	app, _, _ := testApp(t)
	browser := &browserFixture{app: app}
	initial := browser.refresh(t)
	// The browser cookie is replaced by a legitimate operation in another tab;
	// an old tab still has its earlier CSRF/generation marker in memory.
	demo := decodeResponse[reportResponse](t, browser.jsonRequest(t, http.MethodPost, "/api/reports/demo", map[string]any{}), http.StatusOK)
	current := browser.refresh(t)
	if demo.SessionToken == "" || demo.SessionToken != current.CSRFToken || demo.SessionToken == initial.CSRFToken {
		t.Fatal("mode-changing report does not identify its new session generation")
	}
	for _, path := range []string{"/api/reports/current", "/api/catalog", "/api/reports", "/api/supplementary?category=medicine", "/api/documents?category=hospital&id=" + strings.Repeat("a", 48), "/api/auth/captcha"} {
		request := httptest.NewRequest(http.MethodGet, "https://healthbank.test"+path, nil)
		request.AddCookie(browser.cookie)
		request.Header.Set("X-CSRF-Token", initial.CSRFToken)
		response := httptest.NewRecorder()
		app.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"code":"session_changed"`) || strings.Contains(response.Body.String(), `"records"`) || len(response.Header().Values("Set-Cookie")) != 0 {
			t.Fatal("stale tab read a replacement generation or changed the shared cookie")
		}
	}
	request := httptest.NewRequest(http.MethodGet, "https://healthbank.test/api/session", nil)
	request.AddCookie(browser.cookie)
	request.Header.Set("X-CSRF-Token", initial.CSRFToken)
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	resynced := decodeResponse[sessionResponse](t, response, http.StatusOK)
	if resynced.CSRFToken != current.CSRFToken || resynced.Mode != "demo" {
		t.Fatal("explicit session reconciliation changed or could not inspect the current generation")
	}
	for _, token := range []string{"", current.CSRFToken} {
		request := httptest.NewRequest(http.MethodGet, "https://healthbank.test/api/reports/current", nil)
		request.AddCookie(browser.cookie)
		if token != "" {
			request.Header.Set("X-CSRF-Token", token)
		}
		response := httptest.NewRecorder()
		app.Handler().ServeHTTP(response, request)
		got := decodeResponse[reportResponse](t, response, http.StatusOK)
		if got.SessionToken != current.CSRFToken || len(got.Records) != len(demo.Records) {
			t.Fatal("valid or initial untokened read lost the current report")
		}
	}
}

func TestConsecutiveImportsIdentifyDistinctSameModeGenerations(t *testing.T) {
	app, _, _ := testApp(t)
	browser := &browserFixture{app: app}
	browser.refresh(t)
	var first reportResponse
	for index, value := range []string{"17.125", "31.500"} {
		payload := []byte(`{"schema":"healthbank.lab.v1","records":[{"item_name":"合成檢驗","result":"` + value + `"}]}`)
		body, contentType := importMultipart(t, "synthetic.json", bytes.NewReader(payload))
		result := decodeResponse[reportResponse](t, browser.request(http.MethodPost, "/api/reports/import", body, contentType, browser.csrf), http.StatusOK)
		view := browser.refresh(t)
		if view.Mode != "import" || result.SessionToken == "" || result.SessionToken != view.CSRFToken || len(result.Records) != 1 || result.Records[0].Result != value {
			t.Fatal("import report lacks an exact new-generation binding")
		}
		if index == 0 {
			first = result
		} else if result.SessionToken == first.SessionToken {
			t.Fatal("same-mode imports reused a generation marker")
		}
	}
	// Reading ordinary settings must not rotate the report's marker.
	view := browser.refresh(t)
	decodeResponse[lab.Settings](t, browser.jsonRequest(t, http.MethodPut, "/api/settings", settingsPayload(view.Settings)), http.StatusOK)
	got := decodeResponse[reportResponse](t, browser.request(http.MethodGet, "/api/reports/current", nil, "", ""), http.StatusOK)
	if got.SessionToken != view.CSRFToken {
		t.Fatal("ordinary preference updates changed the report generation")
	}
}

func TestRetiredCookieWithKnownGenerationCannotReplaceCurrentCookie(t *testing.T) {
	app, _, _ := testApp(t)
	browser := &browserFixture{app: app}
	initial := browser.refresh(t)
	retiredCookie := *browser.cookie
	demo := decodeResponse[reportResponse](t, browser.jsonRequest(t, http.MethodPost, "/api/reports/demo", map[string]any{}), http.StatusOK)
	current := browser.refresh(t)
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		path := "/api/reports/current"
		if method == http.MethodPost {
			path = "/api/reports/demo"
		}
		// This request was sent with the previous cookie but reaches the server
		// only after another tab has completed its rotation. Its late response
		// must not overwrite the shared browser's current authentication cookie.
		request := httptest.NewRequest(method, "https://healthbank.test"+path, strings.NewReader(`{}`))
		request.AddCookie(&retiredCookie)
		request.Header.Set("X-CSRF-Token", initial.CSRFToken)
		response := httptest.NewRecorder()
		app.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), `"code":"session_changed"`) || len(response.Header().Values("Set-Cookie")) != 0 {
			t.Fatal("a delayed known-generation request can overwrite the current shared browser cookie")
		}
	}
	view := browser.refresh(t)
	got := decodeResponse[reportResponse](t, browser.request(http.MethodGet, "/api/reports/current", nil, "", ""), http.StatusOK)
	if view.Mode != "demo" || view.CSRFToken != current.CSRFToken || got.SessionToken != current.CSRFToken || len(got.Records) != len(demo.Records) {
		t.Fatal("rejecting a retired generation changed the active session")
	}
}

func TestRetiredCookieNativeDocumentCannotReplaceCurrentCookie(t *testing.T) {
	app, _, _ := testApp(t)
	browser := &browserFixture{app: app}
	browser.refresh(t)
	retiredCookie := *browser.cookie
	demo := decodeResponse[reportResponse](t, browser.jsonRequest(t, http.MethodPost, "/api/reports/demo", map[string]any{}), http.StatusOK)
	current := browser.refresh(t)
	// A native attachment link has no custom generation header. Its opaque
	// reference prevents cross-user access, and its delayed rejection must also
	// leave the shared browser's newer cookie intact.
	request := httptest.NewRequest(http.MethodGet, "https://healthbank.test/api/documents?category=hospital&id="+strings.Repeat("a", 48), nil)
	request.AddCookie(&retiredCookie)
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), `"code":"login_required"`) || len(response.Header().Values("Set-Cookie")) != 0 {
		t.Fatal("a delayed native document request can overwrite the current shared browser cookie")
	}
	view := browser.refresh(t)
	got := decodeResponse[reportResponse](t, browser.request(http.MethodGet, "/api/reports/current", nil, "", ""), http.StatusOK)
	if view.Mode != "demo" || view.CSRFToken != current.CSRFToken || got.SessionToken != current.CSRFToken || len(got.Records) != len(demo.Records) {
		t.Fatal("rejecting a retired native document request changed the active session")
	}
}
