// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package lab

import (
	"strings"
	"testing"

	"github.com/bestkakkoii/healthbank/internal/strictjson"
)

func TestEmbeddedCatalogHasUnambiguousSourceAndRuleContracts(t *testing.T) {
	if err := strictjson.Validate(catalogJSON, strictjson.Options{}); err != nil {
		t.Fatal(err)
	}
	catalog := gradingCatalog(t)
	ids := map[string]bool{}
	matches := map[string]string{}
	for _, item := range catalog.Items {
		if item.ID == "" || ids[item.ID] {
			t.Fatal("duplicate or empty item identity")
		}
		ids[item.ID] = true
		if _, ok := catalog.Rules[item.Rule]; !ok {
			t.Fatalf("%s uses an undefined rule", item.ID)
		}
		for _, rule := range item.Alerts {
			if _, ok := catalog.Rules[rule]; !ok {
				t.Fatalf("%s uses an undefined independent alert", item.ID)
			}
		}
		for _, source := range append([]match{item.Match}, item.Aliases...) {
			key := strings.ToUpper(strings.TrimSpace(source.Code)) + "\x00" + source.Name + "\x00" + unitKey(source.Unit)
			if previous := matches[key]; previous != "" && previous != item.ID {
				t.Fatalf("source match belongs to both %s and %s", previous, item.ID)
			}
			matches[key] = item.ID
		}
		if item.Reference != nil {
			if item.Reference.AutomaticFallback || catalog.Sources[item.Reference.Source].Title == "" {
				t.Fatalf("%s has an unverified fallback or missing reference attribution", item.ID)
			}
		}
		for _, bound := range []*string{item.Domain.Minimum, item.Domain.Maximum} {
			if bound != nil {
				if _, ok := decimal(*bound); !ok {
					t.Fatalf("%s has an invalid physical bound", item.ID)
				}
			}
		}
	}
	for id, rule := range catalog.Rules {
		for _, source := range rule.Sources {
			if catalog.Sources[source].Title == "" {
				t.Fatalf("%s has missing source attribution", id)
			}
		}
		if len(rule.Bands) != 0 {
			settings := DefaultSettings()
			settings.Overrides["synthetic"] = Override{Enabled: true, Bands: sourceBands(rule)}
			if err := ValidateSettings(settings, map[string]KnownItem{"synthetic": {Key: "synthetic"}}); err != nil {
				t.Fatalf("%s has invalid or overlapping exact bands: %v", id, err)
			}
		}
	}
}
