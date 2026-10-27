// Copyright (c) 2026 bestkakkoii llc.
// SPDX-License-Identifier: MIT

package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bestkakkoii/healthbank/internal/nhi"
)

func TestUpstreamErrorsKeepDiagnosticsWithoutBecomingGatewayErrors(t *testing.T) {
	for _, tc := range []struct {
		name           string
		err            error
		status         int
		code           string
		upstreamStatus int
		upstreamCode   string
		logged         bool
	}{
		{"unavailable", &nhi.Error{Code: "UPSTREAM_UNAVAILABLE", Message: "官方連線未完成。", HTTPStatus: 503}, 424, "UPSTREAM_UNAVAILABLE", 503, "", true},
		{"anonymous_forbidden", &nhi.Error{Code: "UPSTREAM_ACCESS_DENIED", Message: "官方連線拒絕請求。", HTTPStatus: 403}, 424, "UPSTREAM_ACCESS_DENIED", 403, "", true},
		{"invalid_image", &nhi.Error{Code: "CAPTCHA_INVALID_IMAGE", Message: "驗證碼圖片格式不符。"}, 424, "CAPTCHA_INVALID_IMAGE", 0, "", true},
		{"invalid_login_page", &nhi.Error{Code: "LOGIN_PAGE_INVALID", Message: "官方登入頁格式不符。"}, 424, "LOGIN_PAGE_INVALID", 0, "", true},
		{"dns", &nhi.Error{Code: "DNS_FAILED", Message: "無法完成 DNS 解析。"}, 424, "DNS_FAILED", 0, "", true},
		{"typed_official_code", &nhi.Error{Code: "UPSTREAM_REJECTED", Message: "官方未完成請求。", UpstreamCode: "Z003A"}, 424, "UPSTREAM_REJECTED", 0, "Z003A", true},
		{"input", &nhi.Error{Code: "INVALID_INPUT", Message: "請確認輸入內容。"}, 400, "INVALID_INPUT", 0, "", false},
		{"challenge_required", &nhi.Error{Code: "CHALLENGE_REQUIRED", Message: "請更新驗證碼。"}, 400, "CHALLENGE_REQUIRED", 0, "", false},
		{"authentication", &nhi.Error{Code: "AUTH_FAILED", Message: "登入未完成。", UpstreamCode: "D002"}, 401, "AUTH_FAILED", 0, "D002", false},
		{"session_expired", &nhi.Error{Code: "SESSION_EXPIRED", Message: "請重新登入。", HTTPStatus: 401}, 401, "SESSION_EXPIRED", 401, "", false},
		{"initialization", &nhi.Error{Code: "INITIALIZATION_FAILED", Message: "無法建立官方連線。"}, 500, "INITIALIZATION_FAILED", 0, "", true},
		{"unsafe_metadata", &nhi.Error{Code: "UPSTREAM_UNAVAILABLE", Message: "官方連線未完成。", HTTPStatus: 700, UpstreamCode: "SECRET_ID=123"}, 424, "UPSTREAM_UNAVAILABLE", 0, "", true},
		{"unknown_code", &nhi.Error{Code: "SECRET_ERROR_CODE", Message: "SECRET_PASSWORD"}, 424, "upstream_failed", 0, "", true},
		{"untyped", errors.New("SECRET_URL?password=SECRET_PASSWORD"), 424, "upstream_failed", 0, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			previousLogger := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
			t.Cleanup(func() { slog.SetDefault(previousLogger) })
			app := &App{mux: http.NewServeMux()}
			app.mux.HandleFunc("GET /api/auth/captcha", func(w http.ResponseWriter, r *http.Request) {
				app.upstreamError(w, r, tc.err)
			})
			request := httptest.NewRequest(http.MethodGet, "https://healthbank.test/api/auth/captcha?SECRET_QUERY", nil)
			request.Header.Set("Authorization", "SECRET_AUTHORIZATION")
			response := httptest.NewRecorder()
			app.Handler().ServeHTTP(response, request)
			var envelope struct {
				Error struct {
					Code           string `json:"code"`
					Message        string `json:"message"`
					RequestID      string `json:"request_id"`
					UpstreamStatus int    `json:"upstream_status"`
					UpstreamCode   string `json:"upstream_code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			detail := envelope.Error
			if response.Code != tc.status || detail.Code != tc.code || detail.UpstreamStatus != tc.upstreamStatus || detail.UpstreamCode != tc.upstreamCode {
				t.Fatal("upstream status mapping or safe diagnostic metadata changed")
			}
			if detail.RequestID == "" || detail.RequestID != response.Header().Get("X-Request-ID") || response.Header().Get("Cache-Control") != "no-store, private" || response.Header().Get("X-Accel-Buffering") != "no" || response.Header().Get("Pragma") != "no-cache" {
				t.Fatal("correlation ID or no-store response policy was lost")
			}
			if detail.Message == "" || strings.Contains(response.Body.String(), "SECRET") || strings.Contains(logs.String(), "SECRET") {
				t.Fatal("error response or log leaked upstream/request details")
			}
			lines := strings.Split(strings.TrimSpace(logs.String()), "\n")
			if tc.logged {
				if len(lines) != 1 || lines[0] == "" {
					t.Fatal("dependency failure must produce exactly one sanitized log")
				}
				var entry struct {
					Code           string `json:"code"`
					RequestID      string `json:"request_id"`
					UpstreamStatus int    `json:"upstream_status"`
					UpstreamCode   string `json:"upstream_code"`
				}
				if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
					t.Fatal(err)
				}
				if entry.Code != tc.code || entry.RequestID != detail.RequestID || entry.UpstreamStatus != tc.upstreamStatus || entry.UpstreamCode != tc.upstreamCode {
					t.Fatal("sanitized log lost the response's diagnostic metadata")
				}
			} else if logs.Len() != 0 {
				t.Fatal("ordinary input/authentication rejection created a dependency error log")
			}
		})
	}
}

func TestUpstreamConnectionDiagnosticsAreSanitizedInResponseAndLog(t *testing.T) {
	for _, tc := range []struct {
		name        string
		diagnostics *nhi.Diagnostics
		phase       string
	}{
		{"observed_tls_reset", &nhi.Diagnostics{Operation: "login_document", Phase: "tls_handshake", ElapsedMS: 28, ProxySelected: false}, "tls_handshake"},
		{"unsafe_metadata", &nhi.Diagnostics{Operation: "SECRET_URL", Phase: "SECRET_PASSWORD", ElapsedMS: -1, TLSVersion: "SECRET_CERTIFICATE", TLSProtocol: "SECRET_HEADER"}, "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
			t.Cleanup(func() { slog.SetDefault(previous) })
			app := &App{mux: http.NewServeMux()}
			app.mux.HandleFunc("GET /api/auth/captcha", func(w http.ResponseWriter, r *http.Request) {
				app.upstreamError(w, r, &nhi.Error{Code: "CONNECTION_RESET", Message: "官方連線被重設。", Diagnostics: tc.diagnostics})
			})
			response := httptest.NewRecorder()
			app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "https://healthbank.test/api/auth/captcha", nil))
			var envelope struct {
				Error struct {
					Diagnostics *nhi.Diagnostics `json:"diagnostics"`
				} `json:"error"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if response.Code != http.StatusFailedDependency || envelope.Error.Diagnostics == nil || string(envelope.Error.Diagnostics.Phase) != tc.phase {
				t.Fatal("connection diagnostic phase was lost")
			}
			var entry struct {
				Diagnostics *nhi.Diagnostics `json:"diagnostics"`
			}
			if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
				t.Fatal(err)
			}
			if entry.Diagnostics == nil || string(entry.Diagnostics.Phase) != tc.phase || strings.Contains(response.Body.String(), "SECRET") || strings.Contains(logs.String(), "SECRET") {
				t.Fatal("unsafe connection diagnostics reached the response or log")
			}
		})
	}
}
