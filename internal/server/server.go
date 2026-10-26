// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/bestkakkoii/healthbank/internal/config"
	"github.com/bestkakkoii/healthbank/internal/lab"
	"github.com/bestkakkoii/healthbank/internal/nhi"
	"github.com/bestkakkoii/healthbank/internal/store"
	"github.com/bestkakkoii/healthbank/internal/webui"
)

const cookieName = "healthbank_session"
const sessionLifetime = 30 * time.Minute
const maxSessions = 64

var upstreamCodePattern = regexp.MustCompile(`^(?:[A-Z][0-9]{3}[A-Z]?|[0-9]{4})$`)

type session struct {
	mu                       sync.Mutex
	touched                  time.Time // protected by App.mu
	csrf                     string
	mode, subject, challenge string
	official                 *nhi.Session
	settings                 lab.Settings
	raw                      []lab.RawRecord
	fetched                  string
	complete                 bool
	warnings                 []string
	lastChallenge            time.Time
}
type App struct {
	mu       sync.Mutex
	sessions map[string]*session
	store    *store.Store
	catalog  *lab.Catalog
	secret   []byte
	version  string
	mux      *http.ServeMux
	stop     chan struct{}
	done     chan struct{}
}

func New(db *store.Store, secret []byte, version string) (*App, error) {
	c, err := lab.LoadCatalog()
	if err != nil {
		return nil, err
	}
	a := &App{sessions: map[string]*session{}, store: db, catalog: c, secret: secret, version: version, mux: http.NewServeMux(), stop: make(chan struct{}), done: make(chan struct{})}
	a.mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		a.json(w, http.StatusOK, map[string]string{"status": "ok", "version": a.version})
	})
	a.mux.HandleFunc("GET /api/session", a.withSession(a.getSession))
	a.mux.HandleFunc("GET /api/auth/captcha", a.withSession(a.captcha))
	a.mux.HandleFunc("POST /api/auth/login", a.withSession(a.login))
	a.mux.HandleFunc("POST /api/auth/logout", a.withSession(a.logout))
	a.mux.HandleFunc("GET /api/reports", a.withSession(a.fetchReports))
	a.mux.HandleFunc("GET /api/reports/current", a.withSession(a.currentReports))
	a.mux.HandleFunc("POST /api/reports/import", a.withSession(a.importReports))
	a.mux.HandleFunc("POST /api/reports/demo", a.withSession(a.demo))
	a.mux.HandleFunc("GET /api/catalog", a.withSession(a.catalogResponse))
	a.mux.HandleFunc("PUT /api/settings", a.withSession(a.saveSettings))
	a.mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		a.fail(w, r, 404, "not_found", "找不到這個服務端點。")
	})
	a.mux.Handle("/", webui.Handler())
	go a.cleanup()
	return a, nil
}
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func (a *App) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := randomToken()
		if err != nil {
			http.Error(w, "service unavailable", 503)
			return
		}
		w.Header().Set("X-Request-ID", id[:16])
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		a.mux.ServeHTTP(w, r)
	})
}
func (a *App) cleanup() {
	defer close(a.done)
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-a.stop:
			return
		case now := <-tick.C:
			var expired []*session
			a.mu.Lock()
			for id, s := range a.sessions {
				if now.Sub(s.touched) > sessionLifetime {
					delete(a.sessions, id)
					expired = append(expired, s)
				}
			}
			a.mu.Unlock()
			for _, s := range expired {
				s.mu.Lock()
				s.clear()
				s.mu.Unlock()
			}
		}
	}
}
func (a *App) Close() {
	close(a.stop)
	<-a.done
	a.mu.Lock()
	sessions := a.sessions
	a.sessions = map[string]*session{}
	a.mu.Unlock()
	for _, s := range sessions {
		s.mu.Lock()
		s.clear()
		s.mu.Unlock()
	}
}
func (s *session) clear() {
	if s.official != nil {
		s.official.Close()
	}
	s.official = nil
	s.subject = ""
	s.challenge = ""
	s.mode = "anonymous"
	s.settings = lab.DefaultSettings()
	s.raw = nil
	s.fetched = ""
	s.complete = false
	s.warnings = []string{}
}
func secureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	ip := net.ParseIP(host)
	return err == nil && ip != nil && ip.IsLoopback() && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
func (a *App) setCookie(w http.ResponseWriter, r *http.Request, id string) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: id, Path: "/", HttpOnly: true, Secure: secureRequest(r), SameSite: http.SameSiteStrictMode, MaxAge: int(sessionLifetime.Seconds())})
}
func (a *App) withSession(next func(http.ResponseWriter, *http.Request, *session)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		var s *session
		var id string
		created := false
		if cookie, err := r.Cookie(cookieName); err == nil {
			id = cookie.Value
			s = a.sessions[id]
		}
		if s != nil && time.Since(s.touched) > sessionLifetime {
			delete(a.sessions, id)
			expired := s
			s = nil
			go func() { expired.mu.Lock(); expired.clear(); expired.mu.Unlock() }()
		}
		if s == nil {
			if len(a.sessions) >= maxSessions {
				a.mu.Unlock()
				a.fail(w, r, 503, "session_capacity", "目前連線較多，請稍後重試。")
				return
			}
			var err error
			id, err = randomToken()
			if err != nil {
				a.mu.Unlock()
				a.fail(w, r, 503, "random_unavailable", "無法建立連線。")
				return
			}
			csrf, err := randomToken()
			if err != nil {
				a.mu.Unlock()
				a.fail(w, r, 503, "random_unavailable", "無法建立連線。")
				return
			}
			s = &session{csrf: csrf, mode: "anonymous", settings: lab.DefaultSettings(), warnings: []string{}}
			a.sessions[id] = s
			created = true
		}
		if created {
			s.touched = time.Now()
		}
		a.mu.Unlock()
		s.mu.Lock()
		defer s.mu.Unlock()
		// Login/logout/import can rotate a session while another request waits on
		// its mutex. The waiting request must not read the new person's state or
		// overwrite the browser with the retired session cookie.
		a.mu.Lock()
		current := a.sessions[id] == s
		if current {
			s.touched = time.Now()
		}
		a.mu.Unlock()
		if !current {
			a.fail(w, r, 409, "session_changed", "連線已更換，請重新整理頁面後重試。")
			return
		}
		// Only initial creation and explicit rotation issue a cookie. Ordinary
		// parallel responses must not reissue an older generation.
		if created {
			a.setCookie(w, r, id)
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			if origin := r.Header.Get("Origin"); origin != "" {
				u, err := url.Parse(origin)
				if err != nil || !strings.EqualFold(u.Host, r.Host) || (u.Scheme != "https" && u.Scheme != "http") {
					a.fail(w, r, 403, "origin_mismatch", "請從本站頁面操作。")
					return
				}
			}
			if r.Header.Get("Sec-Fetch-Site") == "cross-site" || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(s.csrf)) != 1 {
				a.fail(w, r, 403, "csrf_mismatch", "頁面連線已更新，請重新整理後重試。")
				return
			}
		}
		next(w, r, s)
	}
}
func (a *App) rotate(w http.ResponseWriter, r *http.Request, s *session) error {
	id, err := randomToken()
	if err != nil {
		return err
	}
	csrf, err := randomToken()
	if err != nil {
		return err
	}
	a.mu.Lock()
	for old, v := range a.sessions {
		if v == s {
			delete(a.sessions, old)
		}
	}
	a.sessions[id] = s
	s.touched = time.Now()
	a.mu.Unlock()
	s.csrf = csrf
	a.setCookie(w, r, id)
	return nil
}
func (a *App) json(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Debug("response write interrupted", "code", "response_write")
	}
}
func (a *App) fail(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	id := w.Header().Get("X-Request-ID")
	// Request query strings, bodies, identifiers, upstream bodies and DSNs are
	// deliberately excluded from logs.
	if status >= 500 {
		slog.Warn("request failed", "code", code, "request_id", id)
	}
	a.json(w, status, map[string]any{"error": map[string]string{"code": code, "message": message, "request_id": id}})
}
func (a *App) upstreamError(w http.ResponseWriter, r *http.Request, err error) {
	detail := struct {
		Code           string `json:"code"`
		Message        string `json:"message"`
		RequestID      string `json:"request_id"`
		UpstreamStatus int    `json:"upstream_status,omitempty"`
		UpstreamCode   string `json:"upstream_code,omitempty"`
	}{
		Code:      "upstream_failed",
		Message:   "官方連線未能完成，請稍後重試或匯入已另存的報告。",
		RequestID: w.Header().Get("X-Request-ID"),
	}
	// This app is reachable, but the requested action depends on an official
	// request that failed. Do not disguise that distinction as an origin 502.
	status := http.StatusFailedDependency
	var e *nhi.Error
	if errors.As(err, &e) {
		known := true
		switch e.Code {
		case "INVALID_INPUT", "CHALLENGE_REQUIRED":
			status = http.StatusBadRequest
		case "SESSION_EXPIRED", "AUTH_FAILED", "CLOSED":
			status = http.StatusUnauthorized
		case "INITIALIZATION_FAILED", "REQUEST_FAILED":
			status = http.StatusInternalServerError
		case "NETWORK", "DNS_FAILED", "TIMEOUT", "TLS_CERTIFICATE", "TLS_FAILED",
			"CONNECTION_REFUSED", "CONNECTION_RESET", "CONNECTION_FAILED", "REQUEST_CANCELED",
			"UPSTREAM_REDIRECT_REJECTED", "RESPONSE_INTERRUPTED", "CAPTCHA_INVALID_IMAGE",
			"UPSTREAM_ACCESS_DENIED", "UPSTREAM_UNAVAILABLE", "UPSTREAM_CHANGED",
			"UPSTREAM_REJECTED", "RESPONSE_TOO_LARGE", "INCOMPLETE_RESPONSE":
		default:
			known = false
		}
		if known {
			detail.Code, detail.Message = e.Code, e.Message
		}
		if e.HTTPStatus >= 100 && e.HTTPStatus <= 599 {
			detail.UpstreamStatus = e.HTTPStatus
		}
		if upstreamCodePattern.MatchString(e.UpstreamCode) {
			detail.UpstreamCode = e.UpstreamCode
		}
	}
	if status == http.StatusFailedDependency || status >= 500 {
		attributes := []slog.Attr{slog.String("code", detail.Code), slog.String("request_id", detail.RequestID)}
		if detail.UpstreamStatus != 0 {
			attributes = append(attributes, slog.Int("upstream_status", detail.UpstreamStatus))
		}
		if detail.UpstreamCode != "" {
			attributes = append(attributes, slog.String("upstream_code", detail.UpstreamCode))
		}
		slog.LogAttrs(r.Context(), slog.LevelWarn, "official request failed", attributes...)
	}
	a.json(w, status, map[string]any{"error": detail})
}
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 256<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("one JSON object required")
	}
	return nil
}
func (a *App) sessionView(s *session) map[string]any {
	return map[string]any{"authenticated": s.mode == "official", "mode": s.mode, "settings": s.settings, "version": a.version, "csrf_token": s.csrf}
}
func (a *App) refreshSettings(ctx context.Context, s *session) error {
	if s.subject == "" {
		return nil
	}
	doc, err := a.store.Get(ctx, s.subject)
	if err != nil {
		return err
	}
	next, err := lab.DecodeSettings(doc.Settings, doc.Revision)
	if err != nil {
		return err
	}
	s.settings = next
	return nil
}
func (a *App) getSession(w http.ResponseWriter, r *http.Request, s *session) {
	if err := a.refreshSettings(r.Context(), s); err != nil {
		a.fail(w, r, 500, "settings_read", "無法讀取個人設定。")
		return
	}
	a.json(w, 200, a.sessionView(s))
}
func (a *App) captcha(w http.ResponseWriter, r *http.Request, s *session) {
	if time.Since(s.lastChallenge) < time.Second {
		a.fail(w, r, 429, "captcha_rate", "請稍候再更新驗證碼。")
		return
	}
	s.lastChallenge = time.Now()
	if s.mode == "official" {
		a.fail(w, r, 409, "already_authenticated", "目前已登入；如要換人，請先登出。")
		return
	}
	if s.official == nil {
		var err error
		s.official, err = nhi.New()
		if err != nil {
			a.fail(w, r, 503, "connector_init", "無法建立官方連線。")
			return
		}
	}
	s.challenge = ""
	ch, err := s.official.Challenge(r.Context())
	if err != nil {
		a.upstreamError(w, r, err)
		return
	}
	s.challenge = ch.ID
	a.json(w, 200, map[string]string{"image_url": "data:" + ch.ContentType + ";base64," + base64.StdEncoding.EncodeToString(ch.Image), "challenge_id": ch.ID})
}
func (a *App) login(w http.ResponseWriter, r *http.Request, s *session) {
	var in struct {
		Identity  string `json:"identity_number"`
		Card      string `json:"card_number"`
		Password  string `json:"password"`
		Captcha   string `json:"captcha"`
		Challenge string `json:"challenge_id"`
	}
	if err := decodeJSON(w, r, &in); err != nil {
		a.fail(w, r, 400, "invalid_login", "登入資料格式不正確。")
		return
	}
	if s.official == nil || s.challenge == "" || subtle.ConstantTimeCompare([]byte(s.challenge), []byte(in.Challenge)) != 1 {
		a.fail(w, r, 400, "challenge_expired", "請更新圖形驗證碼後重新輸入。")
		return
	}
	s.challenge = ""
	if err := s.official.Login(r.Context(), nhi.Credentials{NationalID: in.Identity, CardNumber: in.Card, Password: in.Password, Captcha: in.Captcha}); err != nil {
		a.upstreamError(w, r, err)
		return
	}
	s.subject = config.Subject(a.secret, in.Identity)
	s.mode = "official"
	s.raw = nil
	s.fetched = ""
	s.warnings = []string{}
	if err := a.refreshSettings(r.Context(), s); err != nil {
		s.clear()
		a.fail(w, r, 500, "settings_read", "登入成功但個人設定無法讀取，請稍後重試。")
		return
	}
	if err := a.rotate(w, r, s); err != nil {
		s.clear()
		a.fail(w, r, 500, "session_rotate", "無法建立登入連線。")
		return
	}
	a.json(w, 200, a.sessionView(s))
}
func (a *App) logout(w http.ResponseWriter, r *http.Request, s *session) {
	if s.official != nil && s.mode == "official" {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		_ = s.official.Logout(ctx)
		cancel()
	}
	s.clear()
	if err := a.rotate(w, r, s); err != nil {
		a.fail(w, r, 500, "session_rotate", "無法更新連線。")
		return
	}
	a.json(w, 200, a.sessionView(s))
}
