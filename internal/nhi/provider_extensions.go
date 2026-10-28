// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package nhi

import (
	"net/http"
	"net/url"
)

// Additional contracts found while checking the whole provider surface, rather
// than duplicating only the website's most visible menus.
func extendProviderSpecs(items map[string]providerSpec) {
	for _, source := range []struct{ id, label, module, root string }{
		{"diabetes_ckd_care", "糖尿病合併初期腎病追蹤", "ihke3516s01/SP_IHKE3516S01", "sP_IHKE3516S01_Data"},
		{"metabolic_care", "代謝症候群照護追蹤", "ihke3517s01/SP_IHKE3517S01", "sP_IHKE3517S01_Data"},
	} {
		section := providerSectionSpec(source.root, source.label, "健康存摺／官方照護方案", "case_DATE", providerCareFields)
		section.end, section.detail = "close_DATE", true
		items[source.id] = providerSpec{info: ProviderCategoryInfo{ID: source.id, Label: source.label, Group: "照護與追蹤", RangeMode: "source", HasDetails: true}, coverage: "讀取來源收案與追蹤紀錄；官方頁面標示近三年追蹤，未提供任意日期搜尋或獨立總數。來源階段不等於 Healthbank 診斷。", requests: []providerRequest{{path: "/api/ihke3000/" + source.module, sections: []providerSection{section}}}}
	}
	familyFields := []fieldSpec{{"assay_NAME", "來源檢驗名稱", ""}, {"assay_ZH_NAME", "來源中文名稱", ""}, {"next_CHK_DATE", "來源建議下次檢查日期（非預約）", ""}, {"chk_memo", "來源追蹤註記", ""}}
	family := providerSectionSpec("sP_IHKE3726S02", "家庭醫師健康追蹤項目", "健康存摺／家庭醫師健康管理來源", "", familyFields)
	family.detail, family.titleField = true, "assay_ZH_NAME"
	pressure := family
	pressure.root, pressure.title, pressure.object = "bp_IHKE3726S02", "家庭醫師血壓追蹤項目", true
	items["family_health_followup"] = providerSpec{info: ProviderCategoryInfo{ID: "family_health_followup", Label: "家庭醫師健康與血壓追蹤", Group: "照護與追蹤", RangeMode: "date_search", HasDetails: true}, coverage: "讀取來源健康與血壓追蹤項目，各項明細搜尋所選完整期間；保留來源值與通道，不套用網站風險分級或自行推定診斷。", requests: []providerRequest{{path: "/api/ihke3000/ihke3726s01/SP_IHKE3726S01", sections: []providerSection{family, pressure}}}}
	body := items["body_measurements"]
	body.info.Label, body.info.RangeMode, body.info.HasDetails = "身體量測跨來源歷史", "date_search", true
	body.coverage = "清單保留來源專區項目與最近摘要，各項明細搜尋所選完整期間；院所、自行輸入及預防保健等來源保留分別，不強制合併或推定診斷。無獨立總數。"
	body.requests[0].sections[0].detail = true
	body.requests[0].sections[0].titleField = "c_CURE_CNAME"
	body.requests[0].sections[0].fields = append(body.requests[0].sections[0].fields, fieldSpec{"code", "來源項目代碼", ""})
	items["body_measurements"] = body
	device := providerSectionSpec("sp_IHKE3207S01", "行動運動資料索引", "健康存摺／來源提供的行動運動項目", "", []fieldSpec{{"fun_NAME", "來源運動項目名稱", ""}})
	device.titleField, device.detail = "fun_NAME", true
	items["device_activity"] = providerSpec{info: ProviderCategoryInfo{ID: "device_activity", Label: "行動裝置運動紀錄", Group: "自行記錄來源", RangeMode: "date_search", HasDetails: true}, coverage: "依官方選單實際提供的行動運動項目讀取資料，各項明細查詢所選期間；官方可能限制可提供月份。沒有來源入口不等於沒有運動資料，不在瀏覽器直接存取手機健康資料。", requests: []providerRequest{{path: "/api/ihke3000/IHKEMASTER/SP_IHKE3099S05", method: http.MethodPost, payload: map[string]string{"c_parent_fun_code": "IHKE3207"}, sections: []providerSection{device}}}}
}

func providerDeviceSelector(row wireRow) (string, error) {
	raw, err := rowText(row, "url")
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", changedError()
	}
	if parsed.Scheme != "" || parsed.Host != "" || parsed.User != nil || parsed.Fragment != "" || (parsed.Path != "/IHKE3207S05" && parsed.Path != "IHKE3207S05") {
		return "", nil
	}
	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil || len(values["s"]) != 1 {
		return "", changedError()
	}
	code := values.Get("s")
	if len(code) == 0 || len(code) > 64 {
		return "", changedError()
	}
	for _, character := range code {
		if !(character >= '0' && character <= '9' || character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character == '_' || character == '-') {
			return "", changedError()
		}
	}
	return code, nil
}

func providerExtraDetailRequest(category string, ref providerReference) (providerRequest, error) {
	post := func(path, root, title string, payload map[string]string, fields []fieldSpec) providerRequest {
		return providerRequest{path: path, method: http.MethodPost, payload: payload, sections: []providerSection{providerSectionSpec(root, title, "健康存摺／"+title, "case_DATE", fields)}}
	}
	switch category {
	case "diabetes_ckd_care":
		fields := []fieldSpec{{"stage", "來源追蹤階段", ""}, {"case_DATE", "收案／就醫日期（原文）", ""}, {"chk_DATE", "檢驗日期（原文）", ""}, {"base_SBP", "收縮壓", "mmHg"}, {"base_EBP", "舒張壓", "mmHg"}, {"blood_HBA1C", "糖化血色素", "%"}, {"blood_GA", "糖化白蛋白（沿用官網單位，未校正）", "mg/dL"}, {"blood_LDL", "低密度脂蛋白", "mg/dL"}, {"blood_AC", "飯前血糖", "mg/dL"}, {"blood_PC", "飯後血糖", "mg/dL"}, {"serum_CREAT", "肌酸酐", "mg/dL"}, {"egfr", "來源 eGFR", "mL/min/1.73m²"}, {"uacr", "尿液微量白蛋白（沿用官網標示）", "mg/g"}}
		return post("/api/ihke3000/ihke3516s02/SP_IHKE3516S02", "sP_IHKE3516S02_Data", "糖尿病合併初期腎病追蹤結果", map[string]string{"rid": ref.rowID}, fields), nil
	case "metabolic_care":
		fields := []fieldSpec{{"stage", "來源追蹤階段", ""}, {"case_DATE", "收案／就醫日期（原文）", ""}, {"chk_DATE", "檢驗日期（原文）", ""}, {"bmi", "來源 BMI", "kg/m²"}, {"waist", "腰圍", "cm"}, {"base_SBP", "收縮壓", "mmHg"}, {"base_EBP", "舒張壓", "mmHg"}, {"blood_HBA1C", "糖化血色素", "%"}, {"blood_HDL", "高密度脂蛋白", "mg/dL"}, {"blood_LDL", "低密度脂蛋白", "mg/dL"}, {"blood_AC", "飯前血糖", "mg/dL"}, {"blood_TG", "三酸甘油脂", "mg/dL"}, {"blood_CHOL_T", "總膽固醇", "mg/dL"}}
		return post("/api/ihke3000/ihke3517s02/SP_IHKE3517S02", "sP_IHKE3517S02_Data", "代謝症候群追蹤結果", map[string]string{"rid": ref.rowID}, fields), nil
	case "body_measurements":
		fields := append(append([]fieldSpec{}, providerResultFields...), fieldSpec{"assay_VALUE_1", "來源第二量測通道（原文）", ""})
		request := post("/api/ihke3000/IHKE3512S02/search", "maindata", ref.title, map[string]string{"dCode": ref.rowID, "dStr": ref.query.StartDate, "dEnd": ref.query.EndDate}, fields)
		request.sections[0].date = "real_INSPECT_DATE"
		if ref.rowID == "BP" {
			for i := range request.sections[0].fields {
				switch request.sections[0].fields[i].key {
				case "assay_VALUE":
					request.sections[0].fields[i].label = "來源收縮壓（BP）"
				case "assay_VALUE_1":
					request.sections[0].fields[i].label = "來源舒張壓（BP）"
				}
			}
		}
		return request, nil
	case "family_health_followup":
		fields := []fieldSpec{{"assay_DATE", "檢驗日期（原文）", ""}, {"assay_NAME", "來源檢驗名稱", ""}, {"assay_VALUE", "來源第一量測通道（原文）", ""}, {"assay_VALUE_1", "來源第二量測通道（原文）", ""}, {"c_ASSAY_VALUE", "來源組合顯示值（原文）", ""}, {"assay_SOURCE_NAME", "來源類型（原文）", ""}, {"unit_DATA", "單位（原文）", ""}}
		request := post("/api/ihke3000/ihke3726s01/SP_IHKE3726S07", "sP_IHKE3726S07_Data", ref.title, map[string]string{"dType": ref.rowID, "dStr": ref.query.StartDate, "dEnd": ref.query.EndDate}, fields)
		request.sections[0].date = "assay_DATE"
		return request, nil
	case "device_activity":
		request, err := providerDetailRequest("physiology", ref)
		if err == nil {
			request.sections[0].source = "健康存摺／行動裝置運動資料來源"
		}
		return request, err
	}
	return providerRequest{}, failure("INVALID_INPUT", "此資料類別未提供明細。")
}
