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
	case "official_archive", "official_pdf", "dicom_archive", "prescription_pdf":
		return s.downloadOfficialArtifact(ctx, ref, consume)
	default:
		return failure("INVALID_INPUT", "此文件類型不支援下載。")
	}
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

func (s *Session) downloadHospitalPDF(ctx context.Context, ref documentReference, consume func(DocumentInfo, io.Reader) error) error {
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
	cleanup := func() {
		if cleaned {
			return
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
			_, parseErr := documentObject(data)
			s.invalidateOnAuthError(parseErr)
		}
	}
	defer cleanup()
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
	cleanup()
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
	mediaType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if mediaType == "text/html" || strings.Contains(mediaType, "json") {
		return failure("DOCUMENT_INVALID", "官方回傳錯誤頁或 JSON，未將其當成原始文件。")
	}
	trace.setPhase(PhaseResponseBody)
	reader := bufio.NewReaderSize(response.Body, 32768)
	prefix, err := reader.Peek(5)
	if err != nil {
		return failure("DOCUMENT_INVALID", "官方文件未完整提供可辨識的標頭。")
	}
	valid := bytes.HasPrefix(prefix, []byte("%PDF-"))
	if contentType == "application/zip" {
		valid = bytes.HasPrefix(prefix, []byte{'P', 'K', 3, 4}) || bytes.HasPrefix(prefix, []byte{'P', 'K', 5, 6})
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
