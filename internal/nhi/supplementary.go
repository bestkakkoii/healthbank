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
	"strings"
	"time"
)

// SupplementField preserves a labelled value from the observed public SPA.
// These contracts are taken from the observed public SPA, not guessed routes.
// A returned array is not an independent statement of lifetime completeness.
type SupplementField struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	Unit      string `json:"unit,omitempty"`
	Code      string `json:"code,omitempty"`
	Role      string `json:"role,omitempty"`
	UnitBasis string `json:"unit_basis,omitempty"`
}
type SupplementEntry struct {
	Key             string            `json:"key,omitempty"`
	Kind            string            `json:"kind,omitempty"`
	EndDate         string            `json:"end_date,omitempty"`
	DateRole        string            `json:"date_role,omitempty"`
	EndDateRole     string            `json:"end_date_role,omitempty"`
	Origin          string            `json:"origin,omitempty"`
	ContractID      string            `json:"contract_id,omitempty"`
	Medicine        *MedicineOrder    `json:"medicine,omitempty"`
	Date            string            `json:"date"`
	Institution     string            `json:"institution"`
	Title           string            `json:"title"`
	Source          string            `json:"source"`
	Fields          []SupplementField `json:"fields"`
	DetailID        string            `json:"detail_id,omitempty"`
	Attachments     []Attachment      `json:"attachments,omitempty"`
	DocumentActions []DocumentAction  `json:"document_actions,omitempty"`
}
type SupplementReport struct {
	Category         string             `json:"category"`
	Entries          []SupplementEntry  `json:"entries"`
	Coverage         string             `json:"coverage"`
	FetchedAt        string             `json:"fetched_at"`
	StartDate        string             `json:"start_date,omitempty"`
	EndDate          string             `json:"end_date,omitempty"`
	Complete         bool               `json:"complete"`
	ContractRevision string             `json:"contract_revision,omitempty"`
	Diagnostics      []SourceDiagnostic `json:"diagnostics,omitempty"`
}

const maxSourceTextBytes = 65536

// Projected reports share the portable dataset's existing structural limits.
// Validate the combined result before replacing live references: individually
// bounded arrays and scalars can otherwise exceed a limit after concatenation.
func validateSupplementReport(report SupplementReport) error {
	withinTextBound := func(values ...string) bool {
		for _, value := range values {
			if len(value) > maxSourceTextBytes {
				return false
			}
		}
		return true
	}
	tooLarge := func() error {
		return failure("RESPONSE_TOO_LARGE", "來源資料合併後超過筆數、欄位或文字長度上限，未回傳截斷資料。")
	}
	if len(report.Entries) > MaxReportRecords || len(report.Diagnostics) > maxSourceDiagnostics || !withinTextBound(report.Category, report.Coverage, report.StartDate, report.EndDate) {
		return tooLarge()
	}
	if err := validateSourceMetadata(report); err != nil {
		return err
	}
	for _, entry := range report.Entries {
		if len(entry.Fields) > MaxReportRecords || len(entry.Attachments) > 500 || !withinTextBound(entry.Key, entry.Kind, entry.Date, entry.EndDate, entry.Institution, entry.Title, entry.Source) {
			return tooLarge()
		}
		for _, field := range entry.Fields {
			if !withinTextBound(field.Name, field.Value, field.Unit) {
				return tooLarge()
			}
		}
		if order := entry.Medicine; order != nil && !withinTextBound(order.Code, order.Name, order.Quantity, order.Days) {
			return tooLarge()
		}
	}
	return nil
}

type westernReference struct{ rowID, sourceType, date, institution string }

type fieldSpec struct{ key, label, unit string }
type supplementSpec struct {
	module, root, title, coverage string
	search                        bool
}

var supplements = map[string]supplementSpec{
	"western":          {"ihke3303s01", "sp_IHKE3303S01_data", "西醫就醫", "已明確送出所選日期並讀取搜尋陣列；官方未提供同期間獨立總數。疾病分類不等於已確認診斷。", true},
	"chinese":          {"ihke3304s01", "sp_IHKE3304S01_data", "中醫就醫", "已明確送出所選日期並讀取搜尋陣列；不代表取得方劑成分與實際用量。", true},
	"dental":           {"ihke3305s01", "sp_IHKE3305S01_data", "牙醫就醫", "已明確送出所選日期並讀取搜尋陣列；未另外核對獨立總數。", true},
	"recent_medicine":  {"ihke3301s03", "main_data", "近期用藥就醫摘要", "官方首頁近期摘要，含中藥區塊（若有）；不是完整處方明細、實際服用或全部歷史。", false},
	"recent_chronic":   {"ihke3301s04", "main_data", "近期慢箋摘要", "官方首頁近期摘要；不是完整慢箋明細或實際服用紀錄。", false},
	"recent_procedure": {"ihke3301s05", "main_data", "近期手術摘要", "官方首頁近期摘要；不是全部手術歷史。", false},
	"recent_hospital":  {"ihke3301s06", "main_data", "近期住院摘要", "官方首頁近期摘要；不是全部住院歷史。", false},
}
var adultFields = []fieldSpec{
	{"height", "身高", "cm"}, {"weight", "體重", "kg"}, {"bmi", "BMI", "kg/m²"}, {"waistline", "腰圍（官網未區分單位）", "吋或公分"},
	{"basE_SBP", "收縮壓", "mmHg"}, {"basE_EBP", "舒張壓", "mmHg"}, {"bloD_PRESS_RESULT_TEXT", "血壓判讀", ""},
	{"cho", "總膽固醇", "mg/dL"}, {"bloD_TG", "三酸甘油脂", "mg/dL"}, {"hdl", "HDL", "mg/dL"}, {"ldl", "LDL", "mg/dL"},
	{"bA_DIAG_RESULT_TEXT", "血脂判讀", ""}, {"s_09005C", "飯前血糖", "mg/dL"}, {"s_09005C_DIAG_RESULT_TEXT", "血糖判讀", ""},
	{"urinE_BUN", "尿素氮（沿用官網顯示單位）", "mmol/L"}, {"bloD_CREAT", "肌酸酐（沿用官網顯示單位）", "µmol/L"}, {"egfr", "eGFR", "mL/min/1.73m²"},
	{"rF_DIAG_RESULT_TEXT", "腎功能判讀", ""}, {"urinE_UA_DIAG_ACID", "尿酸", "mg/dL"}, {"urinE_UA_DIAG_RESULT_TEXT", "尿酸判讀", ""},
	{"urinE_PROTEIN_TEXT", "尿蛋白（原文）", "正負值或mg/dL"}, {"metA_SYNDR_RESULT_TEXT", "代謝症候群判讀", ""},
	{"sgot", "AST", "IU/L"}, {"sgpt", "ALT", "IU/L"}, {"lF_DIAG_RESULT_TEXT", "肝功能判讀", ""},
	{"hbsaG_TEXT", "HBsAg", ""}, {"hbV_RESULT_TEXT", "B型肝炎判讀", ""}, {"antI_HCV_TEXT", "Anti-HCV", ""}, {"hcV_RESULT_TEXT", "C型肝炎判讀", ""},
	{"bloD_PRESS_RESULT", "來源血壓判讀代碼", ""}, {"bA_DIAG_RESULT", "來源血脂判讀代碼", ""}, {"s_09005C_DIAG_RESULT", "來源血糖判讀代碼", ""},
	{"rF_DIAG_RESULT", "來源腎功能判讀代碼", ""}, {"urinE_UA_DIAG_RESULT", "來源尿酸判讀代碼", ""}, {"metA_SYNDR_RESULT", "來源代謝症候群判讀代碼", ""}, {"lF_DIAG_RESULT", "來源肝功能判讀代碼", ""},
	{"hbsag", "來源 HBsAg 原始代碼", ""}, {"hbV_RESULT", "來源 B 型肝炎判讀代碼", ""}, {"antI_HCV", "來源 Anti-HCV 原始代碼", ""}, {"hcV_RESULT", "來源 C 型肝炎判讀代碼", ""}, {"tel", "來源院所電話", ""},
}

type wireRow map[string]json.RawMessage

func canonicalRow(row wireRow) (wireRow, error) {
	out := make(wireRow, len(row))
	for key, value := range row {
		key = strings.ToLower(key)
		if _, ok := out[key]; ok {
			return nil, changedError()
		}
		out[key] = value
	}
	return out, nil
}
func rowText(row wireRow, names ...string) (string, error) {
	for _, name := range names {
		value, ok := row[strings.ToLower(name)]
		if !ok {
			continue
		}
		var text wireText
		if json.Unmarshal(value, &text) != nil || len(text) > maxSourceTextBytes {
			return "", changedError()
		}
		if strings.TrimSpace(string(text)) != "" {
			return string(text), nil
		}
	}
	return "", nil
}
func supplementRows(body []byte, root string, observedNull ...*bool) ([]wireRow, error) {
	content := bytes.TrimSpace(body)
	if len(content) == 0 {
		return nil, changedError()
	}
	if content[0] == '{' {
		var object map[string]json.RawMessage
		if decodeJSON(content, &object) != nil {
			return nil, changedError()
		}
		if err := detectAPIError(object); err != nil {
			return nil, err
		}
		if root == "" {
			return nil, changedError()
		}
		var ok bool
		content, ok = object[root]
		if !ok {
			return nil, changedError()
		}
	}
	if bytes.Equal(bytes.TrimSpace(content), []byte("null")) {
		for _, flag := range observedNull {
			*flag = true
		}
		return []wireRow{}, nil
	}
	var rows []wireRow
	if decodeJSON(content, &rows) != nil || len(rows) > MaxReportRecords {
		return nil, changedError()
	}
	out := make([]wireRow, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			return nil, changedError()
		}
		normalized, err := canonicalRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, normalized)
	}
	return out, nil
}
func appendKnownFields(row wireRow, specs []fieldSpec) ([]SupplementField, error) {
	fields := make([]SupplementField, 0, len(specs))
	for _, spec := range specs {
		code, label := strings.ToLower(spec.key), spec.label
		raw := bytes.TrimSpace(row[code])
		var value string
		var err error
		switch {
		case code == "memo" && len(raw) > 0 && raw[0] == '[':
			var notes []json.RawMessage
			if decodeJSON(raw, &notes) != nil || notes == nil || len(notes) > MaxReportRecords {
				return nil, changedError()
			}
			for _, rawNote := range notes {
				var note string
				part := bytes.TrimSpace(rawNote)
				if len(part) == 0 || part[0] != '"' || json.Unmarshal(part, &note) != nil || len(note) > maxSourceTextBytes {
					return nil, changedError()
				}
				fields = append(fields, SupplementField{Name: label, Value: note, Code: code})
			}
			continue
		case (code == "show_price" || code == "show_pay_price") && (bytes.Equal(raw, []byte("true")) || bytes.Equal(raw, []byte("false"))):
			value = string(raw) // Only these reviewed display flags accept booleans.
		case code == "cure_path" && len(raw) > 0 && raw[0] == '[':
			var parts []wireText
			if decodeJSON(raw, &parts) != nil || parts == nil || len(parts) > MaxReportRecords {
				return nil, changedError()
			}
			value, label = string(raw), spec.label+"（來源陣列 JSON）"
		default:
			value, err = rowText(row, spec.key)
		}
		if err != nil {
			return nil, err
		}
		if value != "" {
			field := SupplementField{Name: label, Value: value, Unit: spec.unit, Code: code}
			field.Role = sourceFieldRole(field.Code)
			if spec.unit != "" {
				field.UnitBasis = "public_contract"
			} else if field.Role == "unit" {
				field.UnitBasis = "source"
			}
			fields = append(fields, field)
		}
	}
	return fields, nil
}
func (s *Session) supplementRequest(ctx context.Context, method, path string, params url.Values, payload []byte, root string, reports ...*SupplementReport) ([]wireRow, error) {
	body, _, err := s.request(ctx, method, path, params, payload, maxJSONBytes, true)
	if err != nil {
		return nil, err
	}
	defer clear(body)
	// A top-level null is not the observed list contract. Optional nested
	// sections may be null, but an entire response must not silently erase a
	// previously displayed list as though the official service returned [].
	if bytes.Equal(bytes.TrimSpace(body), []byte("null")) {
		return nil, s.annotateResponseError(changedError())
	}
	var nullSection bool
	rows, err := supplementRows(body, root, &nullSection)
	if err != nil {
		s.invalidateOnAuthError(err)
		return nil, s.annotateResponseError(err)
	}
	for _, report := range reports {
		var raw []byte
		if nullSection {
			raw = []byte("null")
		}
		if err := sourceSectionDiagnostic(report, report.Category, root, raw, true, false, len(rows)); err != nil {
			return nil, err
		}
	}
	return rows, nil
}
func (s *Session) supplementReady(ctx context.Context) error {
	if s.closed {
		return closedError()
	}
	if s.token == "" {
		return sessionExpired()
	}
	if time.Since(s.tokenObtainedAt) >= 8*time.Minute {
		return s.refresh(ctx)
	}
	return nil
}

func (s *Session) FetchSupplement(ctx context.Context, category string, query Query) (SupplementReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	report := SupplementReport{ContractRevision: ProviderContractRevision, Category: category, Entries: []SupplementEntry{}, Complete: false}
	spec, known := supplements[category]
	if !known && category != "adult" && category != "self_paid" && category != "imaging" {
		return report, failure("INVALID_INPUT", "不支援此資料類別。")
	}
	if err := s.supplementReady(ctx); err != nil {
		return report, err
	}
	if RecentMedicalCategory(category) {
		return s.fetchRecentSupplement(ctx, category)
	}
	var rows []wireRow
	var rowScopes []string
	var err error
	switch category {
	case "adult":
		report.Coverage = "官方頁面標示最近兩次成人預防保健；保留實際回傳紀錄，不宣稱完整歷史。單位沿用該頁文字，未核對前不與其他檢驗強制合併。"
		rows, err = s.supplementRequest(ctx, http.MethodGet, "/api/ihke3000/ihke3402s01/SP_IHKE3402S01", nil, nil, "sP_IHKE3402S01_Data", &report)
	case "self_paid":
		report.Coverage = "自費健檢清單，可能含本人輸入及未完成資料；點開才讀取明細，不代表院所驗證或全部歷史。"
		rows, err = s.supplementRequest(ctx, http.MethodPost, "/api/ihke3000/ihke3403s01/IHKE3403S01", nil, nil, "", &report)
	case "imaging":
		report.Coverage = "影像／病理報告清單；分別保留官方首頁與全部類型搜尋回應，不假設兩次回應相同。未提供日期查詢或獨立總數。報告全文及影像須另行取得，影像產製由使用者明確提出。"
		params := url.Values{"s_type": {""}, "s_sort": {"A1"}}
		path := "/api/ihke3000/ihke3408s01/page_load"
		var object wireRow
		object, err = s.providerRead(ctx, providerRequest{path: path, method: http.MethodGet, params: params})
		if err != nil {
			return report, err
		}
		var present bool
		rows, present, err = providerSectionRows(object, providerSection{root: "sp_IHKE3408S01_data"})
		if err != nil || !present {
			return report, changedError()
		}
		if err := sourceSectionDiagnostic(&report, category, "homepage", object["sp_ihke3408s01_data"], true, false, len(rows)); err != nil {
			return report, err
		}
		for range rows {
			rowScopes = append(rowScopes, "homepage")
		}
		if section, observed := medicalAvailabilitySection(category); observed {
			if err := appendProviderMetadata(&report, object, path, category, section, providerReference{}); err != nil {
				return report, err
			}
		}
		var searched []wireRow
		searched, err = s.supplementRequest(ctx, http.MethodGet, "/api/ihke3000/ihke3408s01/search", params, nil, "", &report)
		if err != nil {
			return report, err
		}
		rows = append(rows, searched...)
		for range searched {
			rowScopes = append(rowScopes, "search")
		}
	default:
		report.Coverage = spec.coverage
		if spec.search {
			var resolved Query
			resolved, _, err = resolveQuery(query, time.Now())
			if err != nil {
				return report, err
			}
			report.StartDate = resolved.StartDate
			report.EndDate = resolved.EndDate
			params := url.Values{"s_date": {resolved.StartDate}, "e_date": {resolved.EndDate}, "s_sort": {"A1"}}
			_, err = s.supplementRequest(ctx, http.MethodGet, "/api/ihke3000/"+spec.module+"/page_load", params, nil, spec.root)
			if err == nil {
				rows, err = s.supplementRequest(ctx, http.MethodGet, "/api/ihke3000/"+spec.module+"/search", params, nil, "", &report)
			}
		} else {
			// Preserve both observed recent-medicine sections, without inventing details.
			body, _, requestErr := s.request(ctx, http.MethodGet, "/api/ihke3000/"+spec.module+"/page_load", nil, nil, maxJSONBytes, true)
			if requestErr != nil {
				return report, requestErr
			}
			var nullMain bool
			rows, err = supplementRows(body, spec.root, &nullMain)
			if err == nil {
				var raw []byte
				if nullMain {
					raw = []byte("null")
				}
				err = sourceSectionDiagnostic(&report, category, spec.root, raw, true, false, len(rows))
			}
			if err == nil && category == "recent_medicine" {
				var object map[string]json.RawMessage
				if decodeJSON(body, &object) == nil {
					if raw, ok := object["chinese_data"]; ok {
						var more []wireRow
						more, err = supplementRows(body, "chinese_data")
						if err == nil {
							err = sourceSectionDiagnostic(&report, category, "chinese_data", raw, true, true, len(more))
						}
						rows = append(rows, more...)
					} else {
						err = sourceSectionDiagnostic(&report, category, "chinese_data", nil, false, true, 0)
					}
				}
			}
			clear(body)
		}
	}
	if err != nil {
		s.invalidateOnAuthError(err)
		return report, s.annotateResponseError(err)
	}
	if len(rows) > MaxReportRecords {
		return report, failure("RESPONSE_TOO_LARGE", "此類別資料過多，未載入不完整清單。")
	}
	references := map[string]string{}
	westernRefs := map[string]westernReference{}
	imagingRefs := map[string]westernReference{}
	for rowIndex, row := range rows {
		entry := SupplementEntry{Title: spec.title, Source: "官方回傳", Fields: []SupplementField{}, ContractID: sourceContractID(category, "list"), Origin: "unknown"}
		if category == "imaging" {
			entry.ContractID = sourceContractID(category, rowScopes[rowIndex])
			entry.Source += "／" + rowScopes[rowIndex]
			entry.Key = medicalKey(category, rowScopes[rowIndex], "list", rowIndex)
		}
		entry.Date, err = rowText(row, "firsT_DIAG_DATE", "real_INSPECT_DATE", "funC_DATE")
		if err != nil {
			return report, err
		}
		if entry.Date != "" {
			entry.DateRole = "unknown" // the legacy selector can choose several distinct source dates
		}
		entry.Institution, err = rowText(row, "hosP_ABBR", "HOSP_NAME")
		if err != nil {
			return report, err
		}
		if category == "adult" {
			entry.Origin = "institutional"
			entry.Title = "成人預防保健"
			entry.Fields, err = appendKnownFields(row, adultFields)
		} else if category == "self_paid" {
			entry.Origin = "mixed"
			entry.Title = "自費健檢"
			var seq string
			seq, err = rowText(row, "DATA02_SEQ")
			if err == nil && seq != "" {
				key := make([]byte, 24)
				if _, err = rand.Read(key); err == nil {
					entry.DetailID = hex.EncodeToString(key)
					references[entry.DetailID] = seq
				}
			}
			if err == nil {
				entry.Fields, err = appendKnownFields(row, []fieldSpec{{"DATA_FLAG", "原始資料狀態（S已存／T暫存）", ""}})
			}
		} else {
			fields := []fieldSpec{{"icd9cm_CODE_CNAME", "疾病分類（非診斷確認）", ""}, {"icd9cm_name", "疾病分類（原文）", ""}, {"icD9CM_CODE", "疾病分類代碼", ""}, {"ori_type_name", "資料來源", ""}, {"order_NAME", "檢查名稱", ""}, {"order_CODE_2Word", "檢查類別", ""}}
			entry.Fields, err = appendKnownFields(row, fields)
			if category == "imaging" {
				entry.Title = "影像／病理報告索引"
				var metadata []SupplementField
				metadata, err = appendKnownFields(row, []fieldSpec{{"jpg_STATUS", "來源 JPEG 產製狀態（原文）", ""}, {"img_ORI_SIZE", "來源影像原始大小顯示值（原文）", ""}})
				entry.Fields = append(entry.Fields, metadata...)
			}
		}
		if err != nil {
			return report, s.annotateResponseError(err)
		}
		// Avoid treating an unrelated object or silent upstream schema change as data.
		if entry.Date == "" && entry.Institution == "" && len(entry.Fields) == 0 {
			return report, changedError()
		}
		if category == "western" || category == "imaging" {
			rowID, idErr := rowText(row, "row_ID")
			sourceType, typeErr := rowText(row, "ori_TYPE")
			if idErr != nil || typeErr != nil {
				return report, changedError()
			}
			if rowID != "" && sourceType != "" {
				key := make([]byte, 24)
				if _, err := rand.Read(key); err != nil {
					return report, failure("INITIALIZATION_FAILED", "無法建立明細識別。")
				}
				entry.DetailID = hex.EncodeToString(key)
				ref := westernReference{rowID, sourceType, entry.Date, entry.Institution}
				if category == "imaging" {
					imagingRefs[entry.DetailID] = ref
				} else {
					westernRefs[entry.DetailID] = ref
				}
			}
		}
		report.Entries = append(report.Entries, entry)
	}
	if err := validateSupplementReport(report); err != nil {
		return report, err
	}
	if category == "self_paid" {
		if err := s.reserveCategoryReferences("self_paid", referenceMapSize(references, selfPaidReferenceSize), category); err != nil {
			return report, err
		}
		s.selfPaidReferences = references
	}
	if category == "western" {
		if err := s.reserveCategoryReferences("western", referenceMapSize(westernRefs, westernReferenceSize), category); err != nil {
			return report, err
		}
		s.westernReferences = westernRefs
	}
	if category == "imaging" {
		if err := s.reserveCategoryReferences("imaging", referenceMapSize(imagingRefs, westernReferenceSize), category); err != nil {
			return report, err
		}
		s.imagingReferences = imagingRefs
	}
	report.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	return report, nil
}

func (s *Session) FetchSelfPaidDetail(ctx context.Context, reference string) (SupplementReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	report := SupplementReport{ContractRevision: ProviderContractRevision, Category: "self_paid_detail", Entries: []SupplementEntry{}, Coverage: "使用者或院所上傳的自費健檢原文；未直接合併為其他檢驗結果。", Complete: false}
	if err := s.supplementReady(ctx); err != nil {
		return report, err
	}
	seq, ok := s.selfPaidReferences[reference]
	if !ok {
		return report, failure("INVALID_INPUT", "明細識別已失效，請重新取得自費健檢清單。")
	}
	payload, err := json.Marshal(map[string]string{"DATA02_SEQ": seq})
	if err != nil {
		return report, failure("REQUEST_FAILED", "無法建立明細請求。")
	}
	defer clear(payload)
	body, _, err := s.request(ctx, http.MethodPost, "/api/ihke3000/ihke3403s05/IHKE3403S05", nil, payload, maxJSONBytes, true)
	if err != nil {
		return report, err
	}
	defer clear(body)
	var object wireRow
	if decodeJSON(body, &object) != nil {
		return report, changedError()
	}
	if err = detectAPIError(object); err != nil {
		s.invalidateOnAuthError(err)
		return report, s.annotateResponseError(err)
	}
	row, err := canonicalRow(object)
	if err != nil {
		return report, err
	}
	returnedSeq, err := rowText(row, "DATA02_SEQ")
	if err != nil || returnedSeq != seq {
		return report, changedError()
	}
	date, err := rowText(row, "REAL_INSPECT_DATE")
	if err != nil {
		return report, err
	}
	institution, err := rowText(row, "HOSP_NAME")
	if err != nil {
		return report, err
	}
	found := false
	resultFields := []fieldSpec{{"ASSAY_VALUE", "結果（原文）", ""}, {"UNIT_DATA", "單位（原文）", ""}, {"CONSULT_VALUE", "參考值（原文）", ""}, {"INSPECT_RESULT", "文字報告", ""}, {"ORDER_CODE", "醫令代碼", ""}, {"LEVEL_01", "來源第一層分類代碼（原文）", ""}, {"LEVEL_02", "來源第二層分類代碼（原文）", ""}}
	for _, section := range []string{"OrdinaryInspection", "NumericalData", "ReportData"} {
		raw, present := object[section]
		if !present {
			if err := sourceSectionDiagnostic(&report, "self_paid", section, nil, false, false, 0); err != nil {
				return report, err
			}
			continue
		}
		found = true
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			if err := sourceSectionDiagnostic(&report, "self_paid", section, raw, true, false, 0); err != nil {
				return report, err
			}
			continue
		}
		var groups map[string]json.RawMessage
		if decodeJSON(raw, &groups) != nil {
			return report, changedError()
		}
		names := make([]string, 0, len(groups))
		for name := range groups {
			names = append(names, name)
		}
		sort.Strings(names)
		rowCount, missingResults := 0, 0
		for _, name := range names {
			items, parseErr := supplementRows(groups[name], "")
			if parseErr != nil {
				return report, parseErr
			}
			for index, item := range items {
				rowCount++
				title, parseErr := rowText(item, "ASSAY_ITEM_NAME")
				if parseErr != nil {
					return report, parseErr
				}
				fields, parseErr := appendKnownFields(item, resultFields)
				if parseErr != nil {
					return report, parseErr
				}
				if title == "" && len(fields) == 0 {
					_, known := item["assay_item_name"]
					for _, field := range resultFields {
						_, present := item[strings.ToLower(field.key)]
						known = known || present
					}
					if !known {
						return report, changedError()
					}
				}
				level, parseErr := rowText(item, "LEVEL_02")
				if parseErr != nil {
					return report, parseErr
				}
				kind := ""
				if section == "NumericalData" && level == "000" {
					kind = "source_heading" // Explicit source heading row, not a missing observation.
				}
				if kind != "source_heading" && sourceResultMissing("self_paid", resultFields, fields) {
					missingResults++
				}
				fields = append(fields, SupplementField{Name: "來源群組（原文）", Value: name})
				dateRole := ""
				if date != "" {
					dateRole = "examination"
				}
				report.Entries = append(report.Entries, SupplementEntry{Key: medicalKey("self_paid", seq, section+"\x00"+name, index), Kind: kind, Date: date, DateRole: dateRole, Origin: "mixed", ContractID: sourceContractID("self_paid", section), Institution: institution, Title: title, Source: "自費健檢／" + section, Fields: fields})
				if len(report.Entries) > MaxReportRecords {
					return report, failure("RESPONSE_TOO_LARGE", "自費健檢明細過多，未載入不完整資料。")
				}
			}
		}
		if err := sourceSectionDiagnostic(&report, "self_paid", section, raw, true, false, rowCount); err != nil {
			return report, err
		}
		if missingResults > 0 {
			if err := addSourceDiagnostic(&report, "known_result_missing", sourceContractID("self_paid", section), &missingResults); err != nil {
				return report, err
			}
		}
	}
	if _, present := row["reportdatadownloadlist"]; present {
		found = true
	}
	if !found {
		return report, changedError()
	}
	documents, err := selfPaidDocuments(row, seq, reference, &report)
	if err != nil {
		return report, err
	}
	if err := validateSupplementReport(report); err != nil {
		return report, err
	}
	if err := s.replaceDocumentReferences("self_paid", reference, documents); err != nil {
		return report, err
	}
	report.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	return report, nil
}

// FetchWesternDetail reads the observed outpatient details, including medication
// orders. It does not infer dose, frequency, adherence, or unobserved drug fields.
func (s *Session) FetchWesternDetail(ctx context.Context, reference string) (SupplementReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	report := SupplementReport{ContractRevision: ProviderContractRevision, Category: "western_detail", Entries: []SupplementEntry{}, Coverage: "這次西醫就醫的官方藥品／非藥品醫囑；不是實際服用紀錄，不推定未提供的劑量或頻次。", Complete: false}
	if err := s.supplementReady(ctx); err != nil {
		return report, err
	}
	ref, ok := s.westernReferences[reference]
	if !ok {
		return report, failure("INVALID_INPUT", "明細識別已失效，請重新取得西醫就醫清單。")
	}
	rows, err := s.supplementRequest(ctx, http.MethodGet, "/api/ihke3000/ihke3303s02/page_load", url.Values{"crid": {ref.rowID}, "ctype": {ref.sourceType}}, nil, "ihke3303S02_main_data")
	if err != nil {
		return report, err
	}
	for _, row := range rows {
		returnedID, readErr := rowText(row, "rowid")
		if readErr != nil || returnedID != ref.rowID {
			return report, changedError()
		}
		for _, section := range []struct {
			key, title string
			fields     []fieldSpec
		}{
			{"sp_IHKE3302S04_data", "藥品醫囑", []fieldSpec{{"order_code", "藥品醫令代碼", ""}, {"drug_name", "藥品名稱（原文）", ""}, {"order_drug_day", "給藥日數", "日"}}},
			{"sp_IHKE3302S05", "非藥品醫囑", []fieldSpec{{"order_code", "醫令代碼", ""}, {"cure_CNAME", "醫令中文名稱", ""}, {"cure_ENAME", "醫令英文名稱", ""}, {"order_qty", "醫囑總量", ""}}},
			{"sp_IHKE3302S07_data", "檢驗檢查醫囑", []fieldSpec{{"order_CODE", "醫令代碼", ""}, {"cure_CNAME", "醫令中文名稱", ""}, {"cure_ENAME", "醫令英文名稱", ""}}},
		} {
			raw, ok := row[strings.ToLower(section.key)]
			if !ok {
				continue
			}
			items, parseErr := supplementRows(raw, "")
			if parseErr != nil {
				return report, parseErr
			}
			for _, item := range items {
				fields, parseErr := appendKnownFields(item, section.fields)
				if parseErr != nil {
					return report, parseErr
				}
				if len(fields) == 0 {
					return report, changedError()
				}
				report.Entries = append(report.Entries, SupplementEntry{Date: ref.date, Institution: ref.institution, Title: section.title, Source: "官方西醫就醫明細", Fields: fields})
				if len(report.Entries) > MaxReportRecords {
					return report, failure("RESPONSE_TOO_LARGE", "明細數量過多，未回傳不完整資料。")
				}
			}
		}
	}
	if err := validateSupplementReport(report); err != nil {
		return report, err
	}
	report.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	return report, nil
}
