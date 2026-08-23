package importer

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jrevansjr/ffapp/backend/internal/database"
	"github.com/jrevansjr/ffapp/backend/internal/sleeper"
)

func TestLoadTeamsAndPlayersIsIdempotentAndReconciles(t *testing.T) {
	db := openTestDatabase(t)
	ctx := context.Background()
	if count, err := LoadTeams(ctx, db); err != nil || count != 32 {
		t.Fatalf("LoadTeams() = %d, %v; want 32, nil", count, err)
	}
	importedAt := time.Date(2026, time.August, 23, 12, 0, 0, 0, time.UTC)
	players := fixturePlayers()
	summary, err := LoadPlayers(ctx, db, players, importedAt)
	if err != nil {
		t.Fatalf("LoadPlayers() error = %v", err)
	}
	if summary.Fetched != 7 || summary.Eligible != 4 || summary.Inserted != 4 ||
		summary.Skipped != 1 || summary.UnmappedTeams != 1 {
		t.Fatalf("first summary = %#v", summary)
	}

	delete(players, "qb-1")
	wr := players["wr-1"]
	wr.College = stringValue("Updated University")
	players["wr-1"] = wr
	second, err := LoadPlayers(ctx, db, players, importedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("second LoadPlayers() error = %v", err)
	}
	if second.Inserted != 0 || second.Updated != 3 || second.Deactivated != 1 {
		t.Fatalf("second summary = %#v", second)
	}

	var active int
	if err := db.QueryRow(`SELECT active FROM players WHERE sleeper_player_id = 'qb-1'`).Scan(&active); err != nil {
		t.Fatalf("load deactivated player: %v", err)
	}
	if active != 0 {
		t.Fatalf("removed player active = %d, want 0", active)
	}
	var college, gsisID, rotowireID string
	if err := db.QueryRow(`
		SELECT college, gsis_id, rotowire_id
		FROM players WHERE sleeper_player_id = 'wr-1'
	`).Scan(&college, &gsisID, &rotowireID); err != nil {
		t.Fatalf("load updated player profile: %v", err)
	}
	if college != "Updated University" || gsisID != "gsis-wr-1" || rotowireID != "2001" {
		t.Fatalf("updated identity = %q, %q, %q", college, gsisID, rotowireID)
	}
	var syncedAt string
	if err := db.QueryRow(`SELECT players_synced_at FROM app_settings WHERE id = 1`).Scan(&syncedAt); err != nil {
		t.Fatalf("load sync timestamp: %v", err)
	}
	if syncedAt != importedAt.Add(time.Hour).Format(time.RFC3339) {
		t.Fatalf("players_synced_at = %q", syncedAt)
	}
}

func TestLoadPlayersRollsBackInvalidReplacement(t *testing.T) {
	db := openTestDatabase(t)
	ctx := context.Background()
	if _, err := LoadTeams(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPlayers(ctx, db, fixturePlayers(), time.Now()); err != nil {
		t.Fatal(err)
	}
	before := rowCount(t, db, "players")
	if _, err := LoadPlayers(ctx, db, sleeper.PlayersResponse{}, time.Now()); err == nil {
		t.Fatal("empty LoadPlayers() error = nil, want failure")
	}
	if after := rowCount(t, db, "players"); after != before {
		t.Fatalf("player rows after rejected load = %d, want %d", after, before)
	}
}

func TestRunnerCachesPlayerResponse(t *testing.T) {
	responseBody, err := json.Marshal(fixturePlayers())
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		_, _ = w.Write(responseBody)
	}))
	defer server.Close()

	root := t.TempDir()
	runner := NewRunner(filepath.Join(root, "draft.db"), &bytes.Buffer{})
	runner.CacheDir = filepath.Join(root, "cache")
	runner.MinimumPlayerCount = 1
	runner.Now = func() time.Time {
		return time.Date(2026, time.August, 23, 12, 0, 0, 0, time.UTC)
	}
	runner.SleeperClient.BaseURL = server.URL
	runner.SleeperClient.HTTPClient = server.Client()
	if err := runner.Build(context.Background()); err != nil {
		t.Fatalf("first Build() error = %v", err)
	}
	if err := runner.Build(context.Background()); err != nil {
		t.Fatalf("second Build() error = %v", err)
	}
	if requests != 1 {
		t.Fatalf("Sleeper request count = %d, want 1", requests)
	}
	db, err := database.Open(runner.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if count := rowCount(t, db, "players"); count != 4 {
		t.Fatalf("player count = %d, want 4", count)
	}
}

func TestRebuildPreservesExistingDatabaseWhenValidationFails(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "draft.db")
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO nfl_teams (abbreviation, name) VALUES ('TST', 'Marker')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(sleeper.PlayersResponse{"only-one": fixturePlayer("QB", "ARI")})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer server.Close()
	runner := NewRunner(dbPath, &bytes.Buffer{})
	runner.CacheDir = filepath.Join(root, "cache")
	runner.MinimumPlayerCount = 1
	runner.Now = func() time.Time { return time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC) }
	runner.SleeperClient.BaseURL = server.URL
	runner.SleeperClient.HTTPClient = server.Client()
	if _, err := runner.Rebuild(context.Background()); err == nil {
		t.Fatal("Rebuild() error = nil, want validation failure")
	}

	preserved, err := database.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer preserved.Close()
	var marker string
	if err := preserved.QueryRow(`SELECT name FROM nfl_teams WHERE abbreviation = 'TST'`).Scan(&marker); err != nil {
		t.Fatalf("existing database was not preserved: %v", err)
	}
	backups, err := filepath.Glob(filepath.Join(root, "backups", "*.db"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("backup files = %v, %v; want one", backups, err)
	}
}

func openTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "draft.db"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func fixturePlayers() sleeper.PlayersResponse {
	players := sleeper.PlayersResponse{
		"qb-1":  fixturePlayer("QB", "ARI"),
		"rb-1":  fixturePlayer("RB", "BUF"),
		"wr-1":  fixturePlayer("WR", "UNKNOWN"),
		"te-1":  fixturePlayer("TE", ""),
		"def-1": fixturePlayer("DEF", "ARI"),
		"bad-1": fixturePlayer("WR", "ARI"),
		"old-1": fixturePlayer("RB", "ARI"),
	}
	bad := players["bad-1"]
	bad.FirstName = sleeper.StringValue{}
	bad.LastName = sleeper.StringValue{}
	bad.FullName = sleeper.StringValue{}
	players["bad-1"] = bad
	old := players["old-1"]
	old.Active = false
	players["old-1"] = old
	return players
}

func fixturePlayer(position, team string) sleeper.Player {
	return sleeper.Player{
		FirstName:  stringValue("Test"),
		LastName:   stringValue(position),
		FullName:   stringValue("Test " + position),
		Position:   stringValue(position),
		Team:       stringValue(team),
		BirthDate:  stringValue("2000-01-01"),
		Active:     true,
		Status:     stringValue("Active"),
		Number:     sleeper.IntValue{Value: 10, Valid: true},
		College:    stringValue("Test University"),
		YearsExp:   sleeper.IntValue{Value: 2, Valid: true},
		RotowireID: stringValue("2001"),
		GSISID:     stringValue("gsis-" + strings.ToLower(position) + "-1"),
	}
}

func stringValue(value string) sleeper.StringValue {
	return sleeper.StringValue{Value: value, Valid: value != ""}
}

func rowCount(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var count int
	if err := db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}
