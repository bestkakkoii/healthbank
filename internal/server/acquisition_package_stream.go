// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package server

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/bestkakkoii/healthbank/internal/nhi"
)

type packageOutput struct {
	target io.Writer
	hash   hash.Hash
	bytes  int64
	err    error
}

func (w *packageOutput) Write(data []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	n, err := w.target.Write(data)
	if n > 0 {
		_, _ = w.hash.Write(data[:n])
		w.bytes += int64(n)
	}
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	if err != nil {
		w.err = err
	}
	return n, err
}

type packageProgressWriter struct {
	target io.Writer
	p      *acquisitionPackage
	index  int
}

func (w packageProgressWriter) Write(data []byte) (int, error) {
	n, err := w.target.Write(data)
	w.p.mu.Lock()
	w.p.status.Files[w.index].Bytes += int64(n)
	w.p.status.Bytes += int64(n)
	w.p.mu.Unlock()
	return n, err
}

func packageFilename(value string) string {
	var name strings.Builder
	for _, r := range value {
		if unicode.IsControl(r) || strings.ContainsRune(`/\<>:"|?*`, r) {
			name.WriteByte('_')
		} else {
			name.WriteRune(r)
		}
	}
	result := strings.Trim(name.String(), " .")
	if result == "" {
		result = "document.bin"
	}
	if len(result) > 180 {
		suffix := path.Ext(result)
		if len(suffix) > 20 {
			suffix = ""
		}
		limit := 180 - len(suffix)
		result = result[:limit]
		for !utf8.ValidString(result) {
			result = result[:len(result)-1]
		}
		result = strings.TrimRight(result, " .") + suffix
	}
	base := strings.ToUpper(strings.TrimSuffix(result, path.Ext(result)))
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" ||
		(len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9') {
		result = "_" + result
	}
	return result
}
func packageZipName(p *acquisitionPackage) string {
	return fmt.Sprintf("healthbank-acquisition-%s-part%d.zip", p.input.Plan.ID, p.input.Plan.Index)
}
func safePackageError(err error) string {
	if errors.Is(err, context.Canceled) {
		return "request_cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "budget_exceeded"
	}
	var upstream *nhi.Error
	if errors.As(err, &upstream) {
		switch upstream.Code {
		case "SESSION_EXPIRED", "AUTH_FAILED", "CLOSED", "INVALID_INPUT", "RESOURCE_LIMIT", "DOCUMENT_INVALID",
			"DOCUMENT_NOT_READY", "DOCUMENT_INCOMPLETE", "RESPONSE_TOO_LARGE", "INCOMPLETE_RESPONSE",
			"NETWORK", "DNS_FAILED", "TIMEOUT", "TLS_CERTIFICATE", "TLS_FAILED", "CONNECTION_REFUSED",
			"CONNECTION_RESET", "CONNECTION_FAILED", "REQUEST_CANCELED", "UPSTREAM_REDIRECT_REJECTED",
			"RESPONSE_INTERRUPTED", "UPSTREAM_ACCESS_DENIED", "UPSTREAM_UNAVAILABLE", "UPSTREAM_CHANGED", "UPSTREAM_REJECTED":
			return upstream.Code
		}
	}
	return "document_failed"
}
func createPackageMember(archive *zip.Writer, name string, method uint16, when time.Time) (io.Writer, error) {
	header := &zip.FileHeader{Name: name, Method: method, Modified: when.UTC()}
	header.SetMode(0600)
	return archive.CreateHeader(header)
}

type packageDocumentTransfer func(context.Context, string, string, func(nhi.DocumentInfo, io.Reader) error) error

// streamAcquisitionPackage has no filesystem dependency. It hashes exactly the
// bytes written and records source failures in the final manifest. A downstream
// write failure produces no valid-completion claim or whole-package digest.
func (a *App) streamAcquisitionPackage(ctx context.Context, p *acquisitionPackage, target io.Writer,
	transfer packageDocumentTransfer, workingSet func(string, string) int64) (bool, error) {
	output := &packageOutput{target: target, hash: sha256.New()}
	archive := zip.NewWriter(output)
	now := time.Now()
	p.mu.Lock()
	input := p.input
	p.mu.Unlock()
	dataset := struct {
		Name     string `json:"name"`
		Size     int64  `json:"size"`
		SHA256   string `json:"sha256"`
		Included bool   `json:"included"`
	}{Name: "healthbank.dataset.v2.json", Included: len(input.Dataset) > 0}
	if input.ExternalDataset != nil {
		dataset.Name, dataset.Size, dataset.SHA256 = input.ExternalDataset.Name, input.ExternalDataset.Size, input.ExternalDataset.SHA256
	} else {
		dataset.Size = int64(len(input.Dataset))
		digest := sha256.Sum256(input.Dataset)
		dataset.SHA256 = hex.EncodeToString(digest[:])
		member, err := createPackageMember(archive, dataset.Name, zip.Deflate, now)
		if err != nil {
			return false, err
		}
		if _, err := member.Write(input.Dataset); err != nil {
			return false, err
		}
	}
	instructions, err := createPackageMember(archive, "README.txt", zip.Deflate, now)
	if err != nil {
		return false, err
	}
	if _, err := io.WriteString(instructions, "Healthbank 完整取得資料包\nCopyright (c) 2026 bestkakkoii llc.\n\n"+
		"請先閱讀 manifest.json：completed 僅代表本包所選檔案傳輸完成，不能證明官方終身資料完整。\n"+
		"failed、skipped、cancelled 的檔案不可當作完整原始檔；即使 ZIP 可解壓，仍須核對逐檔狀態及 SHA-256。\n"+
		"若 dataset.included 為 false，請保留另存的完整 JSON，並核對 manifest 中的檔名、位元組長度及 SHA-256。\n"+
		"多包計畫須保留所有分包；此檔案只代表 plan.index 所列部分。資料及原始文件的授權不因程式採 MIT 而改變。\n"+
		"下載傳輸完成不等於已證明裝置保存成功；請確認自己的下載檔案。\n"); err != nil {
		return false, err
	}
	authExpired := false
	stopCode := ""
	buffer := make([]byte, 32768)
	for index, file := range input.Files {
		if ctx.Err() != nil {
			stopCode = safePackageError(ctx.Err())
		}
		p.mu.Lock()
		cancelled := p.status.State == "cancelled"
		remaining := maxPackageOriginalBytes - p.status.Bytes
		p.mu.Unlock()
		if cancelled {
			stopCode = "request_cancelled"
		}
		if stopCode == "" && remaining < nhi.MaxDocumentBytes {
			// Reserve a full unknown-length original before opening it, so a
			// package limit never truncates an otherwise valid source artifact.
			stopCode = "budget_exceeded"
		}
		if stopCode != "" {
			p.mu.Lock()
			state := "skipped"
			if stopCode == "request_cancelled" {
				state = "cancelled"
			}
			p.status.Files[index].State, p.status.Files[index].Code = state, stopCode
			p.mu.Unlock()
			continue
		}
		release, admitted := a.budget.acquireWorking(workingSet(file.Category, file.ID), true)
		if !admitted {
			p.mu.Lock()
			p.status.Files[index].State, p.status.Files[index].Code = "failed", "resource_capacity"
			p.status.Failed++
			p.mu.Unlock()
			continue
		}
		p.mu.Lock()
		p.status.Files[index].State = "streaming"
		p.mu.Unlock()
		digest := sha256.New()
		fileCtx, cancel := context.WithTimeout(ctx, nhi.DocumentDownloadBudget)
		called := false
		transferErr := transfer(fileCtx, file.Category, file.ID, func(info nhi.DocumentInfo, reader io.Reader) error {
			if called {
				return errors.New("duplicate document stream")
			}
			called = true
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if info.Size > nhi.MaxDocumentBytes {
				return &nhi.Error{Code: "RESPONSE_TOO_LARGE"}
			}
			name := "originals/" + file.ArchiveID + "/" + packageFilename(info.Name)
			member, err := createPackageMember(archive, name, zip.Store, now)
			if err != nil {
				return err
			}
			p.mu.Lock()
			p.status.Files[index].Path = name
			p.status.Files[index].ContentType = info.ContentType
			// Only the connector's fixed warning code is exported, never raw
			// upstream warning strings or response data.
			for _, warning := range info.Warnings {
				if warning == "official_temporary_cleanup_unconfirmed" {
					p.status.Files[index].Warnings = []string{warning}
				}
			}
			p.mu.Unlock()
			limited := &io.LimitedReader{R: reader, N: nhi.MaxDocumentBytes}
			n, copyErr := io.CopyBuffer(packageProgressWriter{target: io.MultiWriter(member, digest), p: p, index: index}, limited, buffer)
			if copyErr != nil {
				return copyErr
			}
			if limited.N == 0 {
				var extra [1]byte
				extraN, extraErr := reader.Read(extra[:])
				if extraN != 0 || extraErr == nil {
					return &nhi.Error{Code: "RESPONSE_TOO_LARGE"}
				}
				if extraErr != io.EOF {
					return extraErr
				}
			}
			if info.Size >= 0 && n != info.Size {
				return &nhi.Error{Code: "DOCUMENT_INCOMPLETE"}
			}
			return nil
		})
		cancel()
		release()
		if output.err != nil {
			return authExpired, output.err
		}
		if !called && transferErr == nil {
			transferErr = errors.New("missing document stream")
		}
		p.mu.Lock()
		if called {
			p.status.Files[index].SHA256 = hex.EncodeToString(digest.Sum(nil))
		}
		if transferErr != nil {
			code := safePackageError(transferErr)
			p.status.Files[index].State, p.status.Files[index].Code = "failed", code
			p.status.Failed++
			if code == "SESSION_EXPIRED" || code == "AUTH_FAILED" || code == "CLOSED" {
				authExpired, stopCode = true, "auth_expired"
			}
		} else {
			p.status.Files[index].State = "completed"
			p.status.Completed++
		}
		p.mu.Unlock()
	}
	if ctx.Err() != nil {
		return authExpired, ctx.Err()
	}
	p.mu.Lock()
	if p.status.State == "cancelled" {
		p.mu.Unlock()
		return authExpired, context.Canceled
	}
	transferState := "completed"
	if p.status.Completed != p.status.Total {
		transferState = "partial"
	}
	view := p.status
	view.Files = append([]packageFileStatus{}, p.status.Files...)
	p.mu.Unlock()
	// No live document handles, cookie, CSRF, source URLs or pseudonym are
	// serialized. Local archive IDs are the dataset's exact attachment mapping.
	type manifestFile struct {
		Category string `json:"category"`
		Usable   bool   `json:"usable"`
		packageFileStatus
	}
	files := make([]manifestFile, len(view.Files))
	for i, item := range view.Files {
		files[i] = manifestFile{Category: input.Files[i].Category, Usable: item.State == "completed", packageFileStatus: item}
	}
	manifest := struct {
		Schema              string             `json:"schema"`
		CreatedAt           string             `json:"created_at"`
		State               string             `json:"state"`
		Plan                packagePlan        `json:"plan"`
		Dataset             any                `json:"dataset"`
		Acquisition         packageAcquisition `json:"acquisition"`
		Files               []manifestFile     `json:"files"`
		HistoryCompleteness string             `json:"history_completeness"`
	}{
		Schema: "healthbank.acquisition.package.v1", CreatedAt: now.UTC().Format(time.RFC3339), State: transferState,
		Plan: input.Plan, Dataset: dataset, Acquisition: input.Acquisition, Files: files,
		HistoryCompleteness: "preserved_in_dataset_and_source_statuses_not_inferred_from_transfer",
	}
	member, err := createPackageMember(archive, "manifest.json", zip.Deflate, now)
	if err != nil {
		return authExpired, err
	}
	if err := json.NewEncoder(member).Encode(manifest); err != nil {
		return authExpired, err
	}
	if err := archive.Close(); err != nil {
		return authExpired, err
	}
	if output.err != nil {
		return authExpired, output.err
	}
	p.mu.Lock()
	if p.status.State != "cancelled" {
		p.status.State = transferState
		p.status.PackageSHA256 = hex.EncodeToString(output.hash.Sum(nil))
		p.status.PackageBytes = output.bytes
	}
	p.mu.Unlock()
	return authExpired, nil
}

func (a *App) downloadAcquisitionPackage(w http.ResponseWriter, r *http.Request, s *session) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		a.fail(w, r, 405, "method_not_allowed", "資料包下載請使用 GET。")
		return
	}
	if s.mode != "official" || s.official == nil || s.subject == "" {
		a.fail(w, r, 401, "login_required", "請先完成官方登入，再下載資料包。")
		return
	}
	p := a.lookupPackage(w, r, false)
	if p == nil {
		return
	}
	p.mu.Lock()
	if p.owner != s || p.csrf != s.csrf || p.status.State != "ready" {
		p.mu.Unlock()
		a.fail(w, r, 409, "package_consumed", "此資料包已開始下載或已失效；請核對下載紀錄及狀態，不會自動重播。")
		return
	}
	ctx, cancel := context.WithDeadline(r.Context(), p.expires)
	p.cancel, p.streaming, p.status.State = cancel, true, "streaming"
	p.mu.Unlock()
	defer func() {
		cancel()
		p.mu.Lock()
		p.streaming = false
		p.cancel = nil
		p.releaseContent()
		p.mu.Unlock()
	}()
	if err := http.NewResponseController(w).SetWriteDeadline(p.expires.Add(5 * time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		p.mu.Lock()
		p.status.State = "failed"
		p.mu.Unlock()
		a.fail(w, r, 500, "package_transfer_failed", "無法設定有界資料包傳輸。")
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": packageZipName(p)}))
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; frame-ancestors 'none'")
	authExpired, err := a.streamAcquisitionPackage(ctx, p, w, s.official.DownloadDocument, s.official.DocumentWorkingSet)
	if err != nil {
		p.mu.Lock()
		if p.status.State != "cancelled" {
			p.status.State = "failed"
		}
		p.status.PackageSHA256 = ""
		p.mu.Unlock()
		// Never append a JSON error to a broken ZIP, log health content, or
		// certify a body whose central directory was not successfully written.
		panic(http.ErrAbortHandler)
	}
	if authExpired {
		a.clearSession(s)
		a.mu.Lock()
		for id, existing := range a.sessions {
			if existing == s {
				delete(a.sessions, id)
			}
		}
		a.mu.Unlock()
	}
}
