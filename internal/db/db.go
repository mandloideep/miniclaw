// Package db opens the SQLite store and applies migrations on startup.
// The returned *sql.DB is the single connection pool the rest of the app
// shares.
package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pressly/goose/v3"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, registers "sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Config controls where the DB lives and how it's opened.
type Config struct {
	// Path is the SQLite file path. If empty, DefaultPath() is used.
	Path string
}

// DefaultPath returns ~/.miniclaw/miniclaw.db, creating the dir if needed.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home dir: %w", err)
	}
	dir := filepath.Join(home, ".miniclaw")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create %s: %w", dir, err)
	}
	return filepath.Join(dir, "miniclaw.db"), nil
}

// Open opens the database, applies pending migrations, and returns the pool.
// Caller owns the returned *sql.DB and must Close it on shutdown.
func Open(ctx context.Context, cfg Config) (*sql.DB, error) {
	path := cfg.Path
	if path == "" {
		p, err := DefaultPath()
		if err != nil {
			return nil, err
		}
		path = p
	}

	// _pragma values applied at connect time. Foreign keys are off by default
	// in SQLite — turning them on is non-negotiable for the schema's CASCADEs
	// to do anything. WAL improves concurrent reads while a write is in flight.
	dsn := fmt.Sprintf(
		"file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)",
		path,
	)
	pool, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite at %s: %w", path, err)
	}
	if err := pool.PingContext(ctx); err != nil {
		_ = pool.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	if err := migrate(ctx, pool); err != nil {
		_ = pool.Close()
		return nil, err
	}
	return pool, nil
}

// migrate applies any pending goose migrations from the embedded migrations dir.
func migrate(ctx context.Context, pool *sql.DB) error {
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("goose dialect: %w", err)
	}
	// Goose logs to stderr by default; quiet it unless GOOSE_VERBOSE is set.
	if os.Getenv("GOOSE_VERBOSE") == "" {
		goose.SetLogger(goose.NopLogger())
	}
	if err := goose.UpContext(ctx, pool, "migrations"); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

// ErrNotFound is returned by repository methods when a row lookup misses.
var ErrNotFound = errors.New("db: not found")

// IsNotFound is true for sql.ErrNoRows or our ErrNotFound sentinel.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound) || errors.Is(err, sql.ErrNoRows)
}
