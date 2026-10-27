// Copyright (c) 2026 bestkakkoii llc. All rights reserved.
// SPDX-License-Identifier: MIT

package nhi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Fixtures are synthetic. Every request in these tests is routed to a local
// TLS server; neither a credential nor any test data can reach the NHI host.
type fixtureTransport struct {
	base   *url.URL
	actual http.RoundTripper
}

func (t fixtureTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.Scheme != "https" || request.URL.Host != "myhealthbank.nhi.gov.tw" {
		return nil, errors.New("fixture received an unexpected origin")
	}
	clone := request.Clone(request.Context())
	clone.URL.Scheme, clone.URL.Host = t.base.Scheme, t.base.Host
	clone.Host = t.base.Host
	return t.actual.RoundTrip(clone)
}

func fixtureSession(t *testing.T, handler http.HandlerFunc) *Session {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	base, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	session, err := New()
	if err != nil {
		t.Fatal(err)
	}
	session.client.CloseIdleConnections()
	session.client.Transport = fixtureTransport{base: base, actual: server.Client().Transport}
	t.Cleanup(session.Close)
	return session
}

func respondJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Error(err)
	}
}

func respondLoginDocument(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write([]byte("<!doctype html><html><head><title>Synthetic login</title></head><body>Synthetic fixture</body></html>")); err != nil {
		t.Error(err)
	}
}

func TestSourceDerivedFlowWithFullSearch(t *testing.T) {
	var imageBuffer bytes.Buffer
	if err := png.Encode(&imageBuffer, image.NewRGBA(image.Rect(0, 0, 2, 1))); err != nil {
		t.Fatal(err)
	}
	imageBytes := imageBuffer.Bytes()
	rows := make([]map[string]any, 52)
	for i := range rows {
		rows[i] = map[string]any{
			"reaL_INSPECT_DATE": "114/03/20", "funC_DATE": "114/03/02", "assaY_UPLOAD_DATE": "114/04/01",
			"ordeR_CODE": "TEST01", "ordeR_NAME": "Synthetic panel", "assaY_ITEM_NAME": fmt.Sprintf("Synthetic item %d", i),
			"assaY_VALUE": "大於等於30", "uniT_DATA": "test-unit", "consulT_VALUE": "[1][10]",
			"hosP_ABBR": "Synthetic institution", "orI_TYPE": "A", "assaY_MARK": 1,
		}
	}
	rows[0]["reaL_INSPECT_DATE"], rows[0]["funC_DATE"], rows[0]["assaY_UPLOAD_DATE"] = "112/03/20", "112/03/02", "112/04/01"
	var challengeID string
	var bootstrapCalls, captchaCalls, loginCalls, refreshCalls, pageLoadCalls, searchCalls, logoutCalls atomic.Int32
	session := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case loginPage:
			bootstrapCalls.Add(1)
			if r.Method != http.MethodGet || r.URL.RawQuery != "" || r.Header.Get("Accept") != "text/html" || r.Header.Get("X-Requested-With") != "" || r.Header.Get("Authorization") != "" || r.Header.Get("Origin") != "" {
				t.Error("login document was not requested as an anonymous HTML document")
			}
			if captchaCalls.Load() != 0 || loginCalls.Load() != 0 {
				t.Error("login document was repeated after the session had started")
			}
			http.SetCookie(w, &http.Cookie{Name: "fixture_bootstrap", Value: "synthetic-document", Path: "/", Secure: true, HttpOnly: true})
			respondLoginDocument(t, w)
		case captchaPath:
			captchaCalls.Add(1)
			if r.Method != http.MethodGet || r.URL.Query().Get("id") == "" || r.URL.Query().Get("r") == "" {
				t.Error("missing official CAPTCHA query")
			}
			cookie, cookieErr := r.Cookie("fixture_bootstrap")
			if bootstrapCalls.Load() != 1 || cookieErr != nil || cookie.Value != "synthetic-document" || r.Header.Get("X-Requested-With") != "" || r.Header.Get("Authorization") != "" {
				t.Error("CAPTCHA did not follow the document in the same anonymous cookie jar")
			}
			challengeID = r.URL.Query().Get("id")
			http.SetCookie(w, &http.Cookie{Name: "fixture_session", Value: "synthetic", Path: "/", Secure: true})
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(imageBytes)
		case loginPath:
			loginCalls.Add(1)
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer null" || r.Header.Get("X-Requested-With") != "XMLHttpRequest" {
				t.Error("login request does not match SPA transport")
			}
			cookie, cookieErr := r.Cookie("fixture_session")
			if cookieErr != nil || cookie.Value != "synthetic" {
				t.Error("CAPTCHA cookie not preserved")
			}
			cookie, cookieErr = r.Cookie("fixture_bootstrap")
			if cookieErr != nil || cookie.Value != "synthetic-document" {
				t.Error("login document cookie not preserved through CAPTCHA")
			}
			var form map[string]string
			if err := json.NewDecoder(r.Body).Decode(&form); err != nil {
				t.Error(err)
			}
			if len(form) != 5 || form["c_id"] != "A000000000" || form["c_no"] != "000012345678" || form["c_pw"] != "synthetic-password" || form["c_ValidCode"] != "TEST" || form["c_Codeid"] != challengeID {
				t.Error("unexpected login field transformation")
			}
			respondJSON(t, w, map[string]string{"error_code": "Z003A", "jwt_token": "synthetic.initial.token"})
		case refreshPath:
			refreshCalls.Add(1)
			if r.Header.Get("Authorization") != "Bearer synthetic.initial.token" {
				t.Error("missing original bearer token")
			}
			respondJSON(t, w, map[string]string{"error_code": "0000", "jwt_token": "synthetic.refreshed.token"})
		case pageLoadPath:
			pageLoadCalls.Add(1)
			if r.Header.Get("Authorization") != "Bearer synthetic.refreshed.token" {
				t.Error("missing refreshed bearer token")
			}
			q := r.URL.Query()
			if q.Get("s_sort") != "A1" || q.Get("s_type") != "" || q.Get("nowPage") != "1" || q.Get("pageDataCount") != "50" || q.Get("s_date") != "" || q.Get("e_date") != "" {
				t.Error("incorrect page-load query")
			}
			respondJSON(t, w, map[string]any{"sdate": "2025-03-01", "edate": "2025-12-31", "counts": "51", "sP_IHKE3409S02_Data": rows[1:51]})
		case searchPath:
			searchCalls.Add(1)
			q := r.URL.Query()
			if len(q) != 4 || q.Get("s_type") != "" || q.Get("s_sort") != "A1" || q.Get("s_date") != "2023-01-01" || q.Get("e_date") != "2025-12-31" {
				t.Error("full search must preserve the explicit historical interval and omit pagination")
			}
			if r.Header.Get("Authorization") != "Bearer synthetic.refreshed.token" {
				t.Error("full search lost the refreshed bearer token")
			}
			respondJSON(t, w, rows)
		case logoutPath:
			logoutCalls.Add(1)
			respondJSON(t, w, map[string]string{"error_code": "0000"})
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	ctx := context.Background()
	challenge, err := session.Challenge(ctx)
	if err != nil || challenge.ContentType != "image/png" || len(challenge.Image) != len(imageBytes) || challenge.ID == "" || !challenge.ExpiresAt.After(time.Now()) {
		t.Fatalf("CAPTCHA failed: %v", err)
	}
	decodedImage, format, err := image.Decode(bytes.NewReader(challenge.Image))
	if err != nil || format != "png" || decodedImage.Bounds().Dx() != 2 || decodedImage.Bounds().Dy() != 1 {
		t.Fatal("CAPTCHA bytes did not decode into the complete synthetic image")
	}
	// A user-requested replacement CAPTCHA reuses the established document
	// session instead of loading the document again or losing its cookies.
	challenge, err = session.Challenge(ctx)
	if err != nil || challenge.ID != challengeID || bootstrapCalls.Load() != 1 || captchaCalls.Load() != 2 {
		t.Fatal("manual CAPTCHA refresh recreated the document session")
	}
	err = session.Login(ctx, Credentials{NationalID: "a000000000", CardNumber: "12345678", Password: "synthetic-password", Captcha: " TEST "})
	if err != nil {
		t.Fatal(err)
	}
	session.tokenObtainedAt = time.Now().Add(-9 * time.Minute)
	report, err := session.Fetch(ctx, Query{StartDate: "2023-01-01", EndDate: "2025-12-31"})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Records) != 52 || report.ReportedCount != -1 || pageLoadCalls.Load() != 1 || searchCalls.Load() != 1 || refreshCalls.Load() != 1 || loginCalls.Load() != 1 {
		t.Fatal("did not fetch the full result set")
	}
	if report.StartDate != "2023-01-01" || report.EndDate != "2025-12-31" || report.Mode != QueryModeCustom {
		t.Fatal("requested historical interval was replaced by the official default")
	}
	for i, row := range report.Records {
		if row.ItemName != fmt.Sprintf("Synthetic item %d", i) {
			t.Fatal("initial page was appended to or substituted for the complete result")
		}
	}
	row := report.Records[0]
	if row.CheckDate != "112/03/20" || row.VisitDate != "112/03/02" || row.UploadDate != "112/04/01" || row.Result != "大於等於30" || row.OfficialFlag != "1" || row.Reference != "[1][10]" {
		t.Fatal("original laboratory/date semantics were not preserved")
	}
	if err := session.Logout(ctx); err != nil || logoutCalls.Load() != 1 || session.token != "" || session.client.Jar != nil || !session.closed {
		t.Fatalf("logout did not discard session state: %v", err)
	}
}

func TestFailuresDoNotBecomeEmptySuccessOrLeakUpstreamText(t *testing.T) {
	for _, tc := range []struct {
		name   string
		body   string
		count  int
		status int
		code   string
	}{
		{name: "truncated", body: `[{"assaY_ITEM_NAME":"Synthetic item","assaY_VALUE":"1"}]`, count: 2, status: 200, code: "INCOMPLETE_RESPONSE"},
		{name: "expired", body: `{"error_code":"9999","error_message":"SECRET_DATA"}`, count: 0, status: 200, code: "SESSION_EXPIRED"},
		{name: "html", body: `<html>SECRET_DATA</html>`, count: 0, status: 200, code: "UPSTREAM_CHANGED"},
		{name: "null", body: `null`, count: 0, status: 200, code: "UPSTREAM_CHANGED"},
		{name: "missing_item", body: `[{"assaY_VALUE":"1"}]`, count: 1, status: 200, code: "UPSTREAM_CHANGED"},
		{name: "bad_field", body: `[{"assaY_ITEM_NAME":"Synthetic item","assaY_VALUE":true}]`, count: 1, status: 200, code: "UPSTREAM_CHANGED"},
		{name: "unavailable", body: `SECRET_DATA`, count: 0, status: 503, code: "UPSTREAM_UNAVAILABLE"},
		{name: "authenticated_unauthorized", body: `SECRET_DATA`, count: 0, status: 401, code: "SESSION_EXPIRED"},
		{name: "authenticated_forbidden", body: `SECRET_DATA`, count: 0, status: 403, code: "SESSION_EXPIRED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == pageLoadPath {
					respondJSON(t, w, map[string]any{"sdate": "2025-01-01", "edate": "2025-12-31", "counts": tc.count, "sP_IHKE3409S02_Data": []any{}})
					return
				}
				if r.URL.Path != searchPath {
					t.Error("unexpected request")
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			session.token, session.tokenObtainedAt = "synthetic.token", time.Now()
			report, err := session.Fetch(context.Background(), Query{StartDate: "2025-01-01", EndDate: "2025-12-31"})
			var actual *Error
			if !errors.As(err, &actual) || actual.Code != tc.code {
				t.Fatalf("want %s, got %v", tc.code, err)
			}
			if tc.code == "SESSION_EXPIRED" && session.token != "" {
				t.Fatal("expired official authentication was retained")
			}
			if strings.Contains(err.Error(), "SECRET_DATA") || len(report.Records) != 0 {
				t.Fatal("failed response leaked or was treated as valid records")
			}
			if tc.code == "INCOMPLETE_RESPONSE" && (actual.ExpectedCount != 2 || actual.ReceivedCount != 1 || report.ReportedCount != 2) {
				t.Fatal("lost completeness diagnostic")
			}
			if tc.code == "SESSION_EXPIRED" && session.token != "" {
				t.Fatal("expired token was retained")
			}
		})
	}
}

func TestLoginFailureRequiresNewHumanChallenge(t *testing.T) {
	var calls atomic.Int32
	session := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		respondJSON(t, w, map[string]string{"error_code": "D002", "error_message": "SECRET_DATA"})
	})
	session.challengeID, session.challengeExpiry = "123456789", time.Now().Add(time.Minute)
	credentials := Credentials{NationalID: "A000000000", CardNumber: "12345678", Password: "synthetic-password", Captcha: "TEST"}
	var actual *Error
	err := session.Login(context.Background(), credentials)
	if !errors.As(err, &actual) || actual.Code != "AUTH_FAILED" || actual.UpstreamCode != "D002" || strings.Contains(err.Error(), "SECRET_DATA") {
		t.Fatalf("unexpected safe login error: %v", err)
	}
	err = session.Login(context.Background(), credentials)
	if !errors.As(err, &actual) || actual.Code != "CHALLENGE_REQUIRED" || calls.Load() != 1 {
		t.Fatal("login was replayed without a new manual challenge")
	}
}

func TestCardInputAndRedirectBoundary(t *testing.T) {
	for _, value := range []string{"12345678", "000012345678", "0000-1234-5678", " 0000 1234 5678 "} {
		got, err := NormalizeCardNumber(value)
		if err != nil || got != "000012345678" {
			t.Fatalf("normalization failed for synthetic input: %v", err)
		}
	}
	for _, value := range []string{"", "1234567", "123456789", "abcd12345678", "１２３４５６７８"} {
		if _, err := NormalizeCardNumber(value); err == nil {
			t.Error("invalid card number accepted")
		}
	}
	session, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	for _, target := range []string{"http://myhealthbank.nhi.gov.tw/", "https://example.invalid/", "https://myhealthbank.nhi.gov.tw.example.invalid/"} {
		req, err := http.NewRequest(http.MethodGet, target, nil)
		if err != nil {
			t.Fatal(err)
		}
		if session.client.CheckRedirect(req, nil) == nil {
			t.Error("non-official or plaintext redirect accepted")
		}
	}
}
