// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Attachment contains no upstream URL, file name or patient identifier. Its ID
// is valid only for the current authenticated connector and fetched category.
type Attachment struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size,omitempty"`
}

// DocumentAction is an explicit request to the provider to prepare a temporary
// artifact. Listing or downloading records never implicitly performs it.
type DocumentAction struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type DocumentRequestResult struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

type documentReference struct {
	category, parent, kind, rowID, sourceType, sequence, format, date string
	downloadID                                                        string
	payload                                                           []byte
}

type officialFileReference struct {
	sequence, format, date string
	image                  bool
}

func documentToken() (string, error) {
	key := make([]byte, 24)
	if _, err := rand.Read(key); err != nil {
		return "", failure("INITIALIZATION_FAILED", "無法建立文件識別。")
	}
	return hex.EncodeToString(key), nil
}

func (s *Session) clearDocumentCategory(category string) {
	for key, ref := range s.documentReferences {
		if ref.category == category {
			clear(ref.payload)
			delete(s.documentReferences, key)
		}
	}
}

// Commit new references only after the whole response has been validated.
func (s *Session) replaceDocumentReferences(category, parent string, refs map[string]documentReference) error {
	retained := 0
	for _, ref := range s.documentReferences {
		if ref.category != category || ref.parent != parent {
			retained++
		}
	}
	if retained+len(refs) > MaxReportRecords {
		return failure("RESPONSE_TOO_LARGE", "本次文件識別數量已達上限，請重新取得所需類別。")
	}
	if s.documentReferences == nil {
		s.documentReferences = make(map[string]documentReference)
	}
	for key, ref := range s.documentReferences {
		if ref.category == category && ref.parent == parent {
			clear(ref.payload)
			delete(s.documentReferences, key)
		}
	}
	for key, ref := range refs {
		s.documentReferences[key] = ref
	}
	return nil
}

func addDocumentReference(refs map[string]documentReference, ref documentReference, name, contentType string) (Attachment, error) {
	id, err := documentToken()
	if err != nil {
		return Attachment{}, err
	}
	refs[id] = ref
	return Attachment{ID: id, Name: documentFilename(name, contentType), ContentType: contentType}, nil
}

func documentFilename(name, contentType string) string {
	mediaType, _, _ := mime.ParseMediaType(contentType)
	extension := ".bin"
	switch mediaType {
	case "application/pdf":
		extension = ".pdf"
	case "application/zip":
		extension = ".zip"
	case "text/plain":
		extension = ".txt"
	}
	var safe strings.Builder
	for _, r := range name {
		if unicode.IsControl(r) || strings.ContainsRune(`/\<>:"|?*`, r) {
			r = '_'
		}
		if safe.Len()+utf8.RuneLen(r) > 180 {
			break
		}
		safe.WriteRune(r)
	}
	filename := strings.Trim(safe.String(), ". ")
	if filename == "" {
		filename = "healthbank-document"
	}
	if !strings.HasSuffix(strings.ToLower(filename), extension) {
		filename += extension
	}
	return filename
}

func (s *Session) FetchImagingDetail(ctx context.Context, reference string) (SupplementReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	report := SupplementReport{Category: "imaging_detail", Entries: []SupplementEntry{}, Complete: false,
		Coverage: "保留影像／病理報告原文及官方影像狀態。JPEG 為官方提供的影像副本；DICOM 另行產製下載，不代替醫師閱片。"}
	ref, ok := s.imagingReferences[reference]
	if !ok {
		return report, failure("INVALID_INPUT", "影像明細識別已失效，請重新取得清單。")
	}
	if err := s.supplementReady(ctx); err != nil {
		return report, err
	}
	rows, err := s.supplementRequest(ctx, http.MethodGet, "/api/ihke3000/IHKE3408S02/page_load", url.Values{"crid": {ref.rowID}, "ctype": {ref.sourceType}}, nil, "ihke3408S02_main_data")
	if err != nil {
		return report, err
	}
	if len(rows) == 0 {
		return report, changedError()
	}
	refs := make(map[string]documentReference)
	for index, row := range rows {
		fields, err := appendKnownFields(row, []fieldSpec{
			{"main_tit", "官方標題（原文）", ""}, {"order_NAME", "檢查名稱", ""}, {"order_CODE", "醫令代碼", ""},
			{"func_DATE", "就醫／入院日期（原文）", ""}, {"assay_UPLOAD_DATE", "院所上傳報告時間", ""},
			{"radi_MSV", "醫療輻射參考劑量（原文）", ""}, {"ori_TYPE_NAME", "資料來源（原文）", ""},
			{"desc", "影像／病理報告原文", ""}, {"jpg_STATUS", "影像準備狀態（官方代碼）", ""}, {"img_ORI_SIZE", "醫療影像原始大小（原文）", ""},
		})
		if err != nil || len(fields) == 0 {
			return report, changedError()
		}
		entry := SupplementEntry{Key: medicalKey(ref.sourceType, ref.rowID, "imaging", index), Kind: "report", Date: ref.date, Institution: ref.institution,
			Title: "影像／病理報告", Source: "官方影像／病理資料", Fields: fields}
		sequence, e1 := rowText(row, "ipl_CASE_SEQ_NO")
		status, e2 := rowText(row, "jpg_STATUS")
		rowNumber, e3 := rowText(row, "rownum")
		originalSize, e4 := rowText(row, "img_ORI_SIZE")
		if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
			return report, changedError()
		}
		entry.Fields = appendImageStatus(entry.Fields, status)
		base := documentReference{category: "imaging", parent: reference, rowID: ref.rowID, sourceType: ref.sourceType, sequence: sequence}
		if status == "1" && usableDocumentID(sequence) && originalSize != "-" {
			base.kind = "imaging_jpeg"
			attachment, err := addDocumentReference(refs, base, "官方 JPEG 影像組（ZIP）", "application/zip")
			if err != nil {
				return report, err
			}
			entry.Attachments = append(entry.Attachments, attachment)
		}
		if status == "A" && usableDocumentID(rowNumber) {
			actionRef := base
			actionRef.kind, actionRef.sequence = "prepare_jpeg", rowNumber
			id, err := documentToken()
			if err != nil {
				return report, err
			}
			refs[id] = actionRef
			entry.DocumentActions = append(entry.DocumentActions, DocumentAction{ID: id, Name: "請官方準備 JPEG 影像", Kind: actionRef.kind})
		}
		if usableDocumentID(sequence) && originalSize != "" && originalSize != "-" {
			actionRef := base
			actionRef.kind = "prepare_dicom"
			id, err := documentToken()
			if err != nil {
				return report, err
			}
			refs[id] = actionRef
			entry.DocumentActions = append(entry.DocumentActions, DocumentAction{ID: id, Name: "請官方產製 DICOM 檔", Kind: actionRef.kind})
		}
		report.Entries = append(report.Entries, entry)
	}
	if err := validateSupplementReport(report); err != nil {
		return report, err
	}
	if err := s.replaceDocumentReferences("imaging", reference, refs); err != nil {
		return report, err
	}
	report.FetchedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return report, nil
}

func usableDocumentID(value string) bool {
	return value != "" && value != "-" && len(value) <= 4096 && !strings.ContainsAny(value, "\r\n\x00")
}

func appendImageStatus(fields []SupplementField, status string) []SupplementField {
	label := "官方狀態尚未辨識"
	switch status {
	case "":
		return fields
	case "-":
		label = "官方確認中"
	case "A":
		label = "可向官方提出影像產製"
	case "0":
		label = "官方影像準備中；可稍後重新取得明細"
	case "1":
		label = "官方標示影像已備妥；仍以實際下載結果為準"
	}
	return append(fields, SupplementField{Name: "影像狀態", Value: label})
}

func hospitalDocuments(row wireRow, ref medicalReference, parent string, refs map[string]documentReference) ([]Attachment, error) {
	attachments := []Attachment{}
	for _, spec := range []struct{ field, kind, name, contentType string }{
		{"has_XML", "hospital_markup", "出院病摘原始顯示內容（文字檔）", "text/plain;charset=utf-8"},
		{"has_PDF", "hospital_pdf", "住院 PDF 附件", "application/pdf"},
	} {
		flag, err := rowText(row, spec.field)
		if err != nil {
			return nil, err
		}
		if flag != "Y" {
			continue
		}
		attachment, err := addDocumentReference(refs, documentReference{category: "hospital", parent: parent, kind: spec.kind, rowID: ref.rowID, sourceType: ref.sourceType}, spec.name, spec.contentType)
		if err != nil {
			return nil, err
		}
		attachments = append(attachments, attachment)
	}
	return attachments, nil
}

func (s *Session) PrepareDocument(ctx context.Context, category, reference string) (DocumentRequestResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ref, ok := s.documentReferences[reference]
	exportFormat, exportOK := OfficialExportCategory(category)
	if !ok || ref.category != category || (category != "imaging" && category != "osteoporosis" && !exportOK) || (ref.kind != "prepare_jpeg" && ref.kind != "prepare_dicom" && ref.kind != "prepare_export") {
		return DocumentRequestResult{}, failure("INVALID_INPUT", "產製識別已失效，請重新取得此筆影像的狀態。")
	}
	if err := s.supplementReady(ctx); err != nil {
		return DocumentRequestResult{}, err
	}
	// An ambiguous network failure must not turn an automatic retry into another
	// upstream generation job. A fresh detail request is required before retrying.
	delete(s.documentReferences, reference)
	defer clear(ref.payload)
	endpoint := "/api/ihke3000/IHKE3408S02/add"
	if category == "osteoporosis" {
		endpoint = "/api/ihke3000/IHKE3411S02/add"
	}
	var payload []byte
	if ref.kind == "prepare_export" {
		spec, known := officialExportSpecs[exportFormat]
		if !known || len(ref.payload) == 0 {
			return DocumentRequestResult{}, failure("INVALID_INPUT", "官方產製項目已失效，請重新讀取可產製範圍。")
		}
		endpoint, payload = spec.path+"/add", append([]byte(nil), ref.payload...)
	} else {
		var err error
		payload, err = json.Marshal(map[string]string{"ipl_CASE_SEQ_NO": ref.sequence})
		if err != nil {
			return DocumentRequestResult{}, failure("REQUEST_FAILED", "無法建立文件產製請求。")
		}
	}
	defer clear(payload)
	body, _, err := s.request(ctx, http.MethodPost, endpoint, nil, payload, maxAuthBytes, true)
	if err != nil {
		return DocumentRequestResult{}, err
	}
	defer clear(body)
	row, err := documentObject(body)
	if err != nil {
		s.invalidateOnAuthError(err)
		return DocumentRequestResult{}, err
	}
	status, err := rowText(row, "status")
	if err != nil || status != "Y" {
		return DocumentRequestResult{}, failure("DOCUMENT_NOT_READY", "官方尚未確認產製請求；請重新讀取狀態後再操作。")
	}
	message := "已向官方提出產製請求。請稍後重新取得影像明細；DICOM 請到「官方已產製原始檔」更新清單。"
	if ref.kind == "prepare_export" {
		message = "已向官方提出原始檔產製請求，請稍後更新「官方已產製原始檔」清單。官方原始格式與 Healthbank 可攜資料包分開保存。"
	}
	return DocumentRequestResult{Status: "requested", Message: message}, nil
}

func documentObject(body []byte) (wireRow, error) {
	var row wireRow
	if decodeJSON(body, &row) != nil || row == nil {
		return nil, changedError()
	}
	if err := detectAPIError(row); err != nil {
		return nil, err
	}
	return canonicalRow(row)
}

// Called with the connector mutex already held by FetchProviderDetail. The
// provider owns record projection; this function only binds observed artifacts.
func (s *Session) attachProviderDocuments(category, reference string, ref providerReference, object wireRow, report *SupplementReport) error {
	if category != "e_prescriptions" && category != "osteoporosis" {
		return nil
	}
	refs := make(map[string]documentReference)
	if category == "e_prescriptions" {
		if len(report.Entries) == 0 || !usableDocumentID(ref.rowID) || !usableDocumentID(ref.sourceType) {
			return changedError()
		}
		attachment, err := addDocumentReference(refs, documentReference{category: category, parent: reference, kind: "prescription_pdf", rowID: ref.rowID, sourceType: ref.sourceType}, "官方電子處方 PDF（依官方可提供狀態）", "application/pdf")
		if err != nil {
			return err
		}
		report.Entries[0].Attachments = append(report.Entries[0].Attachments, attachment)
	} else {
		rows, err := supplementRows(object["ihke3411s02_main_data"], "")
		if err != nil || len(rows) != len(report.Entries) {
			return changedError()
		}
		for index, row := range rows {
			status, e1 := rowText(row, "jpg_STATUS")
			sequence, e2 := rowText(row, "ipl_CASE_SEQ_NO")
			rowNumber, e3 := rowText(row, "rownum")
			originalSize, e4 := rowText(row, "img_ORI_SIZE")
			if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
				return changedError()
			}
			entry := &report.Entries[index]
			if status != "" {
				entry.Fields = append(entry.Fields, SupplementField{Name: "影像準備狀態（官方代碼）", Value: status})
			}
			entry.Fields = appendImageStatus(entry.Fields, status)
			if status == "1" && usableDocumentID(sequence) && originalSize != "-" {
				attachment, err := addDocumentReference(refs, documentReference{category: category, parent: reference, kind: "imaging_jpeg", sequence: sequence}, "官方骨密度 JPEG 影像組（ZIP）", "application/zip")
				if err != nil {
					return err
				}
				entry.Attachments = append(entry.Attachments, attachment)
			}
			if status == "A" && usableDocumentID(rowNumber) {
				id, err := documentToken()
				if err != nil {
					return err
				}
				refs[id] = documentReference{category: category, parent: reference, kind: "prepare_jpeg", sequence: rowNumber}
				entry.DocumentActions = append(entry.DocumentActions, DocumentAction{ID: id, Kind: "prepare_jpeg", Name: "請官方準備骨密度 JPEG 影像"})
			}
		}
	}
	return s.replaceDocumentReferences(category, reference, refs)
}

func (s *Session) FetchOfficialFiles(ctx context.Context) (SupplementReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	report := SupplementReport{Category: "official_files", Entries: []SupplementEntry{}, Complete: false,
		Coverage: "官方下載服務僅列最近 7 天已提出的產製紀錄；不是醫療資料保存年限。這裡讀取既有工作與原始檔，不自動提交新的全資料產製。"}
	if err := s.supplementReady(ctx); err != nil {
		return report, err
	}
	rows, err := s.supplementRequest(ctx, http.MethodGet, "/api/ihke3000/ihke3801s01/page_load", nil, nil, "sp_IHKE3801S01_data")
	if err != nil {
		return report, err
	}
	refs := make(map[string]officialFileReference)
	for index, row := range rows {
		sequence, e1 := rowText(row, "seq_NO")
		format, e2 := rowText(row, "title")
		date, e3 := rowText(row, "txt_DATE")
		flag, e4 := rowText(row, "flag")
		if e1 != nil || e2 != nil || e3 != nil || e4 != nil || !usableDocumentID(sequence) || format == "" {
			return report, changedError()
		}
		fields, err := appendKnownFields(row, []fieldSpec{{"title", "官方格式名稱", ""}, {"status", "官方產製狀態", ""}, {"txt_DATE", "提出日期（非檢查日期）", ""}})
		if err != nil {
			return report, err
		}
		key, err := documentToken()
		if err != nil {
			return report, err
		}
		refs[key] = officialFileReference{sequence: sequence, format: format, date: date, image: flag == "5"}
		report.Entries = append(report.Entries, SupplementEntry{Key: medicalKey("official_files", sequence, "list", index), Kind: "document", Title: "官方產製 " + format,
			Date: date, Source: "官方下載服務／提出日期", Fields: fields, DetailID: key})
	}
	if err := validateSupplementReport(report); err != nil {
		return report, err
	}
	s.officialFileReferences = refs
	s.clearDocumentCategory("official_files")
	report.FetchedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return report, nil
}

func (s *Session) FetchOfficialFileDetail(ctx context.Context, reference string) (SupplementReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	report := SupplementReport{Category: "official_files_detail", Entries: []SupplementEntry{}, Complete: false,
		Coverage: "此工作所列的官方產製範圍與狀態；只有官方確認可下載才提供檔案。原始壓縮檔可能有官方密碼保護；Healthbank 不保存或自動推算解壓密碼。"}
	ref, ok := s.officialFileReferences[reference]
	if !ok {
		return report, failure("INVALID_INPUT", "官方檔案識別已失效，請重新取得產製清單。")
	}
	if err := s.supplementReady(ctx); err != nil {
		return report, err
	}
	path := "/api/ihke3000/ihke3801s02/page_load"
	if ref.image {
		path = "/api/ihke3000/ihke3801s02/imgF_page_load"
	}
	body, _, err := s.request(ctx, http.MethodGet, path, url.Values{"seq_no": {ref.sequence}, "type": {ref.format}, "txtdate": {ref.date}}, nil, maxJSONBytes, true)
	if err != nil {
		return report, err
	}
	defer clear(body)
	row, err := documentObject(body)
	if err != nil {
		s.invalidateOnAuthError(err)
		return report, err
	}
	status, err := rowText(row, "status")
	if err != nil || status == "" {
		return report, changedError()
	}
	fields, err := appendKnownFields(row, []fieldSpec{{"status", "產製狀態（1 可下載；其他尚未完成）", ""}, {"real_INSPECT_DATE", "檢查日期（原文）", ""}, {"order_CODE_NAME", "檢查名稱", ""}, {"img_SIZE", "檔案大小（原文）", ""}})
	if err != nil {
		return report, err
	}
	if raw, found := row["sp_ihke3801s02_data"]; found {
		parts, err := supplementRows(raw, "")
		if err != nil {
			return report, err
		}
		for _, part := range parts {
			title, e1 := rowText(part, "title")
			content, e2 := rowText(part, "cont")
			if e1 != nil || e2 != nil {
				return report, changedError()
			}
			if title != "" || content != "" {
				fields = append(fields, SupplementField{Name: "官方產製範圍／" + title, Value: content})
			}
		}
	}
	if raw, found := row["code_cname"]; found && string(raw) != "null" {
		var labels []string
		if decodeJSON(raw, &labels) != nil || len(labels) > MaxReportRecords {
			return report, changedError()
		}
		for _, label := range labels {
			if len(label) > 65536 {
				return report, changedError()
			}
			fields = append(fields, SupplementField{Name: "官方明細項目", Value: label})
		}
	}
	entry := SupplementEntry{Kind: "document", Date: ref.date, Title: "官方產製 " + ref.format, Source: "官方下載服務／提出日期", Fields: fields}
	refs := make(map[string]documentReference)
	if status == "1" {
		downloadID, err := rowText(row, "did")
		if err != nil || downloadID != "" && !usableDocumentID(downloadID) {
			return report, changedError()
		}
		kind, contentType := "official_archive", "application/zip"
		if ref.image {
			kind = "dicom_archive"
		} else if strings.EqualFold(ref.format, "PDF") {
			kind, contentType = "official_pdf", "application/pdf"
		}
		attachment, err := addDocumentReference(refs, documentReference{category: "official_files", parent: reference, kind: kind, sequence: ref.sequence, format: ref.format, date: ref.date, downloadID: downloadID}, fmt.Sprintf("官方 %s 原始檔", ref.format), contentType)
		if err != nil {
			return report, err
		}
		entry.Attachments = []Attachment{attachment}
	}
	report.Entries = append(report.Entries, entry)
	if err := validateSupplementReport(report); err != nil {
		return report, err
	}
	if err := s.replaceDocumentReferences("official_files", reference, refs); err != nil {
		return report, err
	}
	report.FetchedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return report, nil
}
