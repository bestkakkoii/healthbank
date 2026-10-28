// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT

// Package config resolves deployment settings and manages instance identity keys.
package config

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

type Config struct{ Addr, DataDir, Driver, MySQLDSN string }

func Load() (Config, error) {
	exe, err := os.Executable()
	if err != nil {
		// Restricted containers may omit /proc/self/exe. Resolve the actual command
		// path instead of silently placing persistent data in the working directory.
		exe, err = exec.LookPath(os.Args[0])
		if err == nil {
			exe, err = filepath.Abs(exe)
		}
		if err != nil {
			return Config{}, fmt.Errorf("locate executable: %w", err)
		}
	}
	dir := strings.TrimSpace(os.Getenv("HEALTHBANK_DATA_DIR"))
	if dir == "" {
		dir = filepath.Join(filepath.Dir(exe), "data")
	} else if !filepath.IsAbs(dir) {
		dir = filepath.Join(filepath.Dir(exe), dir)
	}
	addr := strings.TrimSpace(os.Getenv("HEALTHBANK_ADDR"))
	if addr == "" {
		port := strings.TrimSpace(os.Getenv("PORT"))
		if port == "" {
			port = "8787"
		}
		n, e := strconv.Atoi(port)
		if e != nil || n < 1 || n > 65535 {
			return Config{}, fmt.Errorf("PORT must be1–65535")
		}
		addr = net.JoinHostPort("127.0.0.1", port)
	}
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return Config{}, fmt.Errorf("HEALTHBANK_ADDR must be host:port")
	}
	driver := os.Getenv("HEALTHBANK_DB_DRIVER")
	if driver == "" {
		driver = "sqlite"
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return Config{}, fmt.Errorf("create data directory: %w", err)
	}
	return Config{addr, dir, driver, os.Getenv("HEALTHBANK_MYSQL_DSN")}, nil
}

// Lock also prevents an export/import process from racing a live server.
func Lock(dir string) (*os.File, error) {
	f, err := os.OpenFile(filepath.Join(dir, "app.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open instance lock: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("this data directory is already in use; stop the app before maintenance")
	}
	return f, nil
}
func Secret(dir string) ([]byte, error) {
	path := filepath.Join(dir, "identity.key")
	b, err := os.ReadFile(path)
	if err == nil {
		if len(b) != 32 {
			return nil, fmt.Errorf("identity.key must contain32bytes; restore the original key")
		}
		return b, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read identity key: %w", err)
	}
	b = make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return nil, fmt.Errorf("generate identity key: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, fmt.Errorf("create identity key: %w", err)
	}
	if _, err = f.Write(b); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("write identity key: %w", err)
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("sync identity key: %w", err)
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	return b, nil
}
func Subject(secret []byte, verifiedID string) string {
	h := hmac.New(sha256.New, secret)
	_, _ = h.Write([]byte("healthbank:official-subject:v1\x00" + strings.ToUpper(strings.TrimSpace(verifiedID))))
	return hex.EncodeToString(h.Sum(nil))
}
