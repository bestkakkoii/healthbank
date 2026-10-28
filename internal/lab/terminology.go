// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package lab

import "strings"

// Terminology is display metadata. It never participates in source matching,
// unit normalization, clinical rule selection or identity reconciliation.
type terminologyLocus struct {
	SourceID string `json:"source_id"`
	Locator  string `json:"locator"`
}

type sourceNameTerm struct {
	Names       []string           `json:"source_names"`
	Name        string             `json:"display_name_zh_tw"`
	EnglishName string             `json:"display_name_en"`
	Note        string             `json:"terminology_note"`
	Sources     []string           `json:"terminology_sources"`
	Loci        []terminologyLocus `json:"terminology_source_loci"`
	Basis       string             `json:"terminology_basis"`
}

// CatalogTerm exposes reviewed, public display vocabulary for raw-name and
// metadata rows that must not become configurable measurements merely to get
// a translation. Source loci remain pinned in the embedded catalog.
type CatalogTerm struct {
	Names       []string `json:"source_names"`
	Name        string   `json:"display_name"`
	EnglishName string   `json:"display_name_en"`
	Note        string   `json:"terminology_note,omitempty"`
	Sources     []string `json:"terminology_sources,omitempty"`
	Basis       string   `json:"terminology_basis"`
}

func (c *Catalog) describeSourceName(item *CatalogItem, rawName string) {
	name := strings.TrimSpace(rawName)
	for _, term := range c.NameGlossary {
		for _, candidate := range term.Names {
			// Case and punctuation are part of the reviewed source spelling. In
			// particular, a name alone cannot establish blood versus urine, a
			// quantitative assay, a specimen method or an institutional identity.
			if name != candidate {
				continue
			}
			item.Name = term.Name
			item.EnglishName = term.EnglishName
			item.TermNote = term.Note
			item.TermSources = append([]string(nil), term.Sources...)
			item.Aliases = appendUnique(item.Aliases, rawName, term.Name, term.EnglishName)
			return
		}
	}
}
