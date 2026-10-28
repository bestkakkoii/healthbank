// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package server

import (
	"context"
	"net"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/bestkakkoii/healthbank/internal/store"
)

// Operators call the listener directly. Reverse proxies must not expose this
// diagnostic endpoint, even when their own upstream address is loopback.
func directLoopback(r *http.Request) bool {
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	ip := net.ParseIP(peer)
	if err != nil || ip == nil || !ip.IsLoopback() || r.Header.Get("Origin") != "" || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	host := r.Host
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		host = parsed
	}
	host = strings.Trim(host, "[]")
	if host != "localhost" {
		ip = net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return false
		}
	}
	for name := range r.Header {
		lower := strings.ToLower(name)
		if lower == "forwarded" || lower == "via" || lower == "x-real-ip" || strings.HasPrefix(lower, "x-forwarded-") {
			return false
		}
	}
	return true
}

func (a *App) readiness(w http.ResponseWriter, r *http.Request) {
	if !directLoopback(r) {
		a.fail(w, r, http.StatusNotFound, "not_found", "找不到這個服務端點。")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.store.Ready(ctx); err != nil {
		a.resourceFailure(w, r, "settings_unavailable", "設定資料庫目前無法使用。")
		return
	}
	origin, err := a.store.IdentityKeyOrigin(ctx)
	if err != nil {
		a.resourceFailure(w, r, "identity_key_status_unavailable", "無法確認設定資料庫的金鑰配對狀態。")
		return
	}
	keyStatus := "recorded"
	if origin == store.IdentityKeyLegacyUnverified {
		keyStatus = "unverified"
	}
	a.mu.Lock()
	sessions, anonymous, pending := len(a.sessions), 0, 0
	for _, s := range a.sessions {
		if s.anonymous {
			anonymous++
		}
		pending += s.pending
	}
	a.mu.Unlock()
	a.budget.mu.Lock()
	working, retained, heavy, refusals := a.budget.working, a.budget.retained, a.budget.heavy, a.budget.refusals
	a.budget.mu.Unlock()
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	a.json(w, http.StatusOK, map[string]any{
		"status": "ready", "version": a.version, "database": "reachable",
		"identity_key_status": keyStatus, "identity_key_origin": origin,
		"resources": map[string]any{
			"sessions": sessions, "anonymous_sessions": anonymous, "pending_requests": pending,
			"heavy_operations": heavy, "working_reserved_bytes": working, "retained_accounted_bytes": retained,
			"admission_refusals": refusals, "heap_allocated_bytes": memory.HeapAlloc, "heap_system_bytes": memory.HeapSys,
			"max_sessions": a.limits.Sessions, "max_anonymous_sessions": a.limits.AnonymousSessions,
			"max_requests": a.limits.Requests, "max_session_queue": a.limits.SessionQueue,
			"max_heavy_operations": a.limits.HeavyRequests, "max_connections": a.limits.Connections,
			"working_budget_bytes": a.limits.WorkingBytes, "retained_budget_bytes": a.limits.RetainedBytes, "session_budget_bytes": a.limits.SessionBytes,
		},
	})
}
