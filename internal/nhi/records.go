// Copyright (c) 2026 bestkakkoii llc. All rights reserved.
// SPDX-License-Identifier: MIT

package nhi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Query dates are Gregorian YYYY-MM-DD. Leave both empty to use the official
// site's default range. Source and assay-name filters are deliberately omitted
// so that all reported laboratory items within the range can be normalized.
type Query struct {
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
}

// Record preserves original text, including qualitative results, inequalities,
// reference bounds and missing-value markers. It performs no medical grading.
type Record struct {
	CheckDate    string `json:"checkDate"`
	OrderCode    string `json:"orderCode"`
	OrderName    string `json:"orderName"`
	ItemName     string `json:"itemName"`
	Result       string `json:"result"`
	Unit         string `json:"unit"`
	Reference    string `json:"reference"`
	Institution  string `json:"institution"`
	VisitDate    string `json:"visitDate"`
	Source       string `json:"source"`
	UploadDate   string `json:"uploadDate,omitempty"`
	OfficialFlag string `json:"officialFlag,omitempty"`
}

type Report struct {
	Records       []Record `json:"records"`
	StartDate     string   `json:"startDate"`
	EndDate       string   `json:"endDate"`
	ReportedCount int      `json:"reportedCount"`
}

// Fetch follows the SPA's initial metadata request with its full search call.
// page_load is only the first 50 records; it must never be returned as though it
// were the complete report. The search endpoint itself is not server-paginated.
func (s *Session) Fetch(ctx context.Context, query Query) (Report, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Report{}, closedError()
	}
	if s.token == "" {
		return Report{}, sessionExpired()
	}
	if err := validateQuery(query); err != nil {
		return Report{}, err
	}
	if time.Since(s.tokenObtainedAt) >= 8*time.Minute {
		if err := s.refresh(ctx); err != nil {
			return Report{}, err
		}
	}
	params := url.Values{
		"s_type": {""}, "s_sort": {"A1"}, "s_date": {query.StartDate}, "e_date": {query.EndDate},
		"nowPage": {"1"}, "pageDataCount": {"50"},
	}
	body, _, err := s.request(ctx, http.MethodGet, pageLoadPath, params, nil, maxJSONBytes, true)
	if err != nil {
		return Report{}, err
	}
	meta, err := parseMetadata(body)
	clear(body)
	if err != nil {
		s.invalidateOnAuthError(err)
		return Report{}, err
	}
	if meta.count > maxRecords {
		return Report{}, failure("RESPONSE_TOO_LARGE", "檢驗項目過多，請縮短日期區間後重試。")
	}
	params.Del("nowPage")
	params.Del("pageDataCount")
	params.Set("s_date", meta.start)
	params.Set("e_date", meta.end)
	body, _, err = s.request(ctx, http.MethodGet, searchPath, params, nil, maxJSONBytes, true)
	if err != nil {
		return Report{}, err
	}
	defer clear(body)
	records, err := parseRecords(body)
	if err != nil {
		s.invalidateOnAuthError(err)
		return Report{}, err
	}
	if meta.count >= 0 && len(records) < meta.count {
		return Report{StartDate: meta.start, EndDate: meta.end, ReportedCount: meta.count}, &Error{
			Code: "INCOMPLETE_RESPONSE", Message: "健保署回傳的筆數不足，尚未匯入；請重新取得資料或縮短日期區間。",
			ExpectedCount: meta.count, ReceivedCount: len(records),
		}
	}
	return Report{Records: records, StartDate: meta.start, EndDate: meta.end, ReportedCount: meta.count}, nil
}

func validateQuery(query Query) error {
	if query.StartDate == "" && query.EndDate == "" {
		return nil
	}
	start, errStart := time.Parse("2006-01-02", query.StartDate)
	end, errEnd := time.Parse("2006-01-02", query.EndDate)
	if errStart != nil || errEnd != nil || end.Before(start) {
		return failure("INVALID_INPUT", "查詢日期請使用完整西元日期，且結束日期不可早於開始日期。")
	}
	return nil
}

type metadata struct {
	start string
	end   string
	count int
}

func parseMetadata(body []byte) (metadata, error) {
	var fields map[string]json.RawMessage
	if decodeJSON(body, &fields) != nil || fields == nil {
		return metadata{}, changedError()
	}
	if err := detectAPIError(fields); err != nil {
		return metadata{}, err
	}
	var start, end string
	if json.Unmarshal(fields["sdate"], &start) != nil || json.Unmarshal(fields["edate"], &end) != nil {
		return metadata{}, changedError()
	}
	if validateQuery(Query{StartDate: start, EndDate: end}) != nil || start == "" || end == "" {
		return metadata{}, changedError()
	}
	// This key distinguishes the expected report wrapper from unrelated JSON.
	firstPage, present := fields["sP_IHKE3409S02_Data"]
	if !present || !(bytes.HasPrefix(bytes.TrimSpace(firstPage), []byte("[")) || bytes.Equal(bytes.TrimSpace(firstPage), []byte("null"))) {
		return metadata{}, changedError()
	}
	count := -1
	if value, ok := fields["counts"]; ok && !bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		var text wireText
		if json.Unmarshal(value, &text) != nil {
			return metadata{}, changedError()
		}
		var err error
		count, err = strconv.Atoi(string(text))
		if err != nil || count < 0 {
			return metadata{}, changedError()
		}
	}
	return metadata{start: start, end: end, count: count}, nil
}

// wireText accepts JSON strings, numbers or null without rounding a result.
type wireText string

func (v *wireText) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if bytes.Equal(raw, []byte("null")) {
		*v = ""
		return nil
	}
	if len(raw) > 0 && raw[0] == '"' {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return err
		}
		*v = wireText(value)
		return nil
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err != nil {
		return errors.New("expected a text or numeric laboratory field")
	}
	*v = wireText(number.String())
	return nil
}

type wireRecord struct {
	CheckDate    wireText `json:"reaL_INSPECT_DATE"`
	OrderCode    wireText `json:"ordeR_CODE"`
	OrderName    wireText `json:"ordeR_NAME"`
	ItemName     wireText `json:"assaY_ITEM_NAME"`
	Result       wireText `json:"assaY_VALUE"`
	Unit         wireText `json:"uniT_DATA"`
	Reference    wireText `json:"consulT_VALUE"`
	Institution  wireText `json:"hosP_ABBR"`
	VisitDate    wireText `json:"funC_DATE"`
	Source       wireText `json:"orI_TYPE"`
	UploadDate   wireText `json:"assaY_UPLOAD_DATE"`
	OfficialFlag wireText `json:"assaY_MARK"`
}

func parseRecords(body []byte) ([]Record, error) {
	trimmed := bytes.TrimSpace(bytes.TrimPrefix(body, []byte{0xef, 0xbb, 0xbf}))
	if !bytes.HasPrefix(trimmed, []byte("[")) {
		var fields map[string]json.RawMessage
		if decodeJSON(trimmed, &fields) == nil {
			if err := detectAPIError(fields); err != nil {
				return nil, err
			}
		}
		return nil, changedError()
	}
	var upstream []wireRecord
	if decodeJSON(trimmed, &upstream) != nil {
		return nil, changedError()
	}
	if len(upstream) > maxRecords {
		return nil, failure("RESPONSE_TOO_LARGE", "檢驗項目過多，請縮短日期區間後重試。")
	}
	records := make([]Record, 0, len(upstream))
	for _, row := range upstream {
		if strings.TrimSpace(string(row.ItemName)) == "" {
			return nil, changedError()
		}
		records = append(records, Record{
			CheckDate: string(row.CheckDate), OrderCode: string(row.OrderCode), OrderName: string(row.OrderName),
			ItemName: string(row.ItemName), Result: string(row.Result), Unit: string(row.Unit), Reference: string(row.Reference),
			Institution: string(row.Institution), VisitDate: string(row.VisitDate), Source: string(row.Source),
			UploadDate: string(row.UploadDate), OfficialFlag: string(row.OfficialFlag),
		})
	}
	return records, nil
}

func detectAPIError(fields map[string]json.RawMessage) error {
	value, present := fields["error_code"]
	if !present {
		return nil
	}
	var code string
	if json.Unmarshal(value, &code) != nil {
		return changedError()
	}
	if code == "0000" || code == "" {
		return nil
	}
	if code == "9999" {
		return sessionExpired()
	}
	return &Error{Code: "UPSTREAM_REJECTED", Message: "健保署未能提供檢驗資料，請重新登入後重試。", UpstreamCode: safeCode(code)}
}

func changedError() *Error {
	return failure("UPSTREAM_CHANGED", "健保署檢驗資料格式與目前版本不符，請先使用官方另存網頁匯入。")
}

func (s *Session) invalidateOnAuthError(err error) {
	var upstream *Error
	if errors.As(err, &upstream) && upstream.Code == "SESSION_EXPIRED" {
		s.clearAuth()
	}
}
