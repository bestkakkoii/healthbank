// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/bestkakkoii/healthbank/internal/lab"
)

func (a *App) demo(w http.ResponseWriter, r *http.Request, s *session) {
	a.clearSession(s)
	s.mode = "demo"
	s.complete = true
	dates := []string{"2023-03-14", "2023-10-05", "2024-04-09", "2024-11-19", "2025-05-13", "2025-11-11", "2026-04-14", "2026-10-13"}
	type item struct {
		code, name, unit, reference string
		values                      []string
	}
	items := []item{
		{"09015C", "eGFR", "mL/min/1.73m2", "[無參考值][]", []string{"77", "75", "72", "68", "65", "62", "59", "56"}},
		{"09016C", "ACR", "mg/g", "[無參考值][]", []string{"18", "24", "29", "35", "44", "58", "65", "78"}},
		{"09004C", "Triglyceride", "mg/dL", "[0][150]", []string{"240", "219", "202", "189", "174", "160", "152", "142"}},
		{"09044C", "LDL-Cholesterol", "mg/dL", "[0][130]", []string{"138", "126", "112", "105", "98", "92", "89", "85"}},
		{"09006C", "HbA1c", "%", "[4.0][6.0]", []string{"6.8", "6.7", "6.5", "6.4", "6.3", "6.2", "6.1", "5.9"}},
		{"09005C", "Glucose-AC(飯前)", "mg/dL", "[70][100]", []string{"128", "124", "118", "115", "110", "106", "102", "98"}},
		{"09022C", "Potassium(K)", "mmol/L", "[3.5][5.1]", []string{"4.0", "4.2", "4.1", "4.4", "4.3", "4.2", "4.5", "4.4"}},
		{"09026C", "ALT", "U/L", "[7][52]", []string{"39", "35", "31", "29", "28", "26", "24", "22"}},
		{"08011C", "Hb", "g/dL", "[12][16]", []string{"14.3", "14.1", "14.0", "13.9", "13.7", "13.6", "13.5", "13.4"}},
		{"06012C", "Protein", "無單位", "[無參考值][]", []string{"Negative", "Negative", "Negative", "Trace", "1+", "1+", "1+", "1+"}},
	}
	for _, it := range items {
		for i, date := range dates {
			s.raw = append(s.raw, lab.RawRecord{CheckDate: date, VisitDate: date, OrderCode: it.code, OrderName: "示範檢驗", ItemName: it.name, Result: it.values[i], Unit: it.unit, Reference: it.reference, Institution: "示範醫療機構", Source: "人工合成示範"})
		}
	}
	// Deliberately demonstrate same-day repeats and non-exact values.
	s.raw = append(s.raw, lab.RawRecord{CheckDate: dates[7], VisitDate: dates[7], OrderCode: "09022C", OrderName: "示範複檢", ItemName: "Potassium(K)", Result: "4.3", Unit: "mmol/L", Reference: "[3.5][5.1]", Institution: "示範醫療機構", Source: "人工合成示範"})
	s.raw = append(s.raw, lab.RawRecord{CheckDate: dates[7], VisitDate: dates[7], OrderCode: "06012C", OrderName: "示範尿液檢驗", ItemName: "A/C ratio", Result: "大於等於300", Unit: "mg/gCr", Reference: "[無參考值][]", Institution: "示範醫療機構", Source: "人工合成示範"})
	s.fetched = time.Now().UTC().Format(time.RFC3339)
	s.warnings = []string{fmt.Sprintf("示範模式：%d筆資料皆為人工合成，不是任何人的真實病歷；設定只保留在本次連線。", len(s.raw))}
	if err := a.rotate(w, r, s); err != nil {
		a.fail(w, r, 500, "session_rotate", "無法建立示範連線。")
		return
	}
	a.json(w, 200, a.currentResponse(s))
}
