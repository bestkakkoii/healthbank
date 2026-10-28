// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestSnapshotExportHonorsItsRestoreCapacity(t *testing.T) {
	snapshot := Snapshot{SchemaVersion: LegacySchemaVersion, ExportedAt: "2026-10-28T00:00:00Z", Entries: []Entry{{SubjectKey: syntheticSubject, Document: Document{Revision: 7, UpdatedAt: "2026-10-28T00:00:00Z", Settings: json.RawMessage(`{"theme":"system"}`)}}}}
	var exported bytes.Buffer
	if err := EncodeSnapshot(&exported, snapshot); err != nil {
		t.Fatal(err)
	}
	restored, err := DecodeSnapshot(bytes.NewReader(exported.Bytes()))
	if err != nil || len(restored.Entries) != 1 || restored.Entries[0].Revision != 7 {
		t.Fatal("successful configuration export cannot be restored")
	}
	// Exercise the same encoder's capacity edge with a small deterministic limit.
	// A larger profile collection must fail instead of reporting an unusable backup.
	var exact bytes.Buffer
	if err := encodeSnapshot(&exact, snapshot, exported.Len()); err != nil || !bytes.Equal(exact.Bytes(), exported.Bytes()) {
		t.Fatal("snapshot exactly at its output bound was rejected or changed")
	}
	var tooSmall bytes.Buffer
	if err := encodeSnapshot(&tooSmall, snapshot, exported.Len()-1); !errors.Is(err, ErrInvalid) {
		t.Fatal("oversized snapshot export reported success")
	}
}

func TestSnapshotImportRejectsNonportableRevisionAndTimestamp(t *testing.T) {
	for _, kind := range []string{"unsafe_revision", "oversized_timestamp"} {
		t.Run(kind, func(t *testing.T) {
			db, _ := openTestStore(t)
			entry := Entry{SubjectKey: syntheticSubject, Document: Document{Revision: 1, UpdatedAt: "2026-10-28T00:00:00Z", Settings: json.RawMessage(`{}`)}}
			if kind == "unsafe_revision" {
				entry.Revision = MaxRevision + 1
			} else {
				entry.UpdatedAt = "2026-10-28T00:00:00." + strings.Repeat("1", 40) + "Z"
			}
			if err := db.Import(context.Background(), Snapshot{SchemaVersion: LegacySchemaVersion, Entries: []Entry{entry}}); !errors.Is(err, ErrInvalid) {
				t.Fatal("import accepted metadata that cannot cross the supported database/browser boundary")
			}
			if saved, err := db.Get(context.Background(), syntheticSubject); err != nil || saved.Revision != 0 {
				t.Fatal("nonportable metadata was partially imported")
			}
		})
	}
	db, _ := openTestStore(t)
	entry := Entry{SubjectKey: syntheticSubject, Document: Document{Revision: MaxRevision - 1, UpdatedAt: "2026-10-28T00:00:00.123456789+08:00", Settings: json.RawMessage(`{}`)}}
	if err := db.Import(context.Background(), Snapshot{SchemaVersion: LegacySchemaVersion, Entries: []Entry{entry}}); err != nil {
		t.Fatal("valid maximum-width timestamp and exact revision were rejected")
	}
	updated, err := db.Put(context.Background(), syntheticSubject, entry.Revision, json.RawMessage(`{}`))
	if err != nil || updated.Revision != MaxRevision {
		t.Fatal("last exactly representable revision was not preserved")
	}
	if _, err := db.Put(context.Background(), syntheticSubject, updated.Revision, json.RawMessage(`{}`)); !errors.Is(err, ErrInvalid) {
		t.Fatal("revision increment exceeded JavaScript exact-integer capacity")
	}
	if err := db.Delete(context.Background(), syntheticSubject, updated.Revision); err != nil {
		t.Fatal("maximum valid revision could not be deleted")
	}
}
