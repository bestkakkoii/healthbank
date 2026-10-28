// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/url"
	"time"
)

func providerMenstrualMonthRequest(ref providerReference) providerRequest {
	section := providerSectionSpec("sP_IHKE3208S01", "來源生理期月曆紀錄", "健康存摺／生理期月曆獨立回應（非預測）", "sdate", []fieldSpec{{"sdate", "來源期間起日（保留原始 YYYYMMDD）", ""}, {"edate", "來源期間迄日（保留原始 YYYYMMDD）", ""}})
	section.end, section.detail = "edate", true
	return providerRequest{path: "/api/ihke3000/ihke3208s01/SP_IHKE3208S01", method: http.MethodGet, params: url.Values{"ym": {ref.rowID}, "ymNext": {ref.sourceType}}, sections: []providerSection{section}}
}

// Month handles describe bounded query selectors, not a claim that a menstrual
// period exists in that month. Upstream reads occur only when details are fetched.
func appendProviderMenstrualMonths(report *SupplementReport, query Query, refs map[string]providerReference) error {
	start, err := time.Parse("2006-01-02", query.StartDate)
	if err != nil {
		return changedError()
	}
	end, err := time.Parse("2006-01-02", query.EndDate)
	if err != nil {
		return changedError()
	}
	month := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC)
	last := time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, time.UTC)
	count := (last.Year()-month.Year())*12 + int(last.Month()-month.Month()) + 1
	if count < 1 || count > (2200-1912+1)*12 || len(refs)+count > MaxReportRecords || len(report.Entries)+count > MaxReportRecords {
		return failure("RESPONSE_TOO_LARGE", "生理期月份索引與既有紀錄超過取得上限；未回傳截斷月份。請縮小取得期間。")
	}
	for index := 0; index < count; index++ {
		next := month.AddDate(0, 1, 0)
		key := make([]byte, 24)
		if _, err := rand.Read(key); err != nil {
			return failure("INITIALIZATION_FAILED", "無法建立月份明細識別。")
		}
		id := hex.EncodeToString(key)
		entry := SupplementEntry{Key: medicalKey("menstrual", month.Format("200601"), "month_index", 0), Kind: "source_index", Origin: "self_entered", ContractID: sourceContractID("menstrual", "month_index"), Title: month.Format("2006-01") + " 來源月曆查詢", Source: "健康存摺／生理期月份查詢入口（非已有經期紀錄）", Fields: []SupplementField{{Name: "來源查詢月份", Value: month.Format("200601"), Code: "ym"}, {Name: "來源下一月份", Value: next.Format("200601"), Code: "ym_next"}, {Name: "取得狀態", Value: "取得此明細後才包含該月份來源回應；未推定生理期、排卵或受孕。"}}, DetailID: id}
		refs[id] = providerReference{rowID: month.Format("200601"), sourceType: next.Format("200601"), kind: "menstrual_month", title: entry.Title, originKey: entry.Key, query: query}
		report.Entries = append(report.Entries, entry)
		month = next
	}
	return nil
}

func (s *Session) fetchProviderMenstrualMonth(ctx context.Context, reference string, ref providerReference, report SupplementReport) (SupplementReport, error) {
	request := providerMenstrualMonthRequest(ref)
	object, err := s.providerRead(ctx, request)
	if err != nil {
		return report, err
	}
	report.StartDate, report.EndDate = ref.query.StartDate, ref.query.EndDate
	report.Coverage = "此回應只查詢來源月份 " + ref.rowID + "（ymNext=" + ref.sourceType + "）。保留全部回傳起迄原文，未依所選期間裁切；與來源期間清單、單日明細分開保留。不推定經期、排卵或受孕，無獨立總數。"
	refs := providerStageChildRefresh(s.providerReferences["menstrual"], reference)
	if err := s.providerAppend(&report, object, request, "menstrual", refs, ref); err != nil {
		return report, err
	}
	for _, entry := range report.Entries {
		if entry.DetailID != "" {
			child := refs[entry.DetailID]
			child.parentReference = reference
			refs[entry.DetailID] = child
		}
	}
	if err := validateSupplementReport(report); err != nil {
		return report, err
	}
	if err := s.reserveReferenceGroups(map[string]int64{"provider/menstrual": referenceMapSize(refs, providerReferenceSize)}); err != nil {
		return report, err
	}
	s.providerReferences["menstrual"] = refs
	report.FetchedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return report, nil
}
