package importer

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/jrevansjr/ffapp/backend/internal/fantasypros"
)

func TestLoadProjectionsMatchesExactIDsAndReplacesAtomically(t *testing.T) {
	db := statsTestDatabase(t)
	if _, err := db.Exec(`UPDATE players SET fantasypros_id = CASE sleeper_player_id WHEN 'qb-1' THEN '101' WHEN 'rb-1' THEN '102' END`); err != nil {
		t.Fatal(err)
	}
	dataset := projectionDatasetFixture()
	summary, err := LoadProjections(context.Background(), db, dataset)
	if err != nil {
		t.Fatalf("LoadProjections() error = %v", err)
	}
	if summary.MatchedRows != 2 || summary.UnmatchedRows != 1 || rowCount(t, db, "player_projections") != 2 {
		t.Fatalf("summary = %#v", summary)
	}
	dataset.Projections[0].PassingYards = floatPointer(4100)
	if _, err := LoadProjections(context.Background(), db, dataset); err != nil {
		t.Fatalf("second LoadProjections() error = %v", err)
	}
	var passingYards float64
	if err := db.QueryRow(`
		SELECT projections.passing_yards
		FROM player_projections AS projections
		JOIN players ON players.id = projections.player_id
		WHERE players.sleeper_player_id = 'qb-1'
	`).Scan(&passingYards); err != nil {
		t.Fatal(err)
	}
	if passingYards != 4100 || rowCount(t, db, "player_projections") != 2 {
		t.Fatalf("passing yards = %.1f; rows = %d", passingYards, rowCount(t, db, "player_projections"))
	}
}

func TestLoadProjectionsRejectsLowCoverageWithoutChangingData(t *testing.T) {
	db := statsTestDatabase(t)
	if _, err := db.Exec(`UPDATE players SET fantasypros_id = '101' WHERE sleeper_player_id = 'qb-1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO player_projections (player_id, season, source, passing_yards, updated_at)
		SELECT id, 2026, 'fantasypros', 999, '2026-08-01T00:00:00Z'
		FROM players WHERE sleeper_player_id = 'qb-1'
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := loadProjectionsWithThreshold(context.Background(), db, projectionDatasetFixture(), 2); err == nil {
		t.Fatal("loadProjectionsWithThreshold() error = nil, want coverage failure")
	}
	var preserved float64
	if err := db.QueryRow(`
		SELECT projections.passing_yards
		FROM player_projections AS projections
		JOIN players ON players.id = projections.player_id
		WHERE players.sleeper_player_id = 'qb-1'
	`).Scan(&preserved); err != nil {
		t.Fatal(err)
	}
	if preserved != 999 {
		t.Fatalf("preserved passing yards = %.1f, want 999", preserved)
	}
}

func TestLoadProjectionsUsesCacheWithoutProviderRequest(t *testing.T) {
	db := statsTestDatabase(t)
	if _, err := db.Exec(`UPDATE players SET fantasypros_id = CASE sleeper_player_id WHEN 'qb-1' THEN '101' WHEN 'rb-1' THEN '102' END`); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	runner := NewRunner(filepath.Join(root, "draft.db"), io.Discard)
	runner.CacheDir = filepath.Join(root, "cache")
	runner.MinimumProjectionRows = 1
	if err := writeFantasyProsCache(runner.CacheDir, "projections", []byte(projectionJSONFixture), time.Date(2026, 8, 30, 0, 49, 31, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	requests := 0
	runner.FantasyProsClient.HTTPClient = &http.Client{Transport: importerRoundTripFunc(func(*http.Request) *http.Response {
		requests++
		return nil
	})}
	if err := runner.loadProjections(context.Background(), db); err != nil {
		t.Fatalf("loadProjections() error = %v", err)
	}
	if requests != 0 {
		t.Fatalf("provider requests = %d, want 0", requests)
	}
}

func projectionDatasetFixture() fantasypros.ProjectionDataset {
	return fantasypros.ProjectionDataset{
		UpdatedAt: time.Date(2026, 8, 30, 0, 49, 31, 0, time.UTC),
		Projections: []fantasypros.PlayerProjection{
			{FantasyProsID: "101", Name: "Provider QB", Position: "QB", PassingYards: floatPointer(4000), PassingTouchdowns: floatPointer(30), RushingYards: floatPointer(300), RushingTouchdowns: floatPointer(4)},
			{FantasyProsID: "102", Name: "Provider RB", Position: "RB", RushingYards: floatPointer(1000), RushingTouchdowns: floatPointer(8), ReceivingYards: floatPointer(400), ReceivingTouchdowns: floatPointer(2)},
			{FantasyProsID: "999", Name: "No Exact Match", Position: "WR", ReceivingYards: floatPointer(900), ReceivingTouchdowns: floatPointer(6)},
		},
	}
}

func floatPointer(value float64) *float64 { return &value }

const projectionJSONFixture = `{"season":"2026","week":"0","count":"3","positions":"QB,RB,WR,TE","players":[{"fpid":101,"name":"Provider QB","position_id":"QB","stats":{"pass_yds":4000,"pass_tds":30,"rush_yds":300,"rush_tds":4}},{"fpid":102,"name":"Provider RB","position_id":"RB","stats":{"rush_yds":1000,"rush_tds":8,"rec_yds":400,"rec_tds":2}},{"fpid":999,"name":"No Exact Match","position_id":"WR","stats":{"rec_yds":900,"rec_tds":6}}]}`
