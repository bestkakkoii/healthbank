// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package nhi

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
)

func collectDocument(info *DocumentInfo, output *bytes.Buffer) func(DocumentInfo, io.Reader) error {
	return func(value DocumentInfo, reader io.Reader) error {
		*info = value
		_, err := io.Copy(output, reader)
		return err
	}
}

func TestAttachmentNamesRetainSafeFormatExtensions(t *testing.T) {
	for _, tc := range []struct{ name, contentType, extension string }{
		{"官方影像組", "application/zip", ".zip"},
		{"官方處方", "application/pdf", ".pdf"},
		{"出院病摘原始顯示內容", "text/plain;charset=utf-8", ".txt"},
		{"../source:report\\name\n", "application/pdf", ".pdf"},
		{strings.Repeat("合成中文", 200), "application/zip", ".zip"},
	} {
		attachment, err := addDocumentReference(map[string]documentReference{}, documentReference{}, tc.name, tc.contentType)
		if err != nil || !strings.HasSuffix(attachment.Name, tc.extension) || strings.ContainsAny(attachment.Name, "/\\<>:\"|?*\r\n") || len(attachment.Name) > 184 || !utf8.ValidString(attachment.Name) {
			t.Fatalf("unsafe or untyped attachment filename: %q (%v)", attachment.Name, err)
		}
	}
}

func TestImagingDetailOriginalTextDocumentsAndExplicitPreparation(t *testing.T) {
	var generated, pictures atomic.Int32
	var picture bytes.Buffer
	if err := jpeg.Encode(&picture, image.NewGray(image.Rect(0, 0, 2, 2)), nil); err != nil {
		t.Fatal(err)
	}
	row := map[string]any{"row_ID": "synthetic-row", "ori_TYPE": "A", "real_INSPECT_DATE": "115/10/01", "hosp_ABBR": "合成院所", "order_NAME": "合成影像"}
	s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic" {
			t.Error("official document authentication missing")
		}
		switch r.URL.Path {
		case "/api/ihke3000/ihke3408s01/page_load":
			respondJSON(t, w, map[string]any{"sp_IHKE3408S01_data": []any{row}})
		case "/api/ihke3000/ihke3408s01/search":
			respondJSON(t, w, []any{row})
		case "/api/ihke3000/IHKE3408S02/page_load":
			if r.URL.Query().Get("crid") != "synthetic-row" || r.URL.Query().Get("ctype") != "A" {
				t.Error("imaging detail did not use the bound row")
			}
			respondJSON(t, w, map[string]any{"ihke3408S02_main_data": []any{map[string]any{"desc": "合成原文 <3；<script>不執行</script>", "jpg_STATUS": "1", "ipl_CASE_SEQ_NO": "private-synthetic-sequence", "img_ORI_SIZE": "2 MB", "ID_NO": "DO_NOT_FORWARD"}}})
		case "/api/ihke3000/IHKE3408S03/page_load":
			pictures.Add(1)
			if r.URL.Query().Get("IPL_CASE_SEQ_NO") != "private-synthetic-sequence" {
				t.Error("picture reference was replaced by caller input")
			}
			encoded := base64.StdEncoding.EncodeToString(picture.Bytes())
			respondJSON(t, w, map[string]any{"pics": []string{encoded, encoded}})
		case "/api/ihke3000/IHKE3408S02/add":
			generated.Add(1)
			if r.Method != http.MethodPost {
				t.Error("generation must be explicit POST")
			}
			var payload map[string]string
			if json.NewDecoder(r.Body).Decode(&payload) != nil || len(payload) != 1 || payload["ipl_CASE_SEQ_NO"] != "private-synthetic-sequence" {
				t.Error("generation sent unbound source identifiers")
			}
			respondJSON(t, w, map[string]any{"status": "Y", "message": "DO_NOT_FORWARD"})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})
	s.token, s.tokenObtainedAt = "synthetic", time.Now()
	list, err := s.FetchSupplement(context.Background(), "imaging", Query{})
	if err != nil || len(list.Entries) != 2 || len(list.Entries[0].DetailID) != 48 || list.Entries[0].ContractID == list.Entries[1].ContractID || list.Entries[0].DetailID == list.Entries[1].DetailID {
		t.Fatalf("missing imaging detail reference: %v", err)
	}
	detail, err := s.FetchImagingDetail(context.Background(), list.Entries[0].DetailID)
	if err != nil || len(detail.Entries) != 1 {
		t.Fatalf("detail failed: %v", err)
	}
	entry := detail.Entries[0]
	if len(entry.Attachments) != 1 || len(entry.DocumentActions) != 1 || generated.Load() != 0 || pictures.Load() != 0 {
		t.Fatal("retrieval lost ready file/action or automatically generated/downloaded images")
	}
	wire, err := json.Marshal(detail)
	if err != nil || !bytes.Contains(wire, []byte("合成原文")) || bytes.Contains(wire, []byte("DO_NOT_FORWARD")) || bytes.Contains(wire, []byte("private-synthetic-sequence")) {
		t.Fatal("report text lost or source identifiers leaked")
	}
	var info DocumentInfo
	var output bytes.Buffer
	if err := s.DownloadDocument(context.Background(), "hospital", entry.Attachments[0].ID, collectDocument(&info, &output)); err == nil || pictures.Load() != 0 {
		t.Fatal("cross-category document fetched")
	}
	if err := s.DownloadDocument(context.Background(), "imaging", entry.Attachments[0].ID, collectDocument(&info, &output)); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(output.Bytes()), int64(output.Len()))
	if err != nil || len(archive.File) != 2 || info.ContentType != "application/zip" || info.Size != int64(output.Len()) {
		t.Fatal("JPEG set not completely packaged")
	}
	for _, file := range archive.File {
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(reader)
		reader.Close()
		if err != nil || !bytes.Equal(data, picture.Bytes()) || strings.Contains(file.Name, "synthetic") {
			t.Fatal("source JPEG bytes or private archive paths changed")
		}
	}
	result, err := s.PrepareDocument(context.Background(), "imaging", entry.DocumentActions[0].ID)
	if err != nil || result.Status != "requested" || generated.Load() != 1 {
		t.Fatalf("explicit generation failed: %v", err)
	}
	if _, err := s.PrepareDocument(context.Background(), "imaging", entry.DocumentActions[0].ID); err == nil || generated.Load() != 1 {
		t.Fatal("consumed generation reference was replayed")
	}
	if _, err := s.FetchSupplement(context.Background(), "imaging", Query{}); err != nil {
		t.Fatal(err)
	}
	if err := s.DownloadDocument(context.Background(), "imaging", entry.Attachments[0].ID, collectDocument(&info, &output)); err == nil || pictures.Load() != 1 {
		t.Fatal("superseded imaging references remained usable")
	}
}

func TestJPEGPreparationUsesObservedRowNumberAndRequiresFreshActionAfterFailure(t *testing.T) {
	var calls atomic.Int32
	s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/page_load") {
			respondJSON(t, w, map[string]any{"ihke3408S02_main_data": []any{map[string]any{"desc": "合成", "jpg_STATUS": "A", "rownum": "synthetic-jpeg-row", "ipl_CASE_SEQ_NO": "-"}}})
			return
		}
		calls.Add(1)
		var body map[string]string
		if json.NewDecoder(r.Body).Decode(&body) != nil || body["ipl_CASE_SEQ_NO"] != "synthetic-jpeg-row" {
			t.Error("JPG preparation must use observed rownum, not missing DICOM sequence")
		}
		http.Error(w, "synthetic failure", http.StatusServiceUnavailable)
	})
	s.token, s.tokenObtainedAt = "synthetic", time.Now()
	s.imagingReferences = map[string]westernReference{"synthetic": {rowID: "row", sourceType: "A"}}
	detail, err := s.FetchImagingDetail(context.Background(), "synthetic")
	if err != nil || len(detail.Entries[0].DocumentActions) != 1 || len(detail.Entries[0].Attachments) != 0 {
		t.Fatal("pending JPEG contract missing")
	}
	action := detail.Entries[0].DocumentActions[0]
	if action.Kind != "prepare_jpeg" {
		t.Fatal("wrong preparation kind")
	}
	if _, err := s.PrepareDocument(context.Background(), "imaging", action.ID); err == nil {
		t.Fatal("ambiguous request failure became success")
	}
	if _, err := s.PrepareDocument(context.Background(), "imaging", action.ID); err == nil || calls.Load() != 1 {
		t.Fatal("ambiguous generation was automatically replayable")
	}
}

func TestHospitalDocumentLifecycleAndMarkupIsPlainText(t *testing.T) {
	var cleanup atomic.Int32
	s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/ihke3000/IHKE3309S02/page_load":
			respondJSON(t, w, map[string]any{"ihke3309S02_main_data": []any{map[string]any{"rowid": "synthetic-row", "has_XML": "Y", "has_PDF": "Y"}}})
		case "/api/ihke3000/IHKE3309S02/getxml":
			respondJSON(t, w, map[string]any{"file_name": "<div>合成病摘<script>no()</script></div>"})
		case "/api/ihke3000/IHKE3309S02/getpdf":
			respondJSON(t, w, map[string]any{"file_name": "synthetic-document.pdf"})
		case "/ihke3000/ViewPdf/synthetic-document.pdf":
			if r.Header.Get("Authorization") != "" {
				t.Error("JWT unnecessarily sent to static PDF path")
			}
			_, _ = io.WriteString(w, "%PDF-1.7\nsynthetic fixture\n%%EOF")
		case "/api/ihke3000/IHKE3309S02/delpdf":
			cleanup.Add(1)
			respondJSON(t, w, map[string]any{"status": "Y"})
		default:
			t.Error("unexpected document path")
			http.NotFound(w, r)
		}
	})
	s.token, s.tokenObtainedAt = "synthetic", time.Now()
	s.medicalReferences = map[string]map[string]medicalReference{"hospital": {"reference": {category: "hospital", rowID: "synthetic-row", sourceType: "A"}}}
	detail, err := s.FetchMedicalDetail(context.Background(), "hospital", "reference")
	if err != nil || len(detail.Entries[0].Attachments) != 2 || cleanup.Load() != 0 {
		t.Fatalf("document availability failed: %v", err)
	}
	for index, attachment := range detail.Entries[0].Attachments {
		var info DocumentInfo
		var output bytes.Buffer
		if err := s.DownloadDocument(context.Background(), "hospital", attachment.ID, collectDocument(&info, &output)); err != nil {
			t.Fatal(err)
		}
		if index == 0 && (info.ContentType != "text/plain;charset=utf-8" || !strings.HasSuffix(info.Name, ".txt") || !strings.Contains(output.String(), "<script>")) {
			t.Fatal("source-rendered markup was executed, stripped or mislabeled as XML")
		}
		if index == 1 && (info.ContentType != "application/pdf" || !strings.HasPrefix(output.String(), "%PDF-") || cleanup.Load() != 1 || len(info.Warnings) != 1 || info.Warnings[0] != "official_temporary_cleanup_unconfirmed") {
			t.Fatal("PDF bytes or temporary cleanup missing")
		}
	}
	s.clearAuth()
	s.token, s.tokenObtainedAt = "another-synthetic", time.Now()
	if len(s.documentReferences) != 0 {
		t.Fatal("document references survived authentication reset")
	}
}

func TestOfficialPDFPathRejectsCrossOriginAndTraversal(t *testing.T) {
	for _, input := range []string{"https://example.invalid/private.pdf", "//example.invalid/x.pdf", "/api/ihke3000/delete.pdf", "../../../../secret.pdf", "file:///a.pdf", "x.pdf?token=secret", "x.pdf#fragment", "x\\a.pdf", "x%2f..%2f..%2fprivate.pdf", "x.txt", "\x00.pdf", "/ihke3000/%2e%2e%5capi/private.pdf", "/ihke3000/x%00.pdf", "/ihke3000/x%2500.pdf"} {
		if _, err := officialPDFPath(input); err == nil {
			t.Errorf("unsafe upstream PDF path accepted: %q", input)
		}
	}
	for _, input := range []string{"synthetic.pdf", "../temp/synthetic.pdf", "/IHKE3000/temp/synthetic.PDF", "https://myhealthbank.nhi.gov.tw/ihke3000/temp/synthetic.pdf"} {
		if _, err := officialPDFPath(input); err != nil {
			t.Errorf("same-origin PDF path rejected: %q", input)
		}
	}
}

func TestPDFCleanupHTTP200DoesNotProveSourceDeletion(t *testing.T) {
	s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/ihke3000/IHKE3309S02/getpdf":
			respondJSON(t, w, map[string]string{"file_name": "synthetic.pdf"})
		case "/ihke3000/ViewPdf/synthetic.pdf":
			_, _ = io.WriteString(w, "%PDF-1.7\nsynthetic\n%%EOF")
		case "/api/ihke3000/IHKE3309S02/delpdf":
			respondJSON(t, w, map[string]string{"status": "N", "error_code": "D002"})
		default:
			t.Error("unexpected PDF lifecycle request")
		}
	})
	s.token, s.tokenObtainedAt = "synthetic", time.Now()
	s.documentReferences = map[string]documentReference{"ref": {category: "hospital", kind: "hospital_pdf", rowID: "synthetic-row", sourceType: "A"}}
	var info DocumentInfo
	var output bytes.Buffer
	if err := s.DownloadDocument(context.Background(), "hospital", "ref", collectDocument(&info, &output)); err != nil {
		t.Fatal(err)
	}
	if len(info.Warnings) != 1 || info.Warnings[0] != "official_temporary_cleanup_unconfirmed" || output.Len() == 0 {
		t.Fatal("HTTP200 cleanup failure was mislabeled as confirmed deletion or discarded the fetched PDF")
	}
}

func TestProviderPrescriptionAndBoneDensityDocumentBindings(t *testing.T) {
	t.Run("electronic_prescription", func(t *testing.T) {
		s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
			query := r.URL.Query()
			if r.URL.Path != "/api/ihke3000/ihke3212s01/IHKE3212_pdf" || r.Method != http.MethodGet || len(query) != 3 || query.Get("tid") != "synthetic-prescription" || query.Get("tid2") != "synthetic-prescription" || query.Get("recipeType") != "synthetic-type" {
				t.Error("electronic prescription PDF contract changed")
			}
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = io.WriteString(w, "%PDF-1.7\nsynthetic\n%%EOF")
		})
		s.token, s.tokenObtainedAt = "synthetic", time.Now()
		report := SupplementReport{Entries: []SupplementEntry{{Title: "合成電子處方"}}}
		if err := s.attachProviderDocuments("e_prescriptions", "parent", providerReference{rowID: "synthetic-prescription", sourceType: "synthetic-type"}, nil, &report); err != nil {
			t.Fatal(err)
		}
		if len(report.Entries[0].Attachments) != 1 {
			t.Fatal("electronic prescription PDF reference missing")
		}
		var info DocumentInfo
		var output bytes.Buffer
		if err := s.DownloadDocument(context.Background(), "e_prescriptions", report.Entries[0].Attachments[0].ID, collectDocument(&info, &output)); err != nil || info.ContentType != "application/pdf" || !strings.HasPrefix(output.String(), "%PDF-") {
			t.Fatalf("electronic prescription download failed: %v", err)
		}
	})
	t.Run("bone_density", func(t *testing.T) {
		var picture bytes.Buffer
		if err := jpeg.Encode(&picture, image.NewGray(image.Rect(0, 0, 2, 2)), nil); err != nil {
			t.Fatal(err)
		}
		var prepared atomic.Int32
		s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/ihke3000/IHKE3411S02/add":
				prepared.Add(1)
				var input map[string]string
				if r.Method != http.MethodPost || json.NewDecoder(r.Body).Decode(&input) != nil || input["ipl_CASE_SEQ_NO"] != "synthetic-row-number" {
					t.Error("bone-density preparation contract changed")
				}
				respondJSON(t, w, map[string]string{"status": "Y"})
			case "/api/ihke3000/IHKE3411S03/page_load":
				if r.URL.Query().Get("IPL_CASE_SEQ_NO") != "synthetic-bone-sequence" {
					t.Error("bone-density image source changed")
				}
				respondJSON(t, w, map[string]any{"pics": []string{base64.StdEncoding.EncodeToString(picture.Bytes())}})
			default:
				t.Error("bone-density document used an imaging-category endpoint")
				http.NotFound(w, r)
			}
		})
		s.token, s.tokenObtainedAt = "synthetic", time.Now()
		report := SupplementReport{Entries: []SupplementEntry{{Title: "合成待產製影像"}, {Title: "合成可下載影像"}}}
		object := wireRow{"ihke3411s02_main_data": json.RawMessage(`[{"jpg_STATUS":"A","rownum":"synthetic-row-number"},{"jpg_STATUS":"1","ipl_CASE_SEQ_NO":"synthetic-bone-sequence"}]`)}
		if err := s.attachProviderDocuments("osteoporosis", "parent", providerReference{}, object, &report); err != nil {
			t.Fatal(err)
		}
		if len(report.Entries[0].DocumentActions) != 1 || len(report.Entries[1].Attachments) != 1 || prepared.Load() != 0 {
			t.Fatal("bone-density availability was lost or preparation was automatic")
		}
		if _, err := s.PrepareDocument(context.Background(), "osteoporosis", report.Entries[0].DocumentActions[0].ID); err != nil {
			t.Fatal(err)
		}
		var info DocumentInfo
		var output bytes.Buffer
		if err := s.DownloadDocument(context.Background(), "osteoporosis", report.Entries[1].Attachments[0].ID, collectDocument(&info, &output)); err != nil || info.ContentType != "application/zip" {
			t.Fatalf("bone-density JPEG failed: %v", err)
		}
	})
}

func TestOfficialFileListDetailAndBoundedDownload(t *testing.T) {
	var downloaded atomic.Int32
	zipBytes := []byte{'P', 'K', 5, 6, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/ihke3000/ihke3801s01/page_load":
			respondJSON(t, w, map[string]any{"sp_IHKE3801S01_data": []any{map[string]any{"seq_NO": "synthetic-job", "title": "DCM", "txt_DATE": "115/10/28", "status": "可下載", "flag": "5"}}})
		case "/api/ihke3000/ihke3801s02/imgF_page_load":
			if r.URL.Query().Get("seq_no") != "synthetic-job" || r.URL.Query().Get("type") != "DCM" || r.URL.Query().Get("txtdate") != "115/10/28" {
				t.Error("job detail inputs changed")
			}
			respondJSON(t, w, map[string]any{"status": "1", "real_INSPECT_DATE": "115/01/01", "order_CODE_NAME": "合成影像", "img_SIZE": "22 bytes"})
		case "/api/ihke3000/ihke3801s02/imgF_download":
			downloaded.Add(1)
			if len(r.URL.Query()) != 1 || r.URL.Query().Get("seq_no") != "synthetic-job" {
				t.Error("unobserved DICOM download inputs")
			}
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(zipBytes)
		default:
			t.Error("unexpected file service path")
			http.NotFound(w, r)
		}
	})
	s.token, s.tokenObtainedAt = "synthetic", time.Now()
	list, err := s.FetchOfficialFiles(context.Background())
	if err != nil || list.Complete || len(list.Entries) != 1 || list.StartDate != "" {
		t.Fatalf("official job list failed: %v", err)
	}
	detail, err := s.FetchOfficialFileDetail(context.Background(), list.Entries[0].DetailID)
	if err != nil || len(detail.Entries[0].Attachments) != 1 {
		t.Fatalf("official job detail failed: %v", err)
	}
	var info DocumentInfo
	var output bytes.Buffer
	if err := s.DownloadDocument(context.Background(), "official_files", detail.Entries[0].Attachments[0].ID, collectDocument(&info, &output)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output.Bytes(), zipBytes) || info.Size != int64(len(zipBytes)) || downloaded.Load() != 1 {
		t.Fatal("source archive was transformed or incompletely transferred")
	}
	if _, err := s.FetchOfficialFiles(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.DownloadDocument(context.Background(), "official_files", detail.Entries[0].Attachments[0].ID, collectDocument(&info, &output)); err == nil || downloaded.Load() != 1 {
		t.Fatal("superseded file reference remained usable")
	}
}

func TestDocumentReaderNeverReportsPartialSuccess(t *testing.T) {
	for _, tc := range []struct {
		name, value   string
		limit, length int64
		want          string
	}{
		{"exact_known", "abcdef", 6, 6, ""},
		{"exact_unknown", "abcdef", 6, -1, ""},
		{"over_limit", "abcdefg", 6, -1, "RESPONSE_TOO_LARGE"},
		{"truncated", "abc", 6, 6, "DOCUMENT_INCOMPLETE"},
		{"unexpected_length", "abcd", 6, 3, "DOCUMENT_INCOMPLETE"},
		{"truncated_at_limit", "abcdef", 6, 7, "DOCUMENT_INCOMPLETE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &documentReader{reader: strings.NewReader(tc.value), limit: tc.limit, expected: tc.length}
			_, err := io.Copy(io.Discard, r)
			if tc.want == "" {
				if err != nil || !r.finished {
					t.Fatal("complete bounded source failed")
				}
			} else {
				var upstream *Error
				if !errors.As(err, &upstream) || upstream.Code != tc.want || r.finished {
					t.Fatalf("partial file passed: %v", err)
				}
			}
		})
	}
}

func TestDocumentDownloadCancellationAndRedirectDoNotDeliverSuccess(t *testing.T) {
	t.Run("canceled", func(t *testing.T) {
		s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/zip")
			_, _ = io.WriteString(w, "PK\x03\x04synthetic-prefix")
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		})
		s.token, s.tokenObtainedAt = "synthetic", time.Now()
		s.documentReferences = map[string]documentReference{"ref": {category: "official_files", kind: "official_archive", sequence: "synthetic"}}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		err := s.DownloadDocument(ctx, "official_files", "ref", func(_ DocumentInfo, reader io.Reader) error {
			cancel()
			_, err := io.Copy(io.Discard, reader)
			return err
		})
		if err == nil {
			t.Fatal("canceled source file transfer reported success")
		}
	})
	t.Run("redirect", func(t *testing.T) {
		var calls atomic.Int32
		s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.Header().Set("Location", "https://myhealthbank.nhi.gov.tw/ihke3000/other.pdf")
			w.WriteHeader(http.StatusFound)
		})
		s.token, s.tokenObtainedAt = "synthetic", time.Now()
		s.documentReferences = map[string]documentReference{"ref": {category: "official_files", kind: "official_archive", sequence: "synthetic"}}
		consumed := false
		err := s.DownloadDocument(context.Background(), "official_files", "ref", func(DocumentInfo, io.Reader) error { consumed = true; return nil })
		var upstream *Error
		if !errors.As(err, &upstream) || upstream.Code != "UPSTREAM_REDIRECT_REJECTED" || calls.Load() != 1 || consumed {
			t.Fatalf("artifact API redirected authenticated headers: %v", err)
		}
	})
}

func TestDocumentDownloadsRejectBadResponsesBeforeConsumer(t *testing.T) {
	for _, tc := range []struct {
		name, contentType, body, length string
		status                          int
		want                            string
	}{
		{"html", "text/html", "<html>sign in</html>", "", 200, "DOCUMENT_INVALID"},
		{"wrong_magic", "application/zip", "not a zip", "", 200, "DOCUMENT_INVALID"},
		{"oversize", "application/zip", "PK\x03\x04synthetic", "524288001", 200, "RESPONSE_TOO_LARGE"},
		{"expired", "application/json", `{}`, "", 401, "SESSION_EXPIRED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				if tc.length != "" {
					w.Header().Set("Content-Length", tc.length)
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			})
			s.token, s.tokenObtainedAt = "synthetic", time.Now()
			s.documentReferences = map[string]documentReference{"ref": {category: "official_files", kind: "official_archive", sequence: "synthetic"}}
			called := false
			err := s.DownloadDocument(context.Background(), "official_files", "ref", func(DocumentInfo, io.Reader) error { called = true; return nil })
			var upstream *Error
			if !errors.As(err, &upstream) || upstream.Code != tc.want || called {
				t.Fatalf("invalid file reached consumer: %v", err)
			}
			if tc.status == 401 && (s.token != "" || len(s.documentReferences) != 0) {
				t.Fatal("expired document authentication was retained")
			}
		})
	}
}
