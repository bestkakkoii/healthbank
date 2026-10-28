// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bestkakkoii/healthbank/internal/strictjson"
)

const (
	maxPackageDataset             = 32 << 20
	maxExternalDataset            = 128 << 20
	maxPackageBody                = 34 << 20
	maxPackageFiles               = 1000
	maxPackageReceipts            = 16
	packageLifetime               = 30 * time.Minute
	maxPackageOriginalBytes int64 = 8 << 30
)

var (
	packageCategoryPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,79}$`)
	packageArchivePattern  = regexp.MustCompile(`^archive_[1-9][0-9]{0,8}$`)
	packagePlanPattern     = regexp.MustCompile(`^[a-f0-9]{32}$`)
	packageDigestPattern   = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

type packagePlan struct {
	ID         string `json:"id"`
	Index      int    `json:"index"`
	Total      int    `json:"total"`
	FilesTotal int    `json:"files_total"`
}
type packageFileReference struct {
	Category  string `json:"category"`
	ID        string `json:"id"`
	ArchiveID string `json:"archive_id"`
}
type externalPackageDataset struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}
type packageAcquisitionStatus struct {
	Category string `json:"category"`
	Kind     string `json:"kind"`
	State    string `json:"state"`
	Count    int    `json:"count,omitempty"`
	Code     string `json:"code,omitempty"`
}
type packageAcquisition struct {
	Scope      string                     `json:"scope"`
	StartedAt  string                     `json:"started_at"`
	FinishedAt string                     `json:"finished_at"`
	Statuses   []packageAcquisitionStatus `json:"statuses"`
}
type packageInput struct {
	Dataset         json.RawMessage         `json:"dataset,omitempty"`
	ExternalDataset *externalPackageDataset `json:"external_dataset,omitempty"`
	Files           []packageFileReference  `json:"files"`
	Acquisition     packageAcquisition      `json:"acquisition"`
	Plan            packagePlan             `json:"plan"`
}
type packageFileStatus struct {
	ArchiveID   string   `json:"archive_id"`
	State       string   `json:"state"`
	Bytes       int64    `json:"bytes"`
	SHA256      string   `json:"sha256,omitempty"`
	Code        string   `json:"code,omitempty"`
	Path        string   `json:"path,omitempty"`
	ContentType string   `json:"content_type,omitempty"`
	Warnings    []string `json:"warnings,omitempty"`
}
type packageStatus struct {
	ID            string              `json:"id"`
	State         string              `json:"state"`
	Total         int                 `json:"total"`
	Completed     int                 `json:"completed"`
	Failed        int                 `json:"failed"`
	Bytes         int64               `json:"bytes"`
	Files         []packageFileStatus `json:"files"`
	ExpiresAt     string              `json:"expires_at"`
	PackageSHA256 string              `json:"package_sha256,omitempty"`
	PackageBytes  int64               `json:"package_bytes"`
}
type acquisitionPackage struct {
	mu                            sync.Mutex
	id, cookie, csrf, requestHash string // immutable, never included in the archive
	owner                         *session
	scope                         *subjectScope
	input                         packageInput
	expires                       time.Time
	status                        packageStatus
	cancel                        context.CancelFunc
	streaming                     bool
	retired                       bool
	memoryBytes                   int64
}

// Package retention participates in the same per-session and process budget as
// reports and source references. It is transient, never a persistent job queue.
func (m *sessionMemory) adjustPackages(delta int64) bool {
	if m == nil {
		return true
	}
	b := m.budget
	b.mu.Lock()
	defer b.mu.Unlock()
	if m.packages+delta < 0 {
		return false
	}
	if delta > 0 && (delta > b.limits.RetainedBytes-b.retained ||
		delta > b.limits.SessionBytes-m.reports-m.references-m.packages) {
		b.refusals++
		return false
	}
	m.packages += delta
	b.retained += delta
	return true
}

// Requires p.mu. Readers never retain a view of dataset bytes beyond a transfer.
func (p *acquisitionPackage) releaseContent() {
	clear(p.input.Dataset)
	p.input.Dataset = nil
	p.input.Acquisition.Statuses = nil
	p.input.Files = nil
	remaining := int64(len(p.status.Files)*1024 + 4096)
	if p.retired {
		remaining = 0
		p.status.Files = nil
	}
	if p.memoryBytes > remaining {
		_ = p.owner.memory.adjustPackages(remaining - p.memoryBytes)
		p.memoryBytes = remaining
	}
}
func (p *acquisitionPackage) retire() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.retired = true
	if p.cancel != nil {
		p.cancel()
	}
	p.status.State = "cancelled"
	if !p.streaming {
		p.releaseContent()
	}
}
func (p *acquisitionPackage) cancelTransfer() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cancel != nil {
		p.cancel()
	}
	p.status.State = "cancelled"
	if !p.streaming {
		p.releaseContent()
	}
}
func (a *App) clearAcquisitionPackages(s *session) {
	a.mu.Lock()
	var removed []*acquisitionPackage
	for id, p := range a.packages {
		if p.owner == s {
			delete(a.packages, id)
			removed = append(removed, p)
		}
	}
	a.mu.Unlock()
	for _, p := range removed {
		p.retire()
	}
}
func (a *App) expireAcquisitionPackages(now time.Time) {
	a.mu.Lock()
	var expired []*acquisitionPackage
	for id, p := range a.packages {
		if !now.Before(p.expires) {
			delete(a.packages, id)
			expired = append(expired, p)
		}
	}
	a.mu.Unlock()
	for _, p := range expired {
		p.retire()
	}
}
func samePackageOrigin(r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.User == nil &&
			strings.EqualFold(u.Host, r.Host)
	}
	return true
}

// A validated user mutation cancels a long native transfer before waiting for
// the existing session gate. An old tab cannot cancel a newer generation's job.
func (a *App) cancelPackagesBeforeMutation(r *http.Request) {
	switch r.URL.Path {
	case "/api/auth/logout", "/api/auth/login", "/api/reports/import", "/api/reports/demo", "/api/settings":
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			return
		}
		if r.URL.Path == "/api/settings" && r.Method != http.MethodDelete {
			return
		}
	case "/api/auth/captcha":
	default:
		return
	}
	if !samePackageOrigin(r) || len(r.Header.Values("X-CSRF-Token")) != 1 {
		return
	}
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return
	}
	a.mu.Lock()
	owner := a.sessions[cookie.Value]
	var found []*acquisitionPackage
	for _, p := range a.packages {
		if owner != nil && p.owner == owner && p.cookie == cookie.Value &&
			subtle.ConstantTimeCompare([]byte(p.csrf), []byte(r.Header.Get("X-CSRF-Token"))) == 1 {
			found = append(found, p)
		}
	}
	a.mu.Unlock()
	for _, p := range found {
		p.cancelTransfer()
	}
}

func validPackageTimestamp(value string) bool {
	if len(value) > 40 {
		return false
	}
	_, err := time.Parse(time.RFC3339Nano, value)
	return err == nil
}
func validatePackageInput(input *packageInput) error {
	bad := errors.New("invalid bounded acquisition package")
	if !packagePlanPattern.MatchString(input.Plan.ID) || input.Plan.Index < 1 ||
		input.Plan.Total < input.Plan.Index || input.Plan.Total > 1000 ||
		input.Plan.FilesTotal < len(input.Files) || input.Plan.FilesTotal > 50000 ||
		len(input.Files) > maxPackageFiles || input.Files == nil {
		return bad
	}
	embedded := len(input.Dataset) != 0 && string(input.Dataset) != "null"
	if embedded == (input.ExternalDataset != nil) {
		return bad
	}
	if embedded {
		if len(input.Dataset) > maxPackageDataset {
			return bad
		}
		var header map[string]json.RawMessage
		if json.Unmarshal(input.Dataset, &header) != nil {
			return bad
		}
		allowed := map[string]bool{"schema": true, "exported_at": true, "producer": true, "imported_from": true,
			"laboratory": true, "catalog": true, "data": true, "attachments": true}
		for name := range header {
			if !allowed[name] {
				return bad
			}
		}
		var schema string
		if json.Unmarshal(header["schema"], &schema) != nil || schema != "healthbank.dataset.v2" {
			return bad
		}
		for _, name := range []string{"exported_at", "producer", "laboratory", "catalog", "data", "attachments"} {
			if len(header[name]) == 0 || string(header[name]) == "null" {
				return bad
			}
		}
		// This endpoint transports an already validated client dataset unchanged;
		// it does not import it, infer ownership, or write it into SQL.
	} else {
		file := input.ExternalDataset
		if file.Name != "healthbank.dataset.v2.json" || file.Size < 1 ||
			file.Size > maxExternalDataset || !packageDigestPattern.MatchString(file.SHA256) {
			return bad
		}
	}
	ids, refs := map[string]bool{}, map[string]bool{}
	for _, file := range input.Files {
		key := file.Category + ":" + file.ID
		if !packageCategoryPattern.MatchString(file.Category) || !validDocumentReference(file.ID) ||
			file.ID != strings.ToLower(file.ID) || !packageArchivePattern.MatchString(file.ArchiveID) ||
			ids[file.ArchiveID] || refs[key] {
			return bad
		}
		ids[file.ArchiveID], refs[key] = true, true
	}
	acquisition := input.Acquisition
	if (acquisition.Scope != "all_available" && acquisition.Scope != "selected") ||
		!validPackageTimestamp(acquisition.StartedAt) || !validPackageTimestamp(acquisition.FinishedAt) ||
		len(acquisition.Statuses) > 4096 || acquisition.Statuses == nil {
		return bad
	}
	start, _ := time.Parse(time.RFC3339Nano, acquisition.StartedAt)
	end, _ := time.Parse(time.RFC3339Nano, acquisition.FinishedAt)
	if end.Before(start) {
		return bad
	}
	states := map[string]string{
		"source":   "|ready|partial|failed|unqueried|cancelled|",
		"prepare":  "|not_requested|queued|requested|indeterminate|source_refreshed|correlation_unavailable|failed|cancelled|",
		"document": "|ready|excluded|failed|cancelled|",
	}
	codes := map[string]bool{"": true, "source_total_unknown": true, "pending_details": true,
		"preparation_outcome_unknown": true, "source_did_not_provide_job_correlation": true,
		"poll_limit": true, "request_failed": true, "scope_excluded": true, "budget_exceeded": true, "auth_expired": true}
	for _, item := range acquisition.Statuses {
		if !packageCategoryPattern.MatchString(item.Category) || item.State == "" ||
			!slices.Contains(strings.Split(states[item.Kind], "|"), item.State) || !codes[item.Code] ||
			item.Count < 0 || item.Count > 250000 {
			return bad
		}
	}
	encoded, err := json.Marshal(acquisition)
	if err != nil || len(encoded) > 1<<20 {
		return bad
	}
	return nil
}

func (a *App) createAcquisitionPackage(w http.ResponseWriter, r *http.Request, s *session) {
	if s.mode != "official" || s.official == nil || s.subject == "" {
		a.fail(w, r, 401, "login_required", "請先完成官方登入，再建立完整取得資料包。")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxPackageBody)
	body, err := io.ReadAll(r.Body)
	defer clear(body)
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			a.fail(w, r, 413, "package_too_large", "資料包請求超過容量；請將完整資料 JSON 另存後使用外部資料檔模式。")
		} else {
			a.fail(w, r, 400, "invalid_package", "無法完整讀取資料包請求。")
		}
		return
	}
	if strictjson.Validate(body, strictjson.Options{MaxDepth: 64, FoldNames: true}) != nil {
		a.fail(w, r, 400, "invalid_package", "資料包請求含不明確或無效的 JSON。")
		return
	}
	var input packageInput
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || validatePackageInput(&input) != nil {
		clear(input.Dataset)
		a.fail(w, r, 400, "invalid_package", "資料包範圍、檔案或版本資訊不符合限制。")
		return
	}
	// Healthbank's portable exporter writes exactly one final LF. JSON's raw
	// value decoder omits surrounding whitespace; restore that file boundary
	// without reserializing values, key order or precise decimal strings.
	if len(input.Dataset) != 0 && string(input.Dataset) != "null" {
		input.Dataset = append(input.Dataset, '\n')
		if len(input.Dataset) > maxPackageDataset {
			clear(input.Dataset)
			a.fail(w, r, 413, "package_too_large", "完整 JSON 超過內嵌容量；請另存原檔後使用外部資料檔模式。")
			return
		}
	}
	digest := sha256.Sum256(body)
	requestHash := hex.EncodeToString(digest[:])
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		clear(input.Dataset)
		a.fail(w, r, 409, "session_changed", "連線已更換，請重新確認使用者。")
		return
	}
	a.expireAcquisitionPackages(time.Now())
	a.mu.Lock()
	var existing *acquisitionPackage
	count := 0
	for _, p := range a.packages {
		if p.owner == s && p.cookie == cookie.Value {
			count++
			if p.input.Plan.ID == input.Plan.ID && p.input.Plan.Index == input.Plan.Index {
				existing = p
			}
		}
	}
	a.mu.Unlock()
	if existing != nil {
		clear(input.Dataset)
		if existing.requestHash != requestHash {
			a.fail(w, r, 409, "package_plan_conflict", "同一資料包批次已有不同內容；請建立新計畫。")
			return
		}
		a.json(w, http.StatusOK, existing.view())
		return
	}
	if count >= maxPackageReceipts {
		clear(input.Dataset)
		a.fail(w, r, 429, "package_capacity", "尚未清除的資料包已達上限；請確認先前檔案已保存後清除其工作。")
		return
	}
	encoded, _ := json.Marshal(input.Acquisition)
	memory := int64(len(input.Dataset) + 2*len(encoded) + len(input.Files)*1024 + 4096)
	if !s.memory.adjustPackages(memory) {
		clear(input.Dataset)
		a.resourceFailure(w, r, "package_retained_capacity", "目前資料包記憶體容量不足；可先另存完整資料 JSON，再以外部資料檔模式打包原始附件。")
		return
	}
	random := make([]byte, 24)
	if _, err := rand.Read(random); err != nil {
		_ = s.memory.adjustPackages(-memory)
		clear(input.Dataset)
		a.fail(w, r, 503, "random_unavailable", "無法建立資料包識別。")
		return
	}
	id := hex.EncodeToString(random)
	p := &acquisitionPackage{id: id, cookie: cookie.Value, csrf: s.csrf, owner: s, scope: s.scope,
		requestHash: requestHash, input: input, expires: time.Now().Add(packageLifetime), memoryBytes: memory}
	p.status = packageStatus{ID: id, State: "ready", Total: len(input.Files), Files: make([]packageFileStatus, len(input.Files)),
		ExpiresAt: p.expires.UTC().Format(time.RFC3339)}
	for i, file := range input.Files {
		p.status.Files[i] = packageFileStatus{ArchiveID: file.ArchiveID, State: "queued"}
	}
	a.mu.Lock()
	if a.packages == nil {
		a.packages = map[string]*acquisitionPackage{}
	}
	a.packages[id] = p
	a.mu.Unlock()
	a.json(w, http.StatusCreated, p.view())
}

func (p *acquisitionPackage) view() packageStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	result := p.status
	result.Files = append([]packageFileStatus{}, p.status.Files...)
	return result
}

// Status/cancel never wait behind the long transfer's session gate. Both prove
// the current cookie generation and CSRF token against immutable receipt data.
func (a *App) lookupPackage(w http.ResponseWriter, r *http.Request, proof bool) *acquisitionPackage {
	cookie, err := r.Cookie(cookieName)
	if err != nil || !samePackageOrigin(r) {
		a.fail(w, r, 401, "login_required", "請從目前已登入的工作階段查看資料包。")
		return nil
	}
	id := r.URL.Query().Get("id")
	plan, part := r.URL.Query().Get("plan"), r.URL.Query().Get("part")
	partNumber, _ := strconv.Atoi(part)
	if id != "" && !validDocumentReference(id) || id == "" && (!packagePlanPattern.MatchString(plan) || partNumber < 1) {
		a.fail(w, r, 400, "invalid_package", "資料包識別不正確。")
		return nil
	}
	a.mu.Lock()
	owner := a.sessions[cookie.Value]
	var p *acquisitionPackage
	if id != "" {
		p = a.packages[id]
	} else {
		for _, candidate := range a.packages {
			if candidate.cookie == cookie.Value && candidate.input.Plan.ID == plan && candidate.input.Plan.Index == partNumber {
				p = candidate
				break
			}
		}
	}
	valid := owner != nil && p != nil && p.owner == owner && p.cookie == cookie.Value &&
		time.Since(owner.touched) <= owner.idleLifetime() && time.Now().Before(p.expires) &&
		(p.scope == nil || p.scope.ctx.Err() == nil)
	if valid {
		owner.touched = time.Now()
	}
	a.mu.Unlock()
	if !valid {
		a.fail(w, r, 404, "package_unavailable", "此資料包不屬於目前連線、已過期或尚未建立。")
		return nil
	}
	if proof && (len(r.Header.Values("X-CSRF-Token")) != 1 ||
		subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(p.csrf)) != 1) {
		a.fail(w, r, 409, "session_changed", "頁面連線已更換，請重新確認使用者。")
		return nil
	}
	return p
}
func (a *App) acquisitionPackageStatus(w http.ResponseWriter, r *http.Request) {
	select {
	case a.requests <- struct{}{}:
		defer func() { <-a.requests }()
	default:
		a.resourceFailure(w, r, "request_capacity", "目前同時處理的請求已達上限，請稍後查看資料包狀態。")
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		a.fail(w, r, 405, "method_not_allowed", "資料包狀態請使用 GET。")
		return
	}
	if p := a.lookupPackage(w, r, true); p != nil {
		a.json(w, http.StatusOK, p.view())
	}
}
func (a *App) cancelAcquisitionPackage(w http.ResponseWriter, r *http.Request) {
	if p := a.lookupPackage(w, r, true); p != nil {
		a.mu.Lock()
		delete(a.packages, p.id)
		a.mu.Unlock()
		p.retire()
		a.json(w, http.StatusOK, map[string]string{"status": "cancelled"})
	}
}
