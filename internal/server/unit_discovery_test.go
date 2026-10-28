// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package server

import (
	"context"
	"strings"
	"testing"

	"github.com/bestkakkoii/healthbank/internal/lab"
)

func TestDiscoveryPersistsUnitIsolationInsteadOfRestoringStaleLimits(t *testing.T) {
	a, db, _ := testApp(t)
	b := &browserFixture{app: a}
	b.refresh(t)
	subject := attachVerifiedFixture(t, b, "SYNTHETIC_UNIT_MIGRATION", nil)
	settings := lab.DefaultSettings()
	high, bound := "1.6", "144"
	settings.Theme = "dark"
	settings.Known["serum_creatinine"] = lab.KnownItem{ID: "serum_creatinine", Key: "serum_creatinine", Name: "Synthetic creatinine", Unit: "Mg/dL", Category: "renal"}
	settings.Overrides["serum_creatinine"] = lab.Override{Enabled: true, ReferenceHigh: &high}
	settings.Known["serum_sodium"] = lab.KnownItem{ID: "serum_sodium", Key: "serum_sodium", Name: "Synthetic sodium", Unit: "mmol/L", Category: "electrolytes"}
	settings.Overrides["serum_sodium"] = lab.Override{Enabled: true, ReferenceHigh: &bound}
	settings.Favorites = []string{"serum_creatinine", "serum_sodium"}
	doc, err := db.Get(context.Background(), subject)
	if err != nil {
		t.Fatal(err)
	}
	data, err := settings.Document()
	if err != nil {
		t.Fatal(err)
	}
	saved, err := db.Put(context.Background(), subject, doc.Revision, data)
	if err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	s := a.sessions[b.cookie.Value]
	a.mu.Unlock()
	s.mu.Lock()
	if err := a.refreshSettings(context.Background(), s); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	rows := []lab.RawRecord{{ItemName: "Creatinine", OrderCode: "09015C", Unit: "mg/dL", Result: "1.2"}}
	a.catalog.Normalize(rows, &s.settings)
	if err := a.persistDiscovered(context.Background(), s); err != nil {
		s.mu.Unlock()
		t.Fatal(err)
	}
	s.mu.Unlock()
	current, err := db.Get(context.Background(), subject)
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != saved.Revision+1 {
		t.Fatal("discovery did not persist the reconciled legacy settings")
	}
	restored, err := ValidatePortableSettings(current.Settings)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := restored.Overrides["serum_creatinine"]; exists || !restored.Overrides["serum_sodium"].Enabled || restored.Theme != "dark" || len(restored.Favorites) != 2 {
		t.Fatal("source discovery resurrected ambiguous limits or lost independent preferences")
	}
	retained := 0
	for key, item := range restored.Known {
		if strings.HasPrefix(key, "unit_review_") {
			retained++
			override := restored.Overrides[key]
			if item.Unit != "Mg/dL" || override.Enabled || override.ReferenceHigh == nil || *override.ReferenceHigh != "1.6" {
				t.Fatal("legacy limits were not preserved as disabled review data")
			}
		}
	}
	if retained != 1 {
		t.Fatal("expected exactly one recoverable unit review entry")
	}
	s.mu.Lock()
	err = a.persistDiscovered(context.Background(), s)
	s.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	again, err := db.Get(context.Background(), subject)
	if err != nil || again.Revision != current.Revision {
		t.Fatal("repeated discovery created a migration write loop")
	}
}
