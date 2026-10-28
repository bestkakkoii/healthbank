// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image/jpeg"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	MaxDocumentBytes       int64 = 500 << 20
	maxBufferedDocument          = 32 << 20
	maxImageDocumentJSON         = 48 << 20
	maxDocumentPictures          = 500
	DocumentDownloadBudget       = 5 * time.Minute
)

// DocumentInfo describes the actual inspected output, rather than an upstream
// claimed MIME type or a caller-controlled file name. Size -1 means unknown.
type DocumentInfo struct {
	Name        string
	ContentType string
	Size        int64
	Warnings    []string
}

// DocumentWorkingSet returns a conservative admission reservation for an opaque
// reference. It does not fetch data, disclose the source route, or authorize a
// transfer. DownloadDocument performs the full reference/authentication checks.
func (s *Session) DocumentWorkingSet(category, reference string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	ref, ok := s.documentReferences[reference]
	if !ok || ref.category != category {
		return 1 << 20
	}
	switch ref.kind {
	case "hospital_pdf", "prescription_calendar", "notification_calendar":
		return 64 << 20
	case "hospital_markup", "self_paid_file", "official_bundle":
		return 192 << 20
	case "imaging_jpeg":
		return 192 << 20
	default:
		return 1 << 20 // Original artifacts stream through bounded small buffers.
	}
}

// DownloadDocument serializes one authenticated transfer, limits it, and never
// writes health content to disk. consume must read synchronously and must not
// retain the reader. Errors after consume has begun require aborting the HTTP
// response: otherwise a partial medical file could be mistaken for success.
func (s *Session) DownloadDocument(ctx context.Context, category, reference string, consume func(DocumentInfo, io.Reader) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ref, ok := s.documentReferences[reference]
	if !ok || ref.category != category || strings.HasPrefix(ref.kind, "prepare_") || consume == nil {
		return failure("INVALID_INPUT", "文件識別已失效或不屬於此類別，請重新取得明細。")
	}
	ctx, cancel := context.WithTimeout(ctx, DocumentDownloadBudget)
	defer cancel()
	if err := s.supplementReady(ctx); err != nil {
		return err
	}
	switch ref.kind {
	case "hospital_markup":
		return s.downloadHospitalMarkup(ctx, ref, consume)
	case "hospital_pdf":
		return s.downloadHospitalPDF(ctx, ref, consume)
	case "imaging_jpeg":
		return s.downloadJPEGs(ctx, ref, consume)
	case "prescription_calendar", "notification_calendar":
		return s.downloadSourceCalendar(ctx, ref, consume)
	case "official_archive", "official_pdf", "dicom_archive", "prescription_pdf", "self_paid_file", "official_bundle":
		return s.downloadOfficialArtifact(ctx, ref, consume)
	default:
		return failure("INVALID_INPUT", "此文件類型不支援下載。")
	}
}

func (s *Session) downloadSourceCalendar(ctx context.Context, ref documentReference, consume func(DocumentInfo, io.Reader) error) error {
	params := url.Values{"tid": {ref.rowID}, "rtimes": {ref.sequence}, "recipeType": {ref.sourceType}}
	endpoint, filename := "/api/ihke3000/ihke3212s01/IHKE3212_ics", "healthbank-suggested-refill.ics"
	if ref.kind == "notification_calendar" {
		endpoint, filename = "/api/ihke3000/ihke3104s01/IHKE3104S08", "healthbank-source-reminder.ics"
		params = url.Values{"SEQ_NO": {ref.sequence}}
	}
	body, _, err := s.request(ctx, http.MethodGet, endpoint, params, nil, maxBufferedDocument, true)
	if err != nil {
		return err
	}
	defer clear(body)
	content := bytes.TrimSpace(bytes.TrimPrefix(body, []byte{0xef, 0xbb, 0xbf}))
	if bytes.HasPrefix(content, []byte("{")) {
		if len(content) > maxAuthBytes {
			return failure("DOCUMENT_INVALID", "官方回傳過大的 JSON 狀態，未當成行事曆附件。")
		}
		_, err := documentObject(content)
		if err != nil {
			s.invalidateOnAuthError(err)
			return err
		}
	}
	if !utf8.Valid(body) || bytes.ContainsRune(body, 0) || !(bytes.HasPrefix(content, []byte("BEGIN:VCALENDAR\r\n")) || bytes.HasPrefix(content, []byte("BEGIN:VCALENDAR\n"))) || !bytes.HasSuffix(content, []byte("END:VCALENDAR")) {
		return failure("DOCUMENT_INVALID", "官方回傳不是完整 UTF-8 行事曆，未將錯誤頁當成來源提醒。")
	}
	return consume(DocumentInfo{Name: filename, ContentType: "text/calendar;charset=utf-8", Size: int64(len(body))}, bytes.NewReader(body))
}

func (s *Session) downloadHospitalMarkup(ctx context.Context, ref documentReference, consume func(DocumentInfo, io.Reader) error) error {
	body, _, err := s.request(ctx, http.MethodGet, "/api/ihke3000/IHKE3309S02/getxml", url.Values{"crid": {ref.rowID}, "ctype": {ref.sourceType}}, nil, maxImageDocumentJSON, true)
	if err != nil {
		return err
	}
	defer clear(body)
	row, err := documentObject(body)
	if err != nil {
		s.invalidateOnAuthError(err)
		return err
	}
	var content string
	if json.Unmarshal(row["file_name"], &content) != nil || content == "" || len(content) > maxBufferedDocument || !utf8.ValidString(content) || strings.ContainsRune(content, 0) {
		return failure("DOCUMENT_INVALID", "官方病摘內容格式或大小不符合可讀取範圍。")
	}
	// The public showXML component puts file_name directly into innerHTML.
	// It is provider-rendered markup, not evidence of original XML bytes.
	return consume(DocumentInfo{Name: "healthbank-discharge-source.txt", ContentType: "text/plain;charset=utf-8", Size: int64(len(content))}, strings.NewReader(content))
}

func officialPDFPath(value string) (string, error) {
	if len(value) == 0 || len(value) > 4096 || strings.ContainsAny(value, "\x00\r\n\\") {
		return "", failure("DOCUMENT_INVALID", "官方 PDF 位址不符合允許的來源。")
	}
	base, _ := url.Parse(officialOrigin + "/ihke3000/ViewPdf/pdfviewer.html")
	relative, err := url.Parse(value)
	if err != nil {
		return "", failure("DOCUMENT_INVALID", "官方 PDF 位址無法解析。")
	}
	u := base.ResolveReference(relative)
	lowerPath := strings.ToLower(u.Path)
	// Reject encoded backslashes, controls and a second percent-decoding layer
	// before a Windows-hosted provider could reinterpret the checked path.
	if strings.ContainsAny(u.Path, "\\%") || strings.ContainsFunc(u.Path, unicode.IsControl) || u.Scheme != "https" || u.Host != "myhealthbank.nhi.gov.tw" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !strings.HasPrefix(lowerPath, "/ihke3000/") || !strings.HasSuffix(lowerPath, ".pdf") || path.Clean(u.Path) != u.Path {
		return "", failure("DOCUMENT_INVALID", "官方 PDF 位址不符合允許的來源。")
	}
	return u.EscapedPath(), nil
}

func (s *Session) downloadHospitalPDF(ctx context.Context, ref documentReference, consume func(DocumentInfo, io.Reader) error) (resultErr error) {
	params := url.Values{"crid": {ref.rowID}, "ctype": {ref.sourceType}}
	body, _, err := s.request(ctx, http.MethodGet, "/api/ihke3000/IHKE3309S02/getpdf", params, nil, maxAuthBytes, true)
	if err != nil {
		return err
	}
	defer clear(body)
	// getpdf prepares a temporary upstream file. The official UI calls delpdf
	// after viewing. Cleanup is attempted once even when fetching the PDF fails;
	// it only removes this generated temporary file, never the source report.
	cleaned := false
	cleanup := func() error {
		if cleaned {
			return nil
		}
		cleaned = true
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		data, _, cleanupErr := s.request(cleanupCtx, http.MethodGet, "/api/ihke3000/IHKE3309S02/delpdf", params, nil, maxAuthBytes, true)
		defer clear(data)
		if cleanupErr == nil && len(bytes.TrimSpace(data)) != 0 {
			// A 200 JSON error can still report expired authentication. The public
			// delpdf component ignores the body and provides no observed positive
			// acknowledgment contract, so even HTTP success cannot prove deletion.
			_, cleanupErr = documentObject(data)
			s.invalidateOnAuthError(cleanupErr)
		}
		var upstream *Error
		if errors.As(cleanupErr, &upstream) && upstream.Code == "SESSION_EXPIRED" {
			return cleanupErr
		}
		return nil
	}
	defer func() {
		// Known authentication expiry must reach the application even when the
		// document itself failed. Other cleanup failures remain unconfirmed and
		// must not replace the actual document error or discard a valid file.
		if cleanupErr := cleanup(); cleanupErr != nil {
			resultErr = cleanupErr
		}
	}()
	row, err := documentObject(body)
	if err != nil {
		s.invalidateOnAuthError(err)
		return err
	}
	filename, err := rowText(row, "file_name")
	if err != nil {
		return err
	}
	filePath, err := officialPDFPath(filename)
	if err != nil {
		return err
	}
	// The official viewer downloads the returned same-origin file using its
	// cookie jar. There is no reason to forward the JWT to a static file path.
	client := *s.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return errRedirectRejected }
	data, _, err := s.requestWithClient(ctx, &client, http.MethodGet, filePath, nil, nil, maxBufferedDocument, false)
	if err != nil {
		return err
	}
	defer clear(data)
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		return failure("DOCUMENT_INVALID", "官方回傳不是可辨識的 PDF，未將錯誤頁當成附件。")
	}
	if err := cleanup(); err != nil {
		return err
	}
	warnings := []string{"official_temporary_cleanup_unconfirmed"}
	return consume(DocumentInfo{Name: "healthbank-hospital-attachment.pdf", ContentType: "application/pdf", Size: int64(len(data)), Warnings: warnings}, bytes.NewReader(data))
}

func (s *Session) downloadJPEGs(ctx context.Context, ref documentReference, consume func(DocumentInfo, io.Reader) error) error {
	endpoint := "/api/ihke3000/IHKE3408S03/page_load"
	if ref.category == "osteoporosis" {
		endpoint = "/api/ihke3000/IHKE3411S03/page_load"
	}
	body, _, err := s.request(ctx, http.MethodGet, endpoint, url.Values{"IPL_CASE_SEQ_NO": {ref.sequence}}, nil, maxImageDocumentJSON, true)
	if err != nil {
		return err
	}
	defer clear(body)
	row, err := documentObject(body)
	if err != nil {
		s.invalidateOnAuthError(err)
		return err
	}
	var pictures []string
	if decodeJSON(row["pics"], &pictures) != nil || len(pictures) == 0 || len(pictures) > maxDocumentPictures {
		return failure("DOCUMENT_INVALID", "官方影像陣列為空、過大或格式已變更。")
	}
	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	defer func() { clear(output.Bytes()) }()
	total := 0
	for index, encoded := range pictures {
		if err := ctx.Err(); err != nil {
			return networkFailure(err)
		}
		if base64.StdEncoding.DecodedLen(len(encoded))+total > maxBufferedDocument {
			return failure("RESPONSE_TOO_LARGE", "這組 JPEG 影像超過 32 MiB 記憶體讀取上限，請使用官方原始影像檔下載。")
		}
		data, err := base64.StdEncoding.Strict().DecodeString(encoded)
		if err != nil {
			return failure("DOCUMENT_INVALID", "官方影像不是有效的 JPEG 編碼。")
		}
		config, err := jpeg.DecodeConfig(bytes.NewReader(data))
		if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 32768 || config.Height > 32768 || int64(config.Width)*int64(config.Height) > 100000000 || len(data) < 4 || !bytes.HasSuffix(data, []byte{0xff, 0xd9}) {
			clear(data)
			return failure("DOCUMENT_INVALID", "官方影像內容或尺寸不符合 JPEG 下載範圍。")
		}
		total += len(data)
		writer, err := archive.CreateHeader(&zip.FileHeader{Name: fmt.Sprintf("images/%04d.jpg", index+1), Method: zip.Store})
		if err == nil {
			_, err = writer.Write(data)
		}
		clear(data)
		if err != nil {
			return failure("DOCUMENT_INVALID", "無法整理官方影像檔。")
		}
	}
	if err := archive.Close(); err != nil {
		return failure("DOCUMENT_INVALID", "無法完成官方影像壓縮檔。")
	}
	return consume(DocumentInfo{Name: "healthbank-official-jpeg-images.zip", ContentType: "application/zip", Size: int64(output.Len())}, bytes.NewReader(output.Bytes()))
}

func (s *Session) downloadOfficialArtifact(ctx context.Context, ref documentReference, consume func(DocumentInfo, io.Reader) error) (resultErr error) {
	endpoint := "/api/ihke3000/ihke3801s02/download"
	params := url.Values{"seq_no": {ref.sequence}, "type": {ref.format}, "txtdate": {ref.date}}
	contentType, filename := "application/zip", "healthbank-official-source.zip"
	if ref.kind == "dicom_archive" {
		endpoint, params = "/api/ihke3000/ihke3801s02/imgF_download", url.Values{"seq_no": {ref.sequence}}
		filename = "healthbank-official-dicom.zip"
	} else if ref.kind == "official_pdf" {
		contentType, filename = "application/pdf", "healthbank-official-source.pdf"
	} else if ref.kind == "prescription_pdf" {
		endpoint = "/api/ihke3000/ihke3212s01/IHKE3212_pdf"
		params = url.Values{"tid": {ref.rowID}, "tid2": {ref.rowID}, "recipeType": {ref.sourceType}}
		contentType, filename = "application/pdf", "healthbank-official-prescription.pdf"
	} else if ref.kind == "self_paid_file" {
		endpoint = "/api/ihke3000/ihke3403S07/download"
		params = url.Values{"DATA02_SEQ": {ref.rowID}, "FILE_SEQ": {ref.sequence}}
		contentType, filename = "application/octet-stream", "healthbank-self-paid-source.bin"
	} else if ref.kind == "official_bundle" {
		endpoint = "/api/ihke3000/ihke3801s01/download"
		contentType, filename = "application/octet-stream", "healthbank-official-complete-source.bin"
	}
	// The official detail view chooses the app-download route when it returns
	// a download ID. Keep that private selector bound to this opaque attachment;
	// it is never accepted as an arbitrary browser parameter or exposed in JSON.
	if ref.downloadID != "" {
		endpoint = "/api/ihke3000/ihke3801s02/app_download"
		if ref.kind == "dicom_archive" {
			endpoint = "/api/ihke3000/ihke3801s02/app_imgF_download"
		}
		params = url.Values{"gid": {ref.downloadID}}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, officialOrigin+endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return failure("REQUEST_FAILED", "無法建立官方文件請求。")
	}
	req, trace := startRequestTrace(req, operationForPath(endpoint))
	defer func() {
		s.lastDiagnostics = trace.finish()
		resultErr = s.annotateResponseError(resultErr)
	}()
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; BestkakkoiHealthbank/1.0)")
	req.Header.Set("Accept-Language", "zh-TW,zh;q=0.9")
	req.Header.Set("Accept", contentType+", application/octet-stream")
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Origin", officialOrigin)
	req.Header.Set("Referer", officialOrigin+"/IHKE3000/IHKE3801S01")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	client := *s.client
	client.Timeout = DocumentDownloadBudget
	// An artifact API cannot redirect to another endpoint or origin. This also
	// prevents forwarding the authenticated download headers to a static page.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return errRedirectRejected }
	response, err := client.Do(req)
	if err != nil {
		return networkFailure(err)
	}
	defer response.Body.Close()
	trace.response(response)
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		s.clearAuth()
		return sessionExpired()
	}
	if response.StatusCode != http.StatusOK {
		return &Error{Code: "UPSTREAM_UNAVAILABLE", Message: "官方文件下載收到 HTTP " + strconv.Itoa(response.StatusCode) + "，請重新取得產製狀態。", HTTPStatus: response.StatusCode}
	}
	if response.ContentLength > MaxDocumentBytes {
		return failure("RESPONSE_TOO_LARGE", "官方文件超過 500 MiB 單次下載上限。")
	}
	trace.setPhase(PhaseResponseBody)
	mediaType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	namedFormat := sourceTextAttachmentFormat(response, ref)
	consumeText := func(reader io.Reader, filename string) error {
		err := s.consumeSourceText(response, reader, filename, namedFormat, consume)
		if err == nil {
			trace.setPhase(PhaseComplete)
		}
		return err
	}
	if strings.Contains(mediaType, "json") {
		if namedFormat == ".json" {
			return consumeText(response.Body, strings.TrimSuffix(filename, ".bin")+".json")
		}
		// The source also reports an expired JWT as HTTP 200 JSON. Treat that
		// exactly like the ordinary data endpoints rather than retaining an
		// authenticated connector after an explicit expiry response.
		body, err := io.ReadAll(io.LimitReader(response.Body, maxAuthBytes+1))
		defer clear(body)
		if err != nil {
			return responseReadFailure(err, response.StatusCode)
		}
		if len(body) <= maxAuthBytes {
			if _, err := documentObject(body); err != nil {
				s.invalidateOnAuthError(err)
				return err
			}
		}
		return failure("DOCUMENT_INVALID", "官方回傳錯誤頁或 JSON，未將其當成原始文件。")
	}
	if mediaType == "text/html" {
		if namedFormat == ".html" || namedFormat == ".htm" {
			return consumeText(response.Body, strings.TrimSuffix(filename, ".bin")+"-markup.txt")
		}
		return failure("DOCUMENT_INVALID", "官方回傳錯誤頁或 JSON，未將其當成原始文件。")
	}
	reader := bufio.NewReaderSize(response.Body, 32768)
	prefix, err := reader.Peek(5)
	if err != nil && (err != io.EOF || len(prefix) == 0 || ref.kind != "self_paid_file" && ref.kind != "official_bundle") {
		return failure("DOCUMENT_INVALID", "官方文件未完整提供可辨識的標頭。")
	}
	if ref.kind == "self_paid_file" || ref.kind == "official_bundle" {
		baseName := strings.TrimSuffix(filename, ".bin")
		typePrefix, _ := reader.Peek(8)
		switch {
		case bytes.HasPrefix(prefix, []byte("%PDF-")):
			contentType, filename = "application/pdf", baseName+".pdf"
		case bytes.HasPrefix(prefix, []byte{'P', 'K', 3, 4}) || bytes.HasPrefix(prefix, []byte{'P', 'K', 5, 6}):
			contentType, filename = "application/zip", baseName+".zip"
		case bytes.HasPrefix(prefix, []byte{0xff, 0xd8, 0xff}):
			contentType, filename = "image/jpeg", baseName+".jpg"
		case bytes.HasPrefix(typePrefix, []byte{0x89, 'P', 'N', 'G', 13, 10, 26, 10}):
			contentType, filename = "image/png", baseName+".png"
		default:
			extension := ".txt"
			if namedFormat == ".json" {
				extension = ".json"
			}
			return consumeText(reader, baseName+extension)
		}
	}
	valid := bytes.HasPrefix(prefix, []byte("%PDF-"))
	if contentType == "application/zip" {
		valid = bytes.HasPrefix(prefix, []byte{'P', 'K', 3, 4}) || bytes.HasPrefix(prefix, []byte{'P', 'K', 5, 6})
	}
	if (ref.kind == "self_paid_file" || ref.kind == "official_bundle") && (contentType == "image/jpeg" || contentType == "image/png") {
		valid = true // The fixed self-paid route's actual image signature was inspected above.
	}
	if !valid {
		return failure("DOCUMENT_INVALID", "官方文件格式與產製類型不符，未提供下載。")
	}
	bounded := &documentReader{reader: reader, limit: MaxDocumentBytes, expected: response.ContentLength}
	if err := consume(DocumentInfo{Name: filename, ContentType: contentType, Size: response.ContentLength}, bounded); err != nil {
		return err
	}
	if !bounded.finished {
		return failure("DOCUMENT_INCOMPLETE", "文件傳輸未讀到結尾。")
	}
	trace.setPhase(PhaseComplete)
	return nil
}

// JSON/HTML status responses are not artifacts. The two routes offering
// unspecified originals may, however, offer explicitly named text attachments.
// The untrusted name is inspected only for this distinction: it is never used
// as a path, returned filename or evidence of medical validity. Source markup
// is delivered as inert .txt bytes, never rendered or executed by Healthbank.
func sourceTextAttachmentFormat(response *http.Response, ref documentReference) string {
	if ref.kind != "self_paid_file" && ref.kind != "official_bundle" {
		return ""
	}
	disposition, parameters, err := mime.ParseMediaType(response.Header.Get("Content-Disposition"))
	filename := parameters["filename"]
	if err != nil || !strings.EqualFold(disposition, "attachment") || len(filename) == 0 || len(filename) > 4096 || !utf8.ValidString(filename) || strings.ContainsAny(filename, "/\\\x00\r\n") {
		return ""
	}
	extension := strings.ToLower(path.Ext(filename))
	switch extension {
	case ".json", ".xml", ".html", ".htm", ".txt":
		return extension
	}
	return ""
}

func (s *Session) consumeSourceText(response *http.Response, reader io.Reader, filename, namedFormat string, consume func(DocumentInfo, io.Reader) error) error {
	if response.ContentLength > maxBufferedDocument {
		return failure("RESPONSE_TOO_LARGE", "來源文字附件超過 32 MiB 驗證上限，未回傳不完整文件。")
	}
	body, err := io.ReadAll(io.LimitReader(reader, maxBufferedDocument+1))
	defer clear(body)
	if err != nil || response.ContentLength >= 0 && response.ContentLength != int64(len(body)) {
		return failure("DOCUMENT_INCOMPLETE", "來源文字附件傳輸未完成。")
	}
	if len(body) > maxBufferedDocument {
		return failure("RESPONSE_TOO_LARGE", "來源文字附件超過 32 MiB 驗證上限，未回傳不完整文件。")
	}
	trimmed := bytes.TrimSpace(bytes.TrimPrefix(body, []byte{0xef, 0xbb, 0xbf}))
	if len(trimmed) == 0 || !utf8.Valid(body) || bytes.ContainsRune(body, 0) {
		return failure("DOCUMENT_INVALID", "來源附件不是完整 UTF-8 文字，未提供下載。")
	}
	if namedFormat == ".json" {
		if !json.Valid(trimmed) {
			return failure("DOCUMENT_INVALID", "來源 JSON 附件格式不完整。")
		}
		if err := originalJSONArtifactError(trimmed); err != nil {
			s.invalidateOnAuthError(err)
			return err
		}
		// A raw source file has case-sensitive keys and may contain original
		// report markup as string values. It is not a canonical provider row,
		// and its bytes are delivered as inert text without interpreting them.
		return consume(DocumentInfo{Name: filename, ContentType: "text/plain;charset=utf-8", Size: int64(len(body))}, bytes.NewReader(body))
	}
	if bytes.HasPrefix(trimmed, []byte("{")) {
		if len(trimmed) > maxAuthBytes {
			return failure("DOCUMENT_INVALID", "官方回傳過大的 JSON 狀態，未當成來源附件。")
		}
		if _, err := documentObject(trimmed); err != nil {
			s.invalidateOnAuthError(err)
			return err
		}
		return failure("DOCUMENT_INVALID", "官方回傳 JSON，未將狀態回應當成來源附件。")
	}
	if bytes.HasPrefix(trimmed, []byte("[")) {
		return failure("DOCUMENT_INVALID", "來源 JSON 附件格式不完整或缺少明確檔案識別。")
	}
	prefix := strings.ToLower(string(trimmed[:min(len(trimmed), 512)]))
	markup := strings.Contains(prefix, "<!doctype html") || strings.Contains(prefix, "<html") || strings.Contains(prefix, "<script") || strings.Contains(prefix, "<body") || strings.Contains(prefix, "<head")
	if markup && namedFormat != ".html" && namedFormat != ".htm" && namedFormat != ".xml" {
		return failure("DOCUMENT_INVALID", "來源附件不是可辨識的文件或 UTF-8 文字，未將錯誤頁當成附件。")
	}
	return consume(DocumentInfo{Name: filename, ContentType: "text/plain;charset=utf-8", Size: int64(len(body))}, bytes.NewReader(body))
}

// Inspect only the explicit top-level status key. Do not case-fold, project or
// decode the original file's medical content into provider rows. Each repeated
// spelling of error_code is inspected, so a later zero cannot conceal expiry.
func originalJSONArtifactError(body []byte) error {
	if len(body) == 0 || body[0] != '{' {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if _, err := decoder.Token(); err != nil {
		return failure("DOCUMENT_INVALID", "來源 JSON 附件格式不完整。")
	}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return failure("DOCUMENT_INVALID", "來源 JSON 附件格式不完整。")
		}
		name, ok := key.(string)
		if !ok {
			return failure("DOCUMENT_INVALID", "來源 JSON 附件格式不完整。")
		}
		var raw json.RawMessage
		if decoder.Decode(&raw) != nil {
			clear(raw)
			return failure("DOCUMENT_INVALID", "來源 JSON 附件格式不完整。")
		}
		if strings.EqualFold(name, "error_code") {
			err = detectAPIError(map[string]json.RawMessage{"error_code": raw})
		}
		clear(raw)
		if err != nil {
			return err
		}
	}
	return nil // Full JSON syntax/trailing-content validation preceded this scan.
}

// documentReader never reports a successful EOF at the byte limit unless the
// upstream also ended. Truncation therefore cannot produce a successful file.
type documentReader struct {
	reader   io.Reader
	limit    int64
	expected int64
	read     int64
	finished bool
}

func (r *documentReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.read == r.limit {
		var extra [1]byte
		n, err := r.reader.Read(extra[:])
		if n > 0 {
			return 0, failure("RESPONSE_TOO_LARGE", "文件傳輸超過 500 MiB 上限，下載已中止。")
		}
		if err == io.EOF {
			if r.expected >= 0 && r.read != r.expected {
				return 0, failure("DOCUMENT_INCOMPLETE", "官方文件大小不完整，請重新下載。")
			}
			r.finished = true
		}
		return 0, err
	}
	if int64(len(p)) > r.limit-r.read {
		p = p[:r.limit-r.read]
	}
	n, err := r.reader.Read(p)
	r.read += int64(n)
	if err != nil && !errors.Is(err, io.EOF) {
		return n, failure("DOCUMENT_INCOMPLETE", "官方文件傳輸未完成，請重新下載。")
	}
	if err == io.EOF {
		if r.expected >= 0 && r.read != r.expected {
			return n, failure("DOCUMENT_INCOMPLETE", "官方文件大小不完整，請重新下載。")
		}
		r.finished = true
	}
	return n, err
}
