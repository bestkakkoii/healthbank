// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

package store

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
)

const (
	// IdentityKeyNew identifies a binding established before any profile existed.
	IdentityKeyNew = "new_instance"
	// IdentityKeyLegacyConfirmed records an explicit operator assertion about an old key.
	IdentityKeyLegacyConfirmed = "operator_confirmed_legacy"
	// IdentityKeyLegacyUnverified preserves the absence of old pairing evidence.
	IdentityKeyLegacyUnverified = "legacy_unverified"
)

func validKeyOrigin(origin string) bool {
	return origin == IdentityKeyNew || origin == IdentityKeyLegacyConfirmed || origin == IdentityKeyLegacyUnverified
}

func (s *Store) identityKeyBinding(ctx context.Context) (string, string, error) {
	var check, origin string
	err := s.db.QueryRowContext(ctx, `SELECT key_check, key_origin FROM instance_settings WHERE instance_id = 1`).Scan(&check, &origin)
	if errors.Is(err, sql.ErrNoRows) {
		return "", IdentityKeyLegacyUnverified, nil
	}
	if err != nil {
		return "", "", fmt.Errorf("read instance-key binding: %w", err)
	}
	if !subjectPattern.MatchString(check) || !validKeyOrigin(origin) {
		return "", "", fmt.Errorf("%w: malformed instance-key binding", ErrInvalid)
	}
	return check, origin, nil
}

// IdentityKeyOrigin exposes only the bounded provenance label, never the key check.
func (s *Store) IdentityKeyOrigin(ctx context.Context) (string, error) {
	_, origin, err := s.identityKeyBinding(ctx)
	return origin, err
}

// VerifyIdentityKey checks existing metadata without adopting an unbound key.
func (s *Store) VerifyIdentityKey(ctx context.Context, check string) error {
	if !subjectPattern.MatchString(check) {
		return fmt.Errorf("%w: malformed instance-key check", ErrInvalid)
	}
	stored, _, err := s.identityKeyBinding(ctx)
	if err != nil {
		return err
	}
	if stored == "" {
		return ErrLegacyIdentityKey
	}
	if subtle.ConstantTimeCompare([]byte(stored), []byte(check)) != 1 {
		return ErrIdentityKeyMismatch
	}
	return nil
}

// BindIdentityKey initializes the non-secret key check for a new empty instance.
// Existing unbound profiles require an explicit operator assertion of the
// original key; this cannot establish a cryptographic fact absent from old data.
// A binding that already exists can never be replaced by the legacy option.
func (s *Store) BindIdentityKey(ctx context.Context, check string, acceptLegacy bool) error {
	if !subjectPattern.MatchString(check) {
		return fmt.Errorf("%w: malformed instance-key check", ErrInvalid)
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin instance-key binding: %w", err)
	}
	defer tx.Rollback()
	var stored, origin string
	err = tx.QueryRowContext(ctx, `SELECT key_check, key_origin FROM instance_settings WHERE instance_id = 1`).Scan(&stored, &origin)
	if err == nil {
		if !subjectPattern.MatchString(stored) || !validKeyOrigin(origin) {
			return fmt.Errorf("%w: malformed instance-key binding", ErrInvalid)
		}
		if subtle.ConstantTimeCompare([]byte(stored), []byte(check)) != 1 {
			return ErrIdentityKeyMismatch
		}
		if acceptLegacy && origin == IdentityKeyLegacyUnverified {
			if _, err := tx.ExecContext(ctx, `UPDATE instance_settings SET key_origin = ? WHERE instance_id = 1`, IdentityKeyLegacyConfirmed); err != nil {
				return fmt.Errorf("confirm legacy key provenance: %w", err)
			}
			return tx.Commit()
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read instance-key binding: %w", err)
	}
	var count int64
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM profile_settings`).Scan(&count); err != nil {
		return fmt.Errorf("check legacy profile count: %w", err)
	}
	if count != 0 && !acceptLegacy {
		return ErrLegacyIdentityKey
	}
	origin = IdentityKeyNew
	if count != 0 {
		origin = IdentityKeyLegacyConfirmed
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO instance_settings (instance_id, key_check, key_origin) VALUES (1, ?, ?)`, check, origin); err != nil {
		return fmt.Errorf("write instance-key binding: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit instance-key binding: %w", err)
	}
	return nil
}

// ValidateSnapshotKey enforces verified new backups and explicit legacy import.
// The check value is an instance binding, never an identity or authentication token.
func ValidateSnapshotKey(snapshot Snapshot, check string, acceptLegacy bool) error {
	if snapshot.SchemaVersion == LegacySchemaVersion && snapshot.IdentityKeyCheck == "" && snapshot.IdentityKeyOrigin == "" {
		if acceptLegacy {
			return nil
		}
		return ErrLegacyIdentityKey
	}
	if snapshot.SchemaVersion != SchemaVersion {
		return fmt.Errorf("%w: snapshot is %d", ErrUnsupportedSchema, snapshot.SchemaVersion)
	}
	if !subjectPattern.MatchString(snapshot.IdentityKeyCheck) || !subjectPattern.MatchString(check) || !validKeyOrigin(snapshot.IdentityKeyOrigin) {
		return fmt.Errorf("%w: missing or malformed snapshot key binding", ErrInvalid)
	}
	if subtle.ConstantTimeCompare([]byte(snapshot.IdentityKeyCheck), []byte(check)) != 1 {
		return ErrIdentityKeyMismatch
	}
	return nil
}
