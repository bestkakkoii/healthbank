// Copyright 2026 bestkakkoi llc.
// SPDX-License-Identifier: MIT

package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func (s *Store) migrate(ctx context.Context) error {
	// MySQL DDL implicitly commits. Each schema step is deliberately idempotent;
	// recording its version only after successful DDL permits safe restart.
	meta := `CREATE TABLE IF NOT EXISTS schema_migrations (
		version BIGINT NOT NULL PRIMARY KEY,
		applied_at VARCHAR(35) NOT NULL
	)`
	if s.driver == "mysql" {
		meta += ` ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin`
	}
	if _, err := s.db.ExecContext(ctx, meta); err != nil {
		return fmt.Errorf("create settings migration table: %w", err)
	}
	var current sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&current); err != nil {
		return fmt.Errorf("read settings schema version: %w", err)
	}
	if current.Valid && current.Int64 > SchemaVersion {
		return fmt.Errorf("%w: database is %d, executable supports %d", ErrUnsupportedSchema, current.Int64, SchemaVersion)
	}
	if current.Valid && current.Int64 == SchemaVersion {
		return nil
	}
	settingsType := "TEXT"
	if s.driver == "mysql" {
		settingsType = "MEDIUMTEXT"
	}
	ddl := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS profile_settings (
		subject_key VARCHAR(64) NOT NULL PRIMARY KEY,
		settings_json %s NOT NULL,
		revision BIGINT NOT NULL,
		updated_at VARCHAR(35) NOT NULL
	)`, settingsType)
	if s.driver == "mysql" {
		ddl += ` ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin`
	}
	if _, err := s.db.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("apply settings schema version 1: %w", err)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`,
		SchemaVersion, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil && !isDuplicate(err) {
		return fmt.Errorf("record settings schema version: %w", err)
	}
	return nil
}
