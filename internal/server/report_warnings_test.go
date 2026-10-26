// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package server

import (
	"github.com/bestkakkoii/healthbank/internal/lab"
	"strings"
	"testing"
)

func TestLDLWarningIncludesOlderBoundedTriglycerides(t *testing.T) {
	app, _, _ := testApp(t)
	s := &session{settings: lab.DefaultSettings(), raw: []lab.RawRecord{
		{CheckDate: "2026/09/01", ItemName: "LDL-Cholesterol", OrderCode: "09044C", Unit: "mg/dL", Result: "80", Institution: "Synthetic"},
		{CheckDate: "2025/08/01", ItemName: "LDL-Cholesterol", OrderCode: "09044C", Unit: "mg/dL", Result: "90", Institution: "Synthetic"},
		{CheckDate: "2025/08/01", ItemName: "Triglyceride", OrderCode: "09004C", Unit: "mg/dL", Result: ">500", Institution: "Synthetic"},
	}}
	response := app.currentResponse(s)
	if len(response.Warnings) != 1 || !strings.Contains(response.Warnings[0], "Friedewald") {
		t.Fatal("bounded older TG/LDL pair omitted")
	}
	if response.Records[2].NumericValue != nil {
		t.Fatal("warning invented a point value")
	}
}
