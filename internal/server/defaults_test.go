// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package server

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/bestkakkoii/healthbank/internal/lab"
)

func TestSettingsBeforeImportAndResetWithoutInventingReports(t *testing.T) {
	app, db, _ := testApp(t)
	browser := &browserFixture{app: app}
	view := browser.refresh(t)
	catalog := decodeResponse[lab.CatalogResponse](t, browser.request(http.MethodGet, "/api/catalog", nil, "", ""), http.StatusOK)
	if len(catalog.Items) != 76 {
		t.Fatalf("empty session omitted built-ins: %d", len(catalog.Items))
	}
	settings := view.Settings
	settings.Theme, settings.FontScale = "dark", 2
	settings.Favorites = []string{"serum_sodium"}
	high := "144"
	settings.Overrides = map[string]lab.Override{
		"serum_sodium":         {Enabled: true, ReferenceHigh: &high},
		"blood_ck_mb_activity": {Enabled: true, ReferenceHigh: &high},
	}
	saved := decodeResponse[lab.Settings](t, browser.jsonRequest(t, http.MethodPut, "/api/settings", settingsPayload(settings)), http.StatusOK)
	if len(saved.Known) != 0 || len(saved.Overrides) != 2 {
		t.Fatal("pre-import save invented encounters or lost overrides")
	}
	document, err := saved.Document()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidatePortableSettings(document); err != nil {
		t.Fatalf("portable defaults rejected without observed items: %v", err)
	}
	delete(saved.Overrides, "serum_sodium")
	resetOne := decodeResponse[lab.Settings](t, browser.jsonRequest(t, http.MethodPut, "/api/settings", settingsPayload(saved)), http.StatusOK)
	if _, exists := resetOne.Overrides["serum_sodium"]; exists || len(resetOne.Overrides) != 1 {
		t.Fatal("single reset affected the wrong overrides")
	}
	resetOne.Overrides = map[string]lab.Override{}
	resetAll := decodeResponse[lab.Settings](t, browser.jsonRequest(t, http.MethodPut, "/api/settings", settingsPayload(resetOne)), http.StatusOK)
	if len(resetAll.Overrides) != 0 || resetAll.Theme != "dark" || resetAll.FontScale != 2 || !reflect.DeepEqual(resetAll.Favorites, settings.Favorites) {
		t.Fatal("reset-all changed unrelated reading or favorite preferences")
	}
	report := decodeResponse[reportResponse](t, browser.request(http.MethodGet, "/api/reports/current", nil, "", ""), http.StatusOK)
	if len(report.Records) != 0 || len(browser.refresh(t).Settings.Known) != 0 {
		t.Fatal("default settings created fabricated report rows")
	}
	noPersistentProfiles(t, db)
}
