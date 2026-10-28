// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package nhi

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

// Source availability periods, query dates and display-state flags describe the
// response. They must not become a measurement date or a locally derived result.
func providerMetadataSection(root, title, source string, fields []fieldSpec) providerSection {
	section := providerSectionSpec(root, title, source, "", fields)
	section.optional, section.metadata = true, true
	return section
}

func appendProviderMetadata(report *SupplementReport, object wireRow, path, category string, section providerSection, inherited providerReference) error {
	rows, present, err := providerSectionRows(object, section)
	if err != nil {
		return err
	}
	if section.root != "" {
		if err := sourceSectionDiagnostic(report, category, section.root, object[strings.ToLower(section.root)], present, section.optional, len(rows)); err != nil {
			return err
		}
	}
	for index, row := range rows {
		known := false
		projected := wireRow{}
		for _, field := range section.fields {
			code := strings.ToLower(field.key)
			raw, exists := row[code]
			if !exists {
				continue
			}
			known = true
			// The source applies these three summaries only when they are
			// literal booleans. Do not reproduce its client-side fallback.
			if code == "hassavedata" || code == "haswarningdata" || code == "hasinvaliddata" {
				switch string(bytes.TrimSpace(raw)) {
				case "true", "false":
					raw = json.RawMessage(strconv.Quote(string(bytes.TrimSpace(raw))))
				default:
					continue
				}
			}
			// Device availability is a source truthiness flag. Preserve the
			// actual scalar, including a boolean, without assigning a platform.
			if code == "act_os1" || code == "act_os2" || code == "act_os3" {
				if bytes.Equal(bytes.TrimSpace(raw), []byte("true")) || bytes.Equal(bytes.TrimSpace(raw), []byte("false")) {
					raw = json.RawMessage(strconv.Quote(string(bytes.TrimSpace(raw))))
				}
			}
			projected[code] = raw
		}
		if !known {
			if section.root == "" {
				continue // optional metadata was not supplied on this response
			}
			return changedError()
		}
		fields, err := appendKnownFields(projected, section.fields)
		if err != nil {
			return err
		}
		if len(projected) == 0 {
			continue // nonboolean screening hints are not authoritative flags
		}
		report.Entries = append(report.Entries, SupplementEntry{
			Key:  medicalKey(category, path+"\x00"+inherited.rowID+"\x00"+inherited.originKey+"\x00"+section.title, section.root, index),
			Kind: "source_metadata", ContractID: sourceContractID(category, section.root+"_metadata"), Origin: providerOrigin(category, section.root, inherited.kind),
			Title: section.title, Source: section.source, Fields: fields,
		})
	}
	return nil
}
