// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bestkakkoii/healthbank/internal/store"
)

type profileResponse struct {
	header http.Header
	status int
	bytes  int64
}

func (w *profileResponse) Header() http.Header    { return w.header }
func (w *profileResponse) WriteHeader(status int) { w.status = status }
func (w *profileResponse) Write(data []byte) (int, error) {
	w.bytes += int64(len(data))
	return len(data), nil
}

// BenchmarkBoundedPortableImport provides a reproducible synthetic capacity
// sample, not a portable RSS ceiling. Use -benchtime=1x outside the race runtime;
// the prepared client payload remains in the baseline, and responses are drained
// instead of retaining an extra full recorder copy. Runtime/proxy margin is extra.
func BenchmarkBoundedPortableImport(b *testing.B) {
	db, err := store.Open(context.Background(), store.Options{SQLitePath: filepath.Join(b.TempDir(), "settings.sqlite")})
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	a, err := New(db, bytes.Repeat([]byte{7}, 32), "synthetic-memory-profile")
	if err != nil {
		b.Fatal(err)
	}
	defer a.Close()
	row := map[string]string{"check_date": "115/01/02", "item_name": "ALT", "order_code": "09026C", "result": "37.81250000000000001", "unit": "U/L", "source": strings.Repeat("SYNTHETIC_SOURCE_", 3800)}
	rows := make([]map[string]string, 500)
	for i := range rows {
		rows[i] = row
	}
	payload, err := json.Marshal(map[string]any{"schema": "healthbank.lab.v1", "records": rows})
	if err != nil {
		b.Fatal(err)
	}
	initial := httptest.NewRecorder()
	a.Handler().ServeHTTP(initial, httptest.NewRequest(http.MethodGet, "http://healthbank.test/api/session", nil))
	var view sessionResponse
	if json.Unmarshal(initial.Body.Bytes(), &view) != nil {
		b.Fatal("initial synthetic session failed")
	}
	cookie := initial.Result().Cookies()[0]
	var envelope bytes.Buffer
	writer := multipart.NewWriter(&envelope)
	if _, err := writer.CreateFormFile("file", "synthetic-profile.json"); err != nil {
		b.Fatal(err)
	}
	header := append([]byte(nil), envelope.Bytes()...)
	envelope.Reset()
	if err := writer.Close(); err != nil {
		b.Fatal(err)
	}
	runtime.GC()
	runtime.GC() // Drop the fixture encoder's sync.Pool backing buffer from baseline.
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	baseline := memory.HeapAlloc
	var peak atomic.Uint64
	peak.Store(baseline)
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		tick := time.NewTicker(5 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				var current runtime.MemStats
				runtime.ReadMemStats(&current)
				for old := peak.Load(); current.HeapAlloc > old; old = peak.Load() {
					if peak.CompareAndSwap(old, current.HeapAlloc) {
						break
					}
				}
			}
		}
	}()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for range b.N {
		body := io.MultiReader(bytes.NewReader(header), bytes.NewReader(payload), bytes.NewReader(envelope.Bytes()))
		r := httptest.NewRequest(http.MethodPost, "http://healthbank.test/api/reports/import", body)
		r.Header.Set("Content-Type", writer.FormDataContentType())
		r.Header.Set("X-CSRF-Token", view.CSRFToken)
		r.AddCookie(cookie)
		w := &profileResponse{header: http.Header{}}
		a.Handler().ServeHTTP(w, r)
		if w.status != http.StatusOK || w.bytes < int64(len(payload)) {
			b.Fatal("large synthetic import did not preserve its source payload")
		}
		response := &http.Response{Header: w.header}
		cookie = response.Cookies()[0]
		a.mu.Lock()
		s := a.sessions[cookie.Value]
		a.mu.Unlock()
		s.mu.Lock()
		view.CSRFToken = s.csrf
		s.mu.Unlock()
	}
	b.StopTimer()
	close(stop)
	<-done
	b.ReportMetric(float64(peak.Load()-baseline)/(1<<20), "MiB-heap-growth")
	b.ReportMetric(float64(a.budget.retained)/(1<<20), "MiB-retained-accounting")
	b.ReportMetric(float64(len(payload))/(1<<20), "MiB-input")
}
