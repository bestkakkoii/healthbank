// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"net/http"
	"sort"
)

type careChartSelector struct {
	value, title, first, second string
}

type careChartSpec struct {
	path, root string
	selectors  []careChartSelector
}

// The source chart endpoints are retained separately from their corresponding
// result tables. Matching column names in public JavaScript cannot establish
// identical unseen server rowsets. The selectors below are public constants;
// the private case identifier comes only from this session's successful list.
var providerCareCharts = map[string]careChartSpec{
	"diabetes_care": {
		path: "/api/ihke3000/ihke3504s03/SP_IHKE3504S03", root: "sP_IHKE3504S03_Data",
		selectors: []careChartSelector{
			{"03", "血壓", "base_SBP", "base_EBP"}, {"04", "糖化血色素", "blood_HBA1C", ""},
			{"05", "低密度脂蛋白", "blood_LDL", ""}, {"06", "飯前血糖", "blood_AC", ""},
			{"07", "肌酸酐", "blood_CREAT", ""}, {"08", "來源腎絲球率通道（blood_ALB）", "blood_ALB", ""},
			{"09", "三酸甘油脂", "blood_TG", ""}, {"10", "來源尿液微量白蛋白通道", "urine_MICRO", ""},
		},
	},
	"ckd_care": {
		path: "/api/ihke3000/ihke3505s03/SP_IHKE3505S03", root: "sP_IHKE3505S03_Data",
		selectors: []careChartSelector{
			{"03", "血壓", "base_SBP", "base_EBP"}, {"04", "來源 eGFR", "egfr", ""},
			{"05", "尿蛋白／肌酸酐比值", "upcr", ""}, {"06", "血清肌酸酐", "serum_CREAT", ""},
			{"07", "低密度脂蛋白", "blood_LDL", ""}, {"08", "糖化血色素", "blood_HBA1C", ""},
			{"09", "來源微量白蛋白比值通道", "uacr", ""},
		},
	},
	"hepatitis_care": {
		path: "/api/ihke3000/ihke3507s03/SP_IHKE3507S03", root: "sP_IHKE3507S03_Data",
		selectors: []careChartSelector{{"03", "GOT", "got", ""}, {"04", "GPT", "gpt", ""}},
	},
	"copd_care": {
		path: "/api/ihke3000/ihke3513s02/get_highchar_val", root: "sp_IHKE3513S03_Data",
		selectors: []careChartSelector{
			{"03", "來源 FEV1 一秒量", "c_FEV1_POST", ""}, {"04", "來源 FEV1 預測值百分比", "c_FEV1_VAL_POST", ""},
			{"05", "來源 FEV1/FVC 實測比值", "c_FF_VAL_POST", ""}, {"06", "來源 CAT 分數", "c_CAT", ""},
			{"07", "來源 mMRC 分數", "c_MMRC", ""},
		},
	},
	"diabetes_ckd_care": {
		path: "/api/ihke3000/ihke3516s03/SP_IHKE3516S03", root: "sP_IHKE3516S03_Data",
		selectors: []careChartSelector{
			{"03", "血壓", "base_SBP", "base_EBP"}, {"04", "糖化血色素", "blood_HBA1C", ""},
			{"05", "低密度脂蛋白", "blood_LDL", ""}, {"06", "飯前血糖", "blood_AC", ""},
			{"07", "飯後血糖", "blood_PC", ""}, {"08", "血清肌酸酐", "serum_CREAT", ""},
			{"09", "來源 eGFR", "egfr", ""}, {"10", "來源尿液微量白蛋白通道", "uacr", ""},
			{"11", "來源糖化白蛋白通道", "blood_GA", ""},
		},
	},
	"metabolic_care": {
		path: "/api/ihke3000/ihke3517s03/SP_IHKE3517S03", root: "sP_IHKE3517S03_Data",
		selectors: []careChartSelector{
			{"01", "來源 BMI", "bmi", ""}, {"02", "腰圍", "waist", ""},
			{"03", "血壓", "base_SBP", "base_EBP"}, {"04", "糖化血色素", "blood_HBA1C", ""},
			{"05", "高密度脂蛋白", "blood_HDL", ""}, {"06", "低密度脂蛋白", "blood_LDL", ""},
			{"07", "飯前血糖", "blood_AC", ""}, {"08", "三酸甘油脂", "blood_TG", ""},
			{"09", "總膽固醇", "blood_CHOL_T", ""},
		},
	},
}

func providerCareChartRequest(category string, ref providerReference) (providerRequest, error) {
	spec, ok := providerCareCharts[category]
	if !ok || ref.kind != "care_chart" || !usableDocumentID(ref.rowID) {
		return providerRequest{}, failure("INVALID_INPUT", "來源追蹤圖表識別已失效，請重新取得明細。")
	}
	for _, selector := range spec.selectors {
		if selector.value != ref.sourceType {
			continue
		}
		fields := []fieldSpec{{"casE_DATE", "來源圖表日期（原文）", ""}, {"valuE1", "來源第一數值（原文）", ""}}
		if category != "copd_care" {
			fields = append(fields, fieldSpec{"valuE2", "來源第二數值（原文）", ""})
		}
		section := providerSectionSpec(spec.root, selector.title+"／來源圖表資料", "健康存摺／照護追蹤來源圖表（保留原值，未重新換算）", "casE_DATE", fields)
		return providerRequest{path: spec.path, method: http.MethodPost, payload: map[string]string{"ItemValue": selector.value, "rid": ref.rowID}, sections: []providerSection{section}}, nil
	}
	return providerRequest{}, failure("INVALID_INPUT", "來源追蹤圖表項目不在已核對範圍。")
}

func appendProviderCareChartIndices(report *SupplementReport, category, reference string, ref providerReference) (map[string]providerReference, error) {
	spec, ok := providerCareCharts[category]
	if !ok || ref.kind == "care_chart" {
		return nil, nil
	}
	if !usableDocumentID(ref.rowID) {
		return nil, changedError()
	}
	refs := make(map[string]providerReference, len(spec.selectors))
	for index, selector := range spec.selectors {
		id, err := documentToken()
		if err != nil {
			return nil, err
		}
		refs[id] = providerReference{rowID: ref.rowID, sourceType: selector.value, kind: "care_chart", title: selector.title, originKey: ref.originKey, parentReference: reference}
		fields := []SupplementField{{Name: "來源圖表第一通道", Value: selector.first}, {Name: "資料範圍", Value: "此來源圖表另外取得原始日期與數值，不假定與追蹤明細表完全相同。來源未提供日期搜尋或獨立總數。"}}
		if selector.second != "" {
			fields = append(fields, SupplementField{Name: "來源圖表第二通道", Value: selector.second})
		}
		report.Entries = append(report.Entries, SupplementEntry{Key: medicalKey(category, ref.rowID+"\x00"+ref.originKey, "care_chart_index", index), Kind: "source_index", Origin: "official_summary", ContractID: sourceContractID(category, "care_chart_index"), Title: selector.title + "／來源圖表入口", Source: "健康存摺／照護追蹤來源圖表", Fields: fields, DetailID: id})
		if len(report.Entries) > MaxReportRecords {
			return nil, failure("RESPONSE_TOO_LARGE", "來源追蹤明細與圖表入口超過上限，未回傳不完整資料。")
		}
	}
	return refs, nil
}

// These chart children are leaves. Refresh only the current parent's children,
// without scanning/copying all other case histories for every detail request.
// Reserve the complete replacement before retiring previously usable handles.
func (s *Session) commitProviderCareChartReferences(category, reference string, parent providerReference, added map[string]providerReference) error {
	if added == nil {
		return nil
	}
	current := s.providerReferences[category]
	old, exists := current[reference]
	if !exists {
		return failure("INVALID_INPUT", "來源追蹤識別已失效，請重新取得清單。")
	}
	retired := make([]string, 0, len(old.children))
	s.initializeReferenceAccounting()
	next := s.referenceGroups["provider/"+category] - providerReferenceSize(old)
	for _, id := range old.children {
		child, ok := current[id]
		if ok && child.parentReference == reference && child.kind == "care_chart" {
			retired = append(retired, id)
			next -= referenceEntryOverhead + int64(len(id)) + providerReferenceSize(child)
		}
	}
	if len(current)-len(retired)+len(added) > MaxReportRecords {
		return referenceLimitError()
	}
	parent.children = make([]string, 0, len(added))
	for id, child := range added {
		if _, collision := current[id]; collision || child.parentReference != reference || child.kind != "care_chart" {
			return changedError()
		}
		parent.children = append(parent.children, id)
		next += referenceEntryOverhead + int64(len(id)) + providerReferenceSize(child)
	}
	sort.Strings(parent.children)
	next += providerReferenceSize(parent)
	if err := s.reserveReferenceGroups(map[string]int64{"provider/" + category: next}); err != nil {
		return err
	}
	for _, id := range retired {
		delete(current, id)
	}
	current[reference] = parent
	for id, child := range added {
		current[id] = child
	}
	return nil
}
