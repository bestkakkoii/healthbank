// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// These are existing records on the public SPA's reminder, questionnaire,
// assessment and rewards pages. Reading them does not submit a questionnaire,
// enroll in a program, recompute a risk result or change notification settings.
func extendPersonalProviderSpecs(items map[string]providerSpec) {
	for _, item := range []struct {
		id, label, group, coverage string
		detail                     bool
	}{
		{"prenatal", "產前檢查來源紀錄", "預防保健", "讀取官方產檢紀錄實際回傳清單；來源標題可能包含日期，未將標題拆成推測的檢查日期。不包含提醒設定，也不保證完整歷史。", false},
		{"child_preventive", "兒童預防保健來源紀錄", "預防保健", "讀取官方兒童預防保健實際回傳清單；保留來源標題、院所及檢查項目。未提供任意日期查詢或獨立總數。", false},
		{"lifestyle_assessment", "生活型態既有評估與作答", "自行記錄來源", "官方提供最近兩筆混合來源評估及其可查看的自行作答；分別保留院所、自行填寫、原始分數與答案。作答索引不等於評估索引，未自行加總或重新判讀分數。", false},
		{"source_liver_risk", "來源肝癌評估回傳快照", "照護與追蹤", "僅讀取官方評估頁回傳的輸入、結果、無資料／顯示狀態及原檢驗資訊；回傳值可能包含來源預設值，不同欄位可能來自不同日期。保留來源原文，不重新計算、不確認其有效性或臨床適用性，也不代表 Healthbank 診斷。", false},
		{"source_kidney_risk", "來源已存腎病評估", "照護與追蹤", "僅讀取官方已存 eGFR、ACR、PCR 輸入與上次評估，以及可用的來源檢驗資訊；未重新計算或將來源分級視為新診斷。沒有歷史查詢參數。", false},
		{"source_cvd_risk", "來源已存心血管風險評估", "照護與追蹤", "僅讀取官方已存輸入、上次評估及血壓／總膽固醇來源資訊；輸入可能經使用者修改，未重新計算或轉成 Healthbank 治療建議。", false},
		{"campaign_measurements", "來源活動量測快照", "自行記錄來源", "保留官方活動頁既有量測、來源日期與旗標，以及該頁連結的生理量測摘要。活動快照不等於完整歷史；來源血糖欄位名稱與顯示名稱不同時保留原識別，未自行判讀。", false},
		{"child_schedule", "已存兒童資料與建議接種時程", "自行記錄來源", "依來源已存兒童資料逐筆讀取建議接種期間及既有通知。這是來源建議時程，不是實際接種紀錄；姓名／暱稱與出生日期僅用於區分所選兒童。", true},
		{"health_coin_clinical", "健康幣來源健康紀錄", "預防保健", "只在來源已有同意狀態下，讀取健康檢查、疫苗及生活型態項目與原始結果；保留來源可提供的期間。未加入活動、領取點數或替使用者同意資料提供。", true},
	} {
		items[item.id] = providerSpec{info: ProviderCategoryInfo{ID: item.id, Label: item.label, Group: item.group, RangeMode: "source", HasDetails: item.detail}, coverage: item.coverage, requests: personalProviderRequests(item.id)}
	}
}

func personalSection(root, title string, fields []fieldSpec, object bool) providerSection {
	return providerSection{root: root, title: title, source: "健康存摺／" + title, fields: fields, object: object}
}

var personalPreventiveFields = []fieldSpec{{"Title", "來源紀錄標題（含來源日期原文）", ""}, {"HOSP_ABBR", "醫事機構", ""}, {"CheckItem", "檢查項目（來源原文）", ""}}
var personalObservationFields = []fieldSpec{{"Title", "來源檢驗名稱", ""}, {"ASSAY_VALUE", "來源檢驗值（原文）", ""}, {"REAL_INSPECT_DATE", "來源檢查日期（原文）", ""}, {"HOSP_ABBR", "來源醫事機構", ""}, {"SRC_NAME", "資料來源（原文）", ""}}
var personalLiverInputFields = []fieldSpec{{"Gender", "來源計算所用性別代碼", ""}, {"AgeRange", "來源計算所用年齡分組代碼", ""}, {"LogingenderStr", "來源目前性別代碼", ""}, {"AgeRangeStr", "來源目前年齡分組代碼", ""}, {"SGPTALT", "來源計算所用 ALT（原文）", "U/L"}, {"HBeAg", "來源計算所用 HBeAg（原文）", ""}, {"NoData", "來源無資料狀態（原文）", ""}, {"AskUpdate", "來源提示資料更新狀態（原文）", ""}}
var personalLiverResultFields = []fieldSpec{{"GenderScore", "來源回傳性別分數", ""}, {"AgeScore", "來源回傳年齡分數", ""}, {"SGPTALTScore", "來源回傳 ALT 分數", ""}, {"HBeAgScore", "來源回傳 HBeAg 分數", ""}, {"TotalScore", "來源回傳總分", ""}, {"FutureCancerRate", "來源回傳風險欄位（原文，未確認有效）", ""}, {"ShowPredictionArea", "來源結果顯示狀態（原文）", ""}}
var personalKidneyInputFields = []fieldSpec{{"EGFR", "來源 eGFR（原文）", ""}, {"ACR", "來源 ACR（原文）", ""}, {"PCR", "來源 PCR（原文）", ""}, {"EGFR_EXAM_DATE", "來源 eGFR 檢查日期", ""}, {"EGFR_HOSP_ABBR", "來源 eGFR 醫事機構", ""}, {"EGFR_SRC_NAME", "來源 eGFR 資料來源", ""}, {"ACR_PCR_EXAM_DATE", "來源 ACR／PCR 檢查日期", ""}, {"ACR_PCR_HOSP_ABBR", "來源 ACR／PCR 醫事機構", ""}, {"ACR_SRC_NAME", "來源 ACR／PCR 資料來源", ""}}
var personalKidneyResultFields = []fieldSpec{{"ComputeResultStatus", "來源計算結果狀態（原文）", ""}, {"ComputeResultTitle", "來源上次評估標題（原文）", ""}, {"ComputeResultDesc", "來源上次評估說明（原文）", ""}, {"EGFR", "上次評估所用 eGFR（原文）", ""}, {"ACR", "上次評估所用 ACR（原文）", ""}, {"PCR", "上次評估所用 PCR（原文）", ""}}
var personalCVDInputFields = []fieldSpec{{"Gender", "來源性別代碼", ""}, {"AgeRange", "來源年齡分組代碼", ""}, {"SBP", "來源收縮壓（原文）", "mmHg"}, {"TC", "來源總膽固醇（原文）", "mg/dL"}, {"SMOKER", "來源吸菸狀態代碼（原文）", ""}, {"DIABETES", "來源糖尿病狀態代碼（原文）", ""}, {"SBP_EXAM_DATE", "來源血壓檢查日期", ""}, {"SBP_HOSP_ABBR", "來源血壓醫事機構", ""}, {"SBP_SRC_NAME", "來源血壓資料來源", ""}, {"TC_EXAM_DATE", "來源總膽固醇檢查日期", ""}, {"TC_HOSP_ABBR", "來源總膽固醇醫事機構", ""}, {"TC_SRC_NAME", "來源總膽固醇資料來源", ""}}
var personalCVDResultFields = []fieldSpec{{"ComputeResultTitle", "來源上次評估結果（原文）", ""}, {"SEX", "上次評估所用性別代碼", ""}, {"AGE_RANGE", "上次評估所用年齡分組代碼", ""}, {"SBP", "上次評估所用收縮壓（原文）", "mmHg"}, {"TC", "上次評估所用總膽固醇（原文）", "mg/dL"}, {"SMOKER", "上次評估所用吸菸狀態代碼", ""}, {"DIABETES", "上次評估所用糖尿病狀態代碼", ""}}
var personalChildFields = []fieldSpec{{"chilD_NAME", "兒童姓名／暱稱（來源已存）", ""}, {"chilD_BIRTH", "兒童出生日期（來源原文）", ""}}
var personalChildScheduleFields = []fieldSpec{{"chilD_NAME", "兒童姓名／暱稱（來源已存）", ""}, {"chilD_BIRTH", "兒童出生日期（來源原文）", ""}, {"suggesT_DATE_S", "來源建議接種起日（非已接種）", ""}, {"suggesT_DATE_E", "來源建議接種迄日（非已接種）", ""}, {"p1_CODE_CNAME", "來源疫苗種類", ""}, {"p1_CODE_ENAME", "來源建議接種年齡", ""}, {"vaC_CODE", "來源疫苗代碼", ""}, {"vaC_DATE", "來源時程欄位 vaC_DATE（非施打證明）", ""}, {"pusH_DATE", "來源已設通知日期（非已接種）", ""}}
var personalNotificationFields = []fieldSpec{{"notifyDate", "來源已設通知日期", ""}, {"notifyMessage", "來源已設通知內容（原文）", ""}}
var personalCoinFields = []fieldSpec{{"pointitemName", "來源項目名稱", ""}, {"checkDate", "來源檢查日期", ""}, {"funcDate", "來源就醫／採檢日期", ""}, {"inoculationDate", "來源接種日期", ""}, {"createDate", "來源填表日期", ""}, {"hospName", "來源醫事機構／施打地點", ""}, {"bloodPressResult", "來源血壓結果代碼", ""}, {"bloodPressResultDisp", "來源血壓結果原文", ""}, {"s09005CDiag", "來源血糖結果代碼", ""}, {"s09005CDiagDisp", "來源血糖結果原文", ""}, {"baDiag", "來源血脂結果代碼", ""}, {"baDiagDisp", "來源血脂結果原文", ""}, {"rfDiag", "來源腎功能結果代碼", ""}, {"rfDiagDisp", "來源腎功能結果原文", ""}, {"lfDiag", "來源肝功能結果代碼", ""}, {"lfDiagDisp", "來源肝功能結果原文", ""}, {"metaSyndrResult", "來源代謝症候群結果代碼", ""}, {"metaSyndrResultDisp", "來源代謝症候群結果原文", ""}, {"hbsag", "來源 B 型肝炎結果代碼", ""}, {"hbsagDisp", "來源 B 型肝炎結果原文", ""}, {"antiHCV", "來源 C 型肝炎結果代碼", ""}, {"antiHCVDisp", "來源 C 型肝炎結果原文", ""}, {"assayResult", "來源檢查結果代碼", ""}, {"assayResultDisp", "來源檢查結果原文", ""}, {"diagnosisCodeDisp", "來源判讀報告原文", ""}, {"cytopathicCodeDisp", "來源細胞病理判讀原文", ""}, {"vaccineName", "來源疫苗名稱", ""}, {"sourceNameDisp", "來源填表資料來源", ""}, {"scale1", "來源正向社會連結分數", ""}, {"scale2", "來源身體活動分數", ""}, {"scale3", "來源避免危害物質分數", ""}, {"scale4", "來源睡眠與壓力管理分數", ""}, {"scale5", "來源營養分數", ""}, {"scaleTotal", "來源生活型態總分", ""}, {"scaleTitle", "來源生活型態說明標題", ""}, {"scaleDesc", "來源生活型態說明原文", ""}, {"status", "來源狀態代碼（原文）", ""}}

func personalCampaignFields() []fieldSpec {
	fields := []fieldSpec{{"c_CREATE_DATE", "來源活動紀錄時間（原文）", ""}}
	for _, item := range []struct{ key, label string }{{"c_WAISTLINE", "腰圍"}, {"c_SBP", "收縮壓"}, {"c_DBP", "舒張壓"}, {"c_HbA1c", "活動頁血糖欄位 c_HbA1c"}, {"c_TG", "三酸甘油脂"}, {"c_HDL", "高密度脂蛋白"}} {
		fields = append(fields, fieldSpec{item.key, "來源" + item.label + "（原文）", ""}, fieldSpec{item.key + "_R", "來源" + item.label + "旗標（原文）", ""})
		if item.key != "c_SBP" && item.key != "c_DBP" {
			fields = append(fields, fieldSpec{item.key + "_D", "來源" + item.label + "日期（原文）", ""}, fieldSpec{item.key + "_S", "來源" + item.label + "資料通道（原文）", ""})
		}
	}
	return append(fields, fieldSpec{"c_BP_D", "來源血壓日期（原文）", ""}, fieldSpec{"c_BP_S", "來源血壓資料通道（原文）", ""})
}

// The declarations include every read dispatched by the custom handlers,
// including bound detail/definition paths, so the registry can pin the entire
// projection instead of only the first visible request.
func personalProviderRequests(category string) []providerRequest {
	post := func(path string, sections ...providerSection) providerRequest {
		return providerRequest{path: "/api/ihke3000/" + path, method: http.MethodPost, sections: sections}
	}
	get := func(path string, sections ...providerSection) providerRequest {
		return providerRequest{path: "/api/ihke3000/" + path, method: http.MethodGet, sections: sections}
	}
	switch category {
	case "prenatal", "child_preventive":
		path, title := "IHKE3104S06", "產前檢查來源紀錄"
		if category == "child_preventive" {
			path, title = "IHKE3104S07", "兒童預防保健來源紀錄"
		}
		section := personalSection("", title, personalPreventiveFields, false)
		section.titleField, section.institution = "Title", "HOSP_ABBR"
		request := post("ihke3104s01/"+path, section)
		request.responseArray = true
		return []providerRequest{request}
	case "source_liver_risk":
		fields := append(append([]fieldSpec{}, personalLiverInputFields...), personalLiverResultFields...)
		sections := []providerSection{personalSection("", "來源肝癌評估輸入、回傳結果與狀態", fields, true)}
		for _, key := range []string{"SGPTALTDetail", "HBeAgDetail"} {
			section := personalSection(key, "來源肝癌評估所載檢驗資訊", personalObservationFields, true)
			section.date, section.institution, section.titleField = "REAL_INSPECT_DATE", "HOSP_ABBR", "Title"
			sections = append(sections, section)
		}
		return []providerRequest{post("ihke3502s01/IHKE3502S01", sections...)}
	case "source_kidney_risk":
		return []providerRequest{post("ihke3503s01/IHKE3503S01", personalSection("InitPersonalData", "來源腎病評估目前輸入與檢驗資訊", personalKidneyInputFields, true), personalSection("LastComputeResultData", "來源腎病上次評估與所用輸入", personalKidneyResultFields, true))}
	case "source_cvd_risk":
		return []providerRequest{post("ihke3506s01/IHKE3506S01", personalSection("InitPersonalData", "來源心血管評估目前輸入與檢驗資訊", personalCVDInputFields, true), personalSection("LastComputeResultData", "來源心血管上次評估與所用輸入", personalCVDResultFields, true))}
	case "lifestyle_assessment":
		return []providerRequest{
			get("IHKE3713S01/SP_IHKE3713S01", personalSection("", "來源問卷群組", []fieldSpec{{"codE_CNAME", "來源問卷群組代碼", ""}}, true)),
			get("IHKE3713S01/SP_IHKE3713S08", personalSection("field_type", "來源評估領域名稱", []fieldSpec{{"code", "來源領域代碼", ""}, {"text", "來源領域名稱", ""}}, false)),
			post("IHKE3713S01/SP_IHKE3713S03", personalSection("list1", "來源問卷選擇題定義", []fieldSpec{{"ques.no", "來源題號", ""}, {"ques.text", "來源題目原文", ""}, {"ques.scoreText", "來源題目分數說明", ""}, {"ans[].no", "來源選項代碼", ""}, {"ans[].text", "來源選項原文", ""}}, false), personalSection("list2", "來源問卷下拉題定義", []fieldSpec{{"ques.no", "來源題號", ""}, {"ques.text", "來源題目原文", ""}, {"ques.scoreText", "來源題目分數說明", ""}, {"ans[].no", "來源選項代碼", ""}, {"ans[].text", "來源選項原文", ""}}, false)),
			post("IHKE3713S01/SP_IHKE3713S07_V02", personalSection("score_list", "來源既有生活型態分數", []fieldSpec{{"field_name", "來源領域代碼", ""}, {"score", "來源原始分數", ""}}, false), personalSection("", "來源評估日期與院所", []fieldSpec{{"date", "來源評估日期", ""}, {"hnm", "來源評估院所", ""}}, true)),
			post("IHKE3713S01/SP_IHKE3713S06", personalSection("ans_list", "來源自行填寫原始答案", []fieldSpec{{"ques", "來源題號", ""}, {"ans", "來源原始答案代碼", ""}}, false), personalSection("", "來源作答日期", []fieldSpec{{"date", "來源作答日期", ""}}, true)),
		}
	case "campaign_measurements":
		sections := []providerSection{}
		for _, root := range []string{"sP_IHKE3205S01", "sP_IHKE3205S07", "sP_IHKE3205S04", "data_112", "data_113"} {
			section := personalSection(root, "來源活動連結的生理量測摘要", []fieldSpec{{"code", "來源量測代碼", ""}, {"codE_NAME", "來源量測名稱", ""}, {"creatE_DATE", "來源顯示更新日期（非推定採檢日期）", ""}, {"value", "來源量測值（原文）", ""}, {"uniT_DATA", "來源單位", ""}, {"abnormal_Status", "來源旗標（原文）", ""}}, false)
			section.date, section.titleField = "creatE_DATE", "codE_NAME"
			sections = append(sections, section)
		}
		updated := personalSection("", "來源血壓摘要顯示更新日期", []fieldSpec{{"blood_Pressurep_Date", "來源血壓摘要顯示更新日期（非推定採檢日期）", ""}}, true)
		updated.optional = true
		sections = append(sections, updated)
		return []providerRequest{get("IHKE3799S01/page_load", personalSection("", "來源活動既存量測快照", personalCampaignFields(), true)), get("ihke3205s01/SP_IHKE3205S01", sections...)}
	case "child_schedule":
		return []providerRequest{get("ihke3703s01/SP_IHKE3703S01", personalSection("sP_IHKE3703S01", "來源已存兒童資料", personalChildFields, false)), post("ihke3703s04/SP_IHKE3703S04", personalSection("sP_IHKE3703S04", "來源建議接種時程（非已接種）", personalChildScheduleFields, false)), post("ihke3703s04/SP_IHKE3104S52", personalSection("", "來源已存兒童通知（非已接種）", personalNotificationFields, true))}
	case "health_coin_clinical":
		return []providerRequest{
			get("ihke3714s06/pageload", personalSection("pointitemList", "來源既有同意下的健康紀錄項目", []fieldSpec{{"pointitemCode", "來源項目代碼", ""}, {"pointitemName", "來源項目名稱", ""}}, false)),
			get("ihke3714s08/pageload", personalSection("pointList", "來源健康紀錄", personalCoinFields, false), personalSection("", "來源健康紀錄（單筆回應）", personalCoinFields, true), personalSection("detaildesc", "來源結果說明原文", []fieldSpec{{"text", "來源說明原文", ""}}, false), personalSection("adviceList", "來源建議原文（非 Healthbank 建議）", []fieldSpec{{"codeCname", "來源建議原文", ""}}, false)),
			get("ihke3714s07/pageload", personalSection("", "來源既有生活型態填寫日期", []fieldSpec{{"lifestyleDate", "來源已填寫日期（原文，非推定評估日期）", ""}}, true)),
		}
	}
	return nil
}

func personalKnownFields(row wireRow, specs []fieldSpec) ([]SupplementField, error) {
	// Preserve the literal source boolean only for observed state/flag fields.
	// A boolean is never accepted as an assay result or a numerical score.
	projected := make(wireRow, len(row))
	for key, value := range row {
		projected[key] = value
	}
	for _, spec := range specs {
		key := strings.ToLower(spec.key)
		allowed := key == "agreepoint" || key == "nodata" || key == "askupdate" || key == "showpredictionarea" || key == "computeresultstatus" || strings.HasPrefix(key, "c_") && strings.HasSuffix(key, "_r")
		value := bytes.TrimSpace(projected[key])
		if allowed && (bytes.Equal(value, []byte("true")) || bytes.Equal(value, []byte("false"))) {
			projected[key] = json.RawMessage(strconv.Quote(string(value)))
		}
	}
	return appendKnownFields(projected, specs)
}

func personalAppendRow(report *SupplementReport, category, scope string, index int, row wireRow, section providerSection, origin string, allowEmpty bool) error {
	fields, err := personalKnownFields(row, section.fields)
	if err != nil {
		return err
	}
	if len(fields) == 0 {
		if allowEmpty {
			known := section.root != "" && len(row) == 0
			for _, field := range section.fields {
				_, present := row[strings.ToLower(field.key)]
				known = known || present
			}
			if known {
				return sourceSectionDiagnostic(report, category, scope, nil, true, false, 0)
			}
		}
		return changedError()
	}
	entry := SupplementEntry{Key: medicalKey(category, scope, section.root, index), Title: section.title, Source: section.source, Origin: origin, ContractID: sourceContractID(category, scope), Fields: fields}
	for _, property := range []struct {
		key string
		out *string
	}{{section.date, &entry.Date}, {section.end, &entry.EndDate}, {section.institution, &entry.Institution}, {section.titleField, &entry.Title}} {
		if property.key == "" {
			continue
		}
		value, readErr := rowText(row, property.key)
		if readErr != nil {
			return readErr
		}
		if value != "" {
			*property.out = value
		}
	}
	if entry.Date != "" {
		entry.DateRole = providerDateRole(category, section.date, false)
	}
	report.Entries = append(report.Entries, entry)
	if len(report.Entries) > MaxReportRecords {
		return failure("RESPONSE_TOO_LARGE", "來源既有紀錄超過上限，未回傳截斷資料。")
	}
	return nil
}

func personalAppendSection(report *SupplementReport, category, scope string, object wireRow, section providerSection, origin string) (bool, error) {
	if section.root == "" && section.optional {
		present := false
		for _, field := range section.fields {
			_, exists := object[strings.ToLower(field.key)]
			present = present || exists
		}
		if !present {
			return false, sourceSectionDiagnostic(report, category, scope, nil, false, true, 0)
		}
	}
	rows, present, err := providerSectionRows(object, section)
	if err != nil {
		return present, err
	}
	if err := sourceSectionDiagnostic(report, category, scope, object[strings.ToLower(section.root)], present, section.optional, len(rows)); err != nil {
		return present, err
	}
	for index, row := range rows {
		if err := personalAppendRow(report, category, scope, index, row, section, origin, section.object); err != nil {
			return present, err
		}
	}
	return present, nil
}

func (s *Session) providerPersonalFetch(ctx context.Context, report *SupplementReport, category string, refs map[string]providerReference) (bool, error) {
	requests := personalProviderRequests(category)
	if len(requests) == 0 {
		return false, nil
	}
	switch category {
	case "prenatal", "child_preventive":
		request := requests[0]
		rows, err := s.supplementRequest(ctx, request.method, request.path, nil, nil, "", report)
		if err != nil {
			return true, err
		}
		for index, row := range rows {
			if err := personalAppendRow(report, category, "root", index, row, request.sections[0], "institutional", false); err != nil {
				return true, err
			}
		}
		return true, nil
	case "lifestyle_assessment":
		return true, s.personalLifestyle(ctx, report, requests)
	case "child_schedule":
		return true, s.personalChildList(ctx, report, requests[0], refs)
	case "health_coin_clinical":
		return true, s.personalCoinList(ctx, report, requests[0], refs)
	}
	recognized := false
	for _, request := range requests {
		object, err := s.providerRead(ctx, request)
		if err != nil {
			return true, err
		}
		for _, section := range request.sections {
			scope := section.root
			if scope == "" {
				scope = "root"
				if category == "campaign_measurements" && request.path == "/api/ihke3000/ihke3205s01/SP_IHKE3205S01" {
					scope = "physiology_update"
				}
			}
			present, err := personalAppendSection(report, category, scope, object, section, "official_summary")
			recognized = recognized || present
			if err != nil {
				return true, err
			}
		}
	}
	if !recognized {
		return true, changedError()
	}
	return true, nil
}

func personalAddReference(refs map[string]providerReference, entry *SupplementEntry, ref providerReference) error {
	if len(refs) >= MaxReportRecords || len(ref.rowID) == 0 || len(ref.rowID) > 4096 || len(ref.sourceType) > 128 {
		return changedError()
	}
	key := make([]byte, 24)
	if _, err := rand.Read(key); err != nil {
		return failure("INITIALIZATION_FAILED", "無法建立來源紀錄明細識別。")
	}
	entry.DetailID = hex.EncodeToString(key)
	ref.originKey, ref.title = entry.Key, entry.Title
	refs[entry.DetailID] = ref
	return nil
}

func (s *Session) personalChildList(ctx context.Context, report *SupplementReport, request providerRequest, refs map[string]providerReference) error {
	object, err := s.providerRead(ctx, request)
	if err != nil {
		return err
	}
	section := request.sections[0]
	section.titleField = "chilD_NAME"
	rows, present, err := providerSectionRows(object, section)
	if err != nil || !present {
		return changedError()
	}
	if err := sourceSectionDiagnostic(report, report.Category, section.root, object[strings.ToLower(section.root)], present, false, len(rows)); err != nil {
		return err
	}
	for index, row := range rows {
		if err := personalAppendRow(report, report.Category, section.root, index, row, section, "self_entered", false); err != nil {
			return err
		}
		id, err := rowText(row, "roW_ID")
		if err != nil {
			return err
		}
		if err := personalAddReference(refs, &report.Entries[len(report.Entries)-1], providerReference{rowID: id, kind: "child_schedule"}); err != nil {
			return err
		}
	}
	return nil
}

func personalConsent(object wireRow) (bool, error) {
	switch string(bytes.TrimSpace(object["agreepoint"])) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, changedError()
	}
}

func (s *Session) personalCoinList(ctx context.Context, report *SupplementReport, request providerRequest, refs map[string]providerReference) error {
	section := request.sections[0]
	section.titleField = "pointitemName"
	// A/B/C are literal categories linked by the public source home. D is
	// first-login/reward activity, not another clinical-record source.
	for _, categoryCode := range []string{"A", "B", "C"} {
		request.params = url.Values{"cCat": {categoryCode}}
		object, err := s.providerRead(ctx, request)
		if err != nil {
			return err
		}
		consent, err := personalConsent(object)
		if err != nil {
			return err
		}
		if !consent {
			if len(report.Entries) != 0 {
				return changedError()
			}
			return personalAppendRow(report, report.Category, "eligibility", 0, object, personalSection("", "來源既有同意狀態", []fieldSpec{{"agreePoint", "來源既有同意狀態（原文）", ""}}, true), "official_summary", false)
		}
		rows, present, err := providerSectionRows(object, section)
		if err != nil || !present {
			return changedError()
		}
		scope := "pointitemlist_" + strings.ToLower(categoryCode)
		if err := sourceSectionDiagnostic(report, report.Category, scope, object["pointitemlist"], present, false, len(rows)); err != nil {
			return err
		}
		for index, row := range rows {
			if err := personalAppendRow(report, report.Category, scope, index, row, section, "official_summary", false); err != nil {
				return err
			}
			code, err := rowText(row, "pointitemCode")
			if err != nil || len(code) < 2 || len(code) > 64 || !strings.HasPrefix(code, categoryCode) {
				return changedError()
			}
			for _, character := range code[1:] {
				if character < '0' || character > '9' {
					return changedError()
				}
			}
			entry := &report.Entries[len(report.Entries)-1]
			entry.Kind = "source_index"
			if err := personalAddReference(refs, entry, providerReference{rowID: code, sourceType: categoryCode, kind: "health_coin_clinical"}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Session) providerPersonalDetail(ctx context.Context, report *SupplementReport, category, reference string, ref providerReference) (bool, error) {
	var err error
	switch category {
	case "child_schedule":
		err = s.personalChildDetail(ctx, report, reference, ref)
	case "health_coin_clinical":
		err = s.personalCoinDetail(ctx, report, ref)
	default:
		return false, nil
	}
	if err != nil {
		return true, err
	}
	// Different children/items may have identical section names and row
	// positions. Bind record keys to the already validated source parent, as
	// the ordinary provider detail projector does, without exposing its ID.
	for index := range report.Entries {
		entry := &report.Entries[index]
		entry.Key = medicalKey(category, ref.originKey+"\x00"+ref.rowID, entry.Key, index)
	}
	return true, nil
}

func (s *Session) personalChildDetail(ctx context.Context, report *SupplementReport, reference string, ref providerReference) error {
	requests := personalProviderRequests("child_schedule")
	request := requests[1]
	request.payload = map[string]string{"c_rowid": ref.rowID}
	object, err := s.providerRead(ctx, request)
	if err != nil {
		return err
	}
	report.Coverage = providerSpecs["child_schedule"].coverage
	section := request.sections[0]
	rows, present, err := providerSectionRows(object, section)
	if err != nil || !present {
		return changedError()
	}
	if err := sourceSectionDiagnostic(report, "child_schedule", section.root, object[strings.ToLower(section.root)], present, false, len(rows)); err != nil {
		return err
	}
	notifications := map[string]bool{}
	for index, row := range rows {
		if err := personalAppendRow(report, "child_schedule", section.root, index, row, section, "official_summary", false); err != nil {
			return err
		}
		id, err := rowText(row, "pusH_SEQ_NO")
		if err != nil || len(id) > 4096 {
			return changedError()
		}
		if id != "" {
			notifications[id] = true
		}
	}
	if len(notifications) > 128 {
		return failure("RESPONSE_TOO_LARGE", "來源兒童通知數量超過單次讀取上限，未回傳截斷通知。")
	}
	ids := make([]string, 0, len(notifications))
	for id := range notifications {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for index, id := range ids {
		request := requests[2]
		request.payload = map[string]string{"c_seq_no": id}
		value, err := s.providerRead(ctx, request)
		if err != nil {
			return err
		}
		if err := personalAppendRow(report, "child_schedule", "stored_notification", index, value, request.sections[0], "self_entered", true); err != nil {
			return err
		}
	}
	if err := validateSupplementReport(*report); err != nil {
		return err
	}
	return s.attachProviderDocuments("child_schedule", reference, ref, object, report)
}

func (s *Session) personalCoinDetail(ctx context.Context, report *SupplementReport, ref providerReference) error {
	request := personalProviderRequests("health_coin_clinical")[1]
	request.params = url.Values{"pCode": {ref.rowID}}
	object, err := s.providerRead(ctx, request)
	if err != nil {
		return err
	}
	consent, err := personalConsent(object)
	if err != nil {
		return err
	}
	if !consent {
		return failure("REQUEST_FAILED", "來源目前未提供此項既有同意下的健康紀錄，請重新取得來源清單。")
	}
	report.Coverage = providerSpecs["health_coin_clinical"].coverage
	section := request.sections[0]
	section.title, section.source, section.institution = ref.title, "健康存摺／健康幣來源健康紀錄", "hospName"
	switch ref.rowID {
	case "A001", "A002":
		section.date = "checkDate"
	case "A003", "A004", "A005", "A006", "A007", "A008":
		section.date = "funcDate"
	case "B001", "B002", "B003":
		section.date = "inoculationDate"
	case "C001":
		section.date = "createDate"
	}
	if _, present := object["pointlist"]; !present {
		// S10 renders the same endpoint's legacy single-object response. The
		// mere presence of a task title/reward balance is not a clinical record.
		hasClinical := false
		for _, field := range personalCoinFields {
			if field.key == "pointitemName" {
				continue
			}
			if _, found := object[strings.ToLower(field.key)]; found {
				hasClinical = true
				break
			}
		}
		if !hasClinical {
			if err := sourceSectionDiagnostic(report, "health_coin_clinical", "pointList", nil, false, false, 0); err != nil {
				return err
			}
			return s.personalCoinLifestyleDate(ctx, report, ref)
		}
		section.root, section.object = "", true
	}
	if _, err := personalAppendSection(report, "health_coin_clinical", "pointlist_"+strings.ToLower(ref.rowID), object, section, "official_summary"); err != nil {
		return err
	}
	for _, section := range request.sections[2:] {
		section.optional = true
		if _, err := personalAppendSection(report, "health_coin_clinical", section.root, object, section, "official_summary"); err != nil {
			return err
		}
	}
	return s.personalCoinLifestyleDate(ctx, report, ref)
}

func (s *Session) personalCoinLifestyleDate(ctx context.Context, report *SupplementReport, ref providerReference) error {
	// The task page can already show the completion date while the later
	// reward-linked result list is still unavailable. Preserve that source
	// fact separately; never create a questionnaire or trigger crediting.
	if ref.rowID != "C001" {
		return nil
	}
	request := personalProviderRequests("health_coin_clinical")[2]
	request.params = url.Values{"pCode": {ref.rowID}}
	object, err := s.providerRead(ctx, request)
	if err != nil {
		return err
	}
	consent, err := personalConsent(object)
	if err != nil {
		return err
	}
	if !consent {
		return failure("REQUEST_FAILED", "來源目前未提供此項既有同意下的填寫紀錄，請重新取得來源清單。")
	}
	section := request.sections[0]
	section.optional = true
	_, err = personalAppendSection(report, "health_coin_clinical", "lifestyle_completion", object, section, "official_summary")
	return err
}

type personalQuestion struct {
	text, scoreText string
	options         map[string]string
}

func personalQuestions(object wireRow) (map[string]personalQuestion, error) {
	questions := map[string]personalQuestion{}
	for _, root := range []string{"list1", "list2"} {
		rows, present, err := providerSectionRows(object, providerSection{root: root})
		if err != nil || !present {
			return nil, changedError()
		}
		for _, row := range rows {
			question, err := providerObject(row["ques"])
			if err != nil {
				return nil, err
			}
			id, err := rowText(question, "no")
			if err != nil || id == "" || len(id) > 128 {
				return nil, changedError()
			}
			if _, duplicate := questions[id]; duplicate {
				return nil, changedError()
			}
			text, err := rowText(question, "text")
			if err != nil {
				return nil, err
			}
			scoreText, err := rowText(question, "scoreText")
			if err != nil {
				return nil, err
			}
			options, err := supplementRows(row["ans"], "")
			if err != nil {
				return nil, err
			}
			definition := personalQuestion{text: text, scoreText: scoreText, options: map[string]string{}}
			for _, option := range options {
				code, err := rowText(option, "no")
				if err != nil || code == "" || len(code) > 128 {
					return nil, changedError()
				}
				value, err := rowText(option, "text")
				if err != nil {
					return nil, err
				}
				if _, duplicate := definition.options[code]; duplicate {
					return nil, changedError()
				}
				definition.options[code] = value
			}
			questions[id] = definition
			if len(questions) > MaxReportRecords {
				return nil, changedError()
			}
		}
	}
	return questions, nil
}

func (s *Session) personalScore(ctx context.Context, request providerRequest) (wireRow, bool, error) {
	payload, err := json.Marshal(request.payload)
	if err != nil {
		return nil, false, changedError()
	}
	defer clear(payload)
	body, _, err := s.request(ctx, request.method, request.path, nil, payload, maxJSONBytes, true)
	if err != nil {
		return nil, false, err
	}
	defer clear(body)
	var status string
	if json.Unmarshal(body, &status) == nil && status == "NoData" {
		return nil, true, nil
	}
	object, err := providerObject(body)
	if err != nil {
		s.invalidateOnAuthError(err)
		return nil, false, s.annotateResponseError(err)
	}
	return object, false, nil
}

func (s *Session) personalLifestyle(ctx context.Context, report *SupplementReport, requests []providerRequest) error {
	group, err := s.providerRead(ctx, requests[0])
	if err != nil {
		return err
	}
	code, err := rowText(group, "codE_CNAME")
	if err != nil || code == "" || len(code) > 128 {
		return changedError()
	}
	labels, err := s.providerRead(ctx, requests[1])
	if err != nil {
		return err
	}
	fieldRows, present, err := providerSectionRows(labels, requests[1].sections[0])
	if err != nil || !present {
		return changedError()
	}
	fieldNames := map[string]string{}
	for _, row := range fieldRows {
		id, err := rowText(row, "code")
		if err != nil || id == "" || len(id) > 128 {
			return changedError()
		}
		name, err := rowText(row, "text")
		if err != nil {
			return err
		}
		if _, duplicate := fieldNames[id]; duplicate {
			return changedError()
		}
		fieldNames[id] = name
	}
	var scores [2]wireRow
	for index := range scores {
		request := requests[3]
		request.payload = map[string]any{"C_GRP_QUES_CODE": code, "C_LAST_N": index + 1}
		object, noData, err := s.personalScore(ctx, request)
		if err != nil {
			return err
		}
		scope := "score_" + strconv.Itoa(index+1)
		if noData {
			if err := sourceSectionDiagnostic(report, report.Category, scope, nil, true, false, 0); err != nil {
				return err
			}
			continue
		}
		scores[index] = object
		date, err := rowText(object, "date")
		if err != nil {
			return err
		}
		institution, err := rowText(object, "hnm")
		if err != nil {
			return err
		}
		institutionID, err := rowText(object, "hid")
		if err != nil {
			return err
		}
		origin := "self_entered"
		if institution != "" || institutionID != "" {
			origin = "institutional"
		}
		if date != "" || institution != "" {
			header := personalSection("", "生活型態來源評估日期與院所", []fieldSpec{{"date", "來源評估日期（原文）", ""}, {"hnm", "來源評估院所", ""}}, true)
			header.date, header.institution = "date", "hnm"
			if err := personalAppendRow(report, report.Category, scope+".summary", 0, object, header, origin, false); err != nil {
				return err
			}
			if date != "" {
				report.Entries[len(report.Entries)-1].DateRole = "recorded"
			}
		}
		section := request.sections[0]
		rows, present, err := providerSectionRows(object, section)
		if err != nil || !present {
			return changedError()
		}
		if err := sourceSectionDiagnostic(report, report.Category, scope, object["score_list"], present, false, len(rows)); err != nil {
			return err
		}
		for rowIndex, row := range rows {
			id, err := rowText(row, "field_name")
			if err != nil {
				return err
			}
			if name := fieldNames[id]; name != "" {
				section.title = name + "（來源既有分數）"
			} else {
				section.title = "生活型態來源既有分數"
			}
			if err := personalAppendRow(report, report.Category, scope, rowIndex, row, section, origin, false); err != nil {
				return err
			}
			entry := &report.Entries[len(report.Entries)-1]
			entry.Date, entry.Institution = date, institution
			if date != "" {
				entry.DateRole = "recorded"
			}
		}
	}
	// Official S03 indexes mixed-source scores, while S04 indexes self answers.
	// Its second-score link is q=1 when the first score has an institution ID.
	answerIndices := map[int]bool{}
	for index, score := range scores {
		if score == nil {
			continue
		}
		date, err := rowText(score, "date")
		if err != nil {
			return err
		}
		institution, err := rowText(score, "hnm")
		if err != nil {
			return err
		}
		if date == "" || institution != "" {
			continue
		}
		answerIndex := index + 1
		if index == 1 {
			firstInstitution, err := rowText(scores[0], "hid")
			if err != nil {
				return err
			}
			if firstInstitution != "" {
				answerIndex = 1
			}
		}
		answerIndices[answerIndex] = true
	}
	if len(answerIndices) == 0 {
		return nil
	}
	definitionRequest := requests[2]
	definitionRequest.payload = map[string]string{"C_GRP_QUES_CODE": code}
	definitions, err := s.providerRead(ctx, definitionRequest)
	if err != nil {
		return err
	}
	questions, err := personalQuestions(definitions)
	if err != nil {
		return err
	}
	for index := 1; index <= 2; index++ {
		if !answerIndices[index] {
			continue
		}
		request := requests[4]
		request.payload = map[string]any{"C_GRP_QUES_CODE": code, "C_LAST_N": index}
		object, err := s.providerRead(ctx, request)
		if err != nil {
			return err
		}
		if err := personalLifestyleAnswers(report, index, object, request.sections[0], questions); err != nil {
			return err
		}
	}
	return nil
}

func personalLifestyleAnswers(report *SupplementReport, slot int, object wireRow, section providerSection, questions map[string]personalQuestion) error {
	date, err := rowText(object, "date")
	if err != nil {
		return err
	}
	rows, present, err := providerSectionRows(object, section)
	if err != nil || !present {
		return changedError()
	}
	scope := "answers_" + strconv.Itoa(slot)
	if err := sourceSectionDiagnostic(report, report.Category, scope, object["ans_list"], present, false, len(rows)); err != nil {
		return err
	}
	if date != "" {
		header := personalSection("", "生活型態來源作答日期", []fieldSpec{{"date", "來源作答日期（原文）", ""}}, true)
		header.date = "date"
		if err := personalAppendRow(report, report.Category, scope+".summary", 0, object, header, "self_entered", false); err != nil {
			return err
		}
		report.Entries[len(report.Entries)-1].DateRole = "recorded"
	}
	for index, row := range rows {
		questionID, err := rowText(row, "ques")
		if err != nil {
			return err
		}
		answer, err := rowText(row, "ans")
		if err != nil {
			return err
		}
		if err := personalAppendRow(report, report.Category, scope, index, row, section, "self_entered", false); err != nil {
			return err
		}
		entry := &report.Entries[len(report.Entries)-1]
		entry.Date = date // Never inherit the possibly different mixed-score date.
		if date != "" {
			entry.DateRole = "recorded"
		}
		if question, found := questions[questionID]; found {
			for _, field := range []SupplementField{{Name: "來源題目原文", Value: question.text, Code: "question_text"}, {Name: "來源題目分數說明", Value: question.scoreText, Code: "question_score_text"}, {Name: "來源所選答案原文", Value: question.options[answer], Code: "answer_text"}} {
				if field.Value != "" {
					entry.Fields = append(entry.Fields, field)
				}
			}
		}
	}
	return nil
}
