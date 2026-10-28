// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package lab

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/bestkakkoii/healthbank/internal/strictjson"
)

func TestClinicalEvidenceRegistryCoversActualRulesAndIndependentBoundaries(t *testing.T) {
	if err := strictjson.Validate(evidenceRegistryJSON, strictjson.Options{MaxDepth: 32, FoldNames: true}); err != nil {
		t.Fatal(err)
	}
	var registry struct {
		SchemaVersion int                        `json:"schema_version"`
		CatalogID     string                     `json:"catalog_id"`
		CatalogSHA256 string                     `json:"catalog_sha256"`
		Sources       map[string]json.RawMessage `json:"sources"`
		Rules         []struct {
			ID             string   `json:"id"`
			DefaultItemIDs []string `json:"default_item_ids"`
			AlertItemIDs   []string `json:"alert_item_ids"`
			Activation     string   `json:"activation"`
			Unit           string   `json:"unit"`
			UnitAliases    []string `json:"verified_unit_aliases"`
			Population     []string `json:"population"`
			Exclusions     []string `json:"exclusions"`
			Loci           []struct {
				SourceID string `json:"source_id"`
				Locator  string `json:"locator"`
			} `json:"source_loci"`
			Vectors []struct {
				Raw   string `json:"raw"`
				Level *int   `json:"expected_rule_level"`
			} `json:"test_vectors"`
		} `json:"rules"`
	}
	if err := json.Unmarshal(evidenceRegistryJSON, &registry); err != nil {
		t.Fatal(err)
	}
	catalog := gradingCatalog(t)
	provenance := catalog.Provenance()
	if registry.SchemaVersion != 1 || registry.CatalogID != provenance.CatalogID || "sha256:"+registry.CatalogSHA256 != provenance.CatalogRevision || len(provenance.EvidenceRevision) != 71 || !strings.HasPrefix(provenance.EvidenceRevision, "sha256:") {
		t.Fatal("clinical evidence does not identify the exact embedded catalog")
	}
	seen := map[string]bool{}
	for _, evidence := range registry.Rules {
		rule, exists := catalog.Rules[evidence.ID]
		if !exists || seen[evidence.ID] || len(evidence.Population) == 0 || len(evidence.Exclusions) == 0 || evidence.Unit != rule.Unit || !slices.Equal(evidence.UnitAliases, rule.Aliases) {
			t.Fatalf("incomplete or stale rule evidence: %s", evidence.ID)
		}
		seen[evidence.ID] = true
		var defaults, alerts []string
		for _, item := range catalog.Items {
			if item.Rule == evidence.ID {
				defaults = append(defaults, item.ID)
			}
			if slices.Contains(item.Alerts, evidence.ID) {
				alerts = append(alerts, item.ID)
			}
		}
		if !slices.Equal(defaults, evidence.DefaultItemIDs) || !slices.Equal(alerts, evidence.AlertItemIDs) {
			t.Fatalf("evidence activation diverged from implementation: %s", evidence.ID)
		}
		var sources []string
		for _, locus := range evidence.Loci {
			if _, ok := registry.Sources[locus.SourceID]; !ok || locus.Locator == "" {
				t.Fatalf("missing evidence source locus for %s", evidence.ID)
			}
			sources = append(sources, locus.SourceID)
		}
		if !slices.Equal(sources, rule.Sources) {
			t.Fatalf("source links diverged from rule %s", evidence.ID)
		}
		if rule.Kind == "numeric_bands" && len(evidence.Vectors) < 2 {
			t.Fatalf("numeric rule lacks independent boundary cases: %s", evidence.ID)
		}
		for _, vector := range evidence.Vectors {
			level, _ := classifyBands(parseValue(vector.Raw), sourceBands(rule))
			if !reflect.DeepEqual(level, vector.Level) {
				t.Errorf("independent evidence boundary failed %s %s: got %v want %v", evidence.ID, vector.Raw, level, vector.Level)
			}
		}
	}
	if len(seen) != len(catalog.Rules) {
		t.Fatal("implemented rule is missing its evidence review")
	}
}
