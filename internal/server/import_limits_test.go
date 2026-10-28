// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/bestkakkoii/healthbank/internal/lab"
)

// Stream the test file through multipart parsing instead of allocating another
// complete copy of a large report or using disk-backed multipart helpers.
func importMultipart(t *testing.T, filename string, payload io.Reader) (io.Reader, string) {
	t.Helper()
	var envelope bytes.Buffer
	writer := multipart.NewWriter(&envelope)
	if _, err := writer.CreateFormFile("file", filename); err != nil {
		t.Fatal(err)
	}
	header := append([]byte(nil), envelope.Bytes()...)
	envelope.Reset()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return io.MultiReader(bytes.NewReader(header), payload, bytes.NewReader(envelope.Bytes())), writer.FormDataContentType()
}

func TestPortableJSONImportAboveSavedPageLimitStaysInMemory(t *testing.T) {
	app, db, dir := testApp(t)
	browser := &browserFixture{app: app}
	browser.refresh(t)
	uploadTemp := t.TempDir()
	t.Setenv("TMPDIR", uploadTemp)
	before, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	const count = 400
	source := strings.Repeat("SYNTHETIC_SOURCE_", 2048)
	row := map[string]string{"check_date": "115/01/02", "order_code": "09026C", "item_name": "ALT", "result": "37.81250000000000001", "unit": "U/L", "reference": "[7][52]", "source": source}
	rows := make([]map[string]string, count)
	for i := range rows {
		rows[i] = row
	}
	data, err := json.Marshal(map[string]any{"schema": "healthbank.lab.v1", "records": rows})
	if err != nil {
		t.Fatal(err)
	}
	if len(data) <= 12<<20 || len(data) >= 32<<20 {
		t.Fatal("fixture must cross the old upload limit without exceeding portable capacity")
	}
	body, contentType := importMultipart(t, "synthetic-history.json", bytes.NewReader(data))
	response := browser.request(http.MethodPost, "/api/reports/import", body, contentType, browser.csrf)
	imported := decodeResponse[reportResponse](t, response, http.StatusOK)
	if len(imported.Records) != count || imported.Complete || imported.OfficialQuery != nil || len(imported.Warnings) == 0 {
		t.Fatal("large portable import lost rows or acquired official completeness")
	}
	if imported.Records[0].Result != row["result"] || imported.Records[count-1].Source != source {
		t.Fatal("large import altered exact source fields")
	}
	view := browser.refresh(t)
	if view.Authenticated || view.Mode != "import" {
		t.Fatal("portable history authenticated its owner")
	}
	noPersistentProfiles(t, db)
	after, err := os.ReadDir(dir)
	if err != nil || len(after) != len(before) {
		t.Fatal("large import created a persistent file")
	}
	for i := range before {
		if before[i].Name() != after[i].Name() {
			t.Fatal("large import changed persistent filenames")
		}
	}
	if files, err := os.ReadDir(uploadTemp); err != nil || len(files) != 0 {
		t.Fatal("large multipart import spooled health data to a temporary file")
	}
}

func TestImportUploadFileAndEnvelopeLimitsAreAtomic(t *testing.T) {
	for _, kind := range []string{"portable_file", "chunked_epilogue", "saved_zip"} {
		t.Run(kind, func(t *testing.T) {
			app, _, _ := testApp(t)
			browser := &browserFixture{app: app}
			browser.refresh(t)
			attachVerifiedFixture(t, browser, "SYNTHETIC_IMPORT_BOUNDARY", []lab.RawRecord{{ItemName: "ALT", Result: "17.125", Source: "SYNTHETIC_PRIOR"}})
			before := *browser.cookie
			var body io.Reader
			var contentType string
			status := http.StatusRequestEntityTooLarge
			switch kind {
			case "portable_file":
				body, contentType = importMultipart(t, "too-large.json", strings.NewReader(strings.Repeat(" ", (32<<20)+1)))
			case "chunked_epilogue":
				body, contentType = importMultipart(t, "small.json", strings.NewReader(`{"schema":"healthbank.lab.v1","records":[]}`))
				body = io.MultiReader(body, strings.NewReader(strings.Repeat(" ", (32<<20)+(64<<10))))
			case "saved_zip":
				body, contentType = importMultipart(t, "saved.zip", strings.NewReader(strings.Repeat(" ", (12<<20)+1)))
				status = http.StatusBadRequest
			}
			response := browser.request(http.MethodPost, "/api/reports/import", body, contentType, browser.csrf)
			if response.Code != status || browser.cookie.Value != before.Value {
				t.Fatalf("oversized %s changed session or returned HTTP %d instead of %d", kind, response.Code, status)
			}
			if kind == "saved_zip" && !strings.Contains(response.Body.String(), "12 MiB") {
				t.Fatal("saved ZIP did not retain its smaller input bound")
			}
			current := decodeResponse[reportResponse](t, browser.request(http.MethodGet, "/api/reports/current", nil, "", ""), http.StatusOK)
			if len(current.Records) != 1 || current.Records[0].Result != "17.125" || current.Records[0].Source != "SYNTHETIC_PRIOR" {
				t.Fatal("failed upload replaced the earlier successful report")
			}
			if view := browser.refresh(t); !view.Authenticated || view.Mode != "official" {
				t.Fatal("failed upload ended the earlier verified fixture session")
			}
		})
	}
}

func TestNginxImportEnvelopeFitsMemoryBuffer(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "deploy", "nginx.healthbank.conf"))
	if err != nil {
		t.Fatal(err)
	}
	configuration := string(data)
	size := func(name string) int64 {
		t.Helper()
		match := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(name) + `\s+(\d+)([km]?)\s*;`).FindStringSubmatch(configuration)
		if len(match) != 3 {
			t.Fatalf("missing bounded nginx directive %s", name)
		}
		value, err := strconv.ParseInt(match[1], 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		switch match[2] {
		case "k":
			value *= 1 << 10
		case "m":
			value *= 1 << 20
		}
		return value
	}
	maximum := size("client_max_body_size")
	if maximum != lab.MaxImportBytes+(64<<10) || size("client_body_buffer_size") < maximum {
		t.Fatal("nginx can reject an accepted upload or spill its request body beyond the memory buffer")
	}
	for _, directive := range []string{"client_body_in_file_only off;", "proxy_request_buffering off;", "proxy_buffering off;", "proxy_max_temp_file_size 0;", "proxy_cache off;", "proxy_store off;"} {
		if !strings.Contains(configuration, directive) {
			t.Fatalf("nginx import privacy setting missing: %s", directive)
		}
	}
}
