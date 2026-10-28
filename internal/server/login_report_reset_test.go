// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package server

import (
	"testing"

	"github.com/bestkakkoii/healthbank/internal/config"
	"github.com/bestkakkoii/healthbank/internal/lab"
	"github.com/bestkakkoii/healthbank/internal/nhi"
)

func TestSuccessfulLoginReportResetReleasesOnlyPriorReport(t *testing.T) {
	for _, mode := range []string{"import", "demo"} {
		t.Run(mode, func(t *testing.T) {
			budget := newResourceBudget(config.DefaultResourceLimits())
			memory := &sessionMemory{budget: budget}
			rows := []lab.RawRecord{{ItemName: "Synthetic original item", Result: "12.000000000000000001", SourceName: "Synthetic prior source"}}
			if !memory.replaceReports(rows) || !memory.referenceBudget(0, 256) {
				t.Fatal("synthetic prior workspace admission failed")
			}
			connector, err := nhi.New()
			if err != nil {
				t.Fatal(err)
			}
			defer connector.Close()
			s := &session{mode: mode, memory: memory, official: connector, raw: rows, reportRevision: 7,
				officialQuery: &officialQueryMetadata{}, cacheProvenance: &cacheReportProvenance{Kind: "device_cache"},
				complete: true, fetched: "2026-01-02T03:04:05Z", warnings: []string{"synthetic prior warning"}}
			// The route calls this only after official authentication succeeds.
			// It must not destroy the connector that just authenticated or leave
			// the previous workspace consuming another session's retained budget.
			s.mu.Lock()
			s.mode, s.subject = "official", "synthetic-verified-subject"
			s.resetReportAfterLogin()
			s.mu.Unlock()
			if len(s.raw) != 0 || s.cacheProvenance != nil || s.officialQuery != nil || s.complete || s.fetched != "" || len(s.warnings) != 0 || s.reportRevision != 8 {
				t.Fatal("successful login retained prior report provenance or restore revision")
			}
			if s.official != connector || s.mode != "official" || s.subject != "synthetic-verified-subject" {
				t.Fatal("report reset changed the authenticated identity or connector")
			}
			budget.mu.Lock()
			retained, reports, references := budget.retained, memory.reports, memory.references
			budget.mu.Unlock()
			if reports != 0 || references != 256 || retained != references {
				t.Fatal("report reset leaked retained bytes or released unrelated reference accounting")
			}
		})
	}
}
