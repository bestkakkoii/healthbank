// Copyright (c) 2026 bestkakkoii llc.
// SPDX-License-Identifier: MIT

// Package nhi implements the card-number login and other laboratory results
// flow used by the official Taiwan NHI My Health Bank web application.
//
// This is a source-derived connector, not an official or supported public API.
// Credentials, cookies, tokens and reports are held in memory only.
package nhi

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"mime"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	officialOrigin    = "https://myhealthbank.nhi.gov.tw"
	loginPage         = "/IHKE3000/IHKE3099S01"
	captchaPath       = "/api/ihke3000/IHKELOGIN/captcha"
	loginPath         = "/api/ihke3000/ihkelogin/Login"
	refreshPath       = "/api/ihke3000/ihkelogin/RefreshJWT"
	logoutPath        = "/api/ihke3000/ihkelogin/Logout"
	pageLoadPath      = "/api/ihke3000/ihke3409s01/page_load"
	searchPath        = "/api/ihke3000/ihke3409s01/search"
	maxJSONBytes      = 16 << 20
	maxImageBytes     = 512 << 10
	maxAuthBytes      = 64 << 10
	maxLoginPageBytes = 1 << 20
	challengeBudget   = 35 * time.Second
	// MaxReportRecords bounds one official query and its count diagnostics.
	MaxReportRecords = 50000
)

var (
	identityPattern = regexp.MustCompile(`^[A-Z][A-Z0-9][0-9]{8}$`)
	codePattern     = regexp.MustCompile(`^(?:[A-Z][0-9]{3}[A-Z]?|[0-9]{4})$`)
)

// Error deliberately excludes the raw upstream body and request details. It is
// safe to return to the application or log; it never embeds a login credential.
type Error struct {
	Code          string       `json:"code"`
	Message       string       `json:"message"`
	HTTPStatus    int          `json:"upstreamStatus,omitempty"`
	UpstreamCode  string       `json:"upstreamCode,omitempty"`
	ExpectedCount int          `json:"expectedCount,omitempty"`
	ReceivedCount int          `json:"receivedCount,omitempty"`
	Diagnostics   *Diagnostics `json:"diagnostics,omitempty"`
}

func (e *Error) Error() string { return e.Message }

func failure(code, message string) *Error { return &Error{Code: code, Message: message} }

// Challenge is a raster image for the person signing in to read. ID is a
// challenge identifier, not the answer; callers must not OCR the image.
type Challenge struct {
	ID          string    `json:"id"`
	Image       []byte    `json:"-"`
	ContentType string    `json:"contentType"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

// Credentials is used for one request only and is not retained in Session.
type Credentials struct {
	NationalID string `json:"nationalId"`
	CardNumber string `json:"cardNumber"`
	Password   string `json:"password"`
	Captcha    string `json:"captcha"`
}

// Session owns an isolated official-site cookie jar and a JWT in memory.
// Calls are serialized so a CAPTCHA cannot be replaced during its login.
// The application must expire and Close sessions when no longer needed.
type Session struct {
	mu                 sync.Mutex
	client             *http.Client
	token              string
	tokenObtainedAt    time.Time
	challengeID        string
	challengeExpiry    time.Time
	bootstrapped       bool
	closed             bool
	lastDiagnostics    *Diagnostics
	selfPaidReferences map[string]string
	westernReferences  map[string]westernReference
	medicalReferences  map[string]map[string]medicalReference
}

// New creates an isolated connector. It cannot target a caller-supplied URL.
func New() (*Session, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, failure("INITIALIZATION_FAILED", "無法建立健保署連線工作階段。")
	}
	transport := &http.Transport{
		Proxy:                 diagnosticProxy,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 25 * time.Second,
		ExpectContinueTimeout: time.Second,
		IdleConnTimeout:       60 * time.Second,
		MaxIdleConns:          2,
		MaxIdleConnsPerHost:   2,
		MaxConnsPerHost:       2,
		ForceAttemptHTTP2:     true,
	}
	client := &http.Client{
		Transport: transport,
		Jar:       jar,
		Timeout:   35 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "https" || req.URL.Host != "myhealthbank.nhi.gov.tw" || len(via) >= 3 {
				return errRedirectRejected
			}
			traceRedirect(req)
			return nil
		},
	}
	return &Session{client: client}, nil
}

// NormalizeCardNumber accepts the requested eight-digit shorthand, prefixing
// 0000, or a complete twelve-digit card number. Spaces and hyphens are cosmetic.
func NormalizeCardNumber(value string) (string, error) {
	value = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '-' {
			return -1
		}
		return r
	}, value)
	if len(value) == 8 {
		value = "0000" + value
	}
	if len(value) != 12 {
		return "", failure("INVALID_INPUT", "健保卡號請輸入 8 碼或完整 12 碼數字。")
	}
	for _, b := range []byte(value) {
		if b < '0' || b > '9' {
			return "", failure("INVALID_INPUT", "健保卡號僅接受數字。")
		}
	}
	return value, nil
}

// Challenge fetches a new image. A previous authenticated session is cleared;
// the new login must use the image returned by this call.
func (s *Session) Challenge(ctx context.Context) (Challenge, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Challenge{}, closedError()
	}
	ctx, cancel := context.WithTimeout(ctx, challengeBudget)
	defer cancel()
	s.clearAuth()
	s.challengeID = ""
	if !s.bootstrapped {
		if err := s.bootstrap(ctx); err != nil {
			return Challenge{}, err
		}
	}
	n, err := rand.Int(rand.Reader, big.NewInt(899999))
	if err != nil {
		return Challenge{}, failure("INITIALIZATION_FAILED", "無法產生驗證碼工作階段。")
	}
	now := time.Now()
	// Match the official SPA's millisecond + six-digit random numeric identifier.
	id := strconv.Itoa(now.Nanosecond()/int(time.Millisecond)) + strconv.FormatInt(n.Int64()+100000, 10)
	query := url.Values{"id": {id}, "r": {strconv.Itoa(now.Nanosecond() / int(time.Millisecond))}}
	body, _, err := s.request(ctx, http.MethodGet, captchaPath, query, nil, maxImageBytes, false)
	if err != nil {
		return Challenge{}, err
	}
	imageInfo, err := InspectChallengeImage(body)
	if err != nil {
		clear(body)
		return Challenge{}, s.annotateResponseError(err)
	}
	s.challengeID = id
	s.challengeExpiry = now.Add(5 * time.Minute)
	return Challenge{ID: id, Image: body, ContentType: "image/" + imageInfo.Format, ExpiresAt: s.challengeExpiry}, nil
}

// bootstrap reproduces the browser's initial document request so cookies set by
// that response are available to the CAPTCHA and login in this session's jar.
// The saved SPA cannot establish whether those server-issued cookies are required.
func (s *Session) bootstrap(ctx context.Context) error {
	body, headers, err := s.request(ctx, http.MethodGet, loginPage, nil, nil, maxLoginPageBytes, false)
	if err != nil {
		var upstream *Error
		if errors.As(err, &upstream) {
			if upstream.Code == "RESPONSE_TOO_LARGE" {
				return s.annotateResponseError(failure("LOGIN_PAGE_INVALID", "官方登入頁回應超過大小限制，無法建立驗證碼連線。"))
			}
			detail := *upstream
			detail.Message = "載入官方登入頁時：" + upstream.Message
			return &detail
		}
		return err
	}
	defer clear(body)
	mediaType, _, err := mime.ParseMediaType(headers.Get("Content-Type"))
	trimmed := bytes.TrimSpace(bytes.TrimPrefix(body, []byte{0xef, 0xbb, 0xbf}))
	if err != nil || mediaType != "text/html" || !strings.HasPrefix(http.DetectContentType(trimmed), "text/html") {
		return s.annotateResponseError(failure("LOGIN_PAGE_INVALID", "官方登入頁未回傳有效的HTML文件，無法建立驗證碼連線；請稍後再試。"))
	}
	s.bootstrapped = true
	return nil
}

type loginResponse struct {
	Code  string `json:"error_code"`
	Token string `json:"jwt_token"`
}

// Login exchanges the user's manually entered credentials and CAPTCHA answer
// for the official token. Failed attempts are never retried automatically.
func (s *Session) Login(ctx context.Context, credentials Credentials) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return closedError()
	}
	if s.challengeID == "" || time.Now().After(s.challengeExpiry) {
		return failure("CHALLENGE_REQUIRED", "請重新取得圖形驗證碼後登入。")
	}
	identity := strings.ToUpper(strings.TrimSpace(credentials.NationalID))
	if !identityPattern.MatchString(identity) {
		return failure("INVALID_INPUT", "請確認身分證號或居留證號格式。")
	}
	card, err := NormalizeCardNumber(credentials.CardNumber)
	if err != nil {
		return err
	}
	answer := strings.TrimSpace(credentials.Captcha)
	if len(credentials.Password) == 0 || len(credentials.Password) > 1024 || len(answer) == 0 || len(answer) > 32 {
		return failure("INVALID_INPUT", "請填寫密碼與圖形驗證碼。")
	}
	payload := struct {
		Identity  string `json:"c_id"`
		Card      string `json:"c_no"`
		Password  string `json:"c_pw"`
		Answer    string `json:"c_ValidCode"`
		Challenge string `json:"c_Codeid"`
	}{identity, card, credentials.Password, answer, s.challengeID}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return failure("INVALID_INPUT", "登入資料格式不正確。")
	}
	s.challengeID = "" // Do not automatically replay a CAPTCHA/login attempt.
	s.clearAuth()
	defer clear(encoded)
	body, _, err := s.request(ctx, http.MethodPost, loginPath, nil, encoded, maxAuthBytes, false)
	if err != nil {
		return err
	}
	defer clear(body)
	var response loginResponse
	if err := decodeJSON(body, &response); err != nil {
		return s.annotateResponseError(failure("UPSTREAM_CHANGED", "健保署登入回應格式已變更，請稍後重試。"))
	}
	if !loginSucceeded(response.Code) {
		return s.annotateResponseError(&Error{Code: "AUTH_FAILED", Message: "健保署登入未完成，請確認登入資料與驗證碼；若持續失敗，請至官網確認帳號狀態。", UpstreamCode: safeCode(response.Code)})
	}
	if !validToken(response.Token) {
		return s.annotateResponseError(failure("UPSTREAM_CHANGED", "健保署未提供有效的登入憑證。"))
	}
	s.token = response.Token
	s.tokenObtainedAt = time.Now()
	return nil
}

// Refresh renews the official token. Fetch also refreshes after eight minutes.
func (s *Session) Refresh(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return closedError()
	}
	return s.refresh(ctx)
}

func (s *Session) refresh(ctx context.Context) error {
	if s.token == "" {
		return sessionExpired()
	}
	body, _, err := s.request(ctx, http.MethodGet, refreshPath, nil, nil, maxAuthBytes, true)
	if err != nil {
		return err
	}
	defer clear(body)
	var response loginResponse
	if decodeJSON(body, &response) != nil || response.Code != "0000" || !validToken(response.Token) {
		s.clearAuth()
		return s.annotateResponseError(sessionExpired())
	}
	s.token = response.Token
	s.tokenObtainedAt = time.Now()
	return nil
}

// Logout attempts the official sign-out and always closes the local session.
// Applications may call Close directly when an upstream request is unsuitable.
func (s *Session) Logout(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	defer s.close()
	if s.token == "" {
		return nil
	}
	_, _, err := s.request(ctx, http.MethodGet, logoutPath, nil, nil, maxAuthBytes, true)
	return err
}

// Close discards all references to this session's official token and cookies.
// It performs no network request and does not promise physical memory erasure.
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.close()
}

func (s *Session) close() {
	s.clearAuth()
	s.challengeID = ""
	s.challengeExpiry = time.Time{}
	s.bootstrapped = false
	s.closed = true
	if s.client != nil {
		s.client.CloseIdleConnections()
		s.client.Jar = nil
	}
}

func (s *Session) clearAuth() {
	s.token, s.tokenObtainedAt = "", time.Time{}
	s.selfPaidReferences = nil
	s.westernReferences = nil
	s.medicalReferences = nil
}

func (s *Session) request(ctx context.Context, method, path string, query url.Values, payload []byte, limit int64, authenticated bool) (result []byte, headers http.Header, requestErr error) {
	if authenticated && s.token == "" {
		return nil, nil, sessionExpired()
	}
	u := officialOrigin + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(payload))
	if err != nil {
		return nil, nil, failure("REQUEST_FAILED", "無法建立健保署請求。")
	}
	req, trace := startRequestTrace(req, operationForPath(path))
	defer func() {
		s.lastDiagnostics = trace.finish()
		requestErr = s.annotateResponseError(requestErr)
	}()
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; BestkakkoiHealthbank/1.0)")
	req.Header.Set("Accept-Language", "zh-TW,zh;q=0.9")
	if path == loginPage {
		req.Header.Set("Accept", "text/html")
	} else {
		req.Header.Set("Accept", "application/json, text/plain, */*")
		req.Header.Set("Referer", officialOrigin+loginPage)
		if authenticated {
			req.Header.Set("Referer", officialOrigin+"/IHKE3000/IHKE3409S01")
		}
		req.Header.Set("Origin", officialOrigin)
		if path == captchaPath {
			req.Header.Set("Accept", "image/png,image/jpeg,image/gif")
		} else {
			req.Header.Set("X-Requested-With", "XMLHttpRequest")
		}
	}
	if len(payload) > 0 {
		req.Header.Set("Content-Type", "application/json;charset=utf-8")
	}
	if authenticated {
		req.Header.Set("Authorization", "Bearer "+s.token)
	} else if path == loginPath {
		// The official Vue interceptor sends the literal string when not signed in.
		req.Header.Set("Authorization", "Bearer null")
	}
	response, err := s.client.Do(req)
	if err != nil {
		return nil, nil, networkFailure(err)
	}
	defer response.Body.Close()
	trace.response(response)
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		if authenticated {
			s.clearAuth()
			return nil, nil, &Error{Code: "SESSION_EXPIRED", Message: "健保署未接受此工作階段，請重新登入。", HTTPStatus: response.StatusCode}
		}
		return nil, nil, &Error{Code: "UPSTREAM_ACCESS_DENIED", Message: "官方連線拒絕此請求（HTTP " + strconv.Itoa(response.StatusCode) + "）。目前尚未完成登入，請網站管理者確認主機的官方連線。", HTTPStatus: response.StatusCode}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, nil, &Error{Code: "UPSTREAM_UNAVAILABLE", Message: "官方連線收到 HTTP " + strconv.Itoa(response.StatusCode) + "，目前無法完成請求，請稍後再試。", HTTPStatus: response.StatusCode}
	}
	trace.setPhase(PhaseResponseBody)
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, nil, responseReadFailure(err, response.StatusCode)
	}
	if int64(len(body)) > limit {
		return nil, nil, failure("RESPONSE_TOO_LARGE", "資料超出單次讀取範圍，請縮短日期區間後重試。")
	}
	trace.setPhase(PhaseComplete)
	return body, response.Header, nil
}

func decodeJSON(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(bytes.TrimPrefix(body, []byte{0xef, 0xbb, 0xbf})))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("unexpected trailing JSON data")
	}
	return nil
}

func loginSucceeded(code string) bool { return code == "0000" || code == "Z003" || code == "Z003A" }

func safeCode(value string) string {
	if codePattern.MatchString(value) {
		return value
	}
	return ""
}

func validToken(value string) bool {
	return len(value) > 0 && len(value) <= 16384 && !strings.ContainsAny(value, "\r\n\t ")
}

func sessionExpired() *Error {
	return failure("SESSION_EXPIRED", "健保署登入已逾時，請重新登入。")
}
func closedError() *Error {
	return failure("CLOSED", "此登入工作階段已結束，請重新登入。")
}
