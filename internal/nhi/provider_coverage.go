// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"bytes"
	"net/http"
	"strconv"
)

// These are existing personal source records found outside the principal
// clinical menus. They remain source summaries/collection events: none of the
// source grades, generated advice, annual totals or dates is recomputed here.
func extendCoverageProviderSpecs(items map[string]providerSpec) {
	add := func(id, label, group, coverage, path, method string, section providerSection) {
		items[id] = providerSpec{
			info:     ProviderCategoryInfo{ID: id, Label: label, Group: group, RangeMode: "source"},
			coverage: coverage,
			requests: []providerRequest{{path: path, method: method, sections: []providerSection{section}}},
		}
	}
	window := "讀取來源本次提供的全部資料列；公開頁面沒有日期搜尋或獨立總數，不能保證全部終身歷史。"
	disease := providerSectionSpec("sP_IHKE3723S01_Data", "來源疾病風險評估", "健康存摺／既有疾病風險評估（非 Healthbank 診斷）", "crT_DATE", []fieldSpec{
		{"disease", "來源疾病分類代碼", ""}, {"diseasE_NAME", "來源評估項目", ""},
		{"diseasE_STAGE", "來源疾病風險代碼（0 可能表示資料不足）", ""}, {"stage", "來源分級", ""},
		{"stagE_DESC", "來源分級說明", ""}, {"crT_DATE", "來源評估更新日期（非採檢日期）", ""}, {"edU_INFO", "來源附註（非本站建議）", ""},
	})
	disease.titleField = "diseasE_NAME"
	add("source_disease_assessments", "來源疾病風險評估與依據", "官方來源摘要", window+"保留來源分級及各筆關鍵檢驗原文；評估更新日期和檢驗日期分開，不計算新的風險。", "/api/ihke3000/IHKE3723S01/Get_Disease_Data", http.MethodPost, disease)

	diabetes := providerSectionSpec("", "來源糖尿病風險評估", "健康存摺／既有糖尿病風險及官方 AI 衛教文字（非 Healthbank 建議）", "", []fieldSpec{
		{"period", "來源資料期間（原文）", ""}, {"grade", "來源風險分級代碼", ""}, {"p4p", "來源共同照護註記", ""},
		{"ht", "來源高血壓註記", ""}, {"hl", "來源高血脂註記", ""}, {"ckd", "來源慢性腎病註記", ""}, {"dcsi", "來源 DCSI 值", ""},
		{"r1", "來源 DCSI 視網膜病變項目", ""}, {"r2", "來源 DCSI 腦血管項目", ""}, {"r3", "來源 DCSI 腎病項目", ""},
		{"r4", "來源 DCSI 神經病變項目", ""}, {"r5", "來源 DCSI 心血管項目", ""}, {"r6", "來源 DCSI 周邊血管項目", ""}, {"r7", "來源 DCSI 代謝併發症項目", ""},
		{"pr1", "來源視網膜事件風險代碼", ""}, {"pr2", "來源腦神經事件風險代碼", ""}, {"pr3", "來源腎事件風險代碼", ""},
		{"pr4", "來源神經事件風險代碼", ""}, {"pr5", "來源心血管事件風險代碼", ""}, {"pr6", "來源周邊血管事件風險代碼", ""},
		{"keyobservationtitle", "官方 AI 觀察標題（原文）", ""}, {"keyobservationcontent", "官方 AI 觀察內容（原文）", ""},
		{"actionplansshorttermtitle", "官方 AI 短期衛教標題（原文）", ""}, {"actionplansshorttermcontent", "官方 AI 短期衛教內容（原文）", ""},
		{"actionplansmidtermtitle", "官方 AI 中期衛教標題（原文）", ""}, {"actionplansmidtermcontent", "官方 AI 中期衛教內容（原文）", ""},
		{"complicationcaretitle", "官方 AI 併發症衛教標題（原文）", ""}, {"complicationcarecontent", "官方 AI 併發症衛教內容（原文）", ""},
		{"closingencouragementtitle", "官方 AI 結語標題（原文）", ""}, {"closingencouragementcontent", "官方 AI 結語內容（原文）", ""},
	})
	add("source_diabetes_assessment", "來源糖尿病風險與衛教紀錄", "官方來源摘要", window+"僅限來源試辦適用帳號的既有輸出；保留原始代碼與官方 AI 文字，不將缺值補為 0 或無，不產生診斷或治療建議。", "/api/ihke3000/ihke3518s01/page_load", http.MethodGet, diabetes)

	annual := providerSectionSpec("maindata", "來源年度就醫與費用總覽", "健康存摺／來源年度統計與醫療輻射參考值", "", []fieldSpec{
		{"year", "來源統計年度", ""}, {"cnt", "來源門診次數", ""}, {"o_cnt", "來源住院次數", ""},
		{"part_AMT", "來源門診部分負擔", ""}, {"part_AMT_BASIC", "來源門診基本部分負擔", ""}, {"part_AMT_DRUG", "來源門診藥品部分負擔", ""}, {"part_AMT_ASSAY", "來源門診檢驗檢查部分負擔", ""},
		{"appl_DOT", "來源門診健保點數", ""}, {"drug_DOT", "來源門診藥費點數", ""}, {"assay_AMIN", "來源門診檢查費點數", ""}, {"other_DOT", "來源門診其他點數", ""},
		{"o_part_AMT", "來源住院部分負擔", ""}, {"o_appl_DOT", "來源住院健保點數", ""}, {"o_drug_DOT", "來源住院藥費點數", ""}, {"o_amin_DOT", "來源住院檢查費點數", ""}, {"o_other_DOT", "來源住院其他點數", ""},
		{"radi_MSV", "來源年度醫療輻射參考劑量（非個人實測）", "mSv"},
	})
	add("annual_overview", "年度就醫與醫療輻射參考值", "官方來源摘要", window+"年度統計保留來源數值，不由已取得明細加總替代；輻射參考值不是個人實測劑量或本站危害評估。", "/api/ihke3000/ihke3302s01/page_load", http.MethodGet, annual)
	annualSpec := items["annual_overview"]
	annualSpec.info.HasDetails = true
	annualSpec.requests[0].sections[0].detail = true
	items["annual_overview"] = annualSpec

	mask := providerSectionSpec("sP_IHKE3702S01", "口罩購買／領取紀錄", "健康存摺／口罩購買與配發來源", "purchasE_DATE", []fieldSpec{
		{"purchasE_DATE", "購買／認定日期（原文）", ""}, {"paY_DATE", "付款／發放日期（原文）", ""}, {"qty", "成人口罩數量", "片"}, {"childreN_QTY", "兒童口罩數量", "片"},
	})
	add("mask_collections", "口罩購買與領取紀錄", "其他官方紀錄", window+"購買或配發不證明實際使用；日期依來源意義保留。", "/api/ihke3000/ihke3702s01/SP_IHKE3702S01", http.MethodGet, mask)
	rapid := providerSectionSpec("ihke3706S01_maindata", "快篩購買／領取紀錄", "健康存摺／快篩購買與免費領取來源", "purchase_DATE", []fieldSpec{
		{"purchase_DATE", "購買／領取日期（原文）", ""}, {"firm_NAME", "廠牌名稱", ""},
	})
	add("rapid_test_collections", "快篩購買與領取紀錄", "其他官方紀錄", window+"來源免費領取旗標另列；購買／領取不等於已檢測或檢驗結果。", "/api/ihke3000/ihke3706s01/pageload", http.MethodGet, rapid)
	for _, chart := range []struct{ id, label, path string }{
		{"diet_source_totals", "來源飲食熱量彙總", "/api/ihke3000/ihke3206s01/SP_IHKE3206S06"},
		{"exercise_source_totals", "來源運動熱量彙總", "/api/ihke3000/ihke3207s02/SP_IHKE3207S07"},
	} {
		section := providerSectionSpec("", chart.label, "健康存摺／自行記錄的來源熱量彙總（非 Healthbank 計算）", "", []fieldSpec{{"DateList[]", "來源日期標籤（依原始索引配對）", ""}, {"CalList[]", "來源熱量估值（原文）", "kcal"}})
		add(chart.id, chart.label, "自行記錄來源", "對來源既有熱量彙總送出所選完整日期區間，保留 DateList 與 CalList 每一原始索引；不由明細自行加總替代，不當成精確營養攝取、熱量消耗或實際行為證明。無獨立歷史總數。", chart.path, http.MethodPost, section)
		spec := items[chart.id]
		spec.info.RangeMode = "date_search"
		items[chart.id] = spec
	}
	consent := providerSectionSpec("", "來源家庭醫師同意頁資料", "健康存摺／家庭醫師同意頁既有資料（非本站代為同意）", "", []fieldSpec{
		{"hosp_abbr", "來源家庭醫師院所", ""}, {"prsn_name", "來源家庭醫師姓名", ""}, {"hosp_tel", "來源院所聯絡電話", ""}, {"agn_genus_name", "來源計畫類別", ""},
		{"cyear", "來源頁面年度（原文）", ""}, {"cmonth", "來源頁面月份（原文）", ""}, {"cday", "來源頁面日（原文）", ""},
		{"isshowDR", "來源已顯示家庭醫師資訊旗標", ""}, {"showcontent", "來源已顯示既有內容旗標", ""}, {"showmain", "來源同意表單可見旗標（非已同意）", ""},
	})
	add("family_doctor_consent_snapshot", "家庭醫師同意頁院所資料", "照護與追蹤", window+"保留該頁既有院所及狀態旗標；候選院所或表單可見不等於已簽署同意，也不替代實際收案紀錄。不讀取或提交使用者同意內容。", "/api/ihke3000/IHKE3709S01/page_load", http.MethodGet, consent)
}

var sourceDiseaseEvidenceFields = []fieldSpec{
	{"assaY_NAME", "來源檢驗代碼／名稱", ""}, {"assaY_ZH_NAME", "來源檢驗中文名稱", ""}, {"assaY_DATE", "來源檢驗日期（原文）", ""},
	{"assaY_VALUE", "結果（原文）", ""}, {"uniT_DATA", "單位（原文）", ""}, {"consulT_VALUE_LOWER", "來源參考下限（原文）", ""}, {"consulT_VALUE_MAX", "來源參考上限（原文）", ""},
	{"hosP_ABBR", "來源院所", ""},
}

// Indexed field groups keep every nested row attached to its actual assessment;
// repeated dates and equal results never collapse into a guessed observation.
func appendProviderCoverageFields(report *SupplementReport, entry *SupplementEntry, category string, row wireRow) error {
	if category == "source_disease_assessments" {
		const root = "sP_IHKE3723S01_Append_Data"
		rows, present, err := providerSectionRows(row, providerSection{root: root})
		if err != nil {
			return err
		}
		if err := sourceSectionDiagnostic(report, category, root, row["sp_ihke3723s01_append_data"], present, false, len(rows)); err != nil {
			return err
		}
		missingResults := 0
		for index, nested := range rows {
			fields, err := appendKnownFields(nested, sourceDiseaseEvidenceFields)
			if err != nil || len(fields) == 0 {
				return changedError()
			}
			if len(entry.Fields)+len(fields) > MaxReportRecords {
				return failure("RESPONSE_TOO_LARGE", "來源評估的檢驗依據過多，未回傳不完整紀錄。")
			}
			if sourceResultMissing(category, sourceDiseaseEvidenceFields, fields) {
				missingResults++
			}
			for i := range fields {
				fields[i].Name = "關鍵檢驗第 " + strconv.Itoa(index+1) + " 筆／" + fields[i].Name
			}
			entry.Fields = append(entry.Fields, fields...)
		}
		if missingResults > 0 {
			if err := addSourceDiagnostic(report, "known_result_missing", sourceContractID(category, root), &missingResults); err != nil {
				return err
			}
		}
	}
	if category == "rapid_test_collections" {
		for _, item := range []struct{ code, name string }{{"show_0to6", "來源 0–6 歲免費領取旗標"}, {"show_up65", "來源 65 歲以上免費領取旗標"}} {
			raw, present := row[item.code]
			if !present || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				continue
			}
			value := string(bytes.TrimSpace(raw))
			if value != "true" && value != "false" {
				return changedError()
			}
			entry.Fields = append(entry.Fields, SupplementField{Name: item.name + "（原值）", Value: value, Code: item.code})
		}
	}
	return nil
}
