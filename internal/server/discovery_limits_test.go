// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/bestkakkoii/healthbank/internal/config"
	"github.com/bestkakkoii/healthbank/internal/lab"
	"github.com/bestkakkoii/healthbank/internal/store"
)

func TestDiscoveryCannotPersistDefinitionsRejectedByPortableSettings(t *testing.T) {
	for _, kind := range []string{"long_name", "long_unit", "too_many_items"} {
		t.Run(kind, func(t *testing.T) {
			app, db, _ := testApp(t)
			browser := &browserFixture{app: app}
			browser.refresh(t)
			rows := []lab.RawRecord{{ItemName: "SYNTHETIC_NEW_ITEM", Result: "17.125", Unit: "U/L"}}
			switch kind {
			case "long_name":
				rows[0].ItemName = strings.Repeat("合成", 51)
			case "long_unit":
				rows[0].Unit = strings.Repeat("S", 81)
			case "too_many_items":
				rows = make([]lab.RawRecord, 1001)
				for i := range rows {
					rows[i] = lab.RawRecord{ItemName: fmt.Sprintf("SYNTHETIC_ITEM_%d", i), Result: "17.125", Unit: "U/L"}
				}
			}
			app.mu.Lock()
			s := app.sessions[browser.cookie.Value]
			app.mu.Unlock()
			s.mu.Lock()
			app.bindSubject(s, config.Subject(app.secret, "SYNTHETIC_DISCOVERY_BOUNDARY"))
			s.mode, s.raw = "official", rows
			app.catalog.Normalize(rows, &s.settings)
			err := app.persistDiscovered(context.Background(), s)
			subject := s.subject
			s.mu.Unlock()
			if !errors.Is(err, store.ErrInvalid) {
				t.Fatal("source discovery persisted definitions that its restore validator rejects")
			}
			if saved, err := db.Get(context.Background(), subject); err != nil || saved.Revision != 0 {
				t.Fatal("failed discovery partially changed stored configuration")
			}
			current := decodeResponse[reportResponse](t, browser.request(http.MethodGet, "/api/reports/current", nil, "", ""), http.StatusOK)
			if len(current.Records) != len(rows) || current.Records[0].ItemName != rows[0].ItemName || current.Records[0].Unit != rows[0].Unit || current.Records[0].Result != rows[0].Result {
				t.Fatal("failed settings discovery truncated or changed original laboratory data")
			}
			// Failed discovery must not prevent unrelated ordinary preferences from
			// being saved from the last valid persistent configuration.
			view := browser.refresh(t)
			payload := settingsPayload(view.Settings)
			payload["theme"] = "dark"
			decodeResponse[lab.Settings](t, browser.jsonRequest(t, http.MethodPut, "/api/settings", payload), http.StatusOK)
			saved, err := db.Get(context.Background(), subject)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ValidatePortableSettings(saved.Settings); err != nil {
				t.Fatal("ordinary settings save produced an un-restorable configuration")
			}
		})
	}
}
