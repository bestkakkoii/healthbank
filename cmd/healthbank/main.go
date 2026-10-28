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
	"golang.org/x/net/netutil"
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
		fmt.Print("healthbank [serve]\nhealthbank --version\nhealthbank diagnose-nhi\nhealthbank settings export <new-file.json>\nhealthbank settings import <file.json>\nhealthbank settings import-legacy <file.json>\nhealthbank settings verify-key\nhealthbank settings bind-legacy-key\n\nDefault:127.0.0.1:8787; SQLite and identity.key in executable-adjacent data/.\nEnvironment:HEALTHBANK_ADDR, PORT, HEALTHBANK_DATA_DIR, HEALTHBANK_DB_DRIVER, HEALTHBANK_MYSQL_DSN.\n\n舊版設定：先停止服務並備份原資料庫與 identity.key；bind-legacy-key 明確確認目前檔案就是原金鑰。import-legacy 僅供沒有金鑰核對值的舊備份，不能略過新版備份的配對檢查。\n診斷：diagnose-nhi 只測試官方登入頁及驗證碼，不登入、不讀取資料庫、不改動現有服務。\nCopyright (c) 2026 bestkakkoii llc.\n")
		return nil
	}
	if len(args) == 1 && args[0] == "diagnose-nhi" {
		return diagnoseNHI(os.Stdout)
	}
	maintenance := len(args) == 3 && args[0] == "settings" && (args[1] == "export" || args[1] == "import" || args[1] == "import-legacy")
	keyMaintenance := len(args) == 2 && args[0] == "settings" && (args[1] == "verify-key" || args[1] == "bind-legacy-key")
	if !(len(args) == 0 || (len(args) == 1 && args[0] == "serve") || maintenance || keyMaintenance) {
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
	if keyMaintenance || (maintenance && args[1] != "export") {
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
	defer clear(secret)
	if keyMaintenance {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if args[1] == "bind-legacy-key" {
			if err := db.BindIdentityKey(ctx, config.KeyCheck(secret), true); err != nil {
				return err
			}
			fmt.Println("已將目前金鑰與設定資料庫綁定。舊資料原本沒有核對值，本次採用營運者對原金鑰的明確確認；請保留原備份。")
			return nil
		}
		if err := db.VerifyIdentityKey(ctx, config.KeyCheck(secret)); err != nil {
			return err
		}
		fmt.Println("目前 identity.key 與設定資料庫的核對值相符。")
		return nil
	}
	if maintenance {
		return transfer(db, secret, args[1], args[2])
	}
	keyCtx, keyCancel := context.WithTimeout(context.Background(), 30*time.Second)
	err = prepareRuntimeIdentity(keyCtx, db, secret)
	keyCancel()
	if err != nil {
		return err
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
	go func() { failures <- httpServer.Serve(netutil.LimitListener(listener, app.ConnectionLimit())) }()
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

func prepareRuntimeIdentity(ctx context.Context, db *store.Store, secret []byte) error {
	err := db.BindIdentityKey(ctx, config.KeyCheck(secret), false)
	if errors.Is(err, store.ErrLegacyIdentityKey) {
		slog.Warn("legacy settings key has no recorded verification; service remains available", "code", "identity_key_unverified")
		return nil
	}
	if err != nil {
		return err
	}
	origin, err := db.IdentityKeyOrigin(ctx)
	if err == nil && origin == store.IdentityKeyLegacyUnverified {
		slog.Warn("restored legacy key pairing remains unverified; service remains available", "code", "identity_key_unverified")
	}
	return err
}

func transfer(db *store.Store, secret []byte, action, path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	check := config.KeyCheck(secret)
	if action == "export" {
		if err := db.BindIdentityKey(ctx, check, false); err != nil && !errors.Is(err, store.ErrLegacyIdentityKey) {
			return err
		}
		snapshot, err := db.Export(ctx)
		if err != nil {
			return err
		}
		if snapshot.IdentityKeyCheck == "" {
			// Bind this backup to the currently supplied key without inventing proof
			// that an older database originally used that key. Serving is unaffected.
			snapshot.SchemaVersion = store.SchemaVersion
			snapshot.IdentityKeyCheck = check
			snapshot.IdentityKeyOrigin = store.IdentityKeyLegacyUnverified
			fmt.Println("舊設定沒有原金鑰核對紀錄；備份已記錄本次使用的金鑰核對值，並保留「原始配對未驗證」來源。服務與原設定不受影響。")
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
		if err := out.Close(); err != nil {
			return fmt.Errorf("close settings export: %w", err)
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
	if err := store.ValidateSnapshotKey(snapshot, check, action == "import-legacy"); err != nil {
		return err
	}
	for i, entry := range snapshot.Entries {
		validated, err := server.ValidatePortableSettings(entry.Settings)
		if err != nil {
			return fmt.Errorf("configuration schema rejected: %w", err)
		}
		// Persist exactly the typed, reconciled configuration that was validated,
		// never the original raw JSON from an untrusted portable file.
		canonical, err := validated.Document()
		if err != nil {
			return errors.New("cannot encode validated configuration")
		}
		snapshot.Entries[i].Settings = canonical
	}
	if err := db.BindIdentityKey(ctx, check, false); err != nil {
		return err
	}
	if err := db.Import(ctx, snapshot); err != nil {
		return err
	}
	if snapshot.SchemaVersion == store.LegacySchemaVersion {
		fmt.Println("已依明確要求匯入舊備份；舊格式沒有金鑰核對值，本次無法由檔案證明金鑰配對，請確認登入後設定並保留原備份。")
	}
	fmt.Printf("Imported %d configuration profiles.\n", len(snapshot.Entries))
	return nil
}
