// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package lab

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
)

//go:embed evidence-registry.json
var evidenceRegistryJSON []byte

type CatalogProvenance struct {
	CatalogID        string `json:"catalog_id"`
	CatalogRevision  string `json:"catalog_revision"`
	EvidenceRevision string `json:"evidence_revision"`
}

func evidenceDigest(data []byte) string {
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:])
}

// Provenance identifies the exact reviewed embedded inputs. The source review
// dates inside those inputs are distinct from their content identity and do not
// imply that a rule is current merely because the application was rebuilt today.
func (c *Catalog) Provenance() CatalogProvenance {
	return CatalogProvenance{CatalogID: c.ID, CatalogRevision: evidenceDigest(catalogJSON), EvidenceRevision: evidenceDigest(evidenceRegistryJSON)}
}
