// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bestkakkoii/healthbank/internal/config"
	"github.com/bestkakkoii/healthbank/internal/lab"
	"github.com/bestkakkoii/healthbank/internal/store"
)

func resourceTestApp(t *testing.T, limits config.ResourceLimits) (*App, *store.Store) {
	t.Helper()
	db, err := store.Open(context.Background(), store.Options{SQLitePath: filepath.Join(t.TempDir(), "settings.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	a, err := NewWithLimits(db, bytes.Repeat([]byte{7}, 32), "synthetic-resource", limits)
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close(); _ = db.Close() })
	return a, db
}

func resourceRequest(b *browserFixture, path string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "https://healthbank.test"+path, nil)
	r.AddCookie(b.cookie)
	return r
}

func awaitPending(t *testing.T, a *App, cookie string, expected int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		a.mu.Lock()
		s := a.sessions[cookie]
		pending := s.pending
		a.mu.Unlock()
		if pending == expected {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("request did not reach the bounded session queue")
}

func TestAnonymousCreationCannotExhaustWorkspaceSessions(t *testing.T) {
	a, _, _ := testApp(t)
	owner := &browserFixture{app: a}
	owner.refresh(t)
	decodeResponse[reportResponse](t, owner.jsonRequest(t, http.MethodPost, "/api/reports/demo", map[string]any{}), http.StatusOK)
	owner.refresh(t)
	for range 80 {
		w := (&browserFixture{app: a}).request(http.MethodGet, "/api/session", nil, "", "")
		if w.Code != http.StatusOK && w.Code != http.StatusTooManyRequests {
			t.Fatalf("unexpected anonymous status %d", w.Code)
		}
	}
	a.mu.Lock()
	if len(a.sessions) > a.limits.AnonymousSessions+1 || a.sessions[owner.cookie.Value] == nil {
		a.mu.Unlock()
		t.Fatal("anonymous reads consumed reserved workspace capacity")
	}
	a.mu.Unlock()
	if got := decodeResponse[reportResponse](t, owner.request(http.MethodGet, "/api/reports/current", nil, "", ""), http.StatusOK); len(got.Records) != 82 {
		t.Fatal("creation flood evicted an existing workspace")
	}
	// Refill logical creation allowance without waiting; oldest untouched
	// anonymous state may be reclaimed, while the data-bearing session survives.
	a.budget.mu.Lock()
	a.budget.creationAt = time.Now().Add(-time.Minute)
	a.budget.mu.Unlock()
	decodeResponse[sessionResponse](t, (&browserFixture{app: a}).request(http.MethodGet, "/api/session", nil, "", ""), http.StatusOK)
	decodeResponse[reportResponse](t, owner.request(http.MethodGet, "/api/reports/current", nil, "", ""), http.StatusOK)
}

func TestAnonymousQuotaProtectsActiveChallengeAndShortensIdleOnly(t *testing.T) {
	limits := config.DefaultResourceLimits()
	limits.Sessions, limits.AnonymousSessions = 3, 1
	a, _ := resourceTestApp(t, limits)
	first := &browserFixture{app: a}
	first.refresh(t)
	a.mu.Lock()
	s := a.sessions[first.cookie.Value]
	s.reclaimable = false
	a.mu.Unlock()
	a.budget.mu.Lock()
	a.budget.creationAt = time.Now().Add(-time.Minute)
	a.budget.mu.Unlock()
	w := (&browserFixture{app: a}).request(http.MethodGet, "/api/session", nil, "", "")
	if w.Code != http.StatusTooManyRequests || !strings.Contains(w.Body.String(), "anonymous_capacity") {
		t.Fatal("challenge was evicted or anonymous quota ignored")
	}
	old := first.cookie.Value
	a.mu.Lock()
	s.touched = time.Now().Add(-anonymousLifetime - time.Second)
	a.mu.Unlock()
	a.budget.mu.Lock()
	a.budget.creationAt = time.Now().Add(-time.Minute)
	a.budget.mu.Unlock()
	first.refresh(t)
	if first.cookie.Value == old {
		t.Fatal("anonymous idle lifetime was not enforced")
	}
	decodeResponse[reportResponse](t, first.jsonRequest(t, http.MethodPost, "/api/reports/demo", map[string]any{}), http.StatusOK)
	a.mu.Lock()
	a.sessions[first.cookie.Value].touched = time.Now().Add(-anonymousLifetime - time.Second)
	a.mu.Unlock()
	if got := decodeResponse[reportResponse](t, first.request(http.MethodGet, "/api/reports/current", nil, "", ""), http.StatusOK); len(got.Records) != 82 {
		t.Fatal("short anonymous lifetime erased a workspace")
	}
}

func TestAnonymousAllowanceIsRestoredAfterWorkspaceLogouts(t *testing.T) {
	limits := config.DefaultResourceLimits()
	limits.Sessions, limits.AnonymousSessions = 4, 1
	a, _ := resourceTestApp(t, limits)
	first, second := &browserFixture{app: a}, &browserFixture{app: a}
	for _, b := range []*browserFixture{first, second} {
		a.budget.mu.Lock()
		a.budget.creationAt = time.Now().Add(-time.Minute)
		a.budget.mu.Unlock()
		b.refresh(t)
		decodeResponse[reportResponse](t, b.jsonRequest(t, http.MethodPost, "/api/reports/demo", map[string]any{}), http.StatusOK)
		b.refresh(t)
	}
	for _, b := range []*browserFixture{first, second} {
		decodeResponse[sessionResponse](t, b.jsonRequest(t, http.MethodPost, "/api/auth/logout", map[string]any{}), http.StatusOK)
	}
	a.mu.Lock()
	count := len(a.sessions)
	_, newest := a.sessions[second.cookie.Value]
	a.mu.Unlock()
	if count != 1 || !newest {
		t.Fatal("empty logout sessions accumulated beyond the anonymous allowance")
	}
}

func TestSessionQueueIsBoundedAndCanceledWaiterDoesNotRun(t *testing.T) {
	limits := config.DefaultResourceLimits()
	limits.SessionQueue = 1
	a, _ := resourceTestApp(t, limits)
	b := &browserFixture{app: a}
	b.refresh(t)
	entered, release, finished := make(chan struct{}, 2), make(chan struct{}), make(chan struct{})
	handler := a.withSession(func(w http.ResponseWriter, _ *http.Request, _ *session) {
		entered <- struct{}{}
		<-release
		w.WriteHeader(http.StatusNoContent)
	})
	defer close(release)
	go func() { defer close(finished); handler(httptest.NewRecorder(), resourceRequest(b, "/api/test/hold")) }()
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	queued, response := make(chan struct{}), httptest.NewRecorder()
	go func() { defer close(queued); handler(response, resourceRequest(b, "/api/test/hold").WithContext(ctx)) }()
	awaitPending(t, a, b.cookie.Value, 2)
	overflow := httptest.NewRecorder()
	handler(overflow, resourceRequest(b, "/api/test/hold"))
	if overflow.Code != http.StatusTooManyRequests {
		t.Fatal("session queue admitted an extra waiter")
	}
	cancel()
	select {
	case <-queued:
	case <-time.After(time.Second):
		t.Fatal("canceled queued request remained blocked")
	}
	if response.Code != http.StatusRequestTimeout || len(entered) != 0 {
		t.Fatal("canceled waiter executed or lost cancellation classification")
	}
	awaitPending(t, a, b.cookie.Value, 1)
}

func TestCanceledRequestCannotAcquireIdleSession(t *testing.T) {
	a, _, _ := testApp(t)
	b := &browserFixture{app: a}
	b.refresh(t)
	ran := false
	handler := a.withSession(func(http.ResponseWriter, *http.Request, *session) { ran = true })
	for range 10 {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		w := httptest.NewRecorder()
		handler(w, resourceRequest(b, "/api/test/canceled").WithContext(ctx))
		if ran || w.Code != http.StatusRequestTimeout {
			t.Fatal("already-canceled request won admission and executed")
		}
	}
}

func TestAggregateHeavyAdmissionAndControlHeadroom(t *testing.T) {
	limits := config.DefaultResourceLimits()
	a, _ := resourceTestApp(t, limits)
	browsers := []*browserFixture{{app: a}, {app: a}, {app: a}}
	for _, b := range browsers {
		b.refresh(t)
	}
	entered, release, completed := make(chan struct{}, 2), make(chan struct{}), make(chan struct{}, 2)
	handler := a.withSession(func(w http.ResponseWriter, _ *http.Request, _ *session) {
		entered <- struct{}{}
		<-release
		w.WriteHeader(http.StatusNoContent)
	})
	defer func() { close(release); <-completed; <-completed }()
	for _, b := range browsers[:2] {
		go func() {
			defer func() { completed <- struct{}{} }()
			handler(httptest.NewRecorder(), resourceRequest(b, "/api/reports"))
		}()
	}
	<-entered
	<-entered
	blocked := httptest.NewRecorder()
	handler(blocked, resourceRequest(browsers[2], "/api/reports"))
	if blocked.Code != http.StatusServiceUnavailable || !strings.Contains(blocked.Body.String(), "resource_capacity") {
		t.Fatal("aggregate heavy admission was not enforced")
	}
	decodeResponse[sessionResponse](t, browsers[2].request(http.MethodGet, "/api/session", nil, "", ""), http.StatusOK)
	if w := browsers[2].request(http.MethodGet, "/api/health", nil, "", ""); w.Code != http.StatusOK {
		t.Fatal("heavy operations blocked liveness")
	}
}

func TestGlobalRequestAdmissionBoundsAllSessionWaiters(t *testing.T) {
	limits := config.DefaultResourceLimits()
	limits.Requests = 2
	a, _ := resourceTestApp(t, limits)
	first, second := &browserFixture{app: a}, &browserFixture{app: a}
	first.refresh(t)
	second.refresh(t)
	entered, release, completed := make(chan struct{}, 2), make(chan struct{}), make(chan struct{}, 2)
	handler := a.withSession(func(http.ResponseWriter, *http.Request, *session) { entered <- struct{}{}; <-release })
	defer func() { close(release); <-completed; <-completed }()
	for _, b := range []*browserFixture{first, second} {
		go func() {
			defer func() { completed <- struct{}{} }()
			handler(httptest.NewRecorder(), resourceRequest(b, "/api/test/hold"))
		}()
	}
	<-entered
	<-entered
	w := first.request(http.MethodGet, "/api/session", nil, "", "")
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "request_capacity") {
		t.Fatal("global request capacity ignored pending work")
	}
	if w := first.request(http.MethodGet, "/api/health", nil, "", ""); w.Code != http.StatusOK {
		t.Fatal("global admission blocked stateless liveness")
	}
}

func TestRetainedCapacityPreservesOldReportAndReleasesOnLogout(t *testing.T) {
	limits := config.DefaultResourceLimits()
	limits.RetainedBytes, limits.SessionBytes = 16<<20, 4<<20
	a, db := resourceTestApp(t, limits)
	b := &browserFixture{app: a}
	b.refresh(t)
	decodeResponse[reportResponse](t, b.jsonRequest(t, http.MethodPost, "/api/reports/demo", map[string]any{}), http.StatusOK)
	b.refresh(t)
	oldCookie, oldToken := b.cookie.Value, b.csrf
	row := map[string]string{"item_name": "Synthetic item", "result": "17.125", "source": strings.Repeat("SYNTHETIC_", 3200)}
	rows := make([]map[string]string, 150)
	for i := range rows {
		rows[i] = row
	}
	data, err := json.Marshal(map[string]any{"schema": "healthbank.lab.v1", "records": rows})
	if err != nil {
		t.Fatal(err)
	}
	body, contentType := importMultipart(t, "synthetic-capacity.json", bytes.NewReader(data))
	w := b.request(http.MethodPost, "/api/reports/import", body, contentType, b.csrf)
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "retained_capacity") || b.cookie.Value != oldCookie {
		t.Fatal("retention rejection changed session or accepted excessive report")
	}
	if current := decodeResponse[reportResponse](t, b.request(http.MethodGet, "/api/reports/current", nil, "", ""), http.StatusOK); len(current.Records) != 82 || current.SessionToken != oldToken {
		t.Fatal("failed capacity admission replaced the old report")
	}
	a.budget.mu.Lock()
	retained := a.budget.retained
	a.budget.mu.Unlock()
	if retained <= 0 || retained > limits.SessionBytes {
		t.Fatal("retained accounting was lost after rejection")
	}
	decodeResponse[sessionResponse](t, b.jsonRequest(t, http.MethodPost, "/api/auth/logout", map[string]any{}), http.StatusOK)
	a.budget.mu.Lock()
	retained = a.budget.retained
	a.budget.mu.Unlock()
	if retained != 0 {
		t.Fatal("logout retained charged medical data")
	}
	noPersistentProfiles(t, db)
}

func TestReferenceAndReportReservationsShareAtomicCapacity(t *testing.T) {
	limits := config.DefaultResourceLimits()
	limits.RetainedBytes, limits.SessionBytes = 16<<20, 8<<20
	budget := newResourceBudget(limits)
	first, second, third := &sessionMemory{budget: budget}, &sessionMemory{budget: budget}, &sessionMemory{budget: budget}
	if !first.referenceBudget(0, 8<<20) || !second.referenceBudget(0, 8<<20) || third.referenceBudget(0, 1) || first.replaceReports([]lab.RawRecord{{Result: "1"}}) {
		t.Fatal("shared aggregate or per-session retention bound was bypassed")
	}
	if !first.referenceBudget(8<<20, 0) || !third.replaceReports([]lab.RawRecord{{Result: "1"}}) {
		t.Fatal("released references did not restore capacity")
	}
	third.releaseReports()
	if !second.referenceBudget(8<<20, 0) {
		t.Fatal("release was rejected")
	}
	if budget.retained != 0 {
		t.Fatal("accounting leaked after all owners released")
	}
}

func TestWorkingReservationLimitsResponseBuffersIndependentlyOfHeavyCount(t *testing.T) {
	budget := newResourceBudget(config.DefaultResourceLimits())
	release, ok := budget.acquireWorking(192<<20, true)
	if !ok {
		t.Fatal("single supported buffered operation was rejected")
	}
	if extra, ok := budget.acquireWorking(96<<20, false); ok {
		extra()
		t.Fatal("response allocation bypassed the total working allowance")
	}
	control, ok := budget.acquireWorking(0, false)
	if !ok {
		t.Fatal("buffer admission exhausted small control responses")
	}
	control()
	release()
	if budget.working != 0 || budget.heavy != 0 {
		t.Fatal("working slots leaked on release")
	}
}
