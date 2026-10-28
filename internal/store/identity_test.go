// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestIdentityKeyBindingRejectsMismatch(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	check := strings.Repeat("a", 64)
	if err := s.BindIdentityKey(ctx, check, false); err != nil {
		t.Fatal(err)
	}
	if err := s.VerifyIdentityKey(ctx, check); err != nil {
		t.Fatal(err)
	}
	for _, acceptLegacy := range []bool{false, true} {
		if err := s.BindIdentityKey(ctx, strings.Repeat("b", 64), acceptLegacy); !errors.Is(err, ErrIdentityKeyMismatch) {
			t.Fatal("existing binding was replaced by a different key")
		}
	}
	if err := s.VerifyIdentityKey(ctx, check); err != nil {
		t.Fatal("rejected mismatch changed the existing binding")
	}
}

func TestLegacyIdentityKeyRequiresExplicitAdoption(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	if _, err := s.Put(ctx, syntheticSubject, 0, json.RawMessage(`{"theme":"dark"}`)); err != nil {
		t.Fatal(err)
	}
	check := strings.Repeat("a", 64)
	if err := s.BindIdentityKey(ctx, check, false); !errors.Is(err, ErrLegacyIdentityKey) {
		t.Fatal("unverified legacy key was silently adopted")
	}
	before, err := s.Export(ctx)
	if err != nil || before.SchemaVersion != LegacySchemaVersion || before.IdentityKeyCheck != "" {
		t.Fatal("legacy export falsely claimed a verified key")
	}
	if err := s.BindIdentityKey(ctx, check, true); err != nil {
		t.Fatal(err)
	}
	after, err := s.Export(ctx)
	if err != nil || after.SchemaVersion != SchemaVersion || after.IdentityKeyCheck != check || len(after.Entries) != 1 || after.Entries[0].Revision != before.Entries[0].Revision {
		t.Fatal("explicit adoption changed profiles or omitted the new backup binding")
	}
}

func TestSchemaOneUpgradePreservesUnverifiedProfiles(t *testing.T) {
	s, path := openTestStore(t)
	ctx := context.Background()
	original, err := s.Put(ctx, syntheticSubject, 0, json.RawMessage(`{"theme":"dark"}`))
	if err != nil {
		t.Fatal(err)
	}
	// Reconstruct the previous released schema, retaining its actual profile row.
	if _, err := s.db.ExecContext(ctx, `DROP TABLE instance_settings`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = ?`, SchemaVersion); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := Open(ctx, Options{SQLitePath: path})
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	after, err := upgraded.Get(ctx, syntheticSubject)
	if err != nil || string(after.Settings) != string(original.Settings) || after.Revision != original.Revision || after.UpdatedAt != original.UpdatedAt {
		t.Fatal("schema upgrade changed an existing configuration")
	}
	if err := upgraded.BindIdentityKey(ctx, strings.Repeat("a", 64), false); !errors.Is(err, ErrLegacyIdentityKey) {
		t.Fatal("schema upgrade fabricated proof of the old identity key")
	}
	if err := upgraded.migrate(ctx); err != nil {
		t.Fatal("completed migration was not idempotent")
	}
}

func TestSnapshotKeyCannotBeBypassedByLegacyImport(t *testing.T) {
	check := strings.Repeat("a", 64)
	for _, tc := range []struct {
		name     string
		snapshot Snapshot
		legacy   bool
		want     error
	}{
		{"legacy_rejected", Snapshot{SchemaVersion: LegacySchemaVersion}, false, ErrLegacyIdentityKey},
		{"legacy_explicit", Snapshot{SchemaVersion: LegacySchemaVersion}, true, nil},
		{"verified", Snapshot{SchemaVersion: SchemaVersion, IdentityKeyCheck: check, IdentityKeyOrigin: IdentityKeyNew}, false, nil},
		{"legacy_origin_preserved", Snapshot{SchemaVersion: SchemaVersion, IdentityKeyCheck: check, IdentityKeyOrigin: IdentityKeyLegacyUnverified}, false, nil},
		{"mismatch", Snapshot{SchemaVersion: SchemaVersion, IdentityKeyCheck: strings.Repeat("b", 64), IdentityKeyOrigin: IdentityKeyNew}, false, ErrIdentityKeyMismatch},
		{"legacy_cannot_bypass", Snapshot{SchemaVersion: SchemaVersion, IdentityKeyCheck: strings.Repeat("b", 64), IdentityKeyOrigin: IdentityKeyNew}, true, ErrIdentityKeyMismatch},
		{"missing_check", Snapshot{SchemaVersion: SchemaVersion}, true, ErrInvalid},
		{"future", Snapshot{SchemaVersion: SchemaVersion + 1}, true, ErrUnsupportedSchema},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateSnapshotKey(tc.snapshot, check, tc.legacy); !errors.Is(err, tc.want) {
				t.Fatalf("key validation returned %v, expected %v", err, tc.want)
			}
		})
	}
}
