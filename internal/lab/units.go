// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package lab

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"
)

// These are spelling aliases of the units actually used by this catalog, not
// general concentration conversion or a case-insensitive UCUM interpreter.
// UCUM 2.2 defines both l and L for liter, but m and M are different prefixes.
// Source/display micro glyphs and superscripts are normalized separately.
var unitSpellingAliases = map[string]string{
	"mg/dl": "mg/dL", "g/dl": "g/dL", "mg/l": "mg/L", "ng/l": "ng/L",
	"ng/ml": "ng/mL", "pg/ml": "pg/mL", "fl": "fL", "U/l": "U/L",
	"mmol/l": "mmol/L", "umol/l": "umol/L",
	"x10^3/ul": "x10^3/uL", "x10^6/ul": "x10^6/uL",
	"ml/min/1.73m2":  "mL/min/1.73m2",
	"ml/min/1.73m^2": "mL/min/1.73m2", "mL/min/1.73m^2": "mL/min/1.73m2",
}

func unitKey(value string) string {
	value = strings.TrimSpace(value)
	value = strings.NewReplacer("μ", "u", "µ", "u", "²", "2", "³", "3", " ", "", "／", "/").Replace(value)
	if canonical, ok := unitSpellingAliases[value]; ok {
		return canonical
	}
	return value
}

const unitReviewRequired = "unit_review_required"

var previousDynamicKey = regexp.MustCompile(`^custom_[a-f0-9]{20}$`)

// The old dynamic key lowercased every unit. Even a normal last-seen unit cannot
// prove which of the collided spellings a saved personal limit was intended for.
// Retain those limits visibly, but never apply them to a new unit-aware identity.
func (c *Catalog) isolateAmbiguousUnitSettings(s *Settings) {
	for id, item := range s.Known {
		if previousDynamicKey.MatchString(id) {
			item.LegacyFor = unitReviewRequired
			if !strings.HasSuffix(item.Name, "（舊版單位待確認）") && len(item.Name)+len("（舊版單位待確認）") <= 300 {
				item.Name += "（舊版單位待確認）"
			}
			s.Known[id] = item
			if previous, ok := s.Overrides[item.Key]; ok && previous.Enabled {
				previous.Enabled = false
				s.Overrides[item.Key] = previous
			}
		}
	}
	for _, definition := range c.Items {
		item, observed := s.Known[definition.ID]
		if !observed || definition.Kind != "measurement" || c.definitionAcceptsUnit(definition, item.Unit) {
			continue
		}
		if previous, exists := s.Overrides[definition.Key]; exists {
			// Include the retained values in the identity so an earlier, distinct
			// quarantined edit is never overwritten during repeated migrations.
			encoded, _ := json.Marshal(previous)
			digest := sha256.Sum256(append([]byte(definition.Key+"\x00"+item.Unit+"\x00"), encoded...))
			id := "unit_review_" + hex.EncodeToString(digest[:10])
			item.ID, item.Key, item.LegacyFor = id, id, unitReviewRequired
			item.Name = definition.Name + "（舊版單位待確認）"
			s.Known[id] = item
			previous.Enabled = false
			s.Overrides[id] = previous
			delete(s.Overrides, definition.Key)
		}
		delete(s.Known, definition.ID)
	}
}

func (c *Catalog) definitionAcceptsUnit(definition Definition, unit string) bool {
	if unitKey(unit) == unitKey(definition.Match.Unit) {
		return true
	}
	for _, alias := range definition.Aliases {
		if unitKey(unit) == unitKey(alias.Unit) {
			return true
		}
	}
	return definition.ID == "egfr" && ruleMatchesUnit(c.Rules[definition.Rule], unit)
}
