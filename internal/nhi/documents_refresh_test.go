// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDocumentPreparationRefreshUsesOnlyObservedCorrelation(t *testing.T) {
	for _, tc := range []struct {
		category, kind, format, endpoint, refreshCategory, correlation string
	}{
		{"imaging", "prepare_jpeg", "", "/api/ihke3000/IHKE3408S02/add", "imaging", "same_record"},
		{"western", "prepare_jpeg", "", "/api/ihke3000/IHKE3408S02/add", "western", "same_record"},
		{"annual_overview", "prepare_jpeg", "", "/api/ihke3000/IHKE3408S02/add", "annual_overview", "same_record"},
		{"osteoporosis", "prepare_jpeg", "", "/api/ihke3000/IHKE3411S02/add", "osteoporosis", "same_record"},
		{"imaging", "prepare_dicom", "", "/api/ihke3000/IHKE3408S02/add", "official_files", "unavailable"},
		{"western", "prepare_dicom", "", "/api/ihke3000/IHKE3408S02/add", "official_files", "unavailable"},
		{"annual_overview", "prepare_dicom", "", "/api/ihke3000/IHKE3408S02/add", "official_files", "unavailable"},
		{"official_export_json", "prepare_export", "json", "/api/ihke3000/ihke3804s01/add", "official_files", "unavailable"},
	} {
		t.Run(tc.category+"_"+tc.kind, func(t *testing.T) {
			var calls atomic.Int32
			s := providerFixture(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodPost || r.URL.Path != tc.endpoint {
					t.Error("unobserved preparation endpoint")
				}
				// The current public success handlers expose only status/message.
				respondJSON(t, w, map[string]string{"status": "Y", "message": "合成來源已接受"})
			})
			parent, action := strings.Repeat("a", 48), strings.Repeat("b", 48)
			s.documentReferences = map[string]documentReference{action: {category: tc.category, parent: parent, kind: tc.kind, format: tc.format, sequence: "SYNTHETIC_PRIVATE_SEQUENCE", payload: []byte(`{"dat":"SYNTHETIC_PRIVATE_SELECTION"}`)}}
			result, err := s.PrepareDocument(context.Background(), tc.category, action)
			if err != nil {
				t.Fatal(err)
			}
			if result.Status != "requested" || result.Refresh.Category != tc.refreshCategory || result.Refresh.Correlation != tc.correlation {
				t.Fatalf("wrong source refresh contract: %+v", result)
			}
			if tc.correlation == "same_record" && result.Refresh.Detail != parent || tc.correlation == "unavailable" && result.Refresh.Detail != "" {
				t.Fatal("invented job correlation or lost original JPEG record")
			}
			body, err := json.Marshal(result)
			if err != nil || strings.Contains(string(body), "SYNTHETIC_PRIVATE") || strings.Contains(string(body), "job_id") {
				t.Fatal("source selector leaked or invented job ID returned")
			}
			if _, err := s.PrepareDocument(context.Background(), tc.category, action); err == nil || calls.Load() != 1 {
				t.Fatal("refresh metadata permitted a preparation replay")
			}
		})
	}
}
