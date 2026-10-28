// Copyright 2026 bestkakkoii llc.
// SPDX-License-Identifier: MIT

package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/bestkakkoii/healthbank/internal/strictjson"
)

const MaxSnapshotBytes = 64 << 20

// Entry binds a configuration to its pseudonymous subject key for migration.
type Entry struct {
	SubjectKey string `json:"subject_key"`
	Document
}

// Snapshot is engine-independent. The application HMAC secret is intentionally
// not included; preserve it separately when moving between database engines.
type Snapshot struct {
	SchemaVersion int     `json:"schema_version"`
	ExportedAt    string  `json:"exported_at"`
	Entries       []Entry `json:"entries"`
}

// Export returns an ordered, consistent read of all configuration documents.
// No login identity, password, card number, or medical result exists in this
// database, so none can be exported by this method.
func (s *Store) Export(ctx context.Context) (Snapshot, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT subject_key, revision, settings_json, updated_at FROM profile_settings ORDER BY subject_key`)
	if err != nil {
		return Snapshot{}, fmt.Errorf("export settings: %w", err)
	}
	defer rows.Close()
	snapshot := Snapshot{
		SchemaVersion: SchemaVersion,
		ExportedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		Entries:       []Entry{},
	}
	for rows.Next() {
		var entry Entry
		var raw string
		if err := rows.Scan(&entry.SubjectKey, &entry.Revision, &raw, &entry.UpdatedAt); err != nil {
			return Snapshot{}, fmt.Errorf("read settings export row: %w", err)
		}
		entry.Settings = json.RawMessage(raw)
		if err := validateEntry(&entry); err != nil {
			return Snapshot{}, fmt.Errorf("stored configuration cannot be exported: %w", err)
		}
		snapshot.Entries = append(snapshot.Entries, entry)
	}
	if err := rows.Err(); err != nil {
		return Snapshot{}, fmt.Errorf("finish settings export: %w", err)
	}
	return snapshot, nil
}

// Import restores a portable snapshot into an empty destination, preserving
// revisions. It validates the whole snapshot before beginning a transaction and
// never merges into or replaces existing configurations. Stop the running web
// service before an operator migration to avoid simultaneous writes.
func (s *Store) Import(ctx context.Context, snapshot Snapshot) error {
	if snapshot.SchemaVersion != SchemaVersion {
		return fmt.Errorf("%w: snapshot is %d, expected %d", ErrUnsupportedSchema, snapshot.SchemaVersion, SchemaVersion)
	}
	seen := make(map[string]struct{}, len(snapshot.Entries))
	validated := make([]Entry, len(snapshot.Entries))
	for i, entry := range snapshot.Entries {
		if err := validateEntry(&entry); err != nil {
			return fmt.Errorf("validate import entry %d: %w", i+1, err)
		}
		if _, exists := seen[entry.SubjectKey]; exists {
			return fmt.Errorf("%w: repeated subject in snapshot", ErrInvalid)
		}
		seen[entry.SubjectKey] = struct{}{}
		validated[i] = entry
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin settings import: %w", err)
	}
	defer tx.Rollback()
	var count int64
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM profile_settings`).Scan(&count); err != nil {
		return fmt.Errorf("check import destination: %w", err)
	}
	if count != 0 {
		return ErrDestinationNotEmpty
	}
	for _, entry := range validated {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO profile_settings (subject_key, settings_json, revision, updated_at) VALUES (?, ?, ?, ?)`,
			entry.SubjectKey, string(entry.Settings), entry.Revision, entry.UpdatedAt); err != nil {
			return fmt.Errorf("import settings row: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit settings import: %w", err)
	}
	return nil
}

func validateEntry(entry *Entry) error {
	if err := validateSubject(entry.SubjectKey); err != nil {
		return err
	}
	if entry.Revision <= 0 || entry.Revision > MaxRevision {
		return fmt.Errorf("%w: imported revision must be a positive JSON-safe integer", ErrInvalid)
	}
	if !validConfigurationTimestamp(entry.UpdatedAt) {
		return fmt.Errorf("%w: invalid configuration timestamp", ErrInvalid)
	}
	settings, err := validateSettings(entry.Settings)
	if err != nil {
		return err
	}
	entry.Settings = settings
	return nil
}

// EncodeSnapshot writes a human-inspectable portable file. The caller should use
// file mode 0600 and must not put this private file inside a public release.
func EncodeSnapshot(w io.Writer, snapshot Snapshot) error {
	return encodeSnapshot(w, snapshot, MaxSnapshotBytes)
}

func encodeSnapshot(w io.Writer, snapshot Snapshot, maxBytes int) error {
	encoder := json.NewEncoder(&snapshotWriter{destination: w, remaining: maxBytes})
	encoder.SetIndent("", "  ")
	return encoder.Encode(snapshot)
}

type snapshotWriter struct {
	destination io.Writer
	remaining   int
}

func (w *snapshotWriter) Write(data []byte) (int, error) {
	if len(data) > w.remaining {
		return 0, fmt.Errorf("%w: settings export exceeds the restorable snapshot size limit", ErrInvalid)
	}
	written, err := w.destination.Write(data)
	w.remaining -= written
	return written, err
}

// DecodeSnapshot rejects unknown envelope fields, trailing JSON, and oversized
// files. Configuration documents still require the application's typed schema
// validator if their source is not an operator's own trusted export.
func DecodeSnapshot(r io.Reader) (Snapshot, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxSnapshotBytes+1))
	defer clear(data)
	if err != nil {
		return Snapshot{}, fmt.Errorf("%w: cannot read settings snapshot", ErrInvalid)
	}
	if len(data) > MaxSnapshotBytes {
		return Snapshot{}, fmt.Errorf("%w: snapshot exceeds %d bytes", ErrInvalid, MaxSnapshotBytes)
	}
	if err := strictjson.Validate(data, strictjson.Options{MaxDepth: 32, FoldNames: true}); err != nil {
		return Snapshot{}, fmt.Errorf("%w: malformed or ambiguous settings snapshot", ErrInvalid)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var snapshot Snapshot
	if err := decoder.Decode(&snapshot); err != nil {
		return Snapshot{}, fmt.Errorf("%w: settings snapshot does not match its schema", ErrInvalid)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return Snapshot{}, fmt.Errorf("%w: snapshot has trailing data", ErrInvalid)
	}
	return snapshot, nil
}
