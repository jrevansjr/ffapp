// Package database owns the application's shared SQLite handle, embedded
// migrations, default settings, and local sample-data writes.
package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jrevansjr/ffapp/backend/migrations"
	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

// DefaultPath is relative to the backend working directory used by the
// documented run commands.
const DefaultPath = "./data/draft.db"

// PathFromEnv returns DB_PATH when configured and DefaultPath otherwise.
func PathFromEnv() string {
	if path := os.Getenv("DB_PATH"); path != "" {
		return path
	}
	return DefaultPath
}

// Open creates the database directory when needed, opens SQLite with the
// required concurrency and integrity pragmas, applies pending migrations, and
// ensures the singleton settings row exists. Existing application data is
// preserved. The caller owns and must close the returned database handle.
func Open(dbPath string) (*sql.DB, error) {
	if dbPath == "" {
		dbPath = DefaultPath
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	db, err := sql.Open("sqlite",
		"file:"+dbPath+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}

	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := ensureSettings(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}

// migrate applies only migrations not already recorded by Goose. Migration SQL
// is embedded in the binary so startup never depends on external migration files.
func migrate(db *sql.DB) error {
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("set migration dialect: %w", err)
	}
	if err := goose.Up(db, "."); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

// ensureSettings establishes the id = 1 application-settings invariant without
// overwriting values a user has already saved.
func ensureSettings(db *sql.DB) error {
	updatedAt := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(`
		INSERT INTO app_settings (
			id,
			sleeper_username,
			sleeper_league_id,
			sleeper_draft_id,
			polling_enabled,
			polling_interval_ms,
			updated_at
		) VALUES (1, '', '', '', 1, 2000, ?)
		ON CONFLICT (id) DO NOTHING
	`, updatedAt)
	if err != nil {
		return fmt.Errorf("ensure default settings: %w", err)
	}
	return nil
}
