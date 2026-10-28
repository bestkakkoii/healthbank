// Copyright 2026 bestkakkoii llc.
// SPDX-License-Identifier: MIT

package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

const syntheticSubject = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
const otherSubject = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

func openTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data", "settings.sqlite")
	s, err := Open(context.Background(), Options{SQLitePath: path})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, path
}

func TestReadDoesNotCreateProfile(t *testing.T) {
	s, _ := openTestStore(t)
	doc, err := s.Get(context.Background(), syntheticSubject)
	if err != nil || doc.Revision != 0 || string(doc.Settings) != `{}` {
		t.Fatalf("unseen profile: %+v, %v", doc, err)
	}
	snapshot, err := s.Export(context.Background())
	if err != nil || len(snapshot.Entries) != 0 {
		t.Fatalf("read created persistent identity: %+v, %v", snapshot, err)
	}
}

func TestOptimisticConcurrencyAndIsolation(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	first, err := s.Put(ctx, syntheticSubject, 0, json.RawMessage(`{"theme":"light"}`))
	if err != nil || first.Revision != 1 {
		t.Fatalf("first write: %+v, %v", first, err)
	}
	if _, err := s.Put(ctx, syntheticSubject, 0, json.RawMessage(`{}`)); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate creation must conflict: %v", err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, theme := range []string{"dark", "system"} {
		wg.Add(1)
		go func(theme string) {
			defer wg.Done()
			_, err := s.Put(ctx, syntheticSubject, first.Revision, json.RawMessage(`{"theme":"`+theme+`"}`))
			results <- err
		}(theme)
	}
	wg.Wait()
	close(results)
	var success, conflicts int
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, ErrConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent-write error: %v", err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("writes success=%d conflicts=%d", success, conflicts)
	}
	updated, err := s.Get(ctx, syntheticSubject)
	if err != nil || updated.Revision != 2 {
		t.Fatalf("revision: %+v, %v", updated, err)
	}
	other, err := s.Get(ctx, otherSubject)
	if err != nil || other.Revision != 0 {
		t.Fatalf("profile isolation: %+v, %v", other, err)
	}
	if err := s.Delete(ctx, syntheticSubject, 1); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale delete must conflict: %v", err)
	}
	if err := s.Delete(ctx, syntheticSubject, 2); err != nil {
		t.Fatal(err)
	}
}

func TestPersistenceAndSQLiteConfiguration(t *testing.T) {
	s, path := openTestStore(t)
	ctx := context.Background()
	written, err := s.Put(ctx, syntheticSubject, 0, json.RawMessage(`{"theme":"dark"}`))
	if err != nil {
		t.Fatal(err)
	}
	var journal string
	var timeout int
	if err := s.db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journal); err != nil || journal != "wal" {
		t.Fatalf("journal=%q error=%v", journal, err)
	}
	if err := s.db.QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&timeout); err != nil || timeout != 5000 {
		t.Fatalf("timeout=%d error=%v", timeout, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("new database permissions: %v, %v", info, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, Options{SQLitePath: path})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	read, err := reopened.Get(ctx, syntheticSubject)
	if err != nil || read.Revision != written.Revision || read.UpdatedAt != written.UpdatedAt || !bytes.Equal(read.Settings, written.Settings) {
		t.Fatalf("reopen changed settings: %+v, %v", read, err)
	}
}

func TestSnapshotRoundTripAndEmptyDestinationGuard(t *testing.T) {
	source, _ := openTestStore(t)
	target, _ := openTestStore(t)
	ctx := context.Background()
	if _, err := source.Put(ctx, syntheticSubject, 0, json.RawMessage(`{"theme":"system"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Put(ctx, syntheticSubject, 1, json.RawMessage(`{"theme":"dark","fontSize":22}`)); err != nil {
		t.Fatal(err)
	}
	snapshot, err := source.Export(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var encoded bytes.Buffer
	if err := EncodeSnapshot(&encoded, snapshot); err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeSnapshot(&encoded)
	if err != nil {
		t.Fatal(err)
	}
	if err := target.Import(ctx, decoded); err != nil {
		t.Fatal(err)
	}
	read, err := target.Get(ctx, syntheticSubject)
	if err != nil || read.Revision != 2 || !bytes.Equal(read.Settings, snapshot.Entries[0].Settings) {
		t.Fatalf("migration did not preserve revision/settings: %+v, %v", read, err)
	}
	if err := target.Import(ctx, decoded); !errors.Is(err, ErrDestinationNotEmpty) {
		t.Fatalf("nonempty migration must not overwrite: %v", err)
	}
}

func TestImportValidationDoesNotPartiallyWrite(t *testing.T) {
	s, _ := openTestStore(t)
	snapshot := Snapshot{
		SchemaVersion: LegacySchemaVersion,
		Entries: []Entry{
			{SubjectKey: syntheticSubject, Document: Document{Revision: 1, Settings: json.RawMessage(`{}`), UpdatedAt: "2026-01-01T00:00:00Z"}},
			{SubjectKey: otherSubject, Document: Document{Revision: 0, Settings: json.RawMessage(`{}`), UpdatedAt: "2026-01-01T00:00:00Z"}},
		},
	}
	if err := s.Import(context.Background(), snapshot); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid entry accepted: %v", err)
	}
	exported, err := s.Export(context.Background())
	if err != nil || len(exported.Entries) != 0 {
		t.Fatalf("invalid snapshot partially applied: %+v, %v", exported, err)
	}
	snapshot.Entries[1] = snapshot.Entries[0]
	if err := s.Import(context.Background(), snapshot); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate snapshot accepted: %v", err)
	}
}

func TestInputValidationAndNewerSchema(t *testing.T) {
	s, _ := openTestStore(t)
	ctx := context.Background()
	for _, raw := range []string{"", "null", `[]`, `{`, strings.Repeat(" ", MaxSettingsBytes) + `{}`} {
		if _, err := s.Put(ctx, syntheticSubject, 0, json.RawMessage(raw)); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid settings accepted (%d bytes): %v", len(raw), err)
		}
	}
	if _, err := s.Get(ctx, "not-a-pseudonym"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("raw identity accepted: %v", err)
	}
	for _, raw := range []string{
		`{"schema_version":1,"entries":[],"unknown":true}`,
		`{"schema_version":1,"entries":[]} {}`,
		`{"schema_version":1,"schema_version":1,"entries":[]}`,
		`{"schema_version":1,"SCHEMA_VERSION":1,"entries":[]}`,
		`{"schema_version":1,"entries":[],"exported_at":"\ud800"}`,
	} {
		if _, err := DecodeSnapshot(strings.NewReader(raw)); err == nil {
			t.Fatal("invalid snapshot envelope accepted")
		}
	}
	for _, raw := range []string{
		`{"theme":"SYNTHETIC_HIDDEN_VALUE","theme":"system"}`,
		`{"theme":"SYNTHETIC_HIDDEN_VALUE","THEME":"system"}`,
		`{"theme":"\ud800"}`,
	} {
		if _, err := s.Put(ctx, syntheticSubject, 0, json.RawMessage(raw)); !errors.Is(err, ErrInvalid) {
			t.Fatal("store accepted ambiguous or lossy configuration content")
		}
	}
	if err := s.Import(ctx, Snapshot{SchemaVersion: SchemaVersion + 1}); !errors.Is(err, ErrUnsupportedSchema) {
		t.Fatalf("newer snapshot accepted: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)`, SchemaVersion+1, "2026-01-01T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if err := s.migrate(ctx); !errors.Is(err, ErrUnsupportedSchema) {
		t.Fatalf("newer database accepted: %v", err)
	}
}

// TestMySQLIntegration is opt-in. It uses a disposable empty database supplied
// by the operator and never drops or truncates a table.
func TestMySQLIntegration(t *testing.T) {
	dsn := os.Getenv("HEALTHBANK_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("HEALTHBANK_TEST_MYSQL_DSN not set; live MySQL has not been tested")
	}
	s, err := Open(context.Background(), Options{Driver: "mysql", DSN: dsn})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	ctx := context.Background()
	before, err := s.Export(ctx)
	if err != nil || len(before.Entries) != 0 {
		t.Fatal("MySQL integration test requires an empty disposable settings database")
	}
	source, _ := openTestStore(t)
	keyCheck := strings.Repeat("c", 64)
	for _, instance := range []*Store{source, s} {
		if err := instance.BindIdentityKey(ctx, keyCheck, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.VerifyIdentityKey(ctx, strings.Repeat("d", 64)); !errors.Is(err, ErrIdentityKeyMismatch) {
		t.Fatal("MySQL accepted a mismatched identity key")
	}
	for _, raw := range []string{`{"theme":"system"}`, `{"theme":"light","font_scale":2,"favorites":[],"overrides":{}}`} {
		current, err := source.Get(ctx, syntheticSubject)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := source.Put(ctx, syntheticSubject, current.Revision, json.RawMessage(raw)); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := source.Export(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Import(ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	currentRevision := int64(2)
	t.Cleanup(func() { _ = s.Delete(context.Background(), syntheticSubject, currentRevision) })
	doc, err := s.Get(ctx, syntheticSubject)
	if err != nil || doc.Revision != 2 || doc.UpdatedAt != snapshot.Entries[0].UpdatedAt || !bytes.Equal(doc.Settings, snapshot.Entries[0].Settings) {
		t.Fatalf("MySQL migrated document: %+v, %v", doc, err)
	}
	if err := s.Import(ctx, snapshot); !errors.Is(err, ErrDestinationNotEmpty) {
		t.Fatalf("MySQL nonempty import destination must not be overwritten: %v", err)
	}
	if _, err := s.Put(ctx, syntheticSubject, 0, json.RawMessage(`{}`)); !errors.Is(err, ErrConflict) {
		t.Fatalf("MySQL duplicate creation must conflict: %v", err)
	}
	other, err := s.Get(ctx, otherSubject)
	if err != nil || other.Revision != 0 || string(other.Settings) != `{}` {
		t.Fatalf("MySQL subject isolation: %+v, %v", other, err)
	}
	updated, err := s.Put(ctx, syntheticSubject, 2, json.RawMessage(`{"theme":"dark"}`))
	if err != nil {
		t.Fatal(err)
	}
	currentRevision = updated.Revision
	if _, err := s.Put(ctx, syntheticSubject, 2, json.RawMessage(`{}`)); !errors.Is(err, ErrConflict) {
		t.Fatalf("MySQL stale write: %v", err)
	}
	if err := s.Delete(ctx, syntheticSubject, 2); !errors.Is(err, ErrConflict) {
		t.Fatalf("MySQL stale delete must conflict: %v", err)
	}
	results := make(chan error, 2)
	for _, theme := range []string{"light", "system"} {
		go func(theme string) {
			_, err := s.Put(ctx, syntheticSubject, updated.Revision, json.RawMessage(`{"theme":"`+theme+`"}`))
			results <- err
		}(theme)
	}
	success, conflicts := 0, 0
	for range 2 {
		switch err := <-results; {
		case err == nil:
			success++
		case errors.Is(err, ErrConflict):
			conflicts++
		default:
			t.Fatalf("MySQL concurrent update: %v", err)
		}
	}
	currentRevision++
	if success != 1 || conflicts != 1 {
		t.Fatalf("MySQL concurrent writes success=%d conflicts=%d", success, conflicts)
	}
	if err := s.migrate(ctx); err != nil {
		t.Fatalf("MySQL schema reapplication: %v", err)
	}
	migrated, err := s.Export(ctx)
	if err != nil || len(migrated.Entries) != 1 {
		t.Fatalf("MySQL export: %v", err)
	}
	var portable bytes.Buffer
	if err := EncodeSnapshot(&portable, migrated); err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeSnapshot(&portable)
	if err != nil {
		t.Fatal(err)
	}
	target, _ := openTestStore(t)
	if err := target.BindIdentityKey(ctx, keyCheck, false); err != nil {
		t.Fatal(err)
	}
	if err := target.Import(ctx, decoded); err != nil {
		t.Fatal(err)
	}
	restored, err := target.Export(ctx)
	if err != nil || !reflect.DeepEqual(restored.Entries, migrated.Entries) {
		t.Fatalf("MySQL-to-SQLite migration changed settings, revision, timestamp or subject: %v", err)
	}
}
