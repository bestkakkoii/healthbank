// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"strings"
)

func (s *Session) providerCoverageFetch(ctx context.Context, report *SupplementReport, category string, query Query, refs map[string]providerReference) (bool, error) {
	switch category {
	case "covid":
		return true, s.providerCOVIDFetch(ctx, report)
	case "family_doctor":
		return true, s.providerFamilyDoctorFetch(ctx, report)
	case "osteoporosis":
		return true, s.providerBoneFetch(ctx, report, refs)
	}
	if category == "care_plans" {
		return true, s.providerCarePlanFetch(ctx, report)
	}
	if category == "family_doctor_consent_snapshot" {
		request := providerSpecs[category].requests[0]
		object, err := s.providerRead(ctx, request)
		if err != nil {
			return true, err
		}
		for _, key := range []string{"isshowdr", "showcontent", "showmain"} {
			value := bytes.TrimSpace(object[key])
			if bytes.Equal(value, []byte("true")) || bytes.Equal(value, []byte("false")) {
				object[key] = json.RawMessage(strconv.Quote(string(value)))
			}
		}
		return true, s.providerAppend(report, object, request, category, nil, providerReference{})
	}
	if category != "diet_source_totals" && category != "exercise_source_totals" {
		return false, nil
	}
	request := providerSpecs[category].requests[0]
	start, end := strings.ReplaceAll(query.StartDate, "-", ""), strings.ReplaceAll(query.EndDate, "-", "")
	if category == "diet_source_totals" {
		request.payload = map[string]string{"SDATE": start, "EDATE": end}
	} else {
		request.payload = map[string]string{"c_sdate": start, "c_edate": end}
	}
	object, err := s.providerRead(ctx, request)
	if err != nil {
		return true, err
	}
	if !bytes.Equal(bytes.TrimSpace(object["resultstatus"]), []byte("true")) {
		return true, failure("REQUEST_FAILED", "官方未確認熱量彙總讀取成功；先前取得資料不會被空圖取代。")
	}
	var dates, values []json.RawMessage
	if decodeJSON(object["datelist"], &dates) != nil || decodeJSON(object["callist"], &values) != nil || dates == nil || values == nil || len(dates) != len(values) || len(dates) > MaxReportRecords {
		return true, changedError()
	}
	if err := sourceSectionDiagnostic(report, category, "daily_totals", nil, true, false, len(dates)); err != nil {
		return true, err
	}
	for index, raw := range dates {
		var date, value wireText
		if json.Unmarshal(raw, &date) != nil || json.Unmarshal(values[index], &value) != nil {
			return true, changedError()
		}
		dateLabel, valueLabel := string(date), string(value)
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			dateLabel = "null"
		}
		if bytes.Equal(bytes.TrimSpace(values[index]), []byte("null")) {
			valueLabel = "null"
		}
		entry := SupplementEntry{Key: medicalKey(category, query.StartDate+"\x00"+query.EndDate, "daily_totals", index), Kind: "source_summary", Date: string(date), Origin: "official_summary", ContractID: sourceContractID(category, "daily_totals"), Title: providerSpecs[category].info.Label, Source: request.sections[0].source,
			Fields: []SupplementField{{Name: "來源日期標籤（原文；null 為未提供）", Code: "datelist", Role: "date", Value: dateLabel}, {Name: "來源熱量估值（原文；null 為未提供）", Code: "callist", Value: valueLabel, Unit: "kcal", UnitBasis: "public_contract"}}}
		if entry.Date != "" {
			entry.DateRole = "recorded"
		}
		report.Entries = append(report.Entries, entry)
	}
	return true, nil
}
