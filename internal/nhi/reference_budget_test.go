// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestReferenceBudgetRejectsReplacementAtomicallyAndReleasesOnClose(t *testing.T) {
	large := false
	s := fixtureSession(t, func(w http.ResponseWriter, _ *http.Request) {
		institution := "合成院所"
		if large {
			institution = strings.Repeat("S", 4096)
		}
		respondJSON(t, w, map[string]any{"sp_IHKE3307S01_data": []any{map[string]any{"row_ID": "synthetic", "ori_TYPE": "A", "func_DATE": "115/10/28", "hosp_ABBR": institution}}})
	})
	var held int64
	limit := int64(8192)
	if err := s.SetReferenceBudget(func(previous, next int64) bool {
		if previous != held {
			t.Fatalf("aggregate reference accounting lost ownership: previous=%d held=%d", previous, held)
		}
		if next > limit && next > previous {
			return false
		}
		held = next
		return true
	}); err != nil {
		t.Fatal(err)
	}
	s.token, s.tokenObtainedAt = "synthetic-token", time.Now()
	first, err := s.FetchMedical(context.Background(), "chronic", Query{})
	if err != nil || len(first.Entries) != 1 || held == 0 {
		t.Fatalf("initial admitted list failed: %v", err)
	}
	oldReference := first.Entries[0].DetailID
	oldPayload := []byte("synthetic-export-selector")
	if err := s.replaceDocumentReferences("chronic", oldReference, map[string]documentReference{"previous-document": {category: "chronic", parent: oldReference, payload: oldPayload}}); err != nil {
		t.Fatal(err)
	}
	limit, large = held, true
	_, err = s.FetchMedical(context.Background(), "chronic", Query{})
	var failure *Error
	if !errors.As(err, &failure) || failure.Code != "RESOURCE_LIMIT" || s.referenceBytes != limit || held != limit {
		t.Fatalf("capacity rejection changed accounting or returned wrong failure: %v", err)
	}
	if _, retained := s.medicalReferences["chronic"][oldReference]; !retained || len(s.medicalReferences["chronic"]) != 1 || s.documentReferences["previous-document"].parent != oldReference || string(oldPayload) != "synthetic-export-selector" {
		t.Fatal("rejected list refresh retired previous detail/document evidence")
	}
	s.Close()
	if held != 0 || s.referenceBytes != 0 || s.hasReferences() {
		t.Fatal("close retained aggregate reference capacity")
	}
}

func TestReferenceBudgetDefaultLimitAndSingleFreshOwner(t *testing.T) {
	s := &Session{}
	budget := func(_, _ int64) bool { return true }
	if s.SetReferenceBudget(nil) == nil || s.SetReferenceBudget(budget) != nil || s.SetReferenceBudget(budget) == nil {
		t.Fatal("reference budget accepted an absent or replacement owner")
	}
	seeded := &Session{selfPaidReferences: map[string]string{"old": "synthetic"}}
	if seeded.SetReferenceBudget(budget) == nil {
		t.Fatal("existing references were attached without accounting")
	}
	standalone := &Session{}
	oversized := make([]byte, defaultReferenceLimit)
	err := standalone.replaceDocumentReferences("official_export_json", "options", map[string]documentReference{"new": {category: "official_export_json", parent: "options", payload: oversized}})
	var failure *Error
	if !errors.As(err, &failure) || failure.Code != "RESOURCE_LIMIT" || standalone.referenceBytes != 0 || len(standalone.documentReferences) != 0 {
		t.Fatal("standalone connector accepted references beyond its own bound")
	}
}

func TestConsumedDocumentAndAuthenticationExpiryReleaseReferenceCapacity(t *testing.T) {
	s := &Session{}
	var held int64
	if err := s.SetReferenceBudget(func(_, next int64) bool { held = next; return true }); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"one", "two"} {
		if err := s.replaceDocumentReferences("official_export_json", id, map[string]documentReference{id: {category: "official_export_json", parent: id, payload: []byte("synthetic-selector")}}); err != nil {
			t.Fatal(err)
		}
	}
	before := held
	s.consumeDocumentReference("one")
	if held <= 0 || held >= before || len(s.documentReferences) != 1 {
		t.Fatal("consumed preparation reference did not release its logical storage")
	}
	s.clearAuth()
	if held != 0 || s.hasReferences() {
		t.Fatal("authentication expiry retained reference budget")
	}
}

func TestReferenceBudgetRejectedNestedDeviceRefreshPreservesParent(t *testing.T) {
	s := fixtureSession(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/ihke3000/ihke3205s01/page_load":
			respondJSON(t, w, map[string]any{"maindata": []any{}, "maindata_10568": []any{}})
		case "/api/ihke3000/ihke3205s02/page_load":
			respondJSON(t, w, map[string]any{"sP_IHKE3205S04": []any{}, "devicelist": []any{map[string]any{"cdate": "115/10/28", "edate": "20261028", "value": "1"}}})
		case "/api/ihke3000/ihke3205s02/SP_IHKE3205S08":
			respondJSON(t, w, map[string]any{"sP_IHKE3205S09": []any{}})
		default:
			t.Errorf("unexpected synthetic route %s", r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
		}
	})
	var held int64
	limit := int64(8192)
	if err := s.SetReferenceBudget(func(_, next int64) bool {
		if next > limit {
			return false
		}
		held = next
		return true
	}); err != nil {
		t.Fatal(err)
	}
	s.token, s.tokenObtainedAt = "synthetic-token", time.Now()
	list, err := s.FetchProvider(context.Background(), "physiology", Query{})
	if err != nil || len(list.Entries) != 1 {
		t.Fatalf("initial source index: %v", err)
	}
	parent := list.Entries[0].DetailID
	limit = held
	_, err = s.FetchProviderDetail(context.Background(), "physiology", parent)
	var failure *Error
	if !errors.As(err, &failure) || failure.Code != "RESOURCE_LIMIT" || len(s.providerReferences["physiology"]) != 1 || s.providerReferences["physiology"][parent].rowID != "10568" || held != limit {
		t.Fatalf("rejected nested refresh lost parent or leaked children: %v", err)
	}
	limit = 8192
	retry, err := s.FetchProviderDetail(context.Background(), "physiology", parent)
	if err != nil || len(retry.Entries) != 1 || retry.Entries[0].DetailID == "" || len(s.providerReferences["physiology"]) != 2 {
		t.Fatalf("explicit retry could not reuse retained parent: %v", err)
	}
}
