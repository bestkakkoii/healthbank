// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package server

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bestkakkoii/healthbank/internal/nhi"
)

const syntheticPackageDataset = `{"schema":"healthbank.dataset.v2","exported_at":"2026-01-02T00:00:00Z","producer":{},"laboratory":{"schema":"healthbank.report.v1","records":[{"item_name":"SYNTHETIC 合成","result":"0.123456789012345678901"}]},"catalog":[],"data":{"reports":{},"details":{},"states":{},"query":{"from":"","to":""}},"attachments":[]}`

func syntheticPackageInput() packageInput {
	return packageInput{
		Dataset: json.RawMessage(syntheticPackageDataset), Files: []packageFileReference{},
		Plan: packagePlan{ID: strings.Repeat("a", 32), Index: 1, Total: 1},
		Acquisition: packageAcquisition{Scope: "all_available", StartedAt: "2026-01-02T00:00:00Z",
			FinishedAt: "2026-01-02T00:00:01Z", Statuses: []packageAcquisitionStatus{
				{Category: "medicine", Kind: "source", State: "ready", Code: "source_total_unknown", Count: 1},
			}},
	}
}
func packageTestBrowser(t *testing.T) (*App, *browserFixture, *session) {
	t.Helper()
	app, db, _ := testApp(t)
	browser := &browserFixture{app: app}
	browser.refresh(t)
	connector, err := nhi.New()
	if err != nil {
		t.Fatal(err)
	}
	app.mu.Lock()
	s := app.sessions[browser.cookie.Value]
	app.mu.Unlock()
	s.mu.Lock()
	app.bindSubject(s, "SYNTHETIC_PACKAGE_SUBJECT")
	s.mode, s.official = "official", connector
	s.mu.Unlock()
	t.Cleanup(func() { noPersistentProfiles(t, db) })
	return app, browser, s
}
func packageProofRequest(browser *browserFixture, method, target string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "https://healthbank.test"+target, nil)
	request.AddCookie(browser.cookie)
	request.Header.Set("X-CSRF-Token", browser.csrf)
	request.Header.Set("Origin", "https://healthbank.test")
	response := httptest.NewRecorder()
	browser.app.Handler().ServeHTTP(response, request)
	return response
}
func zipPackageContents(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	for _, file := range reader.File {
		stream, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(stream)
		_ = stream.Close()
		if err != nil {
			t.Fatal(err)
		}
		if _, exists := out[file.Name]; exists {
			t.Fatal("duplicate archive path")
		}
		out[file.Name] = body
	}
	return out
}

func TestAcquisitionPackageCreateReplayRecoveryAndNativeDownload(t *testing.T) {
	app, browser, s := packageTestBrowser(t)
	input := syntheticPackageInput()
	created := decodeResponse[packageStatus](t, browser.jsonRequest(t, http.MethodPost, "/api/acquisition/packages", input), http.StatusCreated)
	if !validDocumentReference(created.ID) || created.State != "ready" {
		t.Fatal("missing bounded receipt")
	}
	repeated := decodeResponse[packageStatus](t, browser.jsonRequest(t, http.MethodPost, "/api/acquisition/packages", input), http.StatusOK)
	if repeated.ID != created.ID {
		t.Fatal("ambiguous create replay made another package")
	}
	recovered := decodeResponse[packageStatus](t, packageProofRequest(browser, http.MethodGet,
		"/api/acquisition/packages/status?plan="+input.Plan.ID+"&part=1"), http.StatusOK)
	if recovered.ID != created.ID {
		t.Fatal("lost create cannot recover by exact plan")
	}
	withoutProof := browser.request(http.MethodGet, "/api/acquisition/packages/status?id="+created.ID, nil, "", "")
	if withoutProof.Code != http.StatusConflict {
		t.Fatal("status accepted missing generation proof")
	}
	input.Acquisition.Scope = "selected"
	if response := browser.jsonRequest(t, http.MethodPost, "/api/acquisition/packages", input); response.Code != http.StatusConflict {
		t.Fatal("same plan accepted different package content")
	}
	head := browser.request(http.MethodHead, "/api/acquisition/packages/download?id="+created.ID, nil, "", "")
	if head.Code != http.StatusMethodNotAllowed {
		t.Fatal("HEAD consumed package")
	}
	response := browser.request(http.MethodGet, "/api/acquisition/packages/download?id="+created.ID, nil, "", "")
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/zip" {
		t.Fatal("native package failed")
	}
	members := zipPackageContents(t, response.Body.Bytes())
	if string(members["healthbank.dataset.v2.json"]) != syntheticPackageDataset+"\n" {
		t.Fatal("dataset precision, Unicode or final newline changed")
	}
	status := decodeResponse[packageStatus](t, packageProofRequest(browser, http.MethodGet,
		"/api/acquisition/packages/status?id="+created.ID), http.StatusOK)
	digest := sha256.Sum256(response.Body.Bytes())
	if status.State != "completed" || status.PackageSHA256 != hex.EncodeToString(digest[:]) ||
		status.PackageBytes != int64(response.Body.Len()) {
		t.Fatal("whole archive digest/length does not match actual bytes")
	}
	if response := browser.request(http.MethodGet, "/api/acquisition/packages/download?id="+created.ID, nil, "", ""); response.Code != http.StatusConflict {
		t.Fatal("native download capability replayed")
	}
	app.budget.mu.Lock()
	retained := s.memory.packages
	app.budget.mu.Unlock()
	if retained != 4096 {
		t.Fatal("transferred dataset retained or receipt not accounted")
	}
}

func TestAcquisitionPackageValidationAndGenerationIsolation(t *testing.T) {
	app, browser, s := packageTestBrowser(t)
	input := syntheticPackageInput()
	created := decodeResponse[packageStatus](t, browser.jsonRequest(t, http.MethodPost, "/api/acquisition/packages", input), http.StatusCreated)
	other := &browserFixture{app: app}
	other.refresh(t)
	if response := packageProofRequest(other, http.MethodGet, "/api/acquisition/packages/status?id="+created.ID); response.Code != http.StatusNotFound {
		t.Fatal("another cookie generation read package receipt")
	}
	response := browser.jsonRequest(t, http.MethodPost, "/api/auth/logout", map[string]string{})
	if response.Code != http.StatusOK {
		t.Fatal("logout failed")
	}
	app.mu.Lock()
	remaining := len(app.packages)
	app.mu.Unlock()
	app.budget.mu.Lock()
	retained := s.memory.packages
	app.budget.mu.Unlock()
	if remaining != 0 || retained != 0 {
		t.Fatal("logout kept package medical content")
	}
	for _, change := range []func(*packageInput){
		func(p *packageInput) {
			p.Files = []packageFileReference{{Category: "medicine", ID: strings.Repeat("b", 48), ArchiveID: "../escape"}}
			p.Plan.FilesTotal = 1
		},
		func(p *packageInput) { p.Acquisition.Statuses[0].Code = "SYNTHETIC_PRIVATE_VALUE" },
		func(p *packageInput) { p.Acquisition.Statuses[0].Kind = "arbitrary" },
		func(p *packageInput) { p.Acquisition.Statuses[0].State = "ready|partial" },
		func(p *packageInput) { p.Plan.Index = 2 },
		func(p *packageInput) {
			p.ExternalDataset = &externalPackageDataset{Name: "healthbank.dataset.v2.json", Size: 5, SHA256: strings.Repeat("a", 64)}
		},
		func(p *packageInput) {
			p.Dataset = json.RawMessage(`{"schema":"healthbank.dataset.v2","cookie":"SYNTHETIC_PRIVATE"}`)
		},
		func(p *packageInput) { p.Acquisition.FinishedAt = "2025-01-01T00:00:00Z" },
	} {
		value := syntheticPackageInput()
		change(&value)
		if validatePackageInput(&value) == nil {
			t.Fatal("unsafe package input accepted")
		}
	}
}
func TestAcquisitionPackageExternalDatasetAndResourceRejection(t *testing.T) {
	app, browser, s := packageTestBrowser(t)
	input := syntheticPackageInput()
	input.Dataset = nil
	input.ExternalDataset = &externalPackageDataset{Name: "healthbank.dataset.v2.json", Size: 80 << 20, SHA256: strings.Repeat("c", 64)}
	created := decodeResponse[packageStatus](t, browser.jsonRequest(t, http.MethodPost, "/api/acquisition/packages", input), http.StatusCreated)
	response := browser.request(http.MethodGet, "/api/acquisition/packages/download?id="+created.ID, nil, "", "")
	members := zipPackageContents(t, response.Body.Bytes())
	if _, exists := members["healthbank.dataset.v2.json"]; exists {
		t.Fatal("external data manufactured inside ZIP")
	}
	var manifest struct {
		Dataset struct {
			Included bool
			SHA256   string
			Size     int64
		}
	}
	if json.Unmarshal(members["manifest.json"], &manifest) != nil || manifest.Dataset.Included ||
		manifest.Dataset.Size != 80<<20 || manifest.Dataset.SHA256 != input.ExternalDataset.SHA256 {
		t.Fatal("external dataset binding lost")
	}
	input.Plan.ID = strings.Repeat("d", 32)
	app.budget.mu.Lock()
	app.budget.limits.RetainedBytes = 1
	app.budget.mu.Unlock()
	response = browser.jsonRequest(t, http.MethodPost, "/api/acquisition/packages", input)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "package_retained_capacity") {
		t.Fatal("retained limit bypassed")
	}
	app.budget.mu.Lock()
	retained := s.memory.packages
	app.budget.mu.Unlock()
	if retained != 4096 {
		t.Fatal("rejected package reserved bytes or existing receipt accounting changed")
	}
}

func syntheticStreamPackage(input packageInput) *acquisitionPackage {
	p := &acquisitionPackage{input: input, status: packageStatus{State: "streaming", Total: len(input.Files), Files: make([]packageFileStatus, len(input.Files))}}
	for i, file := range input.Files {
		p.status.Files[i] = packageFileStatus{ArchiveID: file.ArchiveID, State: "queued"}
	}
	return p
}
func TestAcquisitionPackageOriginalHashesPartialFailuresAndPrivacy(t *testing.T) {
	app, _, _ := testApp(t)
	input := syntheticPackageInput()
	input.Plan.FilesTotal = 3
	for i := range 3 {
		input.Files = append(input.Files, packageFileReference{Category: "medicine", ID: strings.Repeat(string(rune('a'+i)), 48), ArchiveID: "archive_" + string(rune('1'+i))})
	}
	p := syntheticStreamPackage(input)
	var output bytes.Buffer
	index := 0
	expired, err := app.streamAcquisitionPackage(context.Background(), p, &output, func(_ context.Context, _, _ string, consume func(nhi.DocumentInfo, io.Reader) error) error {
		index++
		payload := "%PDF-SYNTHETIC 原始檔 " + string(rune('0'+index))
		if err := consume(nhi.DocumentInfo{Name: "../../CON.pdf", ContentType: "application/pdf", Size: int64(len(payload))}, strings.NewReader(payload)); err != nil {
			return err
		}
		if index == 2 {
			return errors.New("SYNTHETIC_PRIVATE_UPSTREAM_BODY")
		}
		return nil
	}, func(string, string) int64 { return 1 << 20 })
	if err != nil || expired {
		t.Fatal(err)
	}
	members := zipPackageContents(t, output.Bytes())
	var manifest struct {
		State string
		Files []struct {
			ArchiveID string `json:"archive_id"`
			Path      string
			SHA256    string
			State     string
			Usable    bool
			Code      string
		}
	}
	if json.Unmarshal(members["manifest.json"], &manifest) != nil || manifest.State != "partial" || len(manifest.Files) != 3 {
		t.Fatal("partial archive status lost")
	}
	for i, file := range manifest.Files {
		data, ok := members[file.Path]
		sum := sha256.Sum256(data)
		if !ok || file.SHA256 != hex.EncodeToString(sum[:]) || strings.Contains(file.Path, "../") {
			t.Fatal("unsafe path or per-file digest mismatch")
		}
		if file.Usable != (i != 1) {
			t.Fatal("failed original called usable")
		}
	}
	encoded := string(members["manifest.json"]) + string(members["README.txt"])
	if strings.Contains(encoded, "SYNTHETIC_PRIVATE") || strings.Contains(encoded, strings.Repeat("a", 48)) {
		t.Fatal("raw upstream error or live handle leaked")
	}
	if p.status.Completed != 2 || p.status.Failed != 1 || p.status.State != "partial" {
		t.Fatal("incorrect receipt totals")
	}
}
func TestAcquisitionPackageAuthFailureStopsRemainingFiles(t *testing.T) {
	app, _, _ := testApp(t)
	input := syntheticPackageInput()
	input.Files = []packageFileReference{{Category: "medicine", ID: strings.Repeat("a", 48), ArchiveID: "archive_1"}, {Category: "medicine", ID: strings.Repeat("b", 48), ArchiveID: "archive_2"}}
	p := syntheticStreamPackage(input)
	calls := 0
	var output bytes.Buffer
	expired, err := app.streamAcquisitionPackage(context.Background(), p, &output, func(context.Context, string, string, func(nhi.DocumentInfo, io.Reader) error) error {
		calls++
		return &nhi.Error{Code: "SESSION_EXPIRED", Message: "SYNTHETIC_PRIVATE"}
	}, func(string, string) int64 { return 1 << 20 })
	if err != nil || !expired || calls != 1 || p.status.Files[1].State != "skipped" || p.status.Files[1].Code != "auth_expired" {
		t.Fatal("expired source continued fetching", err)
	}
	zipPackageContents(t, output.Bytes())
}

type packageBrokenWriter struct{}

func (packageBrokenWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func TestAcquisitionPackageBrokenTransferNeverHasCompletionDigest(t *testing.T) {
	app, _, _ := testApp(t)
	p := syntheticStreamPackage(syntheticPackageInput())
	_, err := app.streamAcquisitionPackage(context.Background(), p, packageBrokenWriter{}, nil, nil)
	if err == nil || p.status.PackageSHA256 != "" || p.status.State == "completed" {
		t.Fatal("broken ZIP certified")
	}
}
func TestAcquisitionPackageStatusAndCancelDoNotWaitForSessionLock(t *testing.T) {
	app, browser, s := packageTestBrowser(t)
	input := syntheticPackageInput()
	created := decodeResponse[packageStatus](t, browser.jsonRequest(t, http.MethodPost, "/api/acquisition/packages", input), http.StatusCreated)
	s.mu.Lock()
	done := make(chan struct{})
	go func() {
		defer close(done)
		response := packageProofRequest(browser, http.MethodGet, "/api/acquisition/packages/status?id="+created.ID)
		if response.Code != http.StatusOK {
			t.Error("status blocked or rejected")
		}
		response = packageProofRequest(browser, http.MethodDelete, "/api/acquisition/packages?id="+created.ID)
		if response.Code != http.StatusOK {
			t.Error("cancel blocked or rejected")
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		s.mu.Unlock()
		t.Fatal("receipt control waited for native transfer gate")
	}
	s.mu.Unlock()
	app.budget.mu.Lock()
	retained := s.memory.packages
	app.budget.mu.Unlock()
	if retained != 0 {
		t.Fatal("cancel retained dataset")
	}
}
