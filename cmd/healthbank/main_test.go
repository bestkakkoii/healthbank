// Copyright (c) 2026 bestkakkoi llc. SPDX-License-Identifier: MIT
package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
