// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package server

import (
	"context"
	"errors"
	"net/http"
)

var errSubjectRevoked = errors.New("verified subject session has been revoked")

// subjectScope coordinates only one verified person's configuration mutations.
// Request/session locks always precede mutation admission. Revocation removes
// sessions without waiting for their locks and cancels their active operations.
type subjectScope struct {
	ctx        context.Context
	cancel     context.CancelFunc
	mutation   chan struct{}
	references int // protected by App.mu
}

// bindSubject requires s.mu. The HMAC key is linked only after official login.
func (a *App) bindSubject(s *session, subject string) {
	a.releaseSubject(s)
	a.mu.Lock()
	defer a.mu.Unlock()
	scope := a.subjects[subject]
	if scope == nil || scope.ctx.Err() != nil {
		ctx, cancel := context.WithCancel(context.Background())
		scope = &subjectScope{ctx: ctx, cancel: cancel, mutation: make(chan struct{}, 1)}
		a.subjects[subject] = scope
	}
	scope.references++
	s.subject, s.scope = subject, scope
	s.anonymous, s.reclaimable = false, false
}

// releaseSubject requires s.mu; scope membership is also protected by App.mu so
// deletion never needs to acquire an unrelated session's lock to identify it.
func (a *App) releaseSubject(s *session) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if scope := s.scope; scope != nil {
		scope.references--
		if scope.references == 0 {
			if a.subjects[s.subject] == scope {
				delete(a.subjects, s.subject)
			}
			scope.cancel()
		}
	}
	s.subject, s.scope = "", nil
}

func (a *App) lockSubject(ctx context.Context, s *session) (func(), error) {
	if s.subject == "" {
		return func() {}, nil
	}
	scope := s.scope
	if scope == nil || scope.ctx.Err() != nil {
		return nil, errSubjectRevoked
	}
	select {
	case scope.mutation <- struct{}{}:
		if scope.ctx.Err() != nil {
			<-scope.mutation
			return nil, errSubjectRevoked
		}
		return func() { <-scope.mutation }, nil
	case <-scope.ctx.Done():
		return nil, errSubjectRevoked
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// revokeSubject requires the current session and its subject mutation slot.
// All old generations disappear before deletion is reported as successful.
func (a *App) revokeSubject(current *session) {
	scope := current.scope
	if scope == nil {
		return
	}
	a.mu.Lock()
	if a.subjects[current.subject] == scope {
		delete(a.subjects, current.subject)
	}
	var retired []*session
	for id, candidate := range a.sessions {
		if candidate != current && candidate.scope == scope {
			delete(a.sessions, id)
			retired = append(retired, candidate)
		}
	}
	scope.cancel()
	a.mu.Unlock()
	for _, candidate := range retired {
		go func() {
			candidate.mu.Lock()
			a.clearSession(candidate)
			candidate.mu.Unlock()
		}()
	}
}

func (a *App) requireCurrentSubject(w http.ResponseWriter, r *http.Request, s *session) bool {
	if s.scope != nil && s.scope.ctx.Err() != nil {
		a.fail(w, r, http.StatusConflict, "session_changed", "此身分的同步設定與連線已清除，請重新確認目前使用者。")
		return false
	}
	return true
}

func (a *App) subjectAdmissionError(w http.ResponseWriter, r *http.Request, s *session) {
	if !a.requireCurrentSubject(w, r, s) {
		return
	}
	a.fail(w, r, http.StatusRequestTimeout, "request_canceled", "操作已取消或逾時，未執行設定變更。")
}
