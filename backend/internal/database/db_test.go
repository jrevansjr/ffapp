package database

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/jrevansjr/ffapp/backend/migrations"
	"github.com/pressly/goose/v3"
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

// TestOpenUpgradesExistingPlayerRows proves the additive player-profile
// migration preserves databases that were already created during M1.
func TestOpenUpgradesExistingPlayerRows(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "draft.db")
	db, err := sql.Open("sqlite",
		"file:"+dbPath+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open version-one database: %v", err)
	}
	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatalf("set migration dialect: %v", err)
	}
	if err := goose.UpTo(db, ".", 1); err != nil {
		t.Fatalf("apply initial migration: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO nfl_teams (id, abbreviation, name) VALUES (1, 'TST', 'Test Team');
		INSERT INTO players (
			id, sleeper_player_id, first_name, last_name, position, nfl_team_id, birth_date
		) VALUES (1, 'existing-player', 'Existing', 'Player', 'WR', 1, '2000-01-02');
	`); err != nil {
		t.Fatalf("insert pre-upgrade player: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close version-one database: %v", err)
	}

	upgraded, err := Open(dbPath)
	if err != nil {
		t.Fatalf("upgrade database with Open(): %v", err)
	}
	defer upgraded.Close()

	var (
		name        string
		yearsExp    sql.NullInt64
		sportradar  sql.NullString
		gsis        sql.NullString
		fantasyPros sql.NullString
	)
	if err := upgraded.QueryRow(`
		SELECT first_name, years_exp, sportradar_id, gsis_id, fantasypros_id
		FROM players
		WHERE sleeper_player_id = ?
	`, "existing-player").Scan(&name, &yearsExp, &sportradar, &gsis, &fantasyPros); err != nil {
		t.Fatalf("load upgraded player: %v", err)
	}
	if name != "Existing" {
		t.Fatalf("preserved first_name = %q, want Existing", name)
	}
	if yearsExp.Valid || sportradar.Valid || gsis.Valid || fantasyPros.Valid {
		t.Fatalf("new optional fields = %v, %v, %v, %v; want NULL", yearsExp, sportradar, gsis, fantasyPros)
	}
}

func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
		t.Fatalf("count %s rows: %v", table, err)
	}
	return count
}
