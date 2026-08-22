package database

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

// TestOpenCreatesSchemaAndPreservesData exercises the real migration path with
// a temporary SQLite file, including persistence across separate handles.
func TestOpenCreatesSchemaAndPreservesData(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "nested", "draft.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	var (
		username       string
		leagueID       string
		draftID        string
		pollingEnabled int
		interval       int
		updatedAt      string
	)
	err = db.QueryRow(`
		SELECT
			sleeper_username,
			sleeper_league_id,
			sleeper_draft_id,
			polling_enabled,
			polling_interval_ms,
			updated_at
		FROM app_settings
		WHERE id = 1
	`).Scan(&username, &leagueID, &draftID, &pollingEnabled, &interval, &updatedAt)
	if err != nil {
		t.Fatalf("load default settings: %v", err)
	}
	if username != "" || leagueID != "" || draftID != "" {
		t.Fatalf("Sleeper settings = %q, %q, %q; want empty defaults", username, leagueID, draftID)
	}
	if pollingEnabled != 1 {
		t.Fatalf("polling_enabled = %d, want 1", pollingEnabled)
	}
	if interval != 2000 {
		t.Fatalf("polling_interval_ms = %d, want 2000", interval)
	}
	if _, err := time.Parse(time.RFC3339, updatedAt); err != nil {
		t.Fatalf("updated_at = %q, want RFC3339 UTC timestamp: %v", updatedAt, err)
	}

	if _, err := db.Exec(
		"INSERT INTO nfl_teams (abbreviation, name) VALUES (?, ?)",
		"TST",
		"Persistence Test Team",
	); err != nil {
		t.Fatalf("insert persistence marker: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close first database connection: %v", err)
	}

	reopened, err := Open(dbPath)
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	defer reopened.Close()

	var teamName string
	if err := reopened.QueryRow(
		"SELECT name FROM nfl_teams WHERE abbreviation = ?", "TST",
	).Scan(&teamName); err != nil {
		t.Fatalf("load persistence marker after reopen: %v", err)
	}
	if teamName != "Persistence Test Team" {
		t.Fatalf("persisted team name = %q, want Persistence Test Team", teamName)
	}

	if count := countRows(t, reopened, "app_settings"); count != 1 {
		t.Fatalf("app_settings count after reopen = %d, want 1", count)
	}
}

// TestSeedSampleDataIsIdempotent protects stable row counts and relationships
// across repeated seed runs using SQLite rather than storage mocks.
func TestSeedSampleDataIsIdempotent(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "draft.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	if err := SeedSampleData(context.Background(), db); err != nil {
		t.Fatalf("first SeedSampleData() error = %v", err)
	}
	firstCounts := sampleTableCounts(t, db)

	if err := SeedSampleData(context.Background(), db); err != nil {
		t.Fatalf("second SeedSampleData() error = %v", err)
	}
	secondCounts := sampleTableCounts(t, db)

	wantCounts := map[string]int{
		"nfl_teams":           32,
		"players":             60,
		"player_season_stats": 60,
		"player_week_stats":   480,
		"player_adp":          180,
		"player_tiers":        60,
		"odds":                92,
		"drafts":              1,
		"draft_picks":         6,
		"app_settings":        1,
	}
	for table, want := range wantCounts {
		if firstCounts[table] != want {
			t.Errorf("%s count after first seed = %d, want %d", table, firstCounts[table], want)
		}
		if secondCounts[table] != firstCounts[table] {
			t.Errorf(
				"%s count after second seed = %d, want stable count %d",
				table,
				secondCounts[table],
				firstCounts[table],
			)
		}
	}

	var joinedWeeklyRows int
	if err := db.QueryRow(`
		SELECT COUNT(*)
		FROM player_week_stats AS weekly
		JOIN players ON players.id = weekly.player_id
		JOIN nfl_teams ON nfl_teams.id = players.nfl_team_id
	`).Scan(&joinedWeeklyRows); err != nil {
		t.Fatalf("count joined weekly rows: %v", err)
	}
	if joinedWeeklyRows != 480 {
		t.Fatalf("joined weekly row count = %d, want 480", joinedWeeklyRows)
	}

	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		t.Fatalf("run foreign key check: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign_key_check returned a violation")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read foreign key check results: %v", err)
	}
}

func sampleTableCounts(t *testing.T, db *sql.DB) map[string]int {
	t.Helper()
	tables := []string{
		"nfl_teams",
		"players",
		"player_season_stats",
		"player_week_stats",
		"player_adp",
		"player_tiers",
		"odds",
		"drafts",
		"draft_picks",
		"app_settings",
	}
	counts := make(map[string]int, len(tables))
	for _, table := range tables {
		counts[table] = countRows(t, db, table)
	}
	return counts
}

func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
		t.Fatalf("count %s rows: %v", table, err)
	}
	return count
}
