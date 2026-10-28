// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package nhi

import (
	"net/http"
	"net/url"
	"strings"
)

// Only clinical fields demonstrated by the named public components are exposed.
// Upstream identity numbers, internal row IDs, URLs, QR credentials, menu flags,
// arbitrary JSON fields and executable markup are deliberately not forwarded.
var providerSpecs = buildProviderSpecs()

var providerResultFields = []fieldSpec{
	{"real_INSPECT_DATE", "檢查日期（原文）", ""}, {"order_CODE", "醫令代碼", ""}, {"order_NAME", "檢查名稱", ""},
	{"assay_ITEM_NAME", "檢驗項目名稱", ""}, {"assay_VALUE", "結果（原文）", ""}, {"unit_DATA", "單位（原文）", ""},
	{"consult_VALUE", "參考值（原文）", ""}, {"consult", "參考值（原文）", ""}, {"ori_TYPE_NAME", "來源類型（原文）", ""},
	{"ori_TYPE", "來源代碼（原文）", ""}, {"source_FROM", "來源（原文）", ""},
}

var providerPrescriptionFields = []fieldSpec{
	{"recipe_TYPE_name", "處方類型（原文）", ""}, {"func_DATE", "就醫日期（原文）", ""}, {"func_TYPE_NAME", "科別", ""},
	{"icd_CODE", "疾病分類代碼（非診斷確認）", ""}, {"icd_CODE_CNAME", "疾病分類（非診斷確認）", ""},
	{"drug_CNT", "藥品品項數", ""}, {"part_MARK", "部分負擔註記", ""}, {"chr_SeqNo", "處方可調劑次數（原文）", ""},
}

var providerPrescriptionDrugFields = []fieldSpec{
	{"drug_CODE", "藥品代碼", ""}, {"drug_NAME", "藥品名稱", ""}, {"generic_NAME", "學名", ""},
	{"notreplace_DESC", "來源限制替代理由", ""}, {"user_TIME_NAME", "用量及用法（原文）", ""},
	{"drug_PATH_NAME", "給藥途徑（原文）", ""}, {"drug_DOSE_CODE", "劑型（原文）", ""}, {"dose", "劑量（原文）", ""},
	{"dose_UNIT", "劑量單位（原文）", ""}, {"drug_DAYS", "給藥日數", "日"}, {"drug_NUM", "給藥總量", ""}, {"drug_NUM_UNIT", "給藥單位（原文）", ""},
}

var providerSelfPaidFields = []fieldSpec{
	{"real_INSPECT_DATE", "執行日期（原文）", ""}, {"order_CODE", "醫令代碼", ""},
	{"assay_ITEM_NAME", "藥品成分", ""}, {"assay_VALUE", "藥品含量（原文）", ""},
	{"sh_ORDER_CNAME", "自編中文名稱", ""}, {"sh_ORDER_ENAME", "自編英文名稱", ""},
	{"drug_NUM", "藥品用量（原文）", ""}, {"drug_FRE", "使用頻率（原文）", ""}, {"drug_PATH", "給藥途徑／作用部位（原文）", ""},
	{"order_QTY", "給藥總量", ""}, {"order_DRUG_DAY", "給藥日數", "日"}, {"drug_UNIT", "規格單位（原文）", ""},
}

var providerCareFields = []fieldSpec{{"case_DATE", "收案日期（原文）", ""}, {"close_DATE", "結案日期（原文）", ""}}

func providerSectionSpec(root, title, source, date string, fields []fieldSpec) providerSection {
	return providerSection{root: root, title: title, source: source, date: date, fields: fields}
}

func buildProviderSpecs() map[string]providerSpec {
	items := map[string]providerSpec{}
	add := func(id, label, group, coverage string, requests ...providerRequest) {
		info := ProviderCategoryInfo{ID: id, Label: label, Group: group, RangeMode: "source"}
		for _, request := range requests {
			for _, section := range request.sections {
				info.HasDetails = info.HasDetails || section.detail
			}
		}
		items[id] = providerSpec{info: info, coverage: coverage, requests: requests}
	}
	get := func(path string, sections ...providerSection) providerRequest {
		return providerRequest{path: path, sections: sections}
	}
	sourceCoverage := "讀取官方目前提供的全部回傳陣列；此來源未提供日期搜尋或獨立總數，不宣稱全部終身歷史。"
	allergyFields := []fieldSpec{{"drug_NAME", "過敏內容", ""}, {"allerg_SEVERITY_LEVEL", "嚴重程度（原文）", ""}, {"sympton_NAME", "過敏症狀", ""}, {"allerg_SYMPTON_OTH", "其他症狀說明", ""}, {"upload_FLAG", "收載方式（原文）", ""}, {"upload_D", "來源登錄／上傳日期（原文）", ""}, {"upload_D2", "自行記錄日期（原文）", ""}}
	add("allergies", "過敏紀錄", "個人醫療紀錄", sourceCoverage+"院所登錄與自行記錄分開標示，登錄日期不等於發作日期。",
		get("/api/ihke3000/ihke3202s01/SP_IHKE3202S01", providerSectionSpec("sP_IHKE3202S01", "過敏紀錄", "健保署／院所登錄", "upload_D", allergyFields)),
		get("/api/ihke3000/ihke3202s01/SP_IHKE3202S04", providerSectionSpec("sP_IHKE3202S04", "過敏自行記錄", "健康存摺／自行記錄", "upload_D2", allergyFields)))
	vaccineFields := []fieldSpec{{"code_CNAME", "疫苗名稱", ""}, {"inoculate_D", "接種日期（原文）", ""}, {"source", "來源（原文）", ""}}
	add("vaccinations", "預防接種", "個人醫療紀錄", sourceCoverage+"官方接種資料可能有上傳時間差；自行記錄不是院所驗證。",
		get("/api/ihke3000/ihke3203s01/SP_IHKE3203S01", providerSectionSpec("sP_IHKE3203S01", "預防接種", "健康存摺／官方預防接種來源", "inoculate_D", vaccineFields)),
		get("/api/ihke3000/ihke3203s01/SP_IHKE3203S03", providerSectionSpec("sP_IHKE3203S03", "接種自行記錄", "健康存摺／自行記錄", "inoculate_D", vaccineFields)))
	add("organ_wishes", "器捐與安寧意願註記", "個人醫療紀錄", "僅讀取官方意願註記狀態；健保卡註記日期不是意願簽署日期，不能由此修改意願。",
		get("/api/ihke3000/ihke3204s01/pageload", providerSectionSpec("", "器捐與安寧意願註記", "健康存摺／官方意願註記", "health_DATE", []fieldSpec{{"health_DATE", "健保卡註記日期（非簽署日期）", ""}, {"od_MARK_NAME", "意願註記（原文）", ""}})))
	catastrophic := providerSectionSpec("sP_IHKE3209S01", "重大傷病證明", "健康存摺／重大傷病證明", "valid_S_DATE", []fieldSpec{{"icd10cm_CNAME", "證明項目名稱", ""}, {"valid_S_DATE", "效期起日（原文）", ""}, {"valid_E_DATE", "效期迄日（原文）", ""}})
	catastrophic.end = "valid_E_DATE"
	add("catastrophic", "重大傷病證明", "個人醫療紀錄", sourceCoverage+"官方頁面註明不包括重大傷病第六類；證明效期不等於疾病起訖。", get("/api/ihke3000/ihke3209s01/SP_IHKE3209S01", catastrophic))
	cochlearFields := []fieldSpec{{"met_C_NAME", "醫材名稱", ""}, {"exe_S_DATE", "手術日期（原文）", ""}, {"met_SPEC", "產品型號／規格（代碼）", ""}, {"ear_MARK", "左右耳註記（原文）", ""}}
	add("cochlear", "人工電子耳", "個人醫療紀錄", sourceCoverage,
		get("/api/ihke3000/ihke3211s01/page_load", providerSectionSpec("maindata01", "人工電子耳", "健保署／醫療費用申報", "exe_S_DATE", cochlearFields), providerSectionSpec("maindata02", "人工電子耳", "健康存摺／院所提供", "exe_S_DATE", cochlearFields), providerSectionSpec("maindata03", "人工電子耳自行記錄", "健康存摺／自行記錄", "exe_S_DATE", cochlearFields)))
	eRecent := providerSectionSpec("main_data", "電子處方箋", "健康存摺／電子處方箋近期", "func_DATE", providerPrescriptionFields)
	eRecent.detail = true
	eHistory := eRecent
	eHistory.root, eHistory.source = "hist_list", "健康存摺／電子處方箋歷史"
	add("e_prescriptions", "電子處方箋與續領", "用藥與醫材", sourceCoverage+"同時讀取近期與歷史區塊；已領藥與建議領藥期間分開，處方或領藥不證明實際服用。", get("/api/ihke3000/IHKE3212S01/page_load", eRecent, eHistory))
	paidMedicine := providerSectionSpec("sp_IHKE3220S01_data", "自費藥品", "健康存摺／院所上傳自費藥品", "real_INSPECT_DATE", providerSelfPaidFields)
	paidMedicine.titleField, paidMedicine.detail = "assay_ITEM_NAME", true
	add("self_paid_medicine", "院所自費藥品", "用藥與醫材", sourceCoverage+"不是自行記錄的補充品或已服用紀錄。", get("/api/ihke3000/IHKE3220s01/getItem", paidMedicine))
	paidMaterial := providerSectionSpec("sp_IHKE3221S01_data", "自費醫材", "健康存摺／院所上傳自費醫材", "real_INSPECT_DATE", []fieldSpec{{"real_INSPECT_DATE", "執行日期（原文）", ""}, {"order_CODE", "特殊材碼", ""}, {"assay_ITEM_NAME", "醫材名稱", ""}, {"assay_VALUE", "醫材型號（原文）", ""}, {"sh_ORDER_CNAME", "自編中文名稱", ""}, {"sh_ORDER_ENAME", "自編英文名稱", ""}})
	paidMaterial.titleField, paidMaterial.detail = "assay_ITEM_NAME", true
	add("self_paid_material", "院所自費醫材", "用藥與醫材", sourceCoverage, get("/api/ihke3000/IHKE3221s01/getItem", paidMaterial))
	plan := providerSectionSpec("myplan", "照護計畫收案", "健康存摺／照護計畫", "case_date", []fieldSpec{{"mhbt_name", "計畫名稱", ""}, {"mhbt_memo", "來源計畫說明", ""}, {"case_date", "收案日期（原文）", ""}, {"close_date", "結案日期（原文）", ""}})
	plan.titleField, plan.end = "mhbt_name", "close_date"
	add("care_plans", "參與的照護計畫", "照護與追蹤", sourceCoverage, get("/api/ihke3000/IHKE3213S01/page_load", plan))
	screenFields := []fieldSpec{{"func_DATE", "篩檢日期（原文）", ""}, {"code_CNAME", "篩檢結果（原文）", ""}, {"code_ENAME", "來源結果代碼", ""}, {"cytopathic_CODE", "來源檢驗分類", ""}}
	for _, row := range []struct{ id, title, path, root string }{
		{"screen_colorectal", "大腸癌篩檢", "/api/ihke3000/ihke3404s02/SP_IHKE3404S02", "sP_IHKE3404S02_Data"},
		{"screen_oral", "口腔癌篩檢", "/api/ihke3000/ihke3404s03/SP_IHKE3404S03", "sP_IHKE3404S03_Data"},
		{"screen_breast", "乳癌篩檢", "/api/ihke3000/ihke3404s04/page_load", "sP_IHKE3404S04_Data"},
		{"screen_cervical", "子宮頸癌篩檢", "/api/ihke3000/ihke3404S05/SP_IHKE3404S05", "sP_IHKE3404S05_Data"},
		{"screen_lung", "肺癌篩檢", "/api/ihke3000/ihke3404s06/page_load", "sp_IHKE3404S06_Data"},
		{"screen_hpv", "婦女 HPV 檢測", "/api/ihke3000/ihke3404s07/page_load", "sp_IHKE3404S07_Data"},
		{"screen_hpsa", "胃幽門螺旋桿菌糞便抗原檢測", "/api/ihke3000/ihke3404s08/page_load", "sp_IHKE3404S08_Data"},
	} {
		add(row.id, row.title, "預防與篩檢", "官方頁面通常限最近三次篩檢；保留實際回傳紀錄，不冒充全年度或診斷。", get(row.path, providerSectionSpec(row.root, row.title, "健康存摺／國民健康署篩檢來源", "func_DATE", screenFields)))
	}
	kinmen := providerSectionSpec("sP_IHKE3405S01_Data", "金門健康檢查", "健康存摺／金門健康檢查", "", append(append([]fieldSpec{}, providerResultFields...), fieldSpec{"order_CNAME", "檢查項目", ""}, fieldSpec{"group_NAME", "來源分類", ""}))
	add("kinmen", "金門健康檢查", "預防與篩檢", "來源頁面標示最近一次健康檢查，是否提供取決於官方授權與帳號；未提供個別檢查日期時不推造。", get("/api/ihke3000/ihke3405s01/SP_IHKE3405S01", kinmen))
	glucose := providerSectionSpec("sP_IHKE3406S01_Data", "糖化血色素（HbA1c）", "健康存摺／血糖檢驗專區", "real_INSPECT_DATE", providerResultFields)
	add("glucose", "血糖專區 HbA1c 歷史", "檢驗與量測", "已對官方 HbA1c 搜尋送出所選完整期間；無獨立總數，不宣稱終身資料完整。不把專區名稱誤當包含所有血糖項目。", get("/api/ihke3000/ihke3406s01/SP_IHKE3406S01", glucose))
	lipid := providerSectionSpec("RecordList", "血脂檢驗", "健康存摺／血脂專區", "REAL_INSPECT_DATE", append(append([]fieldSpec{}, providerResultFields...), fieldSpec{"CURE_CNAME", "檢查項目", ""}, fieldSpec{"YN", "來源旗標（原文）", ""}))
	lipid.titleField, lipid.detail = "CURE_CNAME", true
	add("lipids", "血脂專區歷史", "檢驗與量測", "清單是來源專區項目與摘要；各項明細才送出所選完整期間。來源可能重疊，不強制合併檢驗或推定方法相容。", get("/api/ihke3000/ihke3407s01/SP_IHKE3407S01", lipid))
	physioFields := []fieldSpec{{"code", "量測項目代碼", ""}, {"code_NAME", "量測項目", ""}, {"create_DATE", "量測日期（原文）", ""}, {"value", "來源量測值（原文）", ""}, {"unit_DATA", "單位（原文）", ""}, {"abnormal_Status", "來源旗標（原文）", ""}}
	physio := providerSectionSpec("maindata", "生理量測項目", "健康存摺／生理量測（可含自行記錄）", "create_DATE", physioFields)
	physio.titleField, physio.detail = "code_NAME", true
	pressure := providerSectionSpec("maindata_10568", "血壓與心率項目", "健康存摺／生理量測（可含自行記錄）", "", physioFields)
	add("physiology", "生理量測歷史", "檢驗與量測", "清單是來源項目與最近摘要；各項明細查詢所選完整期間。自行記錄與行動裝置來源未等同院所檢驗，官方行動資料可能僅近半年。", get("/api/ihke3000/ihke3205s01/page_load", physio, pressure))
	for _, id := range []string{"glucose", "lipids", "physiology"} {
		spec := items[id]
		spec.info.RangeMode = "date_search"
		items[id] = spec
	}
	covidSections := []providerSection{}
	for _, section := range []struct{ root, title, date string }{
		{"sp_IHKE3410S01", "COVID-19 檢驗", "real_INSPECT_DATE"}, {"sp_IHKE3410S02", "COVID-19 快篩來源", "real_INSPECT_DATE"}, {"sp_IHKE3410S04", "COVID-19 其他來源", "real_INSPECT_DATE"},
		{"sp_IHKE3410S05", "COVID-19 接種", "inoculate_D"}, {"sp_IHKE3410S09", "COVID-19 採檢與確認", "sample_DATE"},
	} {
		fields := append(append([]fieldSpec{}, providerResultFields...), fieldSpec{"code_cname", "疫苗名稱", ""}, fieldSpec{"inoculate_D", "接種日期（原文）", ""}, fieldSpec{"sample_DATE", "採檢日期（原文）", ""}, fieldSpec{"confirm_DATE", "確認日期（原文）", ""}, fieldSpec{"assay_METHOD", "檢測方法（原文）", ""}, fieldSpec{"inspect_RESULT", "文字結果（原文）", ""}, fieldSpec{"lab_UNIT_NAME", "檢驗單位名稱", ""})
		covidSections = append(covidSections, providerSectionSpec(section.root, section.title, "健康存摺／COVID-19 專區", section.date, fields))
	}
	add("covid", "COVID-19 檢驗與接種", "預防與篩檢", sourceCoverage+"不呼叫重整或產生證明操作；不轉出個人 QR 識別。", get("/api/ihke3000/IHKE3410S01/page_load", covidSections...))
	for _, care := range []struct{ id, label, path, root string }{
		{"diabetes_care", "糖尿病照護追蹤", "/api/ihke3000/ihke3504s01/SP_IHKE3504S01", "sP_IHKE3504S01_Data"},
		{"ckd_care", "初期慢性腎病追蹤", "/api/ihke3000/ihke3505s01/SP_IHKE3505S01", "sP_IHKE3505S01_Data"},
		{"hepatitis_care", "B／C 肝炎追蹤", "/api/ihke3000/ihke3507s01/SP_IHKE3507S01", "sP_IHKE3507S01_Data"},
		{"copd_care", "肺阻塞照護追蹤", "/api/ihke3000/IHKE3513S01/page_load", "main_Data"},
	} {
		section := providerSectionSpec(care.root, care.label, "健康存摺／官方照護方案", "case_DATE", providerCareFields)
		section.end, section.detail = "close_DATE", true
		add(care.id, care.label, "照護與追蹤", sourceCoverage+"另有收案項目明細；照護階段／收案不視為 Healthbank 診斷。", get(care.path, section))
	}
	add("hepatitis_test", "C 型肝炎抗體檢測", "照護與追蹤", sourceCoverage, get("/api/ihke3000/ihke3514s01/page_load", providerSectionSpec("maindata", "C 型肝炎抗體", "健康存摺／肝炎檢測來源", "real_INSPECT_DATE", providerResultFields)))
	home := providerSectionSpec("", "居家醫療收案", "健康存摺／居家醫療照護", "c_CASE_DATE", []fieldSpec{{"c_CASE_DATE", "收案日期（原文）", ""}, {"c_CLOSE_DATE", "結案日期（原文）", ""}, {"c_PHASE", "照護階段（原文）", ""}, {"c_MAIN_PRSN_HOSP_ABBR", "主治醫師所屬院所", ""}, {"c_NURSE_HOSP_ABBR", "護理師所屬院所", ""}, {"c_BRE_PRSN_HOSP_ABBR", "呼吸治療師所屬院所", ""}})
	home.end, home.institution = "c_CLOSE_DATE", "c_HOSP_ABBR"
	add("home_care", "居家醫療收案", "照護與追蹤", "官方頁面標示近三年個案管理追蹤；無日期搜尋或獨立總數。", get("/api/ihke3000/IHKE3712S01/page_load", home))
	obesity := providerSectionSpec("maindata", "身體量測專區摘要", "健康存摺／多來源身體量測", "c_REAL_INSPECT_DATE", []fieldSpec{{"c_CURE_CNAME", "檢查項目", ""}, {"c_ASSAY_VALUE", "結果（原文）", ""}, {"c_UNIT_DATA", "單位（原文）", ""}, {"c_CONSULT_VALUE", "來源參考值", ""}, {"c_ORI_TYPE_NAME", "來源類型（原文）", ""}, {"c_REAL_INSPECT_DATE", "檢查日期（原文）", ""}})
	add("body_measurements", "身體量測來源摘要", "檢驗與量測", "官方肥胖防治專區的最近結果摘要；保留來源區別，未複製官方風險計算或推定診斷。歷史生理資料另由生理量測取得。", get("/api/ihke3000/IHKE3512S01/page_load", obesity))
	familyStatus := providerSectionSpec("sP_IHKE3720S01", "家庭醫師計畫狀態", "健康存摺／家庭醫師計畫", "", []fieldSpec{{"status", "來源計畫狀態代碼", ""}, {"curr_YEAR", "來源收案年度", ""}})
	familyStatus.object = true
	familyProvider := providerSectionSpec("sP_IHKE3720S02", "家庭醫師收案院所", "健康存摺／家庭醫師計畫", "", []fieldSpec{{"hosp_ABBR", "收案院所", ""}, {"prsn_NAME", "家庭醫師姓名", ""}, {"tel", "院所聯絡電話", ""}})
	familyProvider.object = true
	add("family_doctor", "家庭醫師計畫收案", "照護與追蹤", sourceCoverage+"只讀取既有收案狀態及院所，不變更加入或同意設定。", get("/api/ihke3000/ihke3720s01/SP_IHKE3720S01", familyStatus, familyProvider))
	followup := providerSectionSpec("sP_IHKE3721S02", "家庭醫師追蹤項目", "健康存摺／家庭醫師追蹤來源", "", []fieldSpec{{"assay_NAME", "來源檢驗名稱", ""}, {"assay_ZH_NAME", "來源中文名稱", ""}, {"next_CHK_DATE", "來源建議下次檢查日期（非預約）", ""}, {"chk_memo", "來源追蹤註記", ""}})
	followup.titleField, followup.detail = "assay_ZH_NAME", true
	add("family_followup", "家庭醫師追蹤檢驗", "照護與追蹤", "清單讀取來源追蹤項目，各項明細搜尋所選完整期間；來源建議與分類不是 Healthbank 自行提出的醫療建議。無獨立總數可核對。", get("/api/ihke3000/ihke3721s01/SP_IHKE3721S01", followup))
	followupSpec := items["family_followup"]
	followupSpec.info.RangeMode = "date_search"
	items["family_followup"] = followupSpec
	diet := providerSectionSpec("dietRecordList", "既有飲食紀錄", "健康存摺／使用者自行記錄", "eatDate", []fieldSpec{{"eatDate", "記錄日期（原文）", ""}, {"foodName", "來源食物名稱", ""}, {"totalCal", "來源熱量估值（未重新計算）", ""}})
	add("diet", "既有飲食紀錄", "自行記錄來源", sourceCoverage+"只讀取已存在的自行紀錄，不要求打卡或新增官方資料。", providerRequest{path: "/api/ihke3000/ihke3206s01/IHKE3206S01", method: http.MethodPost, sections: []providerSection{diet}})
	exercise := providerSectionSpec("sportsRecordList", "既有運動紀錄", "健康存摺／使用者自行記錄", "SPORT_DATE", []fieldSpec{{"SPORT_DATE", "記錄日期（原文）", ""}, {"CODE_NAME", "來源運動項目", ""}, {"TOT_CAL", "來源消耗熱量估值（未重新計算）", ""}})
	exercise.detail = true
	add("exercise", "既有運動紀錄", "自行記錄來源", sourceCoverage+"只讀取既有紀錄與明細，不新增打卡或自行推定熱量。", providerRequest{path: "/api/ihke3000/ihke3207s02/page_load", method: http.MethodPost, sections: []providerSection{exercise}})
	menstrual := providerSectionSpec("sP_IHKE3208S06", "既有生理期紀錄", "健康存摺／使用者自行記錄", "sdate", []fieldSpec{{"sdate", "來源開始日期", ""}, {"edate", "來源結束日期", ""}, {"diffDay", "來源記錄日數", "日"}})
	menstrual.end = "edate"
	add("menstrual", "既有生理期紀錄", "自行記錄來源", sourceCoverage+"只保留來源清單的已記錄期間，不預測生理期、排卵或受孕。", get("/api/ihke3000/ihke3208s01/SP_IHKE3208S06", menstrual))
	bone := providerSectionSpec("sp_IHKE3411S01_data", "骨質疏鬆檢查", "健康存摺／骨質疏鬆檢查來源", "real_INSPECT_DATE", providerResultFields)
	bone.detail = true
	add("osteoporosis", "骨質疏鬆檢查", "檢驗與量測", "已向來源要求全部類型清單；讀取報告文字明細，不自行判定診斷。無獨立總數與全年度保證。", providerRequest{path: "/api/ihke3000/ihke3411s01/page_load", params: url.Values{"s_type": {""}, "s_sort": {"A1"}}, sections: []providerSection{bone}})
	extendProviderSpecs(items)
	return items
}

func providerDetailRequest(category string, ref providerReference) (providerRequest, error) {
	get := func(path string, params url.Values, sections ...providerSection) providerRequest {
		return providerRequest{path: path, params: params, sections: sections}
	}
	post := func(path string, payload map[string]string, sections ...providerSection) providerRequest {
		return providerRequest{path: path, method: http.MethodPost, payload: payload, sections: sections}
	}
	switch category {
	case "e_prescriptions":
		header := providerSectionSpec("treatData", "電子處方箋明細", "健康存摺／電子處方箋", "func_DATE", providerPrescriptionFields)
		header.object = true
		drugs := providerSectionSpec("list", "處方藥品", "健康存摺／電子處方醫囑（非服用紀錄）", "@parent", providerPrescriptionDrugFields)
		return get("/api/ihke3000/IHKE3212S02/page_load", url.Values{"tid": {ref.rowID}, "recipeType": {ref.sourceType}}, header, drugs), nil
	case "self_paid_medicine", "self_paid_material":
		spec := providerSpecs[category]
		return get(spec.requests[0].path, url.Values{"rid": {ref.rowID}}, spec.requests[0].sections...), nil
	case "diabetes_care":
		fields := []fieldSpec{{"stage", "來源追蹤階段", ""}, {"case_DATE", "收案／就醫日期（原文）", ""}, {"blood_CHK_DATE", "抽血日期（原文）", ""}, {"urine_CHK_DATE", "驗尿日期（原文）", ""}, {"base_SBP", "收縮壓", "mmHg"}, {"base_EBP", "舒張壓", "mmHg"}, {"blood_HBA1C", "糖化血色素", "%"}, {"blood_LDL", "低密度脂蛋白", "mg/dL"}, {"blood_AC", "飯前血糖", "mg/dL"}, {"blood_TG", "三酸甘油脂", "mg/dL"}, {"blood_CREAT", "肌酸酐", "mg/dL"}, {"blood_ALB", "來源腎絲球率值（來源欄名 blood_ALB）", "mL/min/1.73m²"}, {"urine_MICRO", "尿液微量白蛋白（沿用官網標示）", "mg/g"}}
		return post("/api/ihke3000/ihke3504s02/SP_IHKE3504S02", map[string]string{"rid": ref.rowID}, providerSectionSpec("sP_IHKE3504S02_Data", "糖尿病追蹤結果", "健康存摺／糖尿病照護追蹤", "case_DATE", fields)), nil
	case "ckd_care":
		fields := []fieldSpec{{"stage", "來源追蹤階段", ""}, {"case_DATE", "收案／就醫日期（原文）", ""}, {"base_SBP", "收縮壓", "mmHg"}, {"base_EBP", "舒張壓", "mmHg"}, {"egfr", "來源 eGFR", "mL/min/1.73m²"}, {"upcr", "尿蛋白／肌酸酐比值", "mg/g"}, {"serum_CREAT", "血清肌酸酐", "mg/dL"}, {"blood_LDL", "低密度脂蛋白", "mg/dL"}, {"blood_HBA1C", "糖化血色素", "%"}, {"uacr", "微量白蛋白比值（原文）", "mg/g"}}
		return post("/api/ihke3000/ihke3505s02/SP_IHKE3505S02", map[string]string{"rid": ref.rowID}, providerSectionSpec("sP_IHKE3505S02_Data", "初期慢性腎病追蹤結果", "健康存摺／腎病照護追蹤", "case_DATE", fields)), nil
	case "hepatitis_care":
		fields := []fieldSpec{{"stage", "來源追蹤階段", ""}, {"case_DATE", "收案／就醫日期（原文）", ""}, {"case_TYPE_NAME", "來源肝炎類型", ""}, {"hbsag_NAME", "HBsAg（原文）", ""}, {"hbeag_NAME", "HBeAg（原文）", ""}, {"antiHCV", "Anti-HCV（原文）", ""}, {"got", "GOT（原文）", ""}, {"gpt", "GPT（原文）", ""}, {"au_SCAN_NAME", "腹部超音波結果（原文）", ""}}
		return post("/api/ihke3000/ihke3507s02/SP_IHKE3507S02", map[string]string{"rid": ref.rowID}, providerSectionSpec("sP_IHKE3507S02_Data", "B／C 肝炎追蹤結果", "健康存摺／肝炎照護追蹤", "case_DATE", fields)), nil
	case "copd_care":
		fields := []fieldSpec{{"c_STAGE", "來源追蹤階段", ""}, {"c_CASE_DATE", "收案／就醫日期（原文）", ""}, {"c_LUNG_DATE", "肺功能檢查日期（原文）", ""}, {"c_FEV1_POST", "FEV1（來源一秒量）", ""}, {"c_FEV1_VAL_POST", "FEV1 預測值", "%"}, {"c_FF_VAL_POST", "FEV1/FVC 實測比值（原文）", ""}, {"c_CAT_DATE", "CAT 評估日期（原文）", ""}, {"c_CAT", "CAT 分數", "分"}, {"c_MMRC_DATE", "mMRC 評估日期（原文）", ""}, {"c_MMRC", "mMRC 分數", "分"}}
		return post("/api/ihke3000/ihke3513s02/page_load", map[string]string{"rid": ref.rowID}, providerSectionSpec("sp_IHKE3513S02_Data", "肺阻塞追蹤結果", "健康存摺／肺阻塞照護追蹤", "c_CASE_DATE", fields)), nil
	case "glucose", "lipids":
		path := "/api/ihke3000/ihke3407s02/get_point"
		if category == "glucose" {
			path = "/api/ihke3000/ihke3406s02/SP_IHKE3406S02"
		}
		fields := append(append([]fieldSpec{}, providerResultFields...), fieldSpec{"assay_VALUE", "來源值（未自動套用摘要單位）", ""})
		return post(path, map[string]string{"dCode": ref.rowID, "dStr": ref.query.StartDate, "dEnd": ref.query.EndDate}, providerSectionSpec("sP_IHKE3406S02_Data", ref.title, "健康存摺／"+providerSpecs[category].info.Label, "real_INSPECT_DATE", fields)), nil
	case "family_followup":
		fields := []fieldSpec{{"assay_DATE", "檢驗日期（原文）", ""}, {"assay_NAME", "來源檢驗名稱", ""}, {"assay_VALUE", "結果（原文）", ""}, {"assay_SOURCE_NAME", "來源類型（原文）", ""}, {"unit_DATA", "單位（原文）", ""}}
		return post("/api/ihke3000/ihke3721s01/SP_IHKE3721S07", map[string]string{"dType": ref.rowID, "dStr": ref.query.StartDate, "dEnd": ref.query.EndDate}, providerSectionSpec("sP_IHKE3721S07_Data", ref.title, "健康存摺／家庭醫師追蹤檢驗", "assay_DATE", fields)), nil
	case "exercise":
		fields := []fieldSpec{{"CODE", "來源運動項目代碼", ""}, {"CODE_LEVEL", "來源強度代碼（原文）", ""}, {"TOT_HR", "來源時數", "小時"}, {"TOT_MIN", "來源分鐘數", "分鐘"}}
		return post("/api/ihke3000/ihke3207s02/SP_IHKE3207S04", map[string]string{"c_rowid": ref.rowID}, providerSectionSpec("oneRecordList", ref.title, "健康存摺／使用者自行記錄", "@parent", fields)), nil
	case "physiology":
		if ref.kind == "device_day" {
			fields := []fieldSpec{{"cdate", "來源量測日期／時間（原文）", ""}, {"value", "行動裝置量測值（原文）", ""}, {"code", "來源量測項目代碼", ""}, {"unit_DATA", "單位（原文）", ""}}
			return get("/api/ihke3000/ihke3205s04/pageload", url.Values{"code": {ref.rowID}, "cal_date": {ref.sourceType}}, providerSectionSpec("devicelist", ref.title+"行動裝置原始紀錄", "健康存摺／行動裝置資料來源", "cdate", fields)), nil
		}
		code1, code2, code3 := ref.rowID, "", ""
		if code1 == "10568" {
			code1, code2, code3 = "105", "106", "108"
		}
		if code1 == "103" {
			code1, code2 = "101", "102"
		}
		fields := []fieldSpec{{"create_DATE", "量測日期（原文）", ""}, {"time", "時段（原文）", ""}}
		for _, item := range []struct{ key, label string }{{"value1", "來源數值 1"}, {"value2", "來源數值 2"}, {"value3", "來源數值 3"}, {"value4", "來源數值 4"}, {"value5", "來源數值 5"}, {"value6", "來源數值 6"}, {"value7", "來源數值 7"}, {"value8", "來源數值 8"}, {"value9", "來源數值 9"}} {
			fields = append(fields, fieldSpec{item.key, item.label + "（保留來源通道）", ""})
		}
		payload := map[string]string{"cSDate": strings.ReplaceAll(ref.query.StartDate, "-", ""), "cEDate": strings.ReplaceAll(ref.query.EndDate, "-", ""), "cCode1": code1, "cCode2": code2, "cCode3": code3}
		return post("/api/ihke3000/ihke3205s02/SP_IHKE3205S08", payload, providerSectionSpec("sP_IHKE3205S09", ref.title, "健康存摺／生理量測（未推定院所驗證）", "create_DATE", fields)), nil
	case "osteoporosis":
		fields := append(append([]fieldSpec{}, providerResultFields...), fieldSpec{"main_tit", "來源報告標題", ""}, fieldSpec{"check_val", "檢查值（原文）", ""}, fieldSpec{"desc", "報告文字（原文）", ""})
		return get("/api/ihke3000/IHKE3411S02/page_load", url.Values{"crid": {ref.rowID}, "ctype": {ref.sourceType}}, providerSectionSpec("ihke3411S02_main_data", "骨質疏鬆檢查明細", "健康存摺／骨質疏鬆報告", "", fields)), nil
	}
	return providerExtraDetailRequest(category, ref)
}
