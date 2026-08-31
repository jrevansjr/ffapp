package importer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jrevansjr/ffapp/backend/internal/database"
)

const minimumRealPlayerCount = 100

// RebuildResult identifies the recoverable backup and installed database.
type RebuildResult struct {
	BackupPath string
	DBPath     string
}

type validationMinimums struct {
	Players     int
	WeeklyStats int
	SeasonStats int
	FantasyPros int
	Projections int
	Odds        int
	Morality    int
}

// Rebuild constructs and validates the complete real-data database before
// replacing the configured database. The caller must require explicit user
// confirmation.
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
	if err := validateRealDataDatabase(temporaryPath, runner.validationMinimums()); err != nil {
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

func (runner *Runner) validationMinimums() validationMinimums {
	return validationMinimums{
		Players:     runner.MinimumPlayerCount,
		WeeklyStats: runner.MinimumWeeklyStats,
		SeasonStats: runner.MinimumSeasonStats,
		FantasyPros: runner.MinimumFantasyProsRows,
		Projections: runner.MinimumProjectionRows,
		Odds:        runner.MinimumOddsRows,
		Morality:    runner.MinimumMoralityRows,
	}
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

func validateRealDataDatabase(dbPath string, minimums validationMinimums) error {
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
	if playerCount < minimums.Players {
		return fmt.Errorf(
			"active fantasy player count is %d; expected at least %d",
			playerCount,
			minimums.Players,
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
	if err := validateUniqueProviderIDs(db); err != nil {
		return err
	}
	weeklyCount, err := tableCount(db, "player_week_stats")
	if err != nil {
		return err
	}
	if weeklyCount < minimums.WeeklyStats {
		return fmt.Errorf("weekly stat count is %d; expected at least %d", weeklyCount, minimums.WeeklyStats)
	}
	seasonCount, err := tableCount(db, "player_season_stats")
	if err != nil {
		return err
	}
	if seasonCount < minimums.SeasonStats {
		return fmt.Errorf("season stat count is %d; expected at least %d", seasonCount, minimums.SeasonStats)
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
		if count < minimums.FantasyPros {
			return fmt.Errorf(
				"FantasyPros row count in %s is %d; expected at least %d",
				table,
				count,
				minimums.FantasyPros,
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
	var projectionCount int
	if err := db.QueryRow(`
		SELECT COUNT(*)
		FROM player_projections
		WHERE season = 2026 AND source = 'fantasypros'
	`).Scan(&projectionCount); err != nil {
		return fmt.Errorf("count FantasyPros projection rows: %w", err)
	}
	if projectionCount < minimums.Projections {
		return fmt.Errorf(
			"FantasyPros projection row count is %d; expected at least %d",
			projectionCount,
			minimums.Projections,
		)
	}
	var invalidProjections int
	if err := db.QueryRow(`
		SELECT COUNT(*)
		FROM player_projections AS projections
		JOIN players ON players.id = projections.player_id
		WHERE projections.season != 2026
			OR projections.source != 'fantasypros'
			OR projections.updated_at = ''
			OR COALESCE(projections.passing_yards, 0) < 0
			OR COALESCE(projections.passing_touchdowns, 0) < 0
			OR COALESCE(projections.rushing_yards, 0) < 0
			OR COALESCE(projections.rushing_touchdowns, 0) < 0
			OR COALESCE(projections.receiving_yards, 0) < 0
			OR COALESCE(projections.receiving_touchdowns, 0) < 0
			OR (players.position = 'QB' AND (
				projections.passing_yards IS NULL OR projections.passing_touchdowns IS NULL
				OR projections.rushing_yards IS NULL OR projections.rushing_touchdowns IS NULL
				OR projections.receiving_yards IS NOT NULL OR projections.receiving_touchdowns IS NOT NULL
			))
			OR (players.position = 'RB' AND (
				projections.rushing_yards IS NULL OR projections.rushing_touchdowns IS NULL
				OR projections.receiving_yards IS NULL OR projections.receiving_touchdowns IS NULL
				OR projections.passing_yards IS NOT NULL OR projections.passing_touchdowns IS NOT NULL
			))
			OR (players.position IN ('WR', 'TE') AND (
				projections.receiving_yards IS NULL OR projections.receiving_touchdowns IS NULL
				OR projections.passing_yards IS NOT NULL OR projections.passing_touchdowns IS NOT NULL
				OR projections.rushing_yards IS NOT NULL OR projections.rushing_touchdowns IS NOT NULL
			))
	`).Scan(&invalidProjections); err != nil {
		return fmt.Errorf("validate FantasyPros projections: %w", err)
	}
	if invalidProjections != 0 {
		return fmt.Errorf("found %d projection rows outside the M6.4 contract", invalidProjections)
	}
	var oddsCount int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM odds
		WHERE season = 2026 AND source = 'sportsbook_consensus'
	`).Scan(&oddsCount); err != nil {
		return fmt.Errorf("count sportsbook consensus rows: %w", err)
	}
	if oddsCount < minimums.Odds {
		return fmt.Errorf(
			"sportsbook consensus row count is %d; expected at least %d",
			oddsCount,
			minimums.Odds,
		)
	}
	var invalidOdds int
	if err := db.QueryRow(`
		SELECT COUNT(*)
		FROM odds
		LEFT JOIN players ON players.id = odds.player_id
		WHERE odds.season != 2026
			OR odds.source != 'sportsbook_consensus'
			OR odds.line <= 0
			OR odds.over_price IS NOT NULL
			OR odds.under_price IS NOT NULL
			OR odds.captured_at = ''
			OR (odds.player_id IS NOT NULL AND (
				(odds.market IN ('passing_yards', 'passing_touchdowns') AND players.position != 'QB')
				OR (odds.market IN ('rushing_yards', 'rushing_touchdowns') AND players.position NOT IN ('QB', 'RB'))
				OR (odds.market IN ('receiving_yards', 'receiving_touchdowns') AND players.position NOT IN ('RB', 'WR', 'TE'))
				OR odds.market NOT IN (
					'passing_yards', 'passing_touchdowns',
					'rushing_yards', 'rushing_touchdowns',
					'receiving_yards', 'receiving_touchdowns'
				)
			))
			OR (odds.nfl_team_id IS NOT NULL AND odds.market != 'regular_season_wins')
	`).Scan(&invalidOdds); err != nil {
		return fmt.Errorf("validate sportsbook consensus rows: %w", err)
	}
	if invalidOdds != 0 {
		return fmt.Errorf("found %d odds rows outside the M8.1 contract", invalidOdds)
	}
	var moralityCount int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM player_morality_scores
		WHERE source = 'user_supplied'
	`).Scan(&moralityCount); err != nil {
		return fmt.Errorf("count user-supplied morality scores: %w", err)
	}
	if moralityCount != minimums.Morality {
		return fmt.Errorf(
			"morality score count is %d; expected exactly %d",
			moralityCount,
			minimums.Morality,
		)
	}
	var invalidMorality int
	if err := db.QueryRow(`
		SELECT COUNT(*)
		FROM player_morality_scores
		WHERE source != 'user_supplied'
			OR score NOT BETWEEN 0 AND 5
			OR snapshot_date != '2026-08-30'
			OR imported_at = ''
	`).Scan(&invalidMorality); err != nil {
		return fmt.Errorf("validate user-supplied morality scores: %w", err)
	}
	if invalidMorality != 0 {
		return fmt.Errorf("found %d morality rows outside the M8.2 contract", invalidMorality)
	}
	for _, table := range []string{
		"drafts",
		"draft_picks",
	} {
		count, err := tableCount(db, table)
		if err != nil {
			return err
		}
		if count != 0 {
			return fmt.Errorf("%s contains %d rows; real-data rebuild expects none", table, count)
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
	if err := rows.Err(); err != nil {
		return err
	}
	if err := validateIntegrity(db); err != nil {
		return err
	}
	return validateReferenceTimestamps(db)
}

func validateUniqueProviderIDs(db *sql.DB) error {
	for _, column := range []string{"gsis_id", "fantasypros_id"} {
		var duplicates int
		if err := db.QueryRow(`
			SELECT COUNT(*) FROM (
				SELECT ` + column + `
				FROM players
				WHERE active = 1 AND ` + column + ` IS NOT NULL
				GROUP BY ` + column + `
				HAVING COUNT(*) > 1
			)
		`).Scan(&duplicates); err != nil {
			return fmt.Errorf("validate unique %s values: %w", column, err)
		}
		if duplicates != 0 {
			return fmt.Errorf("found %d duplicated active-player %s values", duplicates, column)
		}
	}
	return nil
}

func validateIntegrity(db *sql.DB) error {
	var result string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&result); err != nil {
		return fmt.Errorf("run SQLite integrity check: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("SQLite integrity check returned %q", result)
	}
	return nil
}

func validateReferenceTimestamps(db *sql.DB) error {
	rows, err := db.Query(`
		SELECT 'app_settings.updated_at', updated_at FROM app_settings
		UNION ALL SELECT 'app_settings.players_synced_at', players_synced_at FROM app_settings
		UNION ALL SELECT 'player_adp.updated_at', updated_at FROM player_adp
		UNION ALL SELECT 'player_rankings.updated_at', updated_at FROM player_rankings
		UNION ALL SELECT 'player_tiers.updated_at', updated_at FROM player_tiers
		UNION ALL SELECT 'player_projections.updated_at', updated_at FROM player_projections
		UNION ALL SELECT 'odds.captured_at', captured_at FROM odds
		UNION ALL SELECT 'player_morality_scores.imported_at', imported_at FROM player_morality_scores
	`)
	if err != nil {
		return fmt.Errorf("load reference timestamps: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var field, value string
		if err := rows.Scan(&field, &value); err != nil {
			return fmt.Errorf("scan reference timestamp: %w", err)
		}
		if parsed, err := time.Parse(time.RFC3339, value); err != nil || parsed.Location() != time.UTC {
			return fmt.Errorf("%s contains non-UTC RFC3339 timestamp %q", field, value)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read reference timestamps: %w", err)
	}
	return nil
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
