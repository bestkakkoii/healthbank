// Copyright (c) 2026 bestkakkoi llc. All rights reserved.
// SPDX-License-Identifier: MIT

package nhi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
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

func TestSourceDerivedFlowWithFullSearch(t *testing.T) {
	image, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+a+T8AAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	var challengeID string
	var loginCalls, refreshCalls, searchCalls, logoutCalls atomic.Int32
	session := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case captchaPath:
			if r.Method != http.MethodGet || r.URL.Query().Get("id") == "" || r.URL.Query().Get("r") == "" {
				t.Error("missing official CAPTCHA query")
			}
			challengeID = r.URL.Query().Get("id")
			http.SetCookie(w, &http.Cookie{Name: "fixture_session", Value: "synthetic", Path: "/", Secure: true})
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(image)
		case loginPath:
			loginCalls.Add(1)
			if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer null" || r.Header.Get("X-Requested-With") != "XMLHttpRequest" {
				t.Error("login request does not match SPA transport")
			}
			cookie, cookieErr := r.Cookie("fixture_session")
			if cookieErr != nil || cookie.Value != "synthetic" {
				t.Error("CAPTCHA cookie not preserved")
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
			if r.Header.Get("Authorization") != "Bearer synthetic.refreshed.token" {
				t.Error("missing refreshed bearer token")
			}
			q := r.URL.Query()
			if q.Get("s_sort") != "A1" || q.Get("s_type") != "" || q.Get("nowPage") != "1" || q.Get("pageDataCount") != "50" || q.Get("s_date") != "2025-01-01" || q.Get("e_date") != "2025-12-31" {
				t.Error("incorrect page-load query")
			}
			respondJSON(t, w, map[string]any{"sdate": "2025-01-01", "edate": "2025-12-31", "counts": "52", "sP_IHKE3409S02_Data": []any{}})
		case searchPath:
			searchCalls.Add(1)
			if r.URL.Query().Has("nowPage") || r.URL.Query().Has("pageDataCount") {
				t.Error("full-search request must not be paginated")
			}
			rows := make([]map[string]any, 52)
			for i := range rows {
				rows[i] = map[string]any{
					"reaL_INSPECT_DATE": "114/01/20", "funC_DATE": "114/01/02", "assaY_UPLOAD_DATE": "114/02/01",
					"ordeR_CODE": "TEST01", "ordeR_NAME": "Synthetic panel", "assaY_ITEM_NAME": fmt.Sprintf("Synthetic item %d", i),
					"assaY_VALUE": "大於等於30", "uniT_DATA": "test-unit", "consulT_VALUE": "[1][10]",
					"hosP_ABBR": "Synthetic institution", "orI_TYPE": "A", "assaY_MARK": 1,
				}
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
	if err != nil || challenge.ContentType != "image/png" || len(challenge.Image) != len(image) || challenge.ID == "" || !challenge.ExpiresAt.After(time.Now()) {
		t.Fatalf("CAPTCHA failed: %v", err)
	}
	err = session.Login(ctx, Credentials{NationalID: "a000000000", CardNumber: "12345678", Password: "synthetic-password", Captcha: " TEST "})
	if err != nil {
		t.Fatal(err)
	}
	session.tokenObtainedAt = time.Now().Add(-9 * time.Minute)
	report, err := session.Fetch(ctx, Query{StartDate: "2025-01-01", EndDate: "2025-12-31"})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Records) != 52 || report.ReportedCount != 52 || searchCalls.Load() != 1 || refreshCalls.Load() != 1 || loginCalls.Load() != 1 {
		t.Fatal("did not fetch the full result set")
	}
	row := report.Records[0]
	if row.CheckDate != "114/01/20" || row.VisitDate != "114/01/02" || row.UploadDate != "114/02/01" || row.Result != "大於等於30" || row.OfficialFlag != "1" || row.Reference != "[1][10]" {
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
			report, err := session.Fetch(context.Background(), Query{})
			var actual *Error
			if !errors.As(err, &actual) || actual.Code != tc.code {
				t.Fatalf("want %s, got %v", tc.code, err)
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
