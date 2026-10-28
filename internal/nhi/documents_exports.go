// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package nhi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type officialExportSpec struct{ path, root string }

var officialExportSpecs = map[string]officialExportSpec{
	"html": {"/api/ihke3000/ihke3802s01", "sp_IHKE3802S01_data"},
	"xml":  {"/api/ihke3000/ihke3803s01", "sp_IHKE3803S02_data"},
	"json": {"/api/ihke3000/ihke3804s01", "sp_IHKE3804S02_data"},
	"pdf":  {"/api/ihke3000/ihke3805s01", "sp_IHKE3805S01_data"},
}

func OfficialExportCategory(category string) (string, bool) {
	format, prefixed := strings.CutPrefix(category, "official_export_")
	_, known := officialExportSpecs[format]
	return format, prefixed && known
}

// FetchOfficialExportOptions reads the provider's offered scopes, not a guessed
// all-history export parameter. It creates no upstream job. Each returned action
// retains the exact source selection in session memory until explicit POST.
func (s *Session) FetchOfficialExportOptions(ctx context.Context, format string) (SupplementReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	category := "official_export_" + format
	report := SupplementReport{Category: category, Entries: []SupplementEntry{}, Complete: false,
		Coverage: "以下是官方目前允許產製的原始資料範圍。明確按下產製才建立官方暫存工作；並不代表所有年度皆可提供。產製完成後到「官方已產製原始檔」下載。"}
	spec, ok := officialExportSpecs[format]
	if !ok {
		return report, failure("INVALID_INPUT", "不支援此官方原始檔格式。")
	}
	if err := s.supplementReady(ctx); err != nil {
		return report, err
	}
	body, _, err := s.request(ctx, http.MethodGet, spec.path+"/page_load", nil, nil, maxJSONBytes, true)
	if err != nil {
		return report, err
	}
	defer clear(body)
	object, err := documentObject(body)
	if err != nil {
		s.invalidateOnAuthError(err)
		return report, err
	}
	rawRows, found := object[strings.ToLower(spec.root)]
	if !found {
		return report, changedError()
	}
	rows, err := supplementRows(rawRows, "")
	if err != nil {
		return report, err
	}
	refs := make(map[string]documentReference)
	payloadBytes := 0
	appendAction := func(entry SupplementEntry, payload []byte) error {
		payloadBytes += len(payload)
		if payloadBytes > 1<<20 || len(report.Entries) >= MaxReportRecords {
			return failure("RESPONSE_TOO_LARGE", "官方可產製範圍超過本次處理上限。")
		}
		id, err := documentToken()
		if err != nil {
			return err
		}
		refs[id] = documentReference{category: category, parent: "options", kind: "prepare_export", format: format, payload: payload}
		entry.DocumentActions = []DocumentAction{{ID: id, Name: "請官方產製 " + strings.ToUpper(format), Kind: "prepare_export"}}
		report.Entries = append(report.Entries, entry)
		return nil
	}
	if format == "xml" || format == "json" {
		fields := []SupplementField{}
		for _, row := range rows {
			label, err := rowText(row, "code_CNAME")
			if err != nil || label == "" {
				return report, changedError()
			}
			fields = append(fields, SupplementField{Name: "官方可產製資料", Value: label})
		}
		if len(rows) != 0 {
			dat, found := object["dat"]
			if !found || len(dat) == 0 || bytes.Equal(bytes.TrimSpace(dat), []byte("null")) {
				return report, changedError()
			}
			payload, err := json.Marshal(map[string]json.RawMessage{"dat": dat})
			if err != nil {
				return report, changedError()
			}
			entry := SupplementEntry{Title: "官方 " + strings.ToUpper(format) + " 原始資料", Kind: "document_request", Source: "官方產製範圍", Fields: fields}
			if err := appendAction(entry, payload); err != nil {
				return report, err
			}
		}
	} else if format == "html" {
		for _, row := range rows {
			label, err := rowText(row, "code_CNAME")
			if err != nil || label == "" {
				return report, changedError()
			}
			fields, err := appendKnownFields(row, []fieldSpec{{"all_sdate", "官方全部資料起日", ""}, {"all_edate", "官方全部資料迄日", ""}})
			if err != nil {
				return report, err
			}
			var all bool
			if json.Unmarshal(row["has_all"], &all) != nil {
				return report, changedError()
			}
			selections := []json.RawMessage{}
			if all {
				if !validExportSelection(row["val_all"]) {
					return report, changedError()
				}
				selections = append(selections, row["val_all"])
			} else if raw, present := row["details_items"]; present {
				groups, err := supplementRows(raw, "")
				if err != nil {
					return report, err
				}
				for _, group := range groups {
					items, err := supplementRows(group["details"], "")
					if err != nil {
						return report, err
					}
					for _, item := range items {
						label, err := rowText(item, "text")
						if err != nil || !validExportSelection(item["value"]) || len(selections) >= MaxReportRecords {
							return report, changedError()
						}
						selections = append(selections, item["value"])
						fields = append(fields, SupplementField{Name: "官方逐項可產製範圍", Value: label})
					}
				}
			}
			entry := SupplementEntry{Title: "官方 HTML／" + label, Kind: "document_request", Source: "官方產製範圍", Fields: fields}
			if len(selections) == 0 {
				report.Entries = append(report.Entries, entry)
				continue
			}
			payload, err := json.Marshal(map[string][]json.RawMessage{"row_id": selections})
			if err != nil {
				return report, changedError()
			}
			if err := appendAction(entry, payload); err != nil {
				return report, err
			}
		}
	} else {
		for _, row := range rows {
			group, err := rowText(row, "code_ENAME")
			if err != nil {
				return report, err
			}
			items, err := supplementRows(row["sp_ihke3805s02_data"], "")
			if err != nil {
				return report, err
			}
			for _, item := range items {
				fields, err := appendKnownFields(item, []fieldSpec{{"rpt_TYPE_NAME", "官方報告類型", ""}, {"text", "官方產製範圍（原文）", ""}})
				if err != nil || !validExportSelection(item["row_id"]) || len(fields) == 0 {
					return report, changedError()
				}
				payload, err := json.Marshal(map[string]json.RawMessage{"row_ID": item["row_id"]})
				if err != nil {
					return report, changedError()
				}
				entry := SupplementEntry{Title: "官方 PDF／" + group, Kind: "document_request", Source: "官方產製範圍", Fields: fields}
				if err := appendAction(entry, payload); err != nil {
					return report, err
				}
			}
		}
	}
	if err := s.replaceDocumentReferences(category, "options", refs); err != nil {
		return report, err
	}
	report.FetchedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return report, nil
}

func validExportSelection(raw json.RawMessage) bool {
	var value wireText
	return len(raw) != 0 && len(raw) <= 8192 && json.Unmarshal(raw, &value) == nil && usableDocumentID(string(value))
}
