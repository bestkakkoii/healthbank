// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package server

import (
	"strings"
	"testing"

	"github.com/bestkakkoii/healthbank/internal/config"
	"github.com/bestkakkoii/healthbank/internal/lab"
	"github.com/bestkakkoii/healthbank/internal/nhi"
)

func TestOfficialReportPreservesSourceNameAndIndependentUploadDate(t *testing.T) {
	app, db, _ := testApp(t)
	s := &session{settings: lab.DefaultSettings()}
	row := nhi.Record{CheckDate: "", VisitDate: "113/06/01", UploadDate: "113/07/02 09:15", Source: "A", SourceName: "合成原始來源說明", ItemName: "ALT", Result: "17.000000000000000001", Unit: "U/L", OfficialFlag: "合成旗標"}
	s.setOfficialReport(nhi.Report{Records: []nhi.Record{row, row}, ReportedCount: 2, Mode: nhi.QueryModeCustom, StartDate: "2024-01-01", EndDate: "2024-12-31"})
	response := app.currentResponse(s)
	if len(response.Records) != 2 || response.Records[0].ID == response.Records[1].ID {
		t.Fatal("source metadata mapping merged duplicate observations")
	}
	for _, record := range response.Records {
		if record.ExamDate != "" || record.RawCheckDate != "" || record.VisitDate != "2024-06-01" || record.UploadDate != "2024-07-02" || record.RawUploadDate != row.UploadDate || record.Source != "A" || record.SourceName != row.SourceName || record.Result != row.Result || record.OfficialFlag != row.OfficialFlag {
			t.Fatal("official-to-raw mapping dropped source metadata or used an upload date as a clinical date")
		}
	}
	noPersistentProfiles(t, db)
}

func TestSourceMetadataParticipatesInRetainedReportAdmission(t *testing.T) {
	for _, field := range []string{"upload_date", "source_name"} {
		t.Run(field, func(t *testing.T) {
			budget := newResourceBudget(config.DefaultResourceLimits())
			memory := &sessionMemory{budget: budget}
			prior := []lab.RawRecord{{ItemName: "SYNTHETIC_PRIOR", Result: "0"}}
			if !memory.replaceReports(prior) {
				t.Fatal("prior synthetic report was not admitted")
			}
			budget.mu.Lock()
			previous := memory.reports
			budget.limits.SessionBytes = previous + 512
			budget.mu.Unlock()
			next := append([]lab.RawRecord(nil), prior...)
			if field == "upload_date" {
				next[0].UploadDate = strings.Repeat("x", 1024)
			} else {
				next[0].SourceName = strings.Repeat("x", 1024)
			}
			if memory.replaceReports(next) {
				t.Fatal("new raw source metadata bypassed retained-memory admission")
			}
			budget.mu.Lock()
			unchanged := memory.reports == previous && budget.retained == previous
			budget.mu.Unlock()
			if !unchanged {
				t.Fatal("rejected source metadata changed previous retained accounting")
			}
			memory.releaseReports()
			budget.mu.Lock()
			released := memory.reports == 0 && budget.retained == 0
			budget.mu.Unlock()
			if !released {
				t.Fatal("report release leaked source metadata accounting")
			}
		})
	}
}
