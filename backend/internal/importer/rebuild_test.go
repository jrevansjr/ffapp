package importer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jrevansjr/ffapp/backend/internal/database"
)

// TestRebuildUsesEveryApprovedCache exercises the complete M6 pipeline without
// allowing a provider request. Smaller thresholds keep the fixture readable;
// production runners retain the real completeness minimums.
func TestRebuildUsesEveryApprovedCache(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "draft.db")
	cacheDir := filepath.Join(root, "import-cache")
	fetchedAt := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

	existing, err := database.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := existing.Exec(`INSERT INTO nfl_teams (abbreviation, name) VALUES ('TST', 'Backup marker')`); err != nil {
		t.Fatal(err)
	}
	if err := existing.Close(); err != nil {
		t.Fatal(err)
	}

	playerBody, err := json.Marshal(fixturePlayers())
	if err != nil {
		t.Fatal(err)
	}
	if err := writePlayerCache(cacheDir, playerBody, fetchedAt); err != nil {
		t.Fatal(err)
	}
	crosswalk := []byte("sleeper_id,gsis_id,fantasypros_id,name\nqb-1,gsis-qb-1,101,Test QB\nrb-1,gsis-rb-1,102,Test RB\n")
	if err := writeStatsCache(cacheDir, weeklyCSVFixture(), crosswalk, fetchedAt); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string][]byte{
		"adp":         []byte(fantasyprosADPFixture),
		"ecr":         []byte(fantasyprosECRFixture),
		"projections": []byte(projectionJSONFixture),
	} {
		if err := writeFantasyProsCache(cacheDir, name, body, fetchedAt); err != nil {
			t.Fatal(err)
		}
	}

	runner := NewRunner(dbPath, io.Discard)
	runner.CacheDir = cacheDir
	runner.MinimumPlayerCount = 1
	runner.MinimumWeeklyStats = 1
	runner.MinimumSeasonStats = 1
	runner.MinimumFantasyProsRows = 1
	runner.MinimumProjectionRows = 1
	runner.MinimumOddsRows = 1
	oddsSnapshot := oddsSnapshotFixture()
	runner.OddsSnapshot = &oddsSnapshot
	runner.Now = func() time.Time { return fetchedAt }
	blockedClient := &http.Client{Transport: noProviderRequests{t: t}}
	runner.SleeperClient.HTTPClient = blockedClient
	runner.NFLVerseClient.HTTPClient = blockedClient
	runner.FantasyProsClient.HTTPClient = blockedClient

	result, err := runner.Rebuild(context.Background())
	if err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	if result.BackupPath == "" {
		t.Fatal("Rebuild() backup path is empty")
	}
	if _, err := os.Stat(result.BackupPath); err != nil {
		t.Fatalf("stat backup: %v", err)
	}

	rebuilt, err := database.Open(result.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer rebuilt.Close()
	for table, want := range map[string]int{
		"nfl_teams": 32, "players": 4, "player_week_stats": 18,
		"player_season_stats": 1, "player_adp": 2, "player_rankings": 2,
		"player_tiers": 2, "player_projections": 2,
		"odds": 4,
	} {
		var got int
		if err := rebuilt.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&got); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if got != want {
			t.Fatalf("%s rows = %d, want %d", table, got, want)
		}
	}
	var markerCount int
	if err := rebuilt.QueryRow(`SELECT COUNT(*) FROM nfl_teams WHERE abbreviation = 'TST'`).Scan(&markerCount); err != nil {
		t.Fatal(err)
	}
	if markerCount != 0 {
		t.Fatal("rebuild retained the old database marker")
	}

	if _, err := rebuilt.Exec(`
		UPDATE players SET gsis_id = 'duplicate-test-id'
		WHERE sleeper_player_id IN ('qb-1', 'rb-1')
	`); err == nil {
		t.Fatal("database accepted duplicate provider IDs")
	}
	if _, err := rebuilt.Exec(`
		UPDATE player_projections SET updated_at = '2026-08-30 12:00:00';
	`); err != nil {
		t.Fatal(err)
	}
	if err := validateRealDataDatabase(result.DBPath, runner.validationMinimums()); err == nil {
		t.Fatal("non-RFC3339 projection timestamps passed the real-data audit")
	}
}

type noProviderRequests struct{ t *testing.T }

func (transport noProviderRequests) RoundTrip(*http.Request) (*http.Response, error) {
	transport.t.Helper()
	transport.t.Fatal("offline rebuild attempted a provider request")
	return nil, fmt.Errorf("provider requests are disabled in this test")
}
