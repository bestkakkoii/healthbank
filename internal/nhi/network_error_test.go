// Copyright (c) 2026 bestkakkoii llc. All rights reserved.
// SPDX-License-Identifier: MIT

package nhi

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

type errorTransport func(*http.Request) (*http.Response, error)

func (f errorTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type syntheticTimeout struct{}

func (syntheticTimeout) Error() string   { return "SECRET_PROXY timeout" }
func (syntheticTimeout) Timeout() bool   { return true }
func (syntheticTimeout) Temporary() bool { return true }

type interruptedBody struct{ cause error }

func (b interruptedBody) Read([]byte) (int, error) { return 0, b.cause }
func (interruptedBody) Close() error               { return nil }

func TestChallengeClassifiesNetworkFailuresWithoutLeakingDetails(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cause error
		code  string
	}{
		{"dns", &net.DNSError{Name: "SECRET_HOST", Server: "SECRET_DNS", Err: "SECRET_DETAIL"}, "DNS_FAILED"},
		{"dns_timeout", &net.DNSError{Name: "SECRET_HOST", Err: "SECRET_DETAIL", IsTimeout: true}, "DNS_FAILED"},
		{"deadline", context.DeadlineExceeded, "TIMEOUT"},
		{"socket_timeout", syntheticTimeout{}, "TIMEOUT"},
		{"certificate", &tls.CertificateVerificationError{Err: errors.New("SECRET_CERT")}, "TLS_CERTIFICATE"},
		{"certificate_name", x509.HostnameError{Certificate: &x509.Certificate{}, Host: "SECRET_HOST"}, "TLS_CERTIFICATE"},
		{"certificate_expired", x509.CertificateInvalidError{Reason: x509.Expired, Detail: "SECRET_CERT"}, "TLS_CERTIFICATE"},
		{"certificate_authority", x509.UnknownAuthorityError{Cert: &x509.Certificate{}}, "TLS_CERTIFICATE"},
		{"system_roots", x509.SystemRootsError{Err: errors.New("SECRET_CERT_FILE")}, "TLS_CERTIFICATE"},
		{"tls_record", tls.RecordHeaderError{Msg: "SECRET_TLS_RECORD"}, "TLS_FAILED"},
		{"tls_alert", tls.AlertError(40), "TLS_FAILED"},
		{"tls_tcp_alert", &net.OpError{Op: "remote error", Err: errors.New("SECRET_TLS_ALERT")}, "TLS_FAILED"},
		{"refused", &os.SyscallError{Syscall: "connect", Err: syscall.ECONNREFUSED}, "CONNECTION_REFUSED"},
		{"reset", &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}, "CONNECTION_RESET"},
		{"route", &net.OpError{Op: "dial", Net: "tcp", Addr: &net.TCPAddr{IP: net.ParseIP("192.0.2.8"), Port: 443}, Err: syscall.ENETUNREACH}, "CONNECTION_FAILED"},
		{"canceled", context.Canceled, "REQUEST_CANCELED"},
		{"redirect", errRedirectRejected, "UPSTREAM_REDIRECT_REJECTED"},
		{"unknown", errors.New("SECRET_PROXY SECRET_PASSWORD SECRET_COOKIE"), "NETWORK"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session, err := New()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(session.Close)
			calls := 0
			session.client.Transport = errorTransport(func(*http.Request) (*http.Response, error) {
				calls++
				return nil, &url.Error{Op: "Get", URL: "https://SECRET_USER:SECRET_PASSWORD@proxy.invalid/SECRET_PATH?captcha=SECRET_ANSWER", Err: tc.cause}
			})
			challenge, err := session.Challenge(context.Background())
			var actual *Error
			if !errors.As(err, &actual) || actual.Code != tc.code || actual.HTTPStatus != 0 {
				t.Fatalf("expected sanitized %s error", tc.code)
			}
			encoded, err := json.Marshal(actual)
			if err != nil {
				t.Fatal(err)
			}
			for _, forbidden := range []string{"SECRET", "proxy.invalid", "192.0.2.8", "captcha?id", "https://"} {
				if strings.Contains(string(encoded), forbidden) || strings.Contains(actual.Error(), forbidden) {
					t.Fatal("network error retained private request details")
				}
			}
			if calls != 1 || challenge.ID != "" || len(challenge.Image) != 0 || session.challengeID != "" {
				t.Fatal("failed CAPTCHA was retried or exposed usable challenge state")
			}
		})
	}
}

func TestAnonymousChallengeStatusAndImageFailuresRemainDistinct(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		code   string
	}{
		{"unavailable", http.StatusServiceUnavailable, "UPSTREAM_UNAVAILABLE"},
		{"forbidden", http.StatusForbidden, "UPSTREAM_ACCESS_DENIED"},
		{"unauthorized", http.StatusUnauthorized, "UPSTREAM_ACCESS_DENIED"},
		{"invalid_image", http.StatusOK, "CAPTCHA_INVALID_IMAGE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			session := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != captchaPath || r.Method != http.MethodGet {
					t.Error("CAPTCHA flow called an unexpected endpoint")
					http.NotFound(w, r)
					return
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte("<html>SECRET_UPSTREAM_BODY</html>"))
			})
			session.challengeID, session.challengeExpiry = "synthetic-old-challenge", time.Now().Add(time.Minute)
			challenge, err := session.Challenge(context.Background())
			var actual *Error
			if !errors.As(err, &actual) || actual.Code != tc.code || actual.Code == "SESSION_EXPIRED" {
				t.Fatalf("expected %s for anonymous CAPTCHA", tc.code)
			}
			if tc.status != http.StatusOK && actual.HTTPStatus != tc.status {
				t.Fatal("original upstream HTTP status was lost")
			}
			if strings.Contains(actual.Error(), "SECRET") || calls.Load() != 1 || session.challengeID != "" || challenge.ID != "" || len(challenge.Image) != 0 {
				t.Fatal("failed challenge leaked data, retained old state or retried")
			}
		})
	}
}

func TestResponseReadFailuresPreserveSafeCauseAndHTTPStatus(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cause error
		code  string
	}{
		{"truncated", io.ErrUnexpectedEOF, "RESPONSE_INTERRUPTED"},
		{"timeout", syntheticTimeout{}, "TIMEOUT"},
		{"reset", syscall.ECONNRESET, "CONNECTION_RESET"},
		{"unknown", errors.New("SECRET_RESPONSE_READER"), "RESPONSE_INTERRUPTED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session, err := New()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(session.Close)
			session.client.Transport = errorTransport(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: interruptedBody{tc.cause}, Request: r}, nil
			})
			challenge, err := session.Challenge(context.Background())
			var actual *Error
			if !errors.As(err, &actual) || actual.Code != tc.code || actual.HTTPStatus != http.StatusOK || strings.Contains(actual.Error(), "SECRET") {
				t.Fatalf("expected safe %s read failure with original HTTP status", tc.code)
			}
			if challenge.ID != "" || len(challenge.Image) != 0 {
				t.Fatal("partial image became a successful challenge")
			}
		})
	}
}
