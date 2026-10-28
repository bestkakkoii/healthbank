// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package server

import (
	"net/http"
	"sync"
	"time"

	"github.com/bestkakkoii/healthbank/internal/config"
	"github.com/bestkakkoii/healthbank/internal/lab"
)

const anonymousLifetime = 5 * time.Minute
const sessionQueueWait = 5 * time.Second

// idleLifetime and makeAnonymousRoom are called with App.mu held.
func (s *session) idleLifetime() time.Duration {
	if s.anonymous {
		return anonymousLifetime
	}
	return sessionLifetime
}

func (a *App) makeAnonymousRoom() bool {
	return a.trimAnonymous(a.limits.AnonymousSessions - 1)
}

// Active logouts may briefly become anonymous together. Once their request
// completes, retire excess blank sessions without touching another workspace
// or an in-progress CAPTCHA. New creation also requires room in this allowance.
func (a *App) trimAnonymous(target int) bool {
	for {
		count := 0
		var oldest *session
		var oldestID string
		for id, s := range a.sessions {
			if s.anonymous {
				count++
				if s.reclaimable && s.pending == 0 && (oldest == nil || s.touched.Before(oldest.touched)) {
					oldest, oldestID = s, id
				}
			}
		}
		if count <= target {
			return true
		}
		if oldest == nil {
			return false
		}
		delete(a.sessions, oldestID)
		go func() { oldest.mu.Lock(); a.clearSession(oldest); oldest.mu.Unlock() }()
	}
}

func (a *App) protectWorkspace(s *session) {
	a.mu.Lock()
	s.anonymous, s.reclaimable = false, false
	a.mu.Unlock()
}

// ConnectionLimit is the bound for accepted HTTP connections, including readers.
func (a *App) ConnectionLimit() int { return a.limits.Connections }

// The budget counts conservative working-set reservations and logical retained
// bytes separately. Go allocator/GC, sockets and reverse-proxy memory still need
// a measured deployment margin; these counters are not process heap/RSS values.
type resourceBudget struct {
	mu                sync.Mutex
	limits            config.ResourceLimits
	working, retained int64
	heavy             int
	refusals          uint64
	creationTokens    float64
	creationAt        time.Time
}

type sessionMemory struct {
	budget              *resourceBudget
	reports, references int64 // protected by budget.mu
}

func newResourceBudget(limits config.ResourceLimits) *resourceBudget {
	return &resourceBudget{limits: limits, creationTokens: float64(limits.AnonymousSessions), creationAt: time.Now()}
}

func (b *resourceBudget) admitCreation(now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	// A process-wide one-per-second refill avoids retaining IP addresses or
	// fingerprints. Existing session reads never consume creation allowance.
	b.creationTokens = min(float64(b.limits.AnonymousSessions), b.creationTokens+max(0, now.Sub(b.creationAt).Seconds()))
	b.creationAt = now
	if b.creationTokens < 1 {
		b.refusals++
		return false
	}
	b.creationTokens--
	return true
}

func (b *resourceBudget) acquireWorking(bytes int64, heavy bool) (func(), bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if bytes > b.limits.WorkingBytes-b.working || (heavy && b.heavy >= b.limits.HeavyRequests) {
		b.refusals++
		return nil, false
	}
	b.working += bytes
	if heavy {
		b.heavy++
	}
	return func() {
		b.mu.Lock()
		b.working -= bytes
		if heavy {
			b.heavy--
		}
		b.mu.Unlock()
	}, true
}

func (m *sessionMemory) resize(references bool, previous, next int64) bool {
	b := m.budget
	b.mu.Lock()
	defer b.mu.Unlock()
	current := m.reports
	if references {
		current = m.references
	}
	if current != previous || next < 0 {
		return false
	}
	difference := next - previous
	if difference > 0 && (difference > b.limits.RetainedBytes-b.retained || difference > b.limits.SessionBytes-m.reports-m.references) {
		b.refusals++
		return false
	}
	b.retained += difference
	if references {
		m.references = next
	} else {
		m.reports = next
	}
	return true
}

func (m *sessionMemory) referenceBudget(previous, next int64) bool {
	return m.resize(true, previous, next)
}

func (m *sessionMemory) replaceReports(rows []lab.RawRecord) bool {
	// Include slice/string headers, map/key discovery and per-record metadata.
	// Source text is counted even where Go may share string backing storage.
	bytes := int64(len(rows)) * 512
	for _, row := range rows {
		bytes += int64(len(row.CheckDate) + len(row.OrderCode) + len(row.OrderName) + len(row.ItemName) + len(row.Result) + len(row.Unit) + len(row.Reference) + len(row.Institution) + len(row.VisitDate) + len(row.Source) + len(row.OfficialFlag))
	}
	m.budget.mu.Lock()
	previous := m.reports
	m.budget.mu.Unlock()
	return m.resize(false, previous, bytes)
}

func (m *sessionMemory) releaseReports() {
	m.budget.mu.Lock()
	previous := m.reports
	m.budget.mu.Unlock()
	_ = m.resize(false, previous, 0)
}

func (a *App) operationWorkingSet(r *http.Request, s *session) (int64, bool) {
	switch r.URL.Path {
	case "/api/reports/import":
		return 192 << 20, true
	case "/api/reports", "/api/supplementary":
		return 128 << 20, true
	case "/api/documents":
		if s.official != nil {
			return s.official.DocumentWorkingSet(r.URL.Query().Get("category"), r.URL.Query().Get("id")), true
		}
		return 1 << 20, false
	case "/api/reports/current", "/api/catalog":
		return 96 << 20, false
	case "/api/auth/captcha", "/api/auth/login", "/api/documents/prepare":
		return 16 << 20, true
	case "/api/reports/demo", "/api/settings":
		return 8 << 20, false
	default:
		return 0, false // Small session reconciliation/logout responses retain headroom.
	}
}

func (a *App) resourceFailure(w http.ResponseWriter, r *http.Request, code, message string) {
	w.Header().Set("Retry-After", "1")
	a.fail(w, r, http.StatusServiceUnavailable, code, message)
}

func (a *App) admitReport(w http.ResponseWriter, r *http.Request, s *session, rows []lab.RawRecord) bool {
	if s.memory != nil && !s.memory.replaceReports(rows) {
		a.resourceFailure(w, r, "retained_capacity", "目前資料保留容量已達上限；先前資料保持不變，請縮小取得範圍或登出後重新取得。")
		return false
	}
	return true
}
