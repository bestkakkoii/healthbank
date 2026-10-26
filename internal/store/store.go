// Copyright 2026 bestkakkoii llc.
// SPDX-License-Identifier: MIT

// Package store persists configuration documents, never medical observations or
// login credentials. Its caller must derive a subject key only after a verified
// upstream login and validate settings against the application's configuration
// schema before passing them to Put.
package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"modernc.org/sqlite"
)

const (
	// SchemaVersion identifies the database schema and portable export format.
	SchemaVersion = 1
	// MaxSettingsBytes bounds one family's per-person configuration document.
	MaxSettingsBytes = 256 << 10
)

var (
	ErrConflict            = errors.New("settings revision conflict")
	ErrInvalid             = errors.New("invalid settings input")
	ErrDestinationNotEmpty = errors.New("import destination already has settings")
	ErrUnsupportedSchema   = errors.New("unsupported settings schema version")
	subjectPattern         = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// Options selects a storage engine. Driver is "sqlite" (the default) or "mysql".
// DSN is used only by MySQL; it must not be included in application logs.
// Relative SQLitePath values are resolved next to the executable, not the CWD.
type Options struct {
	Driver     string
	DSN        string
	SQLitePath string
}

// Document is one validated configuration document. UpdatedAt is a configuration
// modification timestamp, not a login or access history.
type Document struct {
	Revision  int64           `json:"revision"`
	Settings  json.RawMessage `json:"settings"`
	UpdatedAt string          `json:"updated_at,omitempty"`
}

// Store wraps database/sql. All reads and mutations accept cancellation contexts.
type Store struct {
	db     *sql.DB
	driver string
}

// DefaultSQLitePath returns the executable-adjacent data path used on BaoTa.
func DefaultSQLitePath() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate executable: %w", err)
	}
	return filepath.Join(filepath.Dir(executable), "data", "settings.sqlite"), nil
}

// Open creates the SQLite file when needed, opens the selected database, and
// applies idempotent schema migrations. A MySQL database must already exist.
func Open(ctx context.Context, opts Options) (*Store, error) {
	driver := strings.ToLower(strings.TrimSpace(opts.Driver))
	if driver == "" {
		driver = "sqlite"
	}
	var dsn string
	switch driver {
	case "sqlite":
		if opts.DSN != "" {
			return nil, fmt.Errorf("%w: SQLite accepts SQLitePath, not a DSN", ErrInvalid)
		}
		path, err := prepareSQLitePath(opts.SQLitePath)
		if err != nil {
			return nil, err
		}
		query := url.Values{}
		query.Add("_pragma", "busy_timeout(5000)")
		query.Add("_pragma", "journal_mode(WAL)")
		query.Add("_pragma", "synchronous(NORMAL)")
		query.Add("_pragma", "foreign_keys(ON)")
		query.Set("_txlock", "immediate")
		dsn = (&url.URL{Scheme: "file", Path: filepath.ToSlash(path), RawQuery: query.Encode()}).String()
	case "mysql":
		if strings.TrimSpace(opts.DSN) == "" {
			return nil, fmt.Errorf("%w: MySQL DSN is required", ErrInvalid)
		}
		cfg, err := mysql.ParseDSN(opts.DSN)
		if err != nil {
			// Parse errors can include parts of a DSN. Do not expose them.
			return nil, fmt.Errorf("%w: malformed MySQL DSN", ErrInvalid)
		}
		if cfg.DBName == "" {
			return nil, fmt.Errorf("%w: MySQL database name is required", ErrInvalid)
		}
		if cfg.Timeout == 0 {
			cfg.Timeout = 10 * time.Second
		}
		if cfg.ReadTimeout == 0 {
			cfg.ReadTimeout = 30 * time.Second
		}
		if cfg.WriteTimeout == 0 {
			cfg.WriteTimeout = 30 * time.Second
		}
		// All application statements are single statements; never enable the DSN
		// option that would allow multi-statement execution.
		cfg.MultiStatements = false
		dsn = cfg.FormatDSN()
	default:
		return nil, fmt.Errorf("%w: storage driver must be sqlite or mysql", ErrInvalid)
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("open settings database: %w", err)
	}
	if driver == "sqlite" {
		// One pooled connection keeps small-family writes orderly. PRAGMAs in
		// the DSN also apply to any replacement connection after a failure.
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
	} else {
		db.SetMaxOpenConns(5)
		db.SetMaxIdleConns(2)
		db.SetConnMaxLifetime(3 * time.Minute)
		db.SetConnMaxIdleTime(time.Minute)
	}
	s := &Store{db: db, driver: driver}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connect to settings database: %w", err)
	}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func prepareSQLitePath(path string) (string, error) {
	if path == "" {
		var err error
		path, err = DefaultSQLitePath()
		if err != nil {
			return "", err
		}
	} else if !filepath.IsAbs(path) {
		executable, err := os.Executable()
		if err != nil {
			return "", fmt.Errorf("locate executable: %w", err)
		}
		path = filepath.Join(filepath.Dir(executable), path)
	}
	path = filepath.Clean(path)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("create settings directory: %w", err)
	}
	// Precreate only new files with restricted permissions; do not truncate or
	// silently change permissions of an existing deployment's database.
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err == nil {
		if err := file.Close(); err != nil {
			return "", fmt.Errorf("close new settings database: %w", err)
		}
	} else if !errors.Is(err, os.ErrExist) {
		return "", fmt.Errorf("create settings database: %w", err)
	}
	return path, nil
}

// Driver returns the selected engine name without exposing its DSN.
func (s *Store) Driver() string { return s.driver }

// Close releases pooled database connections.
func (s *Store) Close() error { return s.db.Close() }

// Get returns revision zero and an empty object for an unseen subject. Merely
// logging in or reading configuration does not create a database row.
func (s *Store) Get(ctx context.Context, subjectKey string) (Document, error) {
	if err := validateSubject(subjectKey); err != nil {
		return Document{}, err
	}
	doc, err := scanDocument(s.db.QueryRowContext(ctx,
		`SELECT revision, settings_json, updated_at FROM profile_settings WHERE subject_key = ?`, subjectKey))
	if errors.Is(err, sql.ErrNoRows) {
		return Document{Revision: 0, Settings: json.RawMessage(`{}`)}, nil
	}
	if err != nil {
		return Document{}, fmt.Errorf("read settings: %w", err)
	}
	return doc, nil
}

// Put replaces a validated configuration using optimistic concurrency. Clients
// must send the last revision they read; stale writes return ErrConflict so one
// device cannot silently overwrite another device's changes.
func (s *Store) Put(ctx context.Context, subjectKey string, expectedRevision int64, settings json.RawMessage) (Document, error) {
	if err := validateSubject(subjectKey); err != nil {
		return Document{}, err
	}
	if expectedRevision < 0 || expectedRevision == math.MaxInt64 {
		return Document{}, fmt.Errorf("%w: invalid revision", ErrInvalid)
	}
	compact, err := validateSettings(settings)
	if err != nil {
		return Document{}, err
	}
	doc := Document{
		Revision:  expectedRevision + 1,
		Settings:  compact,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if expectedRevision == 0 {
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO profile_settings (subject_key, settings_json, revision, updated_at) VALUES (?, ?, ?, ?)`,
			subjectKey, string(compact), doc.Revision, doc.UpdatedAt)
		if isDuplicate(err) {
			return Document{}, ErrConflict
		}
		if err != nil {
			return Document{}, fmt.Errorf("create settings: %w", err)
		}
		return doc, nil
	}
	result, err := s.db.ExecContext(ctx,
		`UPDATE profile_settings SET settings_json = ?, revision = ?, updated_at = ? WHERE subject_key = ? AND revision = ?`,
		string(compact), doc.Revision, doc.UpdatedAt, subjectKey, expectedRevision)
	if err != nil {
		return Document{}, fmt.Errorf("update settings: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return Document{}, fmt.Errorf("check settings update: %w", err)
	}
	if count != 1 {
		return Document{}, ErrConflict
	}
	return doc, nil
}

// Delete removes one subject's configuration only if its revision still matches.
func (s *Store) Delete(ctx context.Context, subjectKey string, expectedRevision int64) error {
	if err := validateSubject(subjectKey); err != nil {
		return err
	}
	if expectedRevision <= 0 {
		return fmt.Errorf("%w: deletion requires a positive revision", ErrInvalid)
	}
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM profile_settings WHERE subject_key = ? AND revision = ?`, subjectKey, expectedRevision)
	if err != nil {
		return fmt.Errorf("delete settings: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check settings deletion: %w", err)
	}
	if count != 1 {
		return ErrConflict
	}
	return nil
}

func validateSubject(key string) error {
	if !subjectPattern.MatchString(key) {
		return fmt.Errorf("%w: subject key must be a 64-character lowercase hexadecimal HMAC", ErrInvalid)
	}
	return nil
}

func validateSettings(raw json.RawMessage) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || len(raw) > MaxSettingsBytes || trimmed[0] != '{' || !json.Valid(trimmed) {
		return nil, fmt.Errorf("%w: settings must be a JSON object of at most %d bytes", ErrInvalid, MaxSettingsBytes)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, trimmed); err != nil {
		return nil, fmt.Errorf("%w: malformed settings JSON", ErrInvalid)
	}
	return json.RawMessage(compact.Bytes()), nil
}

type scanner interface{ Scan(...any) error }

func scanDocument(row scanner) (Document, error) {
	var doc Document
	var raw string
	if err := row.Scan(&doc.Revision, &raw, &doc.UpdatedAt); err != nil {
		return Document{}, err
	}
	settings, err := validateSettings(json.RawMessage(raw))
	if err != nil {
		return Document{}, fmt.Errorf("stored configuration is invalid: %w", err)
	}
	if doc.Revision < 1 {
		return Document{}, fmt.Errorf("stored configuration has invalid revision")
	}
	if _, err := time.Parse(time.RFC3339Nano, doc.UpdatedAt); err != nil {
		return Document{}, fmt.Errorf("stored configuration has invalid timestamp")
	}
	doc.Settings = settings
	return doc, nil
}

func isDuplicate(err error) bool {
	if err == nil {
		return false
	}
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062
	}
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		// Extended result codes: SQLITE_CONSTRAINT_PRIMARYKEY / UNIQUE.
		return sqliteErr.Code() == 1555 || sqliteErr.Code() == 2067
	}
	return false
}
