// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestHomeCareAvailabilityRequiresLiteralBoolean(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		entries    int
		invalid    bool
	}{
		{"present", `{"hasdata":true,"c_CASE_DATE":"101/02/03","c_HOSP_ABBR":"Synthetic Clinic"}`, 1, false},
		{"explicit_absence", `{"hasdata":false}`, 0, false},
		{"null_is_not_absence", `{"hasdata":null,"c_CASE_DATE":"101/02/03","c_HOSP_ABBR":"Synthetic Clinic"}`, 0, true},
		{"missing", `{"c_CASE_DATE":"101/02/03","c_HOSP_ABBR":"Synthetic Clinic"}`, 0, true},
		{"string", `{"hasdata":"false"}`, 0, true},
		{"number", `{"hasdata":0}`, 0, true},
		{"array", `{"hasdata":[]}`, 0, true},
		{"object", `{"hasdata":{}}`, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/ihke3000/IHKE3712S01/page_load" || r.URL.RawQuery != "" {
					t.Error("home-care availability used an unrelated source read")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			})
			report, err := s.FetchProvider(context.Background(), "home_care", Query{})
			if tc.invalid {
				var failure *Error
				if !errors.As(err, &failure) || failure.Code != "UPSTREAM_CHANGED" {
					t.Fatalf("unknown availability became an empty successful report: %v", err)
				}
				return
			}
			if err != nil || len(report.Entries) != tc.entries || report.Complete {
				t.Fatalf("literal source availability or incomplete-history semantics changed: entries=%d error=%v", len(report.Entries), err)
			}
		})
	}
}
