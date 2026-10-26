// Copyright (c) 2026 bestkakkoii llc. SPDX-License-Identifier: MIT
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/bestkakkoii/healthbank/internal/config"
	"github.com/bestkakkoii/healthbank/internal/server"
	"github.com/bestkakkoii/healthbank/internal/store"
)

var version = "dev"
var commit = "unknown"
var buildTime = "unknown"

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))
	if err := run(os.Args[1:]); err != nil {
		slog.Error("application stopped", "error", err)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) == 1 && (args[0] == "--version" || args[0] == "version") {
		fmt.Printf("healthbank %s\ncommit %s\nbuilt %s\nCopyright (c) 2026 bestkakkoii llc.\n", version, commit, buildTime)
		return nil
	}
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Print("healthbank [serve]\nhealthbank --version\nhealthbank settings export <new-file.json>\nhealthbank settings import <file.json>\n\nDefault:127.0.0.1:8787; SQLite and identity.key in executable-adjacent data/.\nEnvironment:HEALTHBANK_ADDR, PORT, HEALTHBANK_DATA_DIR, HEALTHBANK_DB_DRIVER, HEALTHBANK_MYSQL_DSN.\n")
		return nil
	}
	maintenance := len(args) == 3 && args[0] == "settings" && (args[1] == "export" || args[1] == "import")
	if !(len(args) == 0 || (len(args) == 1 && args[0] == "serve") || maintenance) {
		return errors.New("unknown command; use --help")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	lock, err := config.Lock(cfg.DataDir)
	if err != nil {
		return err
	}
	defer lock.Close()
	if maintenance && args[1] == "import" {
		if _, err := os.Stat(filepath.Join(cfg.DataDir, "identity.key")); err != nil {
			return errors.New("restore the original data/identity.key before importing settings; never replace it with a new key")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	db, err := store.Open(ctx, store.Options{Driver: cfg.Driver, DSN: cfg.MySQLDSN, SQLitePath: filepath.Join(cfg.DataDir, "settings.sqlite")})
	cancel()
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "identity.key")); os.IsNotExist(err) {
		checkCtx, checkCancel := context.WithTimeout(context.Background(), 30*time.Second)
		existing, checkErr := db.Export(checkCtx)
		checkCancel()
		if checkErr != nil {
			return checkErr
		}
		if len(existing.Entries) > 0 {
			return errors.New("existing configuration profiles require the original identity.key; restore it instead of generating a new key")
		}
	}
	secret, err := config.Secret(cfg.DataDir)
	if err != nil {
		return err
	}
	if maintenance {
		return transfer(db, args[1], args[2])
	}
	app, err := server.New(db, secret, version)
	if err != nil {
		return err
	}
	defer app.Close()
	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	httpServer := &http.Server{Handler: app.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 125 * time.Second, WriteTimeout: 130 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	stopped, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	failures := make(chan error, 1)
	go func() { failures <- httpServer.Serve(listener) }()
	slog.Info("healthbank started", "version", version, "address", cfg.Addr, "database", cfg.Driver, "copyright", "2026 bestkakkoii llc.")
	select {
	case err := <-failures:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-stopped.Done():
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(ctx); err != nil {
			_ = httpServer.Close()
			return fmt.Errorf("shutdown: %w", err)
		}
	}
	return nil
}
func transfer(db *store.Store, action, path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if action == "export" {
		snapshot, err := db.Export(ctx)
		if err != nil {
			return err
		}
		out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("create new export file: %w", err)
		}
		good := false
		defer func() {
			_ = out.Close()
			if !good {
				_ = os.Remove(path)
			}
		}()
		if err := store.EncodeSnapshot(out, snapshot); err != nil {
			return err
		}
		if err := out.Sync(); err != nil {
			return err
		}
		good = true
		fmt.Printf("Exported %d configuration profiles. Back up the matching identity.key separately.\n", len(snapshot.Entries))
		return nil
	}
	in, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open settings export: %w", err)
	}
	defer in.Close()
	snapshot, err := store.DecodeSnapshot(in)
	if err != nil {
		return err
	}
	for _, entry := range snapshot.Entries {
		if _, err := server.ValidatePortableSettings(entry.Settings); err != nil {
			return fmt.Errorf("configuration schema rejected: %w", err)
		}
	}
	if err := db.Import(ctx, snapshot); err != nil {
		return err
	}
	fmt.Printf("Imported %d configuration profiles.\n", len(snapshot.Entries))
	return nil
}
