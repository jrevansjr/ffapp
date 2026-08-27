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

// Rebuild constructs and validates a new M6.3 database before replacing the
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
	if err := validateM63Database(temporaryPath); err != nil {
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

func validateM63Database(dbPath string) error {
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
	weeklyCount, err := tableCount(db, "player_week_stats")
	if err != nil {
		return err
	}
	if weeklyCount < minimumRealWeeklyStatRows {
		return fmt.Errorf("weekly stat count is %d; expected at least %d", weeklyCount, minimumRealWeeklyStatRows)
	}
	seasonCount, err := tableCount(db, "player_season_stats")
	if err != nil {
		return err
	}
	if seasonCount < minimumRealSeasonStatRows {
		return fmt.Errorf("season stat count is %d; expected at least %d", seasonCount, minimumRealSeasonStatRows)
	}
	var coveredWeeks int
	if err := db.QueryRow(`
		SELECT COUNT(DISTINCT week)
		FROM player_week_stats
		WHERE season = ? AND week BETWEEN 1 AND 18
	`, 2025).Scan(&coveredWeeks); err != nil {
		return fmt.Errorf("validate weekly stat coverage: %w", err)
	}
	if coveredWeeks != 18 {
		return fmt.Errorf("weekly stats cover %d weeks; expected 18", coveredWeeks)
	}
	var invalidStats int
	if err := db.QueryRow(`
		SELECT
			(SELECT COUNT(*) FROM player_week_stats WHERE season != 2025 OR week NOT BETWEEN 1 AND 18) +
			(SELECT COUNT(*) FROM player_season_stats WHERE season != 2025)
	`).Scan(&invalidStats); err != nil {
		return fmt.Errorf("validate stat seasons and weeks: %w", err)
	}
	if invalidStats != 0 {
		return fmt.Errorf("found %d stats outside the M6.2 season/week scope", invalidStats)
	}
	var aggregateMismatches int
	if err := db.QueryRow(`
		WITH weekly AS (
			SELECT
				player_id, season, COUNT(*) AS games_played,
				SUM(fantasy_points_half_ppr) AS fantasy_points_half_ppr,
				SUM(targets) AS targets, SUM(receptions) AS receptions,
				SUM(rushing_attempts) AS rushing_attempts,
				SUM(receiving_yards) AS receiving_yards,
				SUM(rushing_yards) AS rushing_yards,
				SUM(receiving_touchdowns) AS receiving_touchdowns,
				SUM(rushing_touchdowns) AS rushing_touchdowns,
				SUM(passing_yards) AS passing_yards
			FROM player_week_stats
			WHERE season = 2025
			GROUP BY player_id, season
		)
		SELECT COUNT(*)
		FROM player_season_stats AS season
		LEFT JOIN weekly
			ON weekly.player_id = season.player_id AND weekly.season = season.season
		WHERE season.season = 2025 AND (
			weekly.player_id IS NULL OR
			season.games_played != weekly.games_played OR
			ABS(season.fantasy_points_half_ppr - weekly.fantasy_points_half_ppr) > 0.001 OR
			season.targets != weekly.targets OR season.receptions != weekly.receptions OR
			season.rushing_attempts != weekly.rushing_attempts OR
			season.receiving_yards != weekly.receiving_yards OR
			season.rushing_yards != weekly.rushing_yards OR
			season.receiving_touchdowns != weekly.receiving_touchdowns OR
			season.rushing_touchdowns != weekly.rushing_touchdowns OR
			season.passing_yards != weekly.passing_yards
		)
	`).Scan(&aggregateMismatches); err != nil {
		return fmt.Errorf("validate season stat aggregates: %w", err)
	}
	if aggregateMismatches != 0 {
		return fmt.Errorf("found %d season rows inconsistent with weekly stats", aggregateMismatches)
	}
	distinctStatPlayers, err := countDistinctStatPlayers(db)
	if err != nil {
		return err
	}
	if seasonCount != distinctStatPlayers {
		return fmt.Errorf("season stats do not contain exactly one row per player with weekly stats")
	}
	for _, table := range []string{"player_adp", "player_rankings", "player_tiers"} {
		var count int
		if err := db.QueryRow(`
			SELECT COUNT(*) FROM ` + table + ` WHERE season = 2026 AND source = 'fantasypros'
		`).Scan(&count); err != nil {
			return fmt.Errorf("count FantasyPros rows in %s: %w", table, err)
		}
		if count < minimumRealFantasyProsRows {
			return fmt.Errorf(
				"FantasyPros row count in %s is %d; expected at least %d",
				table,
				count,
				minimumRealFantasyProsRows,
			)
		}
	}
	var invalidADP int
	if err := db.QueryRow(`
		SELECT COUNT(*)
		FROM player_adp
		WHERE season != 2026
			OR source != 'fantasypros'
			OR adp <= 0
			OR updated_at = ''
	`).Scan(&invalidADP); err != nil {
		return fmt.Errorf("validate ADP rows: %w", err)
	}
	if invalidADP != 0 {
		return fmt.Errorf("found %d ADP rows outside the M6.3 contract", invalidADP)
	}
	var invalidRankings int
	if err := db.QueryRow(`
		SELECT COUNT(*)
		FROM player_rankings
		WHERE season != 2026
			OR source != 'fantasypros'
			OR overall_rank <= 0
			OR position_rank <= 0
			OR rank_min <= 0
			OR rank_max < rank_min
			OR rank_std_dev < 0
			OR updated_at = ''
	`).Scan(&invalidRankings); err != nil {
		return fmt.Errorf("validate ECR rows: %w", err)
	}
	if invalidRankings != 0 {
		return fmt.Errorf("found %d ECR rows outside the M6.3 contract", invalidRankings)
	}
	var invalidTiers int
	if err := db.QueryRow(`
		SELECT COUNT(*)
		FROM player_tiers
		WHERE season != 2026 OR source != 'fantasypros' OR tier <= 0 OR updated_at = ''
	`).Scan(&invalidTiers); err != nil {
		return fmt.Errorf("validate FantasyPros tiers: %w", err)
	}
	if invalidTiers != 0 {
		return fmt.Errorf("found %d tier rows outside the M6.3 contract", invalidTiers)
	}
	for _, table := range []string{
		"odds",
		"drafts",
		"draft_picks",
	} {
		count, err := tableCount(db, table)
		if err != nil {
			return err
		}
		if count != 0 {
			return fmt.Errorf("%s contains %d rows; M6.3 rebuild expects none", table, count)
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

func countDistinctStatPlayers(db *sql.DB) (int, error) {
	var count int
	if err := db.QueryRow(`
		SELECT COUNT(DISTINCT player_id)
		FROM player_week_stats
		WHERE season = 2025
	`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count players with weekly stats: %w", err)
	}
	return count, nil
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
