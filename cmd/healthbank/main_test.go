// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bestkakkoii/healthbank/internal/config"
	"github.com/bestkakkoii/healthbank/internal/lab"
	"github.com/bestkakkoii/healthbank/internal/store"
)

func TestMaintenancePreservesIdentityAndPortableSettings(t *testing.T) {
	source := t.TempDir()
	t.Setenv("HEALTHBANK_DATA_DIR", source)
	t.Setenv("HEALTHBANK_DB_DRIVER", "sqlite")
	t.Setenv("HEALTHBANK_MYSQL_DSN", "")
	secret, err := config.Secret(source)
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(context.Background(), store.Options{SQLitePath: filepath.Join(source, "settings.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.BindIdentityKey(context.Background(), config.KeyCheck(secret), false); err != nil {
		t.Fatal(err)
	}
	settings := lab.DefaultSettings()
	settings.Theme = "dark"
	doc, err := settings.Document()
	if err != nil {
		t.Fatal(err)
	}
	subject := config.Subject(secret, "SYNTHETIC_CLI_SUBJECT")
	if _, err = db.Put(context.Background(), subject, 0, doc); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	export := filepath.Join(t.TempDir(), "settings.json")
	if err := run([]string{"settings", "export", export}); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	t.Setenv("HEALTHBANK_DATA_DIR", target)
	if err := run([]string{"settings", "import", export}); err == nil || !strings.Contains(err.Error(), "original") {
		t.Fatalf("missing key accepted: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "identity.key"), secret, 0600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"settings", "import", export}); err != nil {
		t.Fatal(err)
	}
	restored, err := store.Open(context.Background(), store.Options{SQLitePath: filepath.Join(target, "settings.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := restored.Get(context.Background(), subject)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(saved.Settings), `"theme":"dark"`) {
		t.Fatal("configuration not restored under original subject")
	}
	if err := restored.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(target, "identity.key")); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"settings", "export", filepath.Join(t.TempDir(), "other.json")}); err == nil || !strings.Contains(err.Error(), "original identity.key") {
		t.Fatalf("lost key silently replaced: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "identity.key")); !os.IsNotExist(err) {
		t.Fatal("lost key was recreated")
	}
}

func TestSettingsTransferStoresOnlyCanonicalValidatedConfiguration(t *testing.T) {
	secret, err := config.Secret(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(context.Background(), store.Options{SQLitePath: filepath.Join(t.TempDir(), "settings.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	const subject = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	const hidden = "SYNTHETIC_VALUE_MUST_NOT_PERSIST"
	snapshot := store.Snapshot{SchemaVersion: store.SchemaVersion, IdentityKeyCheck: config.KeyCheck(secret), IdentityKeyOrigin: store.IdentityKeyNew, ExportedAt: time.Now().UTC().Format(time.RFC3339), Entries: []store.Entry{{SubjectKey: subject, Document: store.Document{Revision: 7, UpdatedAt: time.Now().UTC().Format(time.RFC3339), Settings: json.RawMessage(`{"theme":"` + hidden + `","theme":"system","font_scale":1,"favorites":[],"overrides":{}}`)}}}}
	path := filepath.Join(t.TempDir(), "synthetic-settings.json")
	write := func() {
		t.Helper()
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.EncodeSnapshot(file, snapshot); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	write()
	if err := transfer(db, secret, "import", path); err == nil {
		t.Fatal("ambiguous raw configuration was persisted by CLI import")
	}
	if prior, err := db.Get(context.Background(), subject); err != nil || prior.Revision != 0 {
		t.Fatal("failed CLI import wrote a configuration profile")
	}
	// Even valid differently formatted input is stored through Settings.Document,
	// so persisted content is exactly the schema-checked typed representation.
	snapshot.Entries[0].Settings = json.RawMessage(`{ "theme": "system", "font_scale": 1, "favorites": [], "overrides": {} }`)
	write()
	if err := transfer(db, secret, "import", path); err != nil {
		t.Fatal(err)
	}
	stored, err := db.Get(context.Background(), subject)
	if err != nil || stored.Revision != 7 {
		t.Fatal("valid transfer failed to preserve the separate storage revision")
	}
	canonical, err := lab.DefaultSettings().Document()
	if err != nil || string(stored.Settings) != string(canonical) || strings.Contains(string(stored.Settings), hidden) {
		t.Fatal("stored JSON differs from the canonical validated schema")
	}
}
