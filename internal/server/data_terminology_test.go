// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDataCatalogTerminologyIsDisplayOnly(t *testing.T) {
	app, db, _ := testApp(t)
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "https://healthbank.test/api/data/catalog", nil))
	data := decodeResponse[dataCatalogMetadata](t, response, http.StatusOK)
	if data.TerminologyBasis != "healthbank_editorial_translation" {
		t.Fatal("editorial translations lost their attribution")
	}
	original := dataCategories()
	if len(data.Categories) != len(original) || len(data.Categories) != len(dataCategoryEnglish) {
		t.Fatal("terminology vocabulary omitted or invented a source capability")
	}
	labels := make(map[string]string, len(data.Categories))
	for i, category := range data.Categories {
		if category.ProviderCategoryInfo != original[i] {
			t.Fatal("display translation changed the source label or acquisition contract")
		}
		if strings.TrimSpace(category.LabelEN) != category.LabelEN || category.LabelEN == "" || len(category.LabelEN) > 160 || strings.ContainsAny(category.LabelEN, "<>\n\r") {
			t.Fatal("unbounded or missing English display label")
		}
		labels[category.ID] = category.LabelEN
	}
	for category, meaning := range map[string]string{
		"glucose": "HbA1c", "child_schedule": "suggested", "annual_overview": "reference", "chronic": "prescriptions", "organ_wishes": "annotations",
	} {
		if !strings.Contains(labels[category], meaning) {
			t.Fatalf("translation changed a reviewed source meaning for %s", category)
		}
	}
	terms := make(map[string]string, len(data.FieldTerms))
	for _, term := range data.FieldTerms {
		if term.Label == "" || term.LabelEN == "" || len(term.Label) > 160 || len(term.LabelEN) > 160 || terms[term.Label] != "" {
			t.Fatal("invalid or ambiguous exact field-role vocabulary")
		}
		terms[term.Label] = term.LabelEN
	}
	for label, english := range map[string]string{
		"採檢日期": "Specimen collection date", "就醫日期": "Encounter date", "上傳日期": "Upload date",
		"給藥日數": "Prescribed days of supply", "領藥紀錄": "Dispensing record", "實際服用": "Reported actual use",
	} {
		if terms[label] != english {
			t.Fatal("field vocabulary conflated distinct dates or medication events")
		}
	}
	if len(response.Result().Cookies()) != 0 || len(app.sessions) != 0 {
		t.Fatal("public terminology opened a personal session")
	}
	noPersistentProfiles(t, db)
}
