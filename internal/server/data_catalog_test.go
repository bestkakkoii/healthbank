// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bestkakkoii/healthbank/internal/nhi"
)

func TestDataCatalogPublicCapabilitiesAreCompleteAndDoNotCreateSession(t *testing.T) {
	app, db, _ := testApp(t)
	request := httptest.NewRequest(http.MethodGet, "https://healthbank.test/api/data/catalog", nil)
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	data := decodeResponse[struct {
		Categories []nhi.ProviderCategoryInfo `json:"categories"`
	}](t, response, http.StatusOK)
	if len(data.Categories) != 14+len(nhi.ProviderCategories())+5 {
		t.Fatal("provider or document capabilities omitted")
	}
	seen := make(map[string]nhi.ProviderCategoryInfo)
	for _, category := range data.Categories {
		if _, exists := seen[category.ID]; exists || category.ID == "" || category.Label == "" || category.Group == "" {
			t.Fatal("category missing metadata or duplicated")
		}
		if category.RangeMode != "source" && category.RangeMode != "date_search" {
			t.Fatal("unknown range contract exposed")
		}
		seen[category.ID] = category
	}
	for _, category := range nhi.ProviderCategories() {
		if seen[category.ID] != category {
			t.Fatal("provider detail or range capability changed in HTTP catalog")
		}
	}
	for _, id := range []string{"imaging", "hospital", "medicine", "self_paid", "official_files"} {
		if !seen[id].HasDetails {
			t.Fatal("source detail capability omitted")
		}
	}
	for _, id := range []string{"chronic", "adult", "imaging", "official_export_html", "official_export_xml", "official_export_json", "official_export_pdf"} {
		if seen[id].RangeMode != "source" {
			t.Fatal("date search invented for a source-limited contract")
		}
	}
	if len(response.Result().Cookies()) != 0 || len(app.sessions) != 0 {
		t.Fatal("public capabilities created a personal session")
	}
	noPersistentProfiles(t, db)
}
