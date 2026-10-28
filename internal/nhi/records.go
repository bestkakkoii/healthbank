// Copyright (c) 2026 bestkakkoii llc.
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

// QueryMode identifies the requested coverage, not an official retention claim.
type QueryMode string

const (
	QueryModeAllHistory QueryMode = "all_history"
	QueryModeCustom     QueryMode = "custom"
	// AllHistoryStartDate is the application's conservative query floor. It is
	// not an NHI retention date or evidence that records exist from this date.
	AllHistoryStartDate = "1912-01-01"
)

// Query dates are Gregorian YYYY-MM-DD. Leave both empty to request all history
// from the application floor through today's Taiwan date. Source and assay-name
// filters are deliberately omitted so all source channels remain included.
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

// Report retains the exact search bounds. ReportedCount is an independent
// official total only for those bounds; -1 means that total was not supplied.
type Report struct {
	Records       []Record  `json:"records"`
	Mode          QueryMode `json:"mode"`
	StartDate     string    `json:"startDate"`
	EndDate       string    `json:"endDate"`
	ReportedCount int       `json:"reportedCount"`
}

// Fetch follows the SPA's initial metadata request with an explicit full search.
// page_load is only the first 50 records; it must never be returned as though it
// were the complete report or allowed to narrow the requested search range.
// The search endpoint itself is not server-paginated.
func (s *Session) Fetch(ctx context.Context, query Query) (Report, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Report{}, closedError()
	}
	if s.token == "" {
		return Report{}, sessionExpired()
	}
	now := time.Now()
	query, mode, err := resolveQuery(query, now)
	if err != nil {
		return Report{}, err
	}
	if now.Sub(s.tokenObtainedAt) >= 8*time.Minute {
		if err := s.refresh(ctx); err != nil {
			return Report{}, err
		}
	}
	params := url.Values{
		"s_type": {""}, "s_sort": {"A1"}, "s_date": {""}, "e_date": {""},
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
		return Report{}, s.annotateResponseError(err)
	}
	// The initial page describes the official default interval. Its total is
	// valid for this search only when the exact bounds match. If the search
	// contains that interval, the count is still a useful minimum, not a total.
	report := Report{Mode: mode, StartDate: query.StartDate, EndDate: query.EndDate, ReportedCount: -1}
	if query.StartDate == meta.start && query.EndDate == meta.end {
		report.ReportedCount = meta.count
	}
	minimumCount := -1
	if query.StartDate <= meta.start && query.EndDate >= meta.end {
		minimumCount = meta.count
	}
	if minimumCount > MaxReportRecords {
		return Report{}, s.annotateResponseError(failure("RESPONSE_TOO_LARGE", "檢驗項目過多，請縮短日期區間後重試。"))
	}
	params.Del("nowPage")
	params.Del("pageDataCount")
	params.Set("s_date", query.StartDate)
	params.Set("e_date", query.EndDate)
	body, _, err = s.request(ctx, http.MethodGet, searchPath, params, nil, maxJSONBytes, true)
	if err != nil {
		return Report{}, err
	}
	defer clear(body)
	records, err := parseRecords(body)
	if err != nil {
		s.invalidateOnAuthError(err)
		return Report{}, s.annotateResponseError(err)
	}
	if minimumCount >= 0 && len(records) < minimumCount {
		message := "健保署回傳的筆數不足，尚未匯入；請重新取得資料或縮短日期區間。"
		if report.ReportedCount < 0 {
			message = "擴大查詢回傳的筆數少於官方預設期間已知的筆數下限，尚未匯入；請重新取得資料或指定日期區間。"
		}
		return report, s.annotateResponseError(&Error{
			Code: "INCOMPLETE_RESPONSE", Message: message,
			ExpectedCount: minimumCount, ReceivedCount: len(records),
		})
	}
	report.Records = records
	return report, nil
}

// resolveQuery takes one snapshot of today's date before any network work. The current
// Taiwan civil date is UTC+08 and must not depend on the server's local zone.
func resolveQuery(query Query, now time.Time) (Query, QueryMode, error) {
	mode := QueryModeCustom
	if query.StartDate == "" && query.EndDate == "" {
		mode = QueryModeAllHistory
		query.StartDate = AllHistoryStartDate
		query.EndDate = now.In(time.FixedZone("Asia/Taipei", 8*60*60)).Format("2006-01-02")
	}
	if err := validateQuery(query); err != nil {
		return Query{}, "", err
	}
	return query, mode, nil
}

func validateQuery(query Query) error {
	start, errStart := time.Parse("2006-01-02", query.StartDate)
	end, errEnd := time.Parse("2006-01-02", query.EndDate)
	if errStart != nil || errEnd != nil || start.Year() < 1912 || start.Year() > 2200 || end.Year() < 1912 || end.Year() > 2200 || end.Before(start) {
		return failure("INVALID_INPUT", "查詢日期請使用1912至2200年的完整西元日期，且結束日期不可早於開始日期。")
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
	if len(upstream) > MaxReportRecords {
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
	return &Error{Code: "UPSTREAM_REJECTED", Message: "健保署未能提供此項資料，請重新登入後重試。", UpstreamCode: safeCode(code)}
}

func changedError() *Error {
	return failure("UPSTREAM_CHANGED", "健保署回傳的資料格式與目前版本不符，未使用不完整回應。請稍後重試或更新 Healthbank。")
}

func (s *Session) invalidateOnAuthError(err error) {
	var upstream *Error
	if errors.As(err, &upstream) && upstream.Code == "SESSION_EXPIRED" {
		s.clearAuth()
	}
}
