// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJSONRequestBoundaryRejectsAmbiguousOrRepairedInput(t *testing.T) {
	for _, data := range []string{
		`{"password":"SYNTHETIC_FIRST","password":"SYNTHETIC_SECOND"}`,
		`{"password":"SYNTHETIC_FIRST","PASSWORD":"SYNTHETIC_SECOND"}`,
		`{"password":"\ud800"}`,
		"{\"password\":\"\xff\"}",
		`null`,
		`{"password":"SYNTHETIC"} {}`,
	} {
		var input struct {
			Password string `json:"password"`
		}
		r := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(data))
		if err := decodeJSON(httptest.NewRecorder(), r, &input); err == nil {
			t.Fatal("ambiguous, malformed or non-object JSON reached the typed request handler")
		}
	}
	var valid struct {
		Password string `json:"password"`
	}
	r := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"合成\ud83d\ude80"}`))
	if err := decodeJSON(httptest.NewRecorder(), r, &valid); err != nil || valid.Password != "合成🚀" {
		t.Fatal("valid UTF-8 and paired supplementary Unicode were changed")
	}
}

func TestPortableSettingsRejectDuplicateFieldsBeforePersistence(t *testing.T) {
	for _, data := range []string{
		`{"theme":"SYNTHETIC_HIDDEN_VALUE","theme":"system","font_scale":1,"favorites":[],"overrides":{}}`,
		`{"theme":"SYNTHETIC_HIDDEN_VALUE","THEME":"system","font_scale":1,"favorites":[],"overrides":{}}`,
		`{"theme":"system","font_scale":1,"favorites":[],"overrides":{},"known_items":{"synthetic":{"id":"synthetic","configuration_key":"synthetic","name":"\ud800","unit":"","category":""}}}`,
	} {
		if _, err := ValidatePortableSettings([]byte(data)); err == nil {
			t.Fatal("portable validation accepted hidden or silently repaired settings content")
		}
	}
}
