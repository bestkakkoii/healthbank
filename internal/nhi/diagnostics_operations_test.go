// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import "testing"

func TestSourceAndDocumentDiagnosticsUseFixedOperations(t *testing.T) {
	for _, tc := range []struct {
		path      string
		operation Operation
	}{
		{"/api/ihke3000/ihke3306s01/search", OperationSourceList},
		{"/api/ihke3000/IHKE3306S02/page_load", OperationSourceDetail},
		{"/api/ihke3000/ihke3202s01/SP_IHKE3202S01", OperationSourceRead},
		{"/api/ihke3000/ihke3516s02/SP_IHKE3516S02", OperationSourceDetail},
		{"/api/ihke3000/ihke3803s01/page_load", OperationSourceList},
		{"/api/ihke3000/ihke3803s01/add", OperationDocumentPrepare},
		{"/api/ihke3000/IHKE3411S02/add", OperationDocumentPrepare},
		{"/api/ihke3000/ihke3801s02/app_imgF_download", OperationDocumentDownload},
		{"/api/ihke3000/IHKE3309S02/delpdf", OperationDocumentCleanup},
		{"/ihke3000/synthetic-document.pdf", OperationDocumentDownload},
		{"https://other.invalid/ihke3000/SECRET.pdf", OperationUnknown},
		{"/ihke3000/unsafe%25.pdf", OperationUnknown},
		{"/api/ihke3000/SECRET_PATIENT", OperationUnknown},
	} {
		if got := operationForPath(tc.path); got != tc.operation {
			t.Errorf("fixed operation mapping mismatch: %s != %s", got, tc.operation)
		}
	}
	for category, spec := range providerSpecs {
		for _, request := range spec.requests {
			if operationForPath(request.path) == OperationUnknown {
				t.Fatal("provider contract has no bounded diagnostic operation")
			}
		}
		if spec.info.HasDetails {
			request, err := providerDetailRequest(category, providerReference{rowID: "synthetic", sourceType: "synthetic"})
			if err != nil || operationForPath(request.path) == OperationUnknown {
				t.Fatalf("provider detail contract %s has no bounded diagnostic operation", category)
			}
		}
	}
	for _, spec := range medicalSpecs {
		for _, path := range []string{spec.listPath, spec.searchPath, spec.detailPath} {
			if path != "" && operationForPath(path) == OperationUnknown {
				t.Fatal("medical contract has no bounded diagnostic operation")
			}
		}
	}
	for _, operation := range []Operation{OperationSourceList, OperationSourceDetail, OperationSourceRead, OperationDocumentPrepare, OperationDocumentDownload, OperationDocumentCleanup} {
		got := (&Diagnostics{Operation: operation, Phase: PhaseResponseBody, ElapsedMS: 200000}).Sanitized()
		if got.Operation != operation || got.ElapsedMS != 200000 {
			t.Fatal("valid long document diagnostics lost operation or duration")
		}
	}
}

func TestDocumentWorkingReservationsDistinguishStreamingFromBuffered(t *testing.T) {
	s := &Session{documentReferences: map[string]documentReference{}}
	for _, tc := range []struct {
		kind  string
		bytes int64
	}{
		{"official_archive", 1 << 20}, {"official_pdf", 1 << 20}, {"dicom_archive", 1 << 20}, {"prescription_pdf", 1 << 20},
		{"hospital_pdf", 64 << 20}, {"hospital_markup", 192 << 20}, {"imaging_jpeg", 192 << 20},
	} {
		s.documentReferences["synthetic-reference"] = documentReference{kind: tc.kind, category: "synthetic-category"}
		if s.DocumentWorkingSet("synthetic-category", "synthetic-reference") != tc.bytes {
			t.Fatal("buffered document reservation does not match its supported working set")
		}
		if s.DocumentWorkingSet("other-category", "synthetic-reference") != 1<<20 {
			t.Fatal("cross-category reference influenced valid transfer admission")
		}
	}
}
