// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestFailedBootstrapStopsBeforeCaptchaAndRequiresExplicitRetry(t *testing.T) {
	for _, tc := range []struct {
		name        string
		status      int
		contentType string
		body        string
		code        string
	}{
		{name: "unavailable", status: 503, contentType: "text/html", body: "<html>SECRET_UPSTREAM</html>", code: "UPSTREAM_UNAVAILABLE"},
		{name: "forbidden", status: 403, contentType: "text/html", body: "<html>SECRET_UPSTREAM</html>", code: "UPSTREAM_ACCESS_DENIED"},
		{name: "unauthorized", status: 401, contentType: "text/html", body: "<html>SECRET_UPSTREAM</html>", code: "UPSTREAM_ACCESS_DENIED"},
		{name: "json", status: 200, contentType: "application/json", body: `{"error":"SECRET_UPSTREAM"}`, code: "LOGIN_PAGE_INVALID"},
		{name: "mislabeled_html", status: 200, contentType: "text/html", body: `{"error":"SECRET_UPSTREAM"}`, code: "LOGIN_PAGE_INVALID"},
		{name: "empty_html", status: 200, contentType: "text/html", body: "", code: "LOGIN_PAGE_INVALID"},
		{name: "wrong_mime", status: 200, contentType: "application/octet-stream", body: "<html>SECRET_UPSTREAM</html>", code: "LOGIN_PAGE_INVALID"},
		{name: "oversized_html", status: 200, contentType: "text/html", body: "<!doctype html>" + strings.Repeat("x", maxLoginPageBytes), code: "LOGIN_PAGE_INVALID"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var documentCalls, captchaCalls atomic.Int32
			session := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != loginPage {
					captchaCalls.Add(1)
					http.NotFound(w, r)
					return
				}
				documentCalls.Add(1)
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			})
			for explicitAttempt := int32(1); explicitAttempt <= 2; explicitAttempt++ {
				challenge, err := session.Challenge(context.Background())
				var actual *Error
				if !errors.As(err, &actual) || actual.Code != tc.code || strings.Contains(actual.Error(), "SECRET") {
					t.Fatalf("expected a safe %s bootstrap failure", tc.code)
				}
				if tc.status != http.StatusOK && actual.HTTPStatus != tc.status {
					t.Fatal("bootstrap rejection lost its numeric upstream status")
				}
				if tc.name == "unavailable" && !strings.HasPrefix(actual.Message, "載入官方登入頁時：") {
					t.Fatal("document failure did not identify the failed step")
				}
				if documentCalls.Load() != explicitAttempt || captchaCalls.Load() != 0 || session.bootstrapped || session.challengeID != "" || challenge.ID != "" || len(challenge.Image) != 0 {
					t.Fatal("failed document request retried automatically, continued to CAPTCHA or retained successful state")
				}
			}
		})
	}
}

func TestChallengeSharesOneBoundedDeadlineAcrossDocumentAndCaptcha(t *testing.T) {
	session, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(session.Close)
	var deadline time.Time
	calls := 0
	session.client.Transport = errorTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		current, ok := r.Context().Deadline()
		if !ok || time.Until(current) <= 0 || time.Until(current) > 35*time.Second {
			t.Fatal("challenge request lacks the bounded total deadline")
		}
		switch r.URL.Path {
		case loginPage:
			deadline = current
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"text/html; charset=utf-8"}},
				Body:       io.NopCloser(strings.NewReader("<!doctype html><html><body>Synthetic document</body></html>")),
				Request:    r,
			}, nil
		case captchaPath:
			if !current.Equal(deadline) {
				t.Fatal("CAPTCHA received a fresh deadline after the document request")
			}
			return nil, context.DeadlineExceeded
		default:
			t.Fatal("unexpected endpoint in the bounded challenge flow")
			return nil, errors.New("unexpected synthetic endpoint")
		}
	})
	challenge, err := session.Challenge(context.Background())
	var actual *Error
	if !errors.As(err, &actual) || actual.Code != "TIMEOUT" || calls != 2 || !session.bootstrapped || challenge.ID != "" || len(challenge.Image) != 0 {
		t.Fatal("CAPTCHA timeout did not retain only the successfully bootstrapped document state")
	}
}
