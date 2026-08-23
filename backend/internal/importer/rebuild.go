package importer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jrevansjr/ffapp/backend/internal/database"
)

const minimumRealPlayerCount = 100

// RebuildResult identifies the recoverable backup and installed database.
type RebuildResult struct {
	BackupPath string
	DBPath     string
}

// Rebuild constructs and validates a new M6.1 database before replacing the
// configured database. The caller must require explicit user confirmation.
func (runner *Runner) Rebuild(ctx context.Context) (RebuildResult, error) {
	if err := os.MkdirAll(filepath.Dir(runner.DBPath), 0o755); err != nil {
		return RebuildResult{}, fmt.Errorf("create database directory: %w", err)
	}

	backupPath, err := runner.backupCurrentDatabase()
	if err != nil {
		return RebuildResult{}, err
	}
	timestamp := runner.Now().UTC().Format("20060102T150405Z")
	temporaryPath := filepath.Join(
		filepath.Dir(runner.DBPath),
		"."+filepath.Base(runner.DBPath)+".rebuild-"+timestamp,
	)
	if _, err := os.Stat(temporaryPath); err == nil {
		return RebuildResult{}, fmt.Errorf("temporary rebuild database already exists: %s", temporaryPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return RebuildResult{}, fmt.Errorf("inspect temporary rebuild path: %w", err)
	}

	if err := runner.buildAt(ctx, temporaryPath); err != nil {
		return RebuildResult{}, fmt.Errorf(
			"build replacement database (existing database preserved; inspect %s): %w",
			temporaryPath,
			err,
		)
	}
	if err := validateM61Database(temporaryPath); err != nil {
		return RebuildResult{}, fmt.Errorf(
			"validate replacement database (existing database preserved; inspect %s): %w",
			temporaryPath,
			err,
		)
	}
	if err := removeSQLiteSidecars(runner.DBPath); err != nil {
		return RebuildResult{}, err
	}
	if err := os.Rename(temporaryPath, runner.DBPath); err != nil {
		return RebuildResult{}, fmt.Errorf("install replacement database: %w", err)
	}
	return RebuildResult{BackupPath: backupPath, DBPath: runner.DBPath}, nil
}

func (runner *Runner) backupCurrentDatabase() (string, error) {
	if _, err := os.Stat(runner.DBPath); errors.Is(err, os.ErrNotExist) {
		return "", nil
	} else if err != nil {
		return "", fmt.Errorf("inspect existing database: %w", err)
	}
	backupDir := filepath.Join(filepath.Dir(runner.DBPath), "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", fmt.Errorf("create database backup directory: %w", err)
	}
	timestamp := runner.Now().UTC().Format("20060102T150405Z")
	baseName := strings.TrimSuffix(filepath.Base(runner.DBPath), filepath.Ext(runner.DBPath))
	backupPath := filepath.Join(backupDir, baseName+"-"+timestamp+".db")
	absoluteBackupPath, err := filepath.Abs(backupPath)
	if err != nil {
		return "", fmt.Errorf("resolve database backup path: %w", err)
	}

	db, err := database.Open(runner.DBPath)
	if err != nil {
		return "", fmt.Errorf("open existing database for backup: %w", err)
	}
	defer db.Close()
	if _, err := db.Exec(`PRAGMA wal_checkpoint(FULL)`); err != nil {
		return "", fmt.Errorf("checkpoint existing database: %w", err)
	}
	quotedPath := strings.ReplaceAll(absoluteBackupPath, "'", "''")
	if _, err := db.Exec(`VACUUM INTO '` + quotedPath + `'`); err != nil {
		return "", fmt.Errorf("create database backup: %w", err)
	}
	return backupPath, nil
}

func validateM61Database(dbPath string) error {
	db, err := database.Open(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	var teamCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM nfl_teams`).Scan(&teamCount); err != nil {
		return fmt.Errorf("count NFL teams: %w", err)
	}
	if teamCount != len(NFLTeams) {
		return fmt.Errorf("NFL team count is %d; want %d", teamCount, len(NFLTeams))
	}
	var playerCount int
	if err := db.QueryRow(`
		SELECT COUNT(*)
		FROM players
		WHERE active = 1 AND position IN ('QB', 'RB', 'WR', 'TE')
	`).Scan(&playerCount); err != nil {
		return fmt.Errorf("count active fantasy players: %w", err)
	}
	if playerCount < minimumRealPlayerCount {
		return fmt.Errorf(
			"active fantasy player count is %d; expected at least %d",
			playerCount,
			minimumRealPlayerCount,
		)
	}
	var invalidPlayers int
	if err := db.QueryRow(`
		SELECT COUNT(*)
		FROM players
		WHERE sleeper_player_id IS NULL
			OR sleeper_player_id LIKE 'sample-%'
			OR position NOT IN ('QB', 'RB', 'WR', 'TE')
	`).Scan(&invalidPlayers); err != nil {
		return fmt.Errorf("validate player identities: %w", err)
	}
	if invalidPlayers != 0 {
		return fmt.Errorf("found %d invalid or sample player rows", invalidPlayers)
	}
	for _, table := range []string{
		"player_season_stats",
		"player_week_stats",
		"player_adp",
		"player_tiers",
		"odds",
		"drafts",
		"draft_picks",
	} {
		count, err := tableCount(db, table)
		if err != nil {
			return err
		}
		if count != 0 {
			return fmt.Errorf("%s contains %d rows; M6.1 rebuild expects none", table, count)
		}
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("run foreign key check: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		return fmt.Errorf("foreign key check found a violation")
	}
	return rows.Err()
}

func tableCount(db *sql.DB, table string) (int, error) {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
		return 0, fmt.Errorf("count %s: %w", table, err)
	}
	return count, nil
}

func removeSQLiteSidecars(dbPath string) error {
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(dbPath + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove stale SQLite sidecar %s: %w", suffix, err)
		}
	}
	return nil
}
