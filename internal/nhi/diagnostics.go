// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Operation identifies a fixed connector operation, never a URL or query.
type Operation string

const (
	OperationUnknown          Operation = "unknown"
	OperationLoginDocument    Operation = "login_document"
	OperationCaptcha          Operation = "captcha"
	OperationLogin            Operation = "login"
	OperationReportMetadata   Operation = "report_metadata"
	OperationReportSearch     Operation = "report_search"
	OperationTokenRefresh     Operation = "token_refresh"
	OperationLogout           Operation = "logout"
	OperationSourceList       Operation = "source_list"
	OperationSourceDetail     Operation = "source_detail"
	OperationSourceRead       Operation = "source_read"
	OperationDocumentPrepare  Operation = "document_prepare"
	OperationDocumentDownload Operation = "document_download"
	OperationDocumentCleanup  Operation = "document_cleanup"
)

// Phase is the last observed request stage. It does not identify which remote
// system caused an error. In particular, TLS may involve a configured proxy.
type Phase string

const (
	PhaseUnknown         Phase = "unknown"
	PhasePrepare         Phase = "prepare"
	PhaseDNS             Phase = "dns"
	PhaseTCPConnect      Phase = "tcp_connect"
	PhaseProxyConnect    Phase = "proxy_connect"
	PhaseTLSHandshake    Phase = "tls_handshake"
	PhaseConnectionReady Phase = "connection_ready"
	PhaseRequestWrite    Phase = "request_write"
	PhaseResponseHeaders Phase = "response_headers"
	PhaseResponseBody    Phase = "response_body"
	PhaseComplete        Phase = "complete"
)

// Diagnostics contains only bounded, allowlisted transport observations. The
// elapsed time includes allowed redirects and saturates at 310 seconds, covering
// the five-minute document transfer budget and bounded cleanup. Ordinary JSON
// requests retain their shorter existing deadlines.
//
// TLS fields describe the acquired HTTP request connection, when observed, not
// an intermediate HTTPS-proxy handshake. They establish neither the identity of
// an unobserved origin behind an egress proxy nor the cause of a reset.
type Diagnostics struct {
	Operation     Operation `json:"operation"`
	Phase         Phase     `json:"phase"`
	ElapsedMS     int64     `json:"elapsed_ms"`
	ProxySelected bool      `json:"proxy_selected"`
	TLSVersion    string    `json:"tls_version,omitempty"`
	TLSProtocol   string    `json:"tls_protocol,omitempty"`
}

// Sanitized returns a separate bounded copy suitable for API output or logging.
// It also validates values supplied by callers constructing an Error directly.
func (d *Diagnostics) Sanitized() *Diagnostics {
	if d == nil {
		return nil
	}
	copy := *d
	switch copy.Operation {
	case OperationLoginDocument, OperationCaptcha, OperationLogin,
		OperationReportMetadata, OperationReportSearch, OperationTokenRefresh, OperationLogout,
		OperationSourceList, OperationSourceDetail, OperationSourceRead,
		OperationDocumentPrepare, OperationDocumentDownload, OperationDocumentCleanup:
	default:
		copy.Operation = OperationUnknown
	}
	switch copy.Phase {
	case PhasePrepare, PhaseDNS, PhaseTCPConnect, PhaseProxyConnect, PhaseTLSHandshake,
		PhaseConnectionReady, PhaseRequestWrite, PhaseResponseHeaders, PhaseResponseBody, PhaseComplete:
	default:
		copy.Phase = PhaseUnknown
	}
	copy.ElapsedMS = min(max(copy.ElapsedMS, 0), 310000)
	switch copy.TLSVersion {
	case "TLS1.2", "TLS1.3":
	default:
		copy.TLSVersion, copy.TLSProtocol = "", ""
	}
	if copy.TLSProtocol != "h2" && copy.TLSProtocol != "http/1.1" {
		copy.TLSProtocol = ""
	}
	return &copy
}

// Snapshot returns a copy of the last finished request's diagnostics. It is nil
// before the first request. No credential, cookie, address or image is retained.
func (s *Session) Snapshot() *Diagnostics {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastDiagnostics.Sanitized()
}

func operationForPath(path string) Operation {
	switch path {
	case loginPage:
		return OperationLoginDocument
	case captchaPath:
		return OperationCaptcha
	case loginPath:
		return OperationLogin
	case pageLoadPath:
		return OperationReportMetadata
	case searchPath:
		return OperationReportSearch
	case refreshPath:
		return OperationTokenRefresh
	case logoutPath:
		return OperationLogout
	}
	// These comparisons use fixed connector contracts only. Neither paths nor
	// query values are copied into the diagnostic result. Some provider routes
	// serve both lists and details and intentionally have the neutral read label.
	lower := strings.ToLower(path)
	switch lower {
	case "/api/ihke3000/ihke3309s02/delpdf":
		return OperationDocumentCleanup
	case "/api/ihke3000/ihke3408s02/add", "/api/ihke3000/ihke3411s02/add":
		return OperationDocumentPrepare
	case "/api/ihke3000/ihke3309s02/getpdf", "/api/ihke3000/ihke3309s02/getxml",
		"/api/ihke3000/ihke3408s03/page_load", "/api/ihke3000/ihke3411s03/page_load",
		"/api/ihke3000/ihke3801s02/download", "/api/ihke3000/ihke3801s01/download", "/api/ihke3000/ihke3801s02/imgf_download",
		"/api/ihke3000/ihke3801s02/app_download", "/api/ihke3000/ihke3801s02/app_imgf_download",
		"/api/ihke3000/ihke3212s01/ihke3212_pdf", "/api/ihke3000/ihke3212s01/ihke3212_ics",
		"/api/ihke3000/ihke3403s07/download", "/api/ihke3000/ihke3104s01/ihke3104s08":
		return OperationDocumentDownload
	case "/api/ihke3000/ihke3402s01/sp_ihke3402s01", "/api/ihke3000/ihke3408s01/page_load",
		"/api/ihke3000/ihke3408s01/search", "/api/ihke3000/ihke3801s01/page_load":
		return OperationSourceList
	case "/api/ihke3000/ihke3403s05/ihke3403s05", "/api/ihke3000/ihke3408s02/page_load",
		"/api/ihke3000/ihke3801s02/page_load", "/api/ihke3000/ihke3801s02/imgf_page_load",
		"/api/ihke3000/ihke3212s02/page_load", "/api/ihke3000/ihke3504s02/sp_ihke3504s02",
		"/api/ihke3000/ihke3505s02/sp_ihke3505s02", "/api/ihke3000/ihke3507s02/sp_ihke3507s02",
		"/api/ihke3000/ihke3513s02/page_load", "/api/ihke3000/ihke3516s02/sp_ihke3516s02",
		"/api/ihke3000/ihke3517s02/sp_ihke3517s02", "/api/ihke3000/ihke3407s02/get_point",
		"/api/ihke3000/ihke3406s02/sp_ihke3406s02", "/api/ihke3000/ihke3721s01/sp_ihke3721s07",
		"/api/ihke3000/ihke3207s02/sp_ihke3207s04", "/api/ihke3000/ihke3205s04/pageload",
		"/api/ihke3000/ihke3205s02/sp_ihke3205s08", "/api/ihke3000/ihke3411s02/page_load",
		"/api/ihke3000/ihke3205s02/page_load", "/api/ihke3000/ihke3207s05/page_load",
		"/api/ihke3000/ihke3512s02/search", "/api/ihke3000/ihke3726s01/sp_ihke3726s07",
		"/api/ihke3000/ihke3206s01/sp_ihke3206s08", "/api/ihke3000/ihke3303s03/pageload",
		"/api/ihke3000/ihke3303s04/pageload", "/api/ihke3000/ihke3303s05/pageload",
		"/api/ihke3000/ihke3302s02/sp_ihke3302s02", "/api/ihke3000/ihke3302s03/sp_ihke3302s03",
		"/api/ihke3000/ihke3208s01/sp_ihke3208s01", "/api/ihke3000/ihke3208s01/sp_ihke3208s03":
		return OperationSourceDetail
	}
	for _, spec := range medicalSpecs {
		if lower == strings.ToLower(spec.detailPath) {
			return OperationSourceDetail
		}
		if lower == strings.ToLower(spec.listPath) || (spec.searchPath != "" && lower == strings.ToLower(spec.searchPath)) {
			return OperationSourceList
		}
	}
	for _, spec := range supplements {
		if lower == "/api/ihke3000/"+spec.module+"/page_load" || (spec.search && lower == "/api/ihke3000/"+spec.module+"/search") {
			return OperationSourceList
		}
	}
	for _, spec := range providerSpecs {
		for _, request := range spec.requests {
			if lower == strings.ToLower(request.path) {
				return OperationSourceRead
			}
		}
	}
	for _, spec := range providerCareCharts {
		if lower == strings.ToLower(spec.path) {
			return OperationSourceDetail
		}
	}
	for _, spec := range officialExportSpecs {
		switch lower {
		case spec.path + "/page_load":
			return OperationSourceList
		case spec.path + "/add":
			return OperationDocumentPrepare
		}
	}
	if strings.HasPrefix(lower, "/ihke3000/") && strings.HasSuffix(lower, ".pdf") {
		if _, err := officialPDFPath(path); err == nil {
			return OperationDocumentDownload
		}
	}
	return OperationUnknown
}

// A redirect gets a distinct attempt so late hooks from its predecessor cannot
// overwrite its phase or leave a previous connection's TLS state in the result.
// Only the request goroutine replaces current. Hooks access their own attempt.
type requestTrace struct {
	started   time.Time
	operation Operation
	current   *attemptTrace
}

type attemptTrace struct {
	mu              sync.Mutex
	phase           Phase
	proxySelected   bool
	proxyTLS        bool
	tlsStarted      bool
	tlsHandshakes   int
	connectionReady bool
	responseSeen    bool
	connections     int
	ambiguous       bool
	finished        bool
	tlsVersion      string
	tlsProtocol     string
}

type traceContextKey struct{}

type traceContext struct {
	request *requestTrace
	attempt *attemptTrace
}

func startRequestTrace(req *http.Request, operation Operation) (*http.Request, *requestTrace) {
	trace := &requestTrace{started: time.Now(), operation: operation}
	return trace.nextAttempt(req), trace
}

func (trace *requestTrace) nextAttempt(req *http.Request) *http.Request {
	if trace.current != nil {
		trace.current.mu.Lock()
		trace.current.finished = true
		trace.current.mu.Unlock()
	}
	trace.current = &attemptTrace{phase: PhasePrepare}
	ctx := context.WithValue(req.Context(), traceContextKey{}, traceContext{trace, trace.current})
	return req.WithContext(httptrace.WithClientTrace(ctx, trace.current.hooks()))
}

func traceRedirect(req *http.Request) {
	if state, ok := req.Context().Value(traceContextKey{}).(traceContext); ok {
		*req = *state.request.nextAttempt(req)
	}
}

// diagnosticProxy delegates the exact existing environment-proxy decision.
// The URL is neither retained nor rendered; only selection and HTTPS-proxy
// sequencing are observed. It does not introduce or change a proxy setting.
func diagnosticProxy(req *http.Request) (*url.URL, error) {
	proxy, err := http.ProxyFromEnvironment(req)
	observeProxyChoice(req.Context(), proxy)
	return proxy, err
}

func observeProxyChoice(ctx context.Context, proxy *url.URL) {
	state, ok := ctx.Value(traceContextKey{}).(traceContext)
	if !ok {
		return
	}
	a := state.attempt
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.finished {
		a.proxySelected = proxy != nil
		a.proxyTLS = proxy != nil && proxy.Scheme == "https"
	}
}

func (a *attemptTrace) hooks() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		GetConn: func(string) {
			a.mu.Lock()
			defer a.mu.Unlock()
			if a.finished || a.responseSeen {
				return
			}
			a.connections++
			// net/http may internally retry a reused connection. Hooks do not
			// identify those attempts, so do not attribute overlapping events
			// to a specific failed connection. A response remains authoritative.
			if a.connections > 1 {
				a.ambiguous = true
				a.phase, a.tlsVersion, a.tlsProtocol = PhaseUnknown, "", ""
			}
		},
		DNSStart: func(httptrace.DNSStartInfo) {
			a.mu.Lock()
			defer a.mu.Unlock()
			if !a.finished && !a.ambiguous && a.phase == PhasePrepare {
				a.phase = PhaseDNS
			}
		},
		ConnectStart: func(string, string) {
			a.mu.Lock()
			defer a.mu.Unlock()
			if !a.finished && !a.ambiguous && !a.tlsStarted && !a.connectionReady && !a.responseSeen &&
				(a.phase == PhasePrepare || a.phase == PhaseDNS || a.phase == PhaseTCPConnect) {
				a.phase = PhaseTCPConnect
			}
		},
		ConnectDone: func(_, _ string, err error) {
			a.mu.Lock()
			defer a.mu.Unlock()
			if !a.finished && !a.ambiguous && err == nil && a.proxySelected && !a.proxyTLS && !a.tlsStarted && !a.connectionReady && !a.responseSeen {
				a.phase = PhaseProxyConnect
			}
		},
		TLSHandshakeStart: func() {
			a.mu.Lock()
			defer a.mu.Unlock()
			if !a.finished && !a.ambiguous && !a.connectionReady && !a.responseSeen {
				a.tlsStarted = true
				a.tlsHandshakes++
				a.phase = PhaseTLSHandshake
			}
		},
		TLSHandshakeDone: func(_ tls.ConnectionState, err error) {
			a.mu.Lock()
			defer a.mu.Unlock()
			// The first successful handshake to an HTTPS proxy precedes its
			// CONNECT exchange. Never report its TLS as the request's TLS.
			if !a.finished && !a.ambiguous && err == nil && a.proxyTLS && a.tlsHandshakes == 1 && !a.connectionReady && !a.responseSeen {
				a.phase = PhaseProxyConnect
			}
		},
		GotConn: func(info httptrace.GotConnInfo) {
			a.mu.Lock()
			defer a.mu.Unlock()
			if a.finished || a.ambiguous || a.responseSeen {
				return
			}
			a.connectionReady = true
			a.phase = PhaseConnectionReady
			if conn, ok := info.Conn.(*tls.Conn); ok {
				a.observeTLS(conn.ConnectionState())
			}
		},
		WroteHeaders: func() {
			a.mu.Lock()
			defer a.mu.Unlock()
			if !a.finished && !a.ambiguous && !a.responseSeen {
				a.phase = PhaseRequestWrite
			}
		},
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			a.mu.Lock()
			defer a.mu.Unlock()
			if !a.finished && !a.ambiguous && !a.responseSeen {
				a.phase = PhaseRequestWrite
				if info.Err == nil {
					a.phase = PhaseResponseHeaders
				}
			}
		},
		GotFirstResponseByte: func() {
			a.mu.Lock()
			defer a.mu.Unlock()
			if !a.finished && !a.ambiguous && !a.responseSeen {
				a.responseSeen = true
				a.phase = PhaseResponseHeaders
			}
		},
	}
}

// observeTLS is called with the attempt locked, and retains only two fields.
func (a *attemptTrace) observeTLS(state tls.ConnectionState) {
	if !state.HandshakeComplete {
		return
	}
	switch state.Version {
	case tls.VersionTLS12:
		a.tlsVersion = "TLS1.2"
	case tls.VersionTLS13:
		a.tlsVersion = "TLS1.3"
	default:
		return
	}
	if state.NegotiatedProtocol == "h2" || state.NegotiatedProtocol == "http/1.1" {
		a.tlsProtocol = state.NegotiatedProtocol
	}
}

func (trace *requestTrace) response(response *http.Response) {
	a := trace.current
	a.mu.Lock()
	defer a.mu.Unlock()
	a.responseSeen = true
	a.phase = PhaseResponseHeaders
	if response.TLS != nil {
		a.observeTLS(*response.TLS)
	}
}

func (trace *requestTrace) setPhase(phase Phase) {
	a := trace.current
	a.mu.Lock()
	defer a.mu.Unlock()
	a.phase = phase
}

func (trace *requestTrace) finish() *Diagnostics {
	a := trace.current
	a.mu.Lock()
	defer a.mu.Unlock()
	a.finished = true
	return (&Diagnostics{
		Operation: trace.operation, Phase: a.phase, ElapsedMS: time.Since(trace.started).Milliseconds(),
		ProxySelected: a.proxySelected, TLSVersion: a.tlsVersion, TLSProtocol: a.tlsProtocol,
	}).Sanitized()
}

// annotateResponseError is used only immediately after processing a completed
// response. Input validation and expired local sessions must not inherit it.
func (s *Session) annotateResponseError(err error) error {
	var upstream *Error
	if !errors.As(err, &upstream) {
		return err
	}
	copy := *upstream
	copy.Diagnostics = s.lastDiagnostics.Sanitized()
	return &copy
}
