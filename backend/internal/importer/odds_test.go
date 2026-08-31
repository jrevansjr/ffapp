package importer

import (
	"context"
	"testing"
	"time"

	"github.com/jrevansjr/ffapp/backend/internal/odds"
)

func TestLoadOddsMatchesExactlyAndReplacesAtomically(t *testing.T) {
	db := statsTestDatabase(t)
	if _, err := LoadTeams(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	snapshot := oddsSnapshotFixture()
	summary, err := LoadOdds(context.Background(), db, snapshot)
	if err != nil {
		t.Fatalf("LoadOdds() error = %v", err)
	}
	if summary.MatchedRows != 4 || summary.UnmatchedRows != 1 ||
		summary.PositionMismatchRows != 1 || summary.SkippedNoConsensus != 1 {
		t.Fatalf("summary = %#v", summary)
	}
	if count := rowCount(t, db, "odds"); count != 4 {
		t.Fatalf("odds rows = %d, want 4", count)
	}

	snapshot.PlayerLines[0].Line = 4100.5
	if _, err := LoadOdds(context.Background(), db, snapshot); err != nil {
		t.Fatalf("second LoadOdds() error = %v", err)
	}
	var line float64
	if err := db.QueryRow(`
		SELECT odds.line
		FROM odds
		JOIN players ON players.id = odds.player_id
		WHERE players.sleeper_player_id = 'qb-1' AND odds.market = 'passing_yards'
	`).Scan(&line); err != nil {
		t.Fatal(err)
	}
	if line != 4100.5 || rowCount(t, db, "odds") != 4 {
		t.Fatalf("passing line = %.1f; odds rows = %d", line, rowCount(t, db, "odds"))
	}
}

func TestLoadOddsRejectsLowCoverageWithoutChangingData(t *testing.T) {
	db := statsTestDatabase(t)
	if _, err := db.Exec(`
		INSERT INTO odds (season, source, market, player_id, line, captured_at)
		SELECT 2026, 'sportsbook_consensus', 'passing_yards', id, 999,
			'2026-08-01T00:00:00Z'
		FROM players WHERE sleeper_player_id = 'qb-1'
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := loadOddsWithThreshold(
		context.Background(),
		db,
		oddsSnapshotFixture(),
		5,
	); err == nil {
		t.Fatal("loadOddsWithThreshold() error = nil, want coverage failure")
	}
	var preserved float64
	if err := db.QueryRow(`SELECT line FROM odds WHERE source = 'sportsbook_consensus'`).Scan(&preserved); err != nil {
		t.Fatal(err)
	}
	if preserved != 999 {
		t.Fatalf("preserved line = %.1f, want 999", preserved)
	}
}

func oddsSnapshotFixture() odds.Snapshot {
	return odds.Snapshot{
		Season:          2026,
		Source:          "sportsbook_consensus",
		CapturedAt:      time.Date(2026, 8, 31, 1, 23, 28, 0, time.UTC),
		Description:     "test snapshot",
		SourceRows:      7,
		NoConsensusRows: 1,
		PlayerLines: []odds.PlayerLine{
			{Name: "Test QB", Team: "ARI", Market: odds.MarketPassingYards, Line: 4000.5},
			{Name: "Test QB", Team: "ARI", Market: odds.MarketRushingYards, Line: 350.5},
			{Name: "Test RB", Team: "BUF", Market: odds.MarketRushingTouchdowns, Line: 8.5},
			{Name: "No Match", Team: "ARI", Market: odds.MarketPassingYards, Line: 3000.5},
			{Name: "Test QB", Team: "ARI", Market: odds.MarketReceivingYards, Line: 100.5},
		},
		TeamLines: []odds.TeamLine{
			{Team: "Arizona Cardinals", Market: odds.MarketRegularSeasonWins, Line: 7.5},
		},
	}
}
