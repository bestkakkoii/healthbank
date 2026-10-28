// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bestkakkoii/healthbank/internal/config"
	"github.com/bestkakkoii/healthbank/internal/lab"
	"github.com/bestkakkoii/healthbank/internal/store"
)

func TestMaintenanceRejectsWrongBackupKeyBeforeWriting(t *testing.T) {
	ctx := context.Background()
	source := t.TempDir()
	t.Setenv("HEALTHBANK_DATA_DIR", source)
	t.Setenv("HEALTHBANK_DB_DRIVER", "sqlite")
	t.Setenv("HEALTHBANK_MYSQL_DSN", "")
	secret, err := config.Secret(source)
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(ctx, store.Options{SQLitePath: filepath.Join(source, "settings.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.BindIdentityKey(ctx, config.KeyCheck(secret), false); err != nil {
		t.Fatal(err)
	}
	settings := lab.DefaultSettings()
	settings.Theme = "dark"
	document, err := settings.Document()
	if err != nil {
		t.Fatal(err)
	}
	subject := config.Subject(secret, "SYNTHETIC_BACKUP_KEY_REVIEW")
	if _, err := db.Put(ctx, subject, 0, document); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := run([]string{"settings", "export", path}); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	if _, err := config.Secret(target); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEALTHBANK_DATA_DIR", target)
	for _, action := range []string{"import", "import-legacy"} {
		if err := run([]string{"settings", action, path}); !errors.Is(err, store.ErrIdentityKeyMismatch) {
			t.Fatalf("different valid-length key was not rejected: %v", err)
		}
	}
	destination, err := store.Open(ctx, store.Options{SQLitePath: filepath.Join(target, "settings.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	before, err := destination.Export(ctx)
	if err != nil || len(before.Entries) != 0 || before.IdentityKeyCheck != "" {
		t.Fatal("mismatched restore partially wrote settings or adopted the wrong key")
	}
	if err := destination.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "identity.key"), secret, 0600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"settings", "import", path}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"settings", "verify-key"}); err != nil {
		t.Fatal(err)
	}
	destination, err = store.Open(ctx, store.Options{SQLitePath: filepath.Join(target, "settings.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer destination.Close()
	restored, err := destination.Get(ctx, subject)
	if err != nil || restored.Revision != 1 || string(restored.Settings) != string(document) {
		t.Fatal("correct key restore changed the settings or subject mapping")
	}
}

func TestMaintenanceLegacyKeyAdoptionAndImportAreExplicit(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	t.Setenv("HEALTHBANK_DATA_DIR", dir)
	t.Setenv("HEALTHBANK_DB_DRIVER", "sqlite")
	t.Setenv("HEALTHBANK_MYSQL_DSN", "")
	secret, err := config.Secret(dir)
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(ctx, store.Options{SQLitePath: filepath.Join(dir, "settings.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	document, err := lab.DefaultSettings().Document()
	if err != nil {
		t.Fatal(err)
	}
	subject := config.Subject(secret, "SYNTHETIC_LEGACY_KEY_REVIEW")
	if _, err := db.Put(ctx, subject, 0, document); err != nil {
		t.Fatal(err)
	}
	legacy, err := db.Export(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepareRuntimeIdentity(ctx, db, secret); err != nil {
		t.Fatal("normal upgrade stopped serving an unbound legacy database")
	}
	if err := db.VerifyIdentityKey(ctx, config.KeyCheck(secret)); !errors.Is(err, store.ErrLegacyIdentityKey) {
		t.Fatal("normal upgrade silently adopted an unverified legacy key")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "unverified.json")
	if err := run([]string{"settings", "export", path}); err != nil {
		t.Fatal("legacy database could not export a qualified backup")
	}
	exported, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var qualified store.Snapshot
	if err := json.Unmarshal(exported, &qualified); err != nil || qualified.IdentityKeyOrigin != store.IdentityKeyLegacyUnverified || qualified.IdentityKeyCheck != config.KeyCheck(secret) || qualified.SchemaVersion != store.SchemaVersion {
		t.Fatal("legacy export omitted key pairing or falsely promoted original provenance")
	}
	if err := run([]string{"settings", "verify-key"}); !errors.Is(err, store.ErrLegacyIdentityKey) {
		t.Fatal("legacy database passed key verification")
	}
	if err := run([]string{"settings", "bind-legacy-key"}); err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(t.TempDir(), "verified.json")
	if err := run([]string{"settings", "export", path}); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "identity.key"), secret, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HEALTHBANK_DATA_DIR", target)
	legacyPath := filepath.Join(t.TempDir(), "legacy.json")
	legacy.ExportedAt = time.Now().UTC().Format(time.RFC3339)
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"settings", "import", legacyPath}); !errors.Is(err, store.ErrLegacyIdentityKey) {
		t.Fatal("ordinary import accepted an unverified legacy backup")
	}
	if err := run([]string{"settings", "import-legacy", legacyPath}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"settings", "verify-key"}); err != nil {
		t.Fatal(err)
	}
	restored, err := store.Open(ctx, store.Options{SQLitePath: filepath.Join(target, "settings.sqlite")})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	if origin, err := restored.IdentityKeyOrigin(ctx); err != nil || origin != store.IdentityKeyLegacyUnverified {
		t.Fatal("legacy import erased its unverified original-key provenance")
	}
}
