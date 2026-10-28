// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func annualProviderRequests(ref providerReference) []providerRequest {
	fields := append(append([]fieldSpec{}, medicalHeaderFields...), fieldSpec{"hosP_DATA_TYPE_NAME", "來源就醫類型", ""})
	outpatient := providerSectionSpec("sP_IHKE3302S01_Data", "來源年度門診紀錄", "健康存摺／年度門診來源", "func_DATE", fields)
	inpatient := providerSectionSpec("sP_IHKE3302S02_Data", "來源年度住院紀錄", "健康存摺／年度住院來源", "in_DATE", fields)
	inpatient.end = "out_DATE"
	return []providerRequest{
		{path: "/api/ihke3000/ihke3302s02/SP_IHKE3302S02", method: http.MethodGet, params: url.Values{"c_year": {ref.rowID}}, sections: []providerSection{outpatient}},
		{path: "/api/ihke3000/ihke3302s03/SP_IHKE3302S03", method: http.MethodGet, params: url.Values{"c_year": {ref.rowID}}, sections: []providerSection{inpatient}},
	}
}

func (s *Session) fetchAnnualProviderDetail(ctx context.Context, reference string, ref providerReference, report SupplementReport) (SupplementReport, error) {
	if ref.kind != "annual_year" || !usableDocumentID(ref.rowID) {
		return report, failure("INVALID_INPUT", "年度來源識別已失效，請重新取得年度總覽。")
	}
	report.Coverage = "依來源總覽實際回傳的年度，分別取得該年度門診與住院清單及逐筆明細入口；不以年度總數取代原始列，也不假設與其他清單完全相同。保留來源回應的期間原文。"
	children := make(map[string]medicalReference)
	for _, request := range annualProviderRequests(ref) {
		object, err := s.providerRead(ctx, request)
		if err != nil {
			return report, err
		}
		section := request.sections[0]
		rows, present, err := providerSectionRows(object, section)
		if err != nil || !present {
			return report, changedError()
		}
		if err := sourceSectionDiagnostic(&report, "annual_overview", section.root, object[strings.ToLower(section.root)], true, false, len(rows)); err != nil {
			return report, err
		}
		period, err := appendKnownFields(object, []fieldSpec{{"c_SDATE", "來源年度清單起日（原文）", ""}, {"c_EDATE", "來源年度清單迄日（原文）", ""}})
		if err != nil {
			return report, err
		}
		if len(period) > 0 {
			period = append([]SupplementField{{Name: "來源總覽年度", Code: "year", Value: ref.rowID}}, period...)
			report.Entries = append(report.Entries, SupplementEntry{Key: medicalKey("annual_overview", ref.rowID, section.root+"_period", 0), Kind: "source_index", Origin: "official_summary", ContractID: sourceContractID("annual_overview", section.root+"_period"), Title: section.title + "提供期間", Source: section.source, Fields: period})
		}
		for index, row := range rows {
			entry, err := providerEntry(row, section, providerReference{})
			if err != nil {
				return report, err
			}
			category, sourceTitle := "hospital", ""
			if section.end == "" {
				typeName, err := rowText(row, "hosP_DATA_TYPE_NAME")
				if err != nil {
					return report, err
				}
				switch typeName {
				case "西醫":
					category = "western"
				case "中醫":
					category = "chinese"
				case "牙醫":
					category = "dental"
				default:
					// The source routes all other labels through its default
					// outpatient detail view. This is a route choice, not
					// evidence that an unknown/empty label means western care.
					category, sourceTitle = "western", "來源年度門診"
				}
			}
			entry.Key = medicalKey("annual_overview", ref.rowID, section.root, index)
			entry.Kind = "encounter"
			medicalEntryMetadata(&entry, category, section.root, false)
			entry.ContractID = sourceContractID("annual_overview", section.root)
			id, e1 := rowText(row, "roW_ID")
			sourceType, e2 := rowText(row, "orI_TYPE")
			if e1 != nil || e2 != nil || !usableDocumentID(id) || !usableDocumentID(sourceType) {
				return report, changedError()
			}
			child := medicalReference{category: category, rowID: id, sourceType: sourceType, date: entry.Date, endDate: entry.EndDate, institution: entry.Institution, sourceTitle: sourceTitle}
			if err := addMedicalChild(&entry, child, children); err != nil {
				return report, err
			}
			report.Entries = append(report.Entries, entry)
			if len(report.Entries) > MaxReportRecords {
				return report, failure("RESPONSE_TOO_LARGE", "來源年度就醫清單過多，未回傳不完整資料。")
			}
		}
	}
	if err := validateSupplementReport(report); err != nil {
		return report, err
	}
	if err := s.commitMedicalDetailReferences("annual_overview", reference, medicalReference{kind: "annual_year", rowID: ref.rowID}, children, nil); err != nil {
		return report, err
	}
	report.FetchedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return report, nil
}
