// Copyright 2026 bestkakkoi llc.
// SPDX-License-Identifier: MIT

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/bestkakkoii/healthbank/internal/config"
	"github.com/bestkakkoii/healthbank/internal/lab"
	"github.com/bestkakkoii/healthbank/internal/store"
)

type browserFixture struct {
	app    *App
	cookie *http.Cookie
	csrf   string
}

type sessionResponse struct {
	Authenticated bool         `json:"authenticated"`
	Mode          string       `json:"mode"`
	Settings      lab.Settings `json:"settings"`
	CSRFToken     string       `json:"csrf_token"`
	Version       string       `json:"version"`
}

func testApp(t *testing.T) (*App, *store.Store, string) {
	t.Helper()
	dir := t.TempDir()
	secret, err := config.Secret(dir)
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(context.Background(), store.Options{SQLitePath: filepath.Join(dir, "settings.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	app, err := New(db, secret, "synthetic-test")
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Close(); _ = db.Close() })
	return app, db, dir
}

func (b *browserFixture) request(method, path string, body io.Reader, contentType string, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "https://healthbank.test"+path, body)
	if b.cookie != nil {
		request.AddCookie(b.cookie)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if method != http.MethodGet && method != http.MethodHead {
		request.Header.Set("Origin", "https://healthbank.test")
		request.Header.Set("X-CSRF-Token", token)
	}
	recorder := httptest.NewRecorder()
	b.app.Handler().ServeHTTP(recorder, request)
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == cookieName {
			b.cookie = cookie
		}
	}
	return recorder
}

func (b *browserFixture) jsonRequest(t *testing.T, method, path string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return b.request(method, path, bytes.NewReader(data), "application/json", b.csrf)
}

func decodeResponse[T any](t *testing.T, recorder *httptest.ResponseRecorder, status int) T {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("HTTP status=%d, want=%d: %s", recorder.Code, status, recorder.Body.String())
	}
	if recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("API response can be cached")
	}
	var result T
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func (b *browserFixture) refresh(t *testing.T) sessionResponse {
	t.Helper()
	view := decodeResponse[sessionResponse](t, b.request(http.MethodGet, "/api/session", nil, "", ""), http.StatusOK)
	if view.CSRFToken == "" || b.cookie == nil {
		t.Fatal("session did not establish a cookie and CSRF token")
	}
	b.csrf = view.CSRFToken
	return view
}

func settingsPayload(settings lab.Settings) map[string]any {
	return map[string]any{
		"revision":   settings.Revision,
		"theme":      settings.Theme,
		"font_scale": settings.FontScale,
		"favorites":  settings.Favorites,
		"overrides":  settings.Overrides,
	}
}

func noPersistentProfiles(t *testing.T, db *store.Store) {
	t.Helper()
	snapshot, err := db.Export(context.Background())
	if err != nil || len(snapshot.Entries) != 0 {
		t.Fatalf("temporary session persisted a profile: %d entries, %v", len(snapshot.Entries), err)
	}
}

func TestDemoSessionDynamicSettingsAndRevisionGuards(t *testing.T) {
	app, db, _ := testApp(t)
	browser := &browserFixture{app: app}
	initial := browser.refresh(t)
	if initial.Mode != "anonymous" || initial.Authenticated || initial.Settings.Revision != 0 {
		t.Fatalf("unexpected initial session: %+v", initial)
	}
	if !browser.cookie.HttpOnly || !browser.cookie.Secure || browser.cookie.SameSite != http.SameSiteStrictMode {
		t.Fatal("session cookie attributes were lost")
	}
	denied := browser.request(http.MethodPost, "/api/reports/demo", strings.NewReader(`{}`), "application/json", "invalid-csrf")
	if denied.Code != http.StatusForbidden {
		t.Fatalf("invalid CSRF accepted: %d", denied.Code)
	}
	if denied := browser.request(http.MethodGet, "/api/reports", nil, "", ""); denied.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous official report fetch accepted: %d", denied.Code)
	}
	before := decodeResponse[lab.CatalogResponse](t, browser.request(http.MethodGet, "/api/catalog", nil, "", ""), http.StatusOK)
	if len(before.Items) != 0 {
		t.Fatal("unseen items appeared as personal configuration")
	}
	demo := decodeResponse[reportResponse](t, browser.jsonRequest(t, http.MethodPost, "/api/reports/demo", map[string]any{}), http.StatusOK)
	if len(demo.Records) == 0 || !demo.Complete || len(demo.Warnings) == 0 {
		t.Fatal("demonstration did not identify its synthetic records")
	}
	view := browser.refresh(t)
	if view.Mode != "demo" || view.Authenticated || view.CSRFToken == initial.CSRFToken {
		t.Fatal("demo mode did not rotate and identify its temporary session")
	}
	catalog := decodeResponse[lab.CatalogResponse](t, browser.request(http.MethodGet, "/api/catalog", nil, "", ""), http.StatusOK)
	seen := map[string]bool{}
	var alt lab.CatalogItem
	for _, item := range catalog.Items {
		if seen[item.Key] {
			t.Fatal("duplicate configuration key in encountered-item catalog")
		}
		seen[item.Key] = true
		if item.ID == "serum_alt" {
			alt = item
		}
	}
	if len(catalog.Items) == 0 || alt.Key == "" {
		t.Fatal("encountered ALT configuration is unavailable")
	}
	settings := view.Settings
	settings.Theme, settings.FontScale = "dark", 1.2
	settings.Favorites = []string{alt.Key}
	high := "10"
	settings.Overrides = map[string]lab.Override{alt.Key: {Enabled: true, ReferenceHigh: &high}}
	saved := decodeResponse[lab.Settings](t, browser.jsonRequest(t, http.MethodPut, "/api/settings", settingsPayload(settings)), http.StatusOK)
	if saved.Revision != 1 || saved.Theme != "dark" || !reflect.DeepEqual(saved.Favorites, []string{alt.Key}) {
		t.Fatalf("display preferences were not saved in this session: %+v", saved)
	}
	current := decodeResponse[reportResponse](t, browser.request(http.MethodGet, "/api/reports/current", nil, "", ""), http.StatusOK)
	if len(current.Records) != len(demo.Records) {
		t.Fatal("changing display settings lost report rows")
	}
	checkedALT := false
	for _, record := range current.Records {
		if record.ItemID != alt.ID {
			continue
		}
		checkedALT = true
		if record.Level == nil || *record.Level != 3 || record.RuleSource != "personal" || record.ReferenceStatus != "within" || record.ReferenceHigh == nil || *record.ReferenceHigh != "52" {
			t.Fatalf("personal target changed the original reference or failed to classify: %+v", record)
		}
	}
	if !checkedALT {
		t.Fatal("ALT observations disappeared")
	}
	stale := browser.jsonRequest(t, http.MethodPut, "/api/settings", settingsPayload(settings))
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale setting revision accepted: %d", stale.Code)
	}
	for _, field := range []string{"password", "identity_number", "records", "known_items"} {
		injected := settingsPayload(saved)
		injected[field] = "SYNTHETIC_FORBIDDEN_PAYLOAD"
		response := browser.jsonRequest(t, http.MethodPut, "/api/settings", injected)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("forbidden settings field %q accepted: %d", field, response.Code)
		}
	}
	unknownItem := settingsPayload(saved)
	unknownItem["favorites"] = []string{"not_an_encountered_item"}
	if response := browser.jsonRequest(t, http.MethodPut, "/api/settings", unknownItem); response.Code != http.StatusBadRequest {
		t.Fatalf("unseen item accepted as a configuration: %d", response.Code)
	}
	noPersistentProfiles(t, db)
}

const importFixture = `<!doctype html><html><body>
<aside>SYNTHETIC_PATIENT_BANNER_NOT_REAL</aside>
<script>window.doNotExecute = "SYNTHETIC_SCRIPT_NOT_REPORT";</script>
<table><thead><tr><th>檢查日期</th><th>醫令代碼</th><th>檢驗檢查醫囑名稱</th><th>檢驗檢查項目名稱</th><th>檢查結果</th><th>單位</th><th>參考值</th><th>醫事機構</th><th>就醫日期/入院日期</th><th>資料來源</th></tr></thead>
<tbody><tr><td>115/02/03</td><td>09026C</td><td>合成ALT檢驗</td><td>ALT</td><td class="sign-red">37.8125</td><td>U/L</td><td>[7][52]</td><td>SYNTHETIC_CLINIC_NOT_REAL</td><td>115/01/20</td><td>synthetic</td></tr></tbody></table>
</body></html>`

func TestHTMLImportAndLogoutLeaveNoPersistentReport(t *testing.T) {
	app, db, dir := testApp(t)
	browser := &browserFixture{app: app}
	browser.refresh(t)
	beforeFiles, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "synthetic-report.html")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, importFixture); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	response := browser.request(http.MethodPost, "/api/reports/import", &body, writer.FormDataContentType(), browser.csrf)
	imported := decodeResponse[reportResponse](t, response, http.StatusOK)
	if len(imported.Records) != 1 || imported.Complete || len(imported.Warnings) == 0 {
		t.Fatal("import did not preserve saved-page incompleteness")
	}
	for _, sentinel := range []string{"SYNTHETIC_PATIENT_BANNER_NOT_REAL", "SYNTHETIC_SCRIPT_NOT_REPORT"} {
		if strings.Contains(response.Body.String(), sentinel) {
			t.Fatal("page banner or executable content entered API report data")
		}
	}
	record := imported.Records[0]
	if record.Result != "37.8125" || record.ExamDate != "2026-02-03" || record.VisitDate != "2026-01-20" || record.OfficialFlag != "1" {
		t.Fatalf("import changed source values, dates, or flag: %+v", record)
	}
	view := browser.refresh(t)
	if view.Mode != "import" || view.Authenticated {
		t.Fatal("imported page was treated as verified official ownership")
	}
	view.Settings.Theme = "dark"
	decodeResponse[lab.Settings](t, browser.jsonRequest(t, http.MethodPut, "/api/settings", settingsPayload(view.Settings)), http.StatusOK)
	noPersistentProfiles(t, db)
	afterFiles, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterFiles) != len(beforeFiles) {
		t.Fatal("report import created a file on disk")
	}
	for i := range beforeFiles {
		if beforeFiles[i].Name() != afterFiles[i].Name() {
			t.Fatal("report import changed files in the persistent data directory")
		}
	}
	oldCookie := *browser.cookie
	loggedOut := decodeResponse[sessionResponse](t, browser.jsonRequest(t, http.MethodPost, "/api/auth/logout", map[string]any{}), http.StatusOK)
	if loggedOut.Mode != "anonymous" || loggedOut.Authenticated || loggedOut.CSRFToken == browser.csrf {
		t.Fatal("logout did not rotate the anonymous session")
	}
	browser.csrf = loggedOut.CSRFToken
	empty := decodeResponse[reportResponse](t, browser.request(http.MethodGet, "/api/reports/current", nil, "", ""), http.StatusOK)
	if len(empty.Records) != 0 {
		t.Fatal("logout retained clinical rows")
	}
	staleBrowser := &browserFixture{app: app, cookie: &oldCookie}
	stale := decodeResponse[reportResponse](t, staleBrowser.request(http.MethodGet, "/api/reports/current", nil, "", ""), http.StatusOK)
	if len(stale.Records) != 0 {
		t.Fatal("previous session cookie still exposes a logged-out report")
	}
	noPersistentProfiles(t, db)
}

// attachVerifiedFixture exercises the application boundary AFTER verification.
// It intentionally uses no NHI credentials or network calls and is not an
// official-login test. The synthetic identifier is not a valid national ID.
func attachVerifiedFixture(t *testing.T, browser *browserFixture, syntheticID string, rows []lab.RawRecord) string {
	t.Helper()
	app := browser.app
	app.mu.Lock()
	s := app.sessions[browser.cookie.Value]
	app.mu.Unlock()
	if s == nil {
		t.Fatal("fixture session is missing")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subject = config.Subject(app.secret, syntheticID)
	s.mode = "official"
	s.raw = rows
	_ = app.catalog.Normalize(rows, &s.settings)
	if err := app.persistDiscovered(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	return s.subject
}

func TestVerifiedFixtureSharesSettingsAcrossDevicesButNotResults(t *testing.T) {
	app, db, _ := testApp(t)
	const syntheticID = "SYNTHETIC_ID_NOT_VALID_01"
	first := &browserFixture{app: app}
	first.refresh(t)
	subject := attachVerifiedFixture(t, first, syntheticID, []lab.RawRecord{{
		CheckDate: "2026-02-03", VisitDate: "2026-01-20", OrderCode: "09026C", ItemName: "ALT",
		Result: "37.8125", Unit: "U/L", Reference: "[7][52]", Institution: "SYNTHETIC_CLINIC_NOT_REAL",
	}})
	view := first.refresh(t)
	if !view.Authenticated || view.Settings.Revision != 1 {
		t.Fatalf("verified fixture did not persist encountered definitions: %+v", view)
	}
	catalog := decodeResponse[lab.CatalogResponse](t, first.request(http.MethodGet, "/api/catalog", nil, "", ""), http.StatusOK)
	if len(catalog.Items) != 1 {
		t.Fatalf("fixture should configure only its one encountered item: %d", len(catalog.Items))
	}
	key := catalog.Items[0].Key
	settings := view.Settings
	settings.Theme, settings.FontScale = "dark", 2
	settings.Favorites = []string{key}
	high := "20"
	settings.Overrides = map[string]lab.Override{key: {Enabled: true, ReferenceHigh: &high}}
	saved := decodeResponse[lab.Settings](t, first.jsonRequest(t, http.MethodPut, "/api/settings", settingsPayload(settings)), http.StatusOK)
	if saved.Revision != 2 {
		t.Fatal("configuration revision did not advance")
	}
	// The largest reading size must survive a real API/SQLite round trip;
	// an out-of-range update must not replace it or advance the revision.
	oversized := saved
	oversized.FontScale = 2.01
	if response := first.jsonRequest(t, http.MethodPut, "/api/settings", settingsPayload(oversized)); response.Code != http.StatusBadRequest {
		t.Fatalf("out-of-range reading size accepted: %d", response.Code)
	}
	second := &browserFixture{app: app}
	second.refresh(t)
	if secondSubject := attachVerifiedFixture(t, second, syntheticID, nil); secondSubject != subject {
		t.Fatal("same verified identity did not map to one pseudonym")
	}
	secondView := second.refresh(t)
	if secondView.Settings.Revision != 2 || secondView.Settings.Theme != "dark" || secondView.Settings.FontScale != 2 || !reflect.DeepEqual(secondView.Settings.Favorites, []string{key}) {
		t.Fatalf("second device did not load shared preferences: %+v", secondView.Settings)
	}
	secondReports := decodeResponse[reportResponse](t, second.request(http.MethodGet, "/api/reports/current", nil, "", ""), http.StatusOK)
	if len(secondReports.Records) != 0 {
		t.Fatal("new device loaded historical results from persistent storage")
	}
	secondView.Settings.Theme = "light"
	decodeResponse[lab.Settings](t, second.jsonRequest(t, http.MethodPut, "/api/settings", settingsPayload(secondView.Settings)), http.StatusOK)
	if response := first.jsonRequest(t, http.MethodPut, "/api/settings", settingsPayload(saved)); response.Code != http.StatusConflict {
		t.Fatalf("old device overwrote newer shared preferences: %d", response.Code)
	}
	if latest := first.refresh(t); latest.Settings.Theme != "light" || latest.Settings.Revision != 3 {
		t.Fatal("old device did not refresh the new revision")
	}
	snapshot, err := db.Export(context.Background())
	if err != nil || len(snapshot.Entries) != 1 || snapshot.Entries[0].SubjectKey != subject {
		t.Fatalf("unexpected persisted subjects: %+v, %v", snapshot, err)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{syntheticID, "37.8125", "SYNTHETIC_CLINIC_NOT_REAL", "2026-02-03", "2026-01-20", `"result"`, `"records"`, `"password"`, `"identity_number"`, `"card_number"`} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatalf("non-configuration information persisted: %q", forbidden)
		}
	}
	if _, err := ValidatePortableSettings(snapshot.Entries[0].Settings); err != nil {
		t.Fatalf("API-saved document does not pass CLI-import validation: %v", err)
	}
}

func TestPortableSettingsRejectClinicalAndNestedPayloads(t *testing.T) {
	valid, err := lab.DefaultSettings().Document()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidatePortableSettings(valid); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		`{"theme":"system","font_scale":1,"favorites":[],"overrides":{},"records":[{"result":"SYNTHETIC"}]}`,
		`{"theme":"system","font_scale":1,"favorites":[],"overrides":{},"known_items":{"serum_alt":{"id":"serum_alt","configuration_key":"serum_alt","name":"ALT","unit":"U/L","category":"肝功能","result":"SYNTHETIC"}}}`,
		`{"theme":"system","font_scale":1,"favorites":[],"overrides":{"serum_alt":{"enabled":true,"password":"SYNTHETIC"}}}`,
	} {
		if _, err := ValidatePortableSettings([]byte(raw)); err == nil {
			t.Fatal("portable settings accepted clinical/credential content outside its schema")
		}
	}
}

func TestParallelExistingResponsesCannotReissueRetiredSessionCookies(t *testing.T) {
	app, db, _ := testApp(t)
	browser := &browserFixture{app: app}
	initial := browser.refresh(t)
	initialCookie := *browser.cookie

	// Keep these completed responses until after rotation, as a browser can
	// receive an earlier parallel response after a later login/import response.
	// No scheduler-dependent barrier or production-only testing hook is needed.
	responses := make(chan *httptest.ResponseRecorder, 3)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for _, path := range []string{"/api/session", "/api/catalog", "/api/reports/current"} {
		workers.Add(1)
		go func(path string) {
			defer workers.Done()
			<-start
			requestBrowser := &browserFixture{app: app, cookie: &initialCookie}
			responses <- requestBrowser.request(http.MethodGet, path, nil, "", "")
		}(path)
	}
	close(start)
	workers.Wait()
	close(responses)
	var delayedResponses []*httptest.ResponseRecorder
	for response := range responses {
		if response.Code != http.StatusOK {
			t.Fatalf("ordinary parallel GET failed: %d", response.Code)
		}
		delayedResponses = append(delayedResponses, response)
	}
	preferences := initial.Settings
	preferences.Theme = "light"
	changed := browser.jsonRequest(t, http.MethodPut, "/api/settings", settingsPayload(preferences))
	decodeResponse[lab.Settings](t, changed, http.StatusOK)
	delayedResponses = append(delayedResponses, changed)
	denied := browser.request(http.MethodPost, "/api/reports/demo", strings.NewReader(`{}`), "application/json", "retired-or-invalid-token")
	if denied.Code != http.StatusForbidden {
		t.Fatalf("invalid CSRF returned %d", denied.Code)
	}
	delayedResponses = append(delayedResponses, denied)

	rotated := browser.jsonRequest(t, http.MethodPost, "/api/reports/demo", map[string]any{})
	demo := decodeResponse[reportResponse](t, rotated, http.StatusOK)
	if len(demo.Records) == 0 || len(rotated.Header().Values("Set-Cookie")) != 1 || browser.cookie.Value == initialCookie.Value {
		t.Fatal("explicit mode change did not issue exactly one new cookie generation")
	}
	for _, response := range delayedResponses {
		if len(response.Header().Values("Set-Cookie")) != 0 {
			t.Fatal("an earlier ordinary response can overwrite the browser's new session cookie")
		}
	}
	newGeneration := browser.refresh(t)
	if newGeneration.Mode != "demo" || newGeneration.CSRFToken == initial.CSRFToken {
		t.Fatal("rotation did not replace the CSRF token and mode")
	}

	// A request arriving with a retired cookie must get an independent empty
	// session, never the records or configuration of the replacement generation.
	retiredBrowser := &browserFixture{app: app, cookie: &initialCookie}
	retiredView := retiredBrowser.refresh(t)
	if retiredView.Mode != "anonymous" || retiredView.Authenticated || len(retiredView.Settings.Known) != 0 || retiredBrowser.cookie.Value == browser.cookie.Value {
		t.Fatal("retired cookie accessed the replacement session")
	}
	empty := decodeResponse[reportResponse](t, retiredBrowser.request(http.MethodGet, "/api/reports/current", nil, "", ""), http.StatusOK)
	if len(empty.Records) != 0 {
		t.Fatal("retired cookie exposed the replacement session's reports")
	}
	current := decodeResponse[reportResponse](t, browser.request(http.MethodGet, "/api/reports/current", nil, "", ""), http.StatusOK)
	if len(current.Records) != len(demo.Records) {
		t.Fatal("probing the retired cookie changed the replacement session's records")
	}
	noPersistentProfiles(t, db)
}
