// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"testing"
)

func TestDiagnosticsTracksNetworkPhasesWithoutSensitiveDetails(t *testing.T) {
	for _, tc := range []struct {
		name  string
		phase Phase
		proxy string
		code  string
		cause error
		emit  func(*httptrace.ClientTrace)
	}{
		{name: "dns", phase: PhaseDNS, code: "DNS_FAILED", cause: &net.DNSError{Name: "SECRET_HOST", Err: "SECRET_DNS"}, emit: func(trace *httptrace.ClientTrace) {
			trace.DNSStart(httptrace.DNSStartInfo{Host: "SECRET_HOST"})
		}},
		{name: "tcp", phase: PhaseTCPConnect, code: "CONNECTION_RESET", cause: syscall.ECONNRESET, emit: func(trace *httptrace.ClientTrace) {
			trace.ConnectStart("tcp", "SECRET_ADDRESS")
			trace.ConnectDone("tcp", "SECRET_ADDRESS", syscall.ECONNRESET)
		}},
		{name: "tls", phase: PhaseTLSHandshake, code: "CONNECTION_RESET", cause: syscall.ECONNRESET, emit: func(trace *httptrace.ClientTrace) {
			trace.ConnectStart("tcp", "SECRET_ADDRESS")
			trace.ConnectDone("tcp", "SECRET_ADDRESS", nil)
			trace.TLSHandshakeStart()
			// A losing Happy Eyeballs dial must not regress the TLS stage.
			trace.ConnectStart("tcp", "SECRET_OTHER_ADDRESS")
			trace.ConnectDone("tcp", "SECRET_OTHER_ADDRESS", syscall.ECONNREFUSED)
			trace.TLSHandshakeDone(tls.ConnectionState{}, syscall.ECONNRESET)
		}},
		{name: "http_proxy_connect", phase: PhaseProxyConnect, proxy: "http", code: "CONNECTION_RESET", cause: syscall.ECONNRESET, emit: func(trace *httptrace.ClientTrace) {
			trace.ConnectStart("tcp", "SECRET_PROXY")
			trace.ConnectDone("tcp", "SECRET_PROXY", nil)
			trace.ConnectStart("tcp", "SECRET_OTHER_PROXY_ADDRESS")
			trace.ConnectDone("tcp", "SECRET_OTHER_PROXY_ADDRESS", syscall.ECONNREFUSED)
		}},
		{name: "https_proxy_connect", phase: PhaseProxyConnect, proxy: "https", code: "CONNECTION_RESET", cause: syscall.ECONNRESET, emit: func(trace *httptrace.ClientTrace) {
			trace.ConnectStart("tcp", "SECRET_PROXY")
			trace.ConnectDone("tcp", "SECRET_PROXY", nil)
			trace.TLSHandshakeStart()
			trace.TLSHandshakeDone(tls.ConnectionState{HandshakeComplete: true, Version: tls.VersionTLS13, NegotiatedProtocol: "h2", ServerName: "SECRET_PROXY"}, nil)
		}},
		{name: "write", phase: PhaseRequestWrite, code: "CONNECTION_RESET", cause: syscall.ECONNRESET, emit: func(trace *httptrace.ClientTrace) {
			trace.GotConn(httptrace.GotConnInfo{})
			trace.WroteHeaders()
			trace.WroteRequest(httptrace.WroteRequestInfo{Err: syscall.ECONNRESET})
		}},
		{name: "await_headers", phase: PhaseResponseHeaders, code: "CONNECTION_RESET", cause: syscall.ECONNRESET, emit: func(trace *httptrace.ClientTrace) {
			trace.GotConn(httptrace.GotConnInfo{})
			trace.WroteHeaders()
			trace.WroteRequest(httptrace.WroteRequestInfo{})
		}},
		{name: "internal_retry_ambiguous", phase: PhaseUnknown, code: "CONNECTION_RESET", cause: syscall.ECONNRESET, emit: func(trace *httptrace.ClientTrace) {
			trace.GotConn(httptrace.GotConnInfo{Reused: true})
			trace.WroteRequest(httptrace.WroteRequestInfo{Err: syscall.ECONNRESET})
			trace.GetConn("SECRET_RETRY_ADDRESS")
			trace.TLSHandshakeStart()
			trace.TLSHandshakeDone(tls.ConnectionState{}, syscall.ECONNRESET)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session, err := New()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(session.Close)
			if session.Snapshot() != nil {
				t.Fatal("new session retained a previous request's diagnostics")
			}
			calls, inheritedHooks := 0, 0
			session.client.Transport = errorTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				trace := httptrace.ContextClientTrace(r.Context())
				if trace == nil {
					t.Fatal("request has no transport trace")
				}
				if tc.proxy != "" {
					observeProxyChoice(r.Context(), &url.URL{Scheme: tc.proxy, Host: "SECRET_PROXY", User: url.UserPassword("SECRET_USER", "SECRET_PASSWORD")})
				}
				trace.GetConn("SECRET_ADDRESS")
				tc.emit(trace)
				return nil, &url.Error{Op: "Get", URL: "https://SECRET_USER:SECRET_PASSWORD@SECRET_HOST/SECRET_PATH?captcha=SECRET_ANSWER", Err: tc.cause}
			})
			ctx := httptrace.WithClientTrace(context.Background(), &httptrace.ClientTrace{GetConn: func(string) { inheritedHooks++ }})
			_, err = session.Challenge(ctx)
			var actual *Error
			if !errors.As(err, &actual) || actual.Code != tc.code || actual.Diagnostics == nil {
				t.Fatal("failed request lost its safe typed cause or diagnostics")
			}
			diagnostic := actual.Diagnostics
			if diagnostic.Operation != OperationLoginDocument || diagnostic.Phase != tc.phase || diagnostic.ProxySelected != (tc.proxy != "") || diagnostic.TLSVersion != "" || diagnostic.TLSProtocol != "" || diagnostic.ElapsedMS < 0 || diagnostic.ElapsedMS > 60000 {
				t.Fatalf("unexpected bounded transport state: %#v", diagnostic)
			}
			expectedHooks := 1
			if tc.name == "internal_retry_ambiguous" {
				expectedHooks = 2
			}
			if calls != 1 || inheritedHooks != expectedHooks {
				t.Fatal("diagnostics added a request or replaced inherited trace hooks")
			}
			encoded, err := json.Marshal(actual)
			if err != nil || strings.Contains(string(encoded), "SECRET") || strings.Contains(string(encoded), "https://") {
				t.Fatal("diagnostic output retained sensitive upstream details")
			}
			actual.Diagnostics.Phase = PhaseUnknown
			snapshot := session.Snapshot()
			if snapshot == nil || snapshot.Phase != tc.phase {
				t.Fatal("error diagnostics shared mutable state with session snapshot")
			}
			snapshot.Operation = OperationUnknown
			if session.Snapshot().Operation != OperationLoginDocument {
				t.Fatal("snapshot accessor exposed mutable session state")
			}
		})
	}
}

func TestDiagnosticsPreservesResponsePhaseAndNegotiatedTLS(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   io.ReadCloser
		phase  Phase
		code   string
	}{
		{name: "http_rejection", status: http.StatusServiceUnavailable, body: io.NopCloser(strings.NewReader("SECRET_BODY")), phase: PhaseResponseHeaders, code: "UPSTREAM_UNAVAILABLE"},
		{name: "body_reset", status: http.StatusOK, body: interruptedBody{cause: syscall.ECONNRESET}, phase: PhaseResponseBody, code: "CONNECTION_RESET"},
		{name: "invalid_html_after_complete_read", status: http.StatusOK, body: io.NopCloser(strings.NewReader("SECRET_BODY")), phase: PhaseComplete, code: "LOGIN_PAGE_INVALID"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session, err := New()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(session.Close)
			calls := 0
			session.client.Transport = errorTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				observeProxyChoice(r.Context(), &url.URL{Scheme: "http", Host: "SECRET_PROXY"})
				return &http.Response{
					StatusCode: tc.status, Header: http.Header{"Content-Type": {"text/html"}}, Body: tc.body, Request: r,
					TLS: &tls.ConnectionState{HandshakeComplete: true, Version: tls.VersionTLS13, NegotiatedProtocol: "h2", ServerName: "SECRET_TLS_PEER"},
				}, nil
			})
			_, err = session.Challenge(context.Background())
			var actual *Error
			if !errors.As(err, &actual) || actual.Code != tc.code || actual.Diagnostics == nil {
				t.Fatal("response failure lost its safe diagnostics")
			}
			d := actual.Diagnostics
			if calls != 1 || d.Phase != tc.phase || d.TLSVersion != "TLS1.3" || d.TLSProtocol != "h2" || !d.ProxySelected {
				t.Fatal("response phase or observed negotiated TLS was incorrect")
			}
			if tc.code != "LOGIN_PAGE_INVALID" && actual.HTTPStatus != tc.status {
				t.Fatal("transport diagnostics replaced the original HTTP status")
			}
			encoded, err := json.Marshal(actual)
			if err != nil || strings.Contains(string(encoded), "SECRET") {
				t.Fatal("response diagnostics leaked body or TLS peer details")
			}
		})
	}
}

func TestDiagnosticsRedirectStartsCleanAttempt(t *testing.T) {
	serverCalls := 0
	session := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
		serverCalls++
		http.Redirect(w, r, officialOrigin+"/IHKE3000/synthetic-redirect", http.StatusFound)
	})
	firstTransport := session.client.Transport
	var firstHooks *httptrace.ClientTrace
	calls, firstTLSObserved := 0, false
	session.client.Transport = errorTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			firstHooks = httptrace.ContextClientTrace(r.Context())
			observeProxyChoice(r.Context(), &url.URL{Scheme: "https", Host: "SECRET_PROXY"})
			response, err := firstTransport.RoundTrip(r)
			state := r.Context().Value(traceContextKey{}).(traceContext)
			state.attempt.mu.Lock()
			firstTLSObserved = state.attempt.tlsVersion != ""
			state.attempt.mu.Unlock()
			return response, err
		}
		if calls != 2 || r.URL.Path != "/IHKE3000/synthetic-redirect" {
			t.Fatal("diagnostic collection changed the redirect request count or route")
		}
		trace := httptrace.ContextClientTrace(r.Context())
		trace.GetConn("SECRET_SECOND_ADDRESS")
		trace.DNSStart(httptrace.DNSStartInfo{Host: "SECRET_SECOND_HOST"})
		// Late events from the completed TLS hop cannot rewrite the DNS failure.
		firstHooks.TLSHandshakeStart()
		firstHooks.WroteRequest(httptrace.WroteRequestInfo{})
		return nil, &net.DNSError{Name: "SECRET_SECOND_HOST", Err: "SECRET_DNS"}
	})
	_, err := session.Challenge(context.Background())
	var actual *Error
	if !errors.As(err, &actual) || actual.Code != "DNS_FAILED" || actual.Diagnostics == nil {
		t.Fatal("redirect failure did not retain a typed cause and diagnostics")
	}
	d := actual.Diagnostics
	if serverCalls != 1 || calls != 2 || !firstTLSObserved || d.Operation != OperationLoginDocument || d.Phase != PhaseDNS || d.ProxySelected || d.TLSVersion != "" || d.TLSProtocol != "" {
		t.Fatal("redirect failure inherited the previous hop's phase, proxy or TLS state")
	}
}

func TestDiagnosticsConcurrentLateHooksCannotRegressOrMutateSnapshot(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, officialOrigin+loginPage, nil)
	if err != nil {
		t.Fatal(err)
	}
	req, trace := startRequestTrace(req, OperationLoginDocument)
	hooks := httptrace.ContextClientTrace(req.Context())
	hooks.GetConn("SECRET_ADDRESS")
	hooks.ConnectStart("tcp", "SECRET_ADDRESS")
	hooks.ConnectDone("tcp", "SECRET_ADDRESS", nil)
	hooks.TLSHandshakeStart()
	var pending sync.WaitGroup
	for range 12 {
		pending.Go(func() {
			hooks.DNSStart(httptrace.DNSStartInfo{Host: "SECRET_HOST"})
			hooks.ConnectStart("tcp", "SECRET_OTHER_ADDRESS")
			hooks.ConnectDone("tcp", "SECRET_OTHER_ADDRESS", syscall.ECONNREFUSED)
		})
	}
	pending.Wait()
	before := trace.finish()
	for range 12 {
		pending.Go(func() {
			hooks.GetConn("SECRET_LATE_ADDRESS")
			hooks.GotConn(httptrace.GotConnInfo{})
			hooks.WroteHeaders()
			hooks.WroteRequest(httptrace.WroteRequestInfo{})
			hooks.GotFirstResponseByte()
		})
	}
	pending.Wait()
	after := trace.finish()
	if before.Phase != PhaseTLSHandshake || after.Phase != PhaseTLSHandshake || before.TLSVersion != "" || after.TLSVersion != "" {
		t.Fatal("concurrent or late callbacks changed a completed snapshot's phase")
	}
}

func TestDiagnosticsSanitizerBoundsUntrustedConstruction(t *testing.T) {
	if (*Diagnostics)(nil).Sanitized() != nil {
		t.Fatal("nil diagnostic became an invented observation")
	}
	untrusted := &Diagnostics{
		Operation: "SECRET_URL", Phase: "SECRET_ADDRESS", ElapsedMS: 1 << 60,
		ProxySelected: true, TLSVersion: "SECRET_VERSION", TLSProtocol: "SECRET_PROTOCOL",
	}
	safe := untrusted.Sanitized()
	if safe.Operation != OperationUnknown || safe.Phase != PhaseUnknown || safe.ElapsedMS != 310000 || !safe.ProxySelected || safe.TLSVersion != "" || safe.TLSProtocol != "" || untrusted.Operation != "SECRET_URL" {
		t.Fatal("diagnostic sanitizer failed to bound values or changed its input")
	}
	negative := (&Diagnostics{Operation: OperationCaptcha, Phase: PhaseComplete, ElapsedMS: -1, TLSVersion: "TLS1.2", TLSProtocol: "SECRET_PROTOCOL"}).Sanitized()
	if negative.ElapsedMS != 0 || negative.TLSVersion != "TLS1.2" || negative.TLSProtocol != "" {
		t.Fatal("diagnostic sanitizer did not constrain time and negotiated protocol")
	}
}

func TestDiagnosticsPreservesConfiguredEnvironmentProxyChoice(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, officialOrigin+loginPage, nil)
	if err != nil {
		t.Fatal(err)
	}
	req, trace := startRequestTrace(req, OperationLoginDocument)
	expected, expectedErr := http.ProxyFromEnvironment(req)
	actual, actualErr := diagnosticProxy(req)
	if !errors.Is(actualErr, expectedErr) || (actual == nil) != (expected == nil) {
		t.Fatal("diagnostic observer changed the configured environment-proxy decision")
	}
	if actual != nil && actual.String() != expected.String() {
		t.Fatal("diagnostic observer changed the configured proxy")
	}
	if trace.finish().ProxySelected != (expected != nil) {
		t.Fatal("diagnostic observer did not record the actual proxy selection")
	}
}
