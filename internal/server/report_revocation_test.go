// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bestkakkoii/healthbank/internal/nhi"
)

func TestOfficialReportPublicationStopsAfterSubjectRevocation(t *testing.T) {
	app, db, _ := testApp(t)
	browser := &browserFixture{app: app}
	browser.refresh(t)
	app.mu.Lock()
	s := app.sessions[browser.cookie.Value]
	app.mu.Unlock()
	s.mu.Lock()
	app.bindSubject(s, strings.Repeat("a", 64))
	s.mode = "official"
	scope := s.scope
	s.mu.Unlock()
	// Represent a second session holding the same subject's mutation slot.
	// Report retrieval has already succeeded; only settings discovery waits.
	scope.mutation <- struct{}{}
	defer func() { <-scope.mutation }()
	request := httptest.NewRequest(http.MethodGet, "/api/reports", nil)
	response := httptest.NewRecorder()
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		s.mu.Lock()
		defer s.mu.Unlock()
		app.publishOfficialReport(response, request, s, nhi.Report{
			Records: []nhi.Record{{CheckDate: "110/02/03", ItemName: "SYNTHETIC_REVOCATION_RESULT", Result: "2.000000000000000001", Unit: "synthetic"}},
			Mode:    nhi.QueryModeCustom, StartDate: "2021-01-01", EndDate: "2021-12-31", ReportedCount: 1,
		})
	}()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	waiting := true
	for waiting {
		app.budget.mu.Lock()
		admitted := s.memory.reports > 0
		app.budget.mu.Unlock()
		if admitted {
			break
		}
		select {
		case <-tick.C:
		case <-deadline.C:
			scope.cancel()
			<-finished
			t.Fatal("synthetic report did not reach post-fetch admission")
		case <-finished:
			waiting = false
			t.Fatalf("report was published before the mutation slot was released: %d", response.Code)
		}
	}
	scope.cancel()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("revoked publication did not stop")
	}
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "session_changed") || strings.Contains(response.Body.String(), "SYNTHETIC_REVOCATION_RESULT") || strings.Contains(response.Body.String(), "2.000000000000000001") {
		t.Fatal("revoked report was exposed as a successful response")
	}
	noPersistentProfiles(t, db)
}
