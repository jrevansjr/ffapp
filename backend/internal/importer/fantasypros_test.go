package importer

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jrevansjr/ffapp/backend/internal/fantasypros"
	"github.com/jrevansjr/ffapp/backend/internal/nflverse"
)

func TestLoadFantasyProsMatchesExactIDsAndIsIdempotent(t *testing.T) {
	db := statsTestDatabase(t)
	adp, ecr := fantasyProsDatasets()
	crosswalk := nflverse.PlayerIDDataset{Rows: []nflverse.PlayerID{
		{SleeperID: "qb-1", FantasyProsID: "101", Name: "Wrong Name Is Ignored"},
		{SleeperID: "rb-1", FantasyProsID: "102", Name: "Also Ignored"},
	}}

	first, err := LoadFantasyPros(context.Background(), db, adp, ecr, crosswalk)
	if err != nil {
		t.Fatalf("LoadFantasyPros() error = %v", err)
	}
	if first.FantasyProsBackfills != 2 || first.ADP.MatchedRows != 2 || first.ECR.MatchedRows != 2 {
		t.Fatalf("first summary = %#v", first)
	}
	for _, table := range []string{"player_adp", "player_rankings", "player_tiers"} {
		if count := rowCount(t, db, table); count != 2 {
			t.Fatalf("%s rows = %d, want 2", table, count)
		}
	}
	var adpValue float64
	var ecrValue, tier int
	if err := db.QueryRow(`
		SELECT adp.adp, rankings.overall_rank, tiers.tier
		FROM players
		JOIN player_adp AS adp ON adp.player_id = players.id
		JOIN player_rankings AS rankings ON rankings.player_id = players.id
		JOIN player_tiers AS tiers ON tiers.player_id = players.id
		WHERE players.sleeper_player_id = 'qb-1'
	`).Scan(&adpValue, &ecrValue, &tier); err != nil {
		t.Fatal(err)
	}
	if adpValue != 10 || ecrValue != 9 || tier != 2 {
		t.Fatalf("draft values = %.2f, %d, %d", adpValue, ecrValue, tier)
	}

	second, err := LoadFantasyPros(context.Background(), db, adp, ecr, crosswalk)
	if err != nil {
		t.Fatalf("second LoadFantasyPros() error = %v", err)
	}
	if second.FantasyProsBackfills != 0 || rowCount(t, db, "player_rankings") != 2 {
		t.Fatalf("second summary = %#v", second)
	}
}

func TestLoadFantasyProsNeverNameMatchesAndPreservesCommittedData(t *testing.T) {
	db := statsTestDatabase(t)
	if _, err := db.Exec(`
		INSERT INTO player_adp (player_id, season, source, adp, updated_at)
		SELECT id, 2026, 'fantasypros', 99, '2026-08-23T12:00:00Z'
		FROM players WHERE sleeper_player_id = 'qb-1'
	`); err != nil {
		t.Fatal(err)
	}
	adp, ecr := fantasyProsDatasets()
	crosswalk := nflverse.PlayerIDDataset{Rows: []nflverse.PlayerID{
		{SleeperID: "not-local", FantasyProsID: "101", Name: "Test QB"},
	}}
	if _, err := LoadFantasyPros(context.Background(), db, adp, ecr, crosswalk); err == nil {
		t.Fatal("LoadFantasyPros() error = nil, want exact-match failure")
	}
	var preserved float64
	if err := db.QueryRow(`SELECT adp FROM player_adp WHERE source = 'fantasypros'`).Scan(&preserved); err != nil {
		t.Fatal(err)
	}
	if preserved != 99 {
		t.Fatalf("preserved ADP = %.2f, want 99", preserved)
	}
}

func TestLoadFantasyProsRollsBackAllTablesOnWriteFailure(t *testing.T) {
	db := statsTestDatabase(t)
	adp, ecr := fantasyProsDatasets()
	crosswalk := nflverse.PlayerIDDataset{Rows: []nflverse.PlayerID{
		{SleeperID: "qb-1", FantasyProsID: "101"},
		{SleeperID: "rb-1", FantasyProsID: "102"},
	}}
	if _, err := LoadFantasyPros(context.Background(), db, adp, ecr, crosswalk); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE TRIGGER reject_ecr
		BEFORE INSERT ON player_rankings
		BEGIN
			SELECT RAISE(ABORT, 'test rejection');
		END
	`); err != nil {
		t.Fatal(err)
	}
	adp.Rankings[0].ADP = 1
	if _, err := LoadFantasyPros(context.Background(), db, adp, ecr, crosswalk); err == nil {
		t.Fatal("LoadFantasyPros() error = nil, want trigger failure")
	}
	var preserved float64
	if err := db.QueryRow(`
		SELECT adp FROM player_adp AS adp
		JOIN players ON players.id = adp.player_id
		WHERE players.sleeper_player_id = 'qb-1' AND adp.source = 'fantasypros'
	`).Scan(&preserved); err != nil {
		t.Fatal(err)
	}
	if preserved != 10 {
		t.Fatalf("ADP after rollback = %.2f, want 10", preserved)
	}
}

func TestRefreshFantasyProsMakesOneRequestAndCachesOneDataset(t *testing.T) {
	root := t.TempDir()
	runner := NewRunner(filepath.Join(root, "draft.db"), io.Discard)
	runner.CacheDir = filepath.Join(root, "cache")
	runner.MinimumFantasyProsRows = 1
	runner.FantasyProsClient.APIKey = "test-key"
	runner.Now = func() time.Time { return time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC) }
	requests := 0
	runner.FantasyProsClient.HTTPClient = &http.Client{Transport: importerRoundTripFunc(func(request *http.Request) *http.Response {
		requests++
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(fantasyprosADPFixture)),
			Header:     make(http.Header),
		}
	})}

	if err := runner.RefreshFantasyPros(context.Background(), fantasypros.DatasetADP); err != nil {
		t.Fatalf("RefreshFantasyPros() error = %v", err)
	}
	if requests != 1 {
		t.Fatalf("request count = %d, want 1", requests)
	}
	if _, _, err := readFantasyProsCache(runner.CacheDir, "adp"); err != nil {
		t.Fatalf("read ADP cache: %v", err)
	}
	if _, _, err := readFantasyProsCache(runner.CacheDir, "ecr"); err == nil {
		t.Fatal("ECR cache unexpectedly exists")
	}
}

func TestLoadFantasyProsUsesCachesWithoutProviderRequest(t *testing.T) {
	db := statsTestDatabase(t)
	root := t.TempDir()
	runner := NewRunner(filepath.Join(root, "draft.db"), io.Discard)
	runner.CacheDir = filepath.Join(root, "cache")
	runner.MinimumFantasyProsRows = 1
	fetchedAt := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	if err := writeFantasyProsCache(runner.CacheDir, "adp", []byte(fantasyprosADPFixture), fetchedAt); err != nil {
		t.Fatal(err)
	}
	if err := writeFantasyProsCache(runner.CacheDir, "ecr", []byte(fantasyprosECRFixture), fetchedAt); err != nil {
		t.Fatal(err)
	}
	crosswalk := []byte("sleeper_id,gsis_id,fantasypros_id,name\nqb-1,gsis-qb-1,101,Test QB\nrb-1,gsis-rb-1,102,Test RB\n")
	if err := writeStatsCache(runner.CacheDir, []byte("unused"), crosswalk, fetchedAt); err != nil {
		t.Fatal(err)
	}
	requests := 0
	runner.FantasyProsClient.HTTPClient = &http.Client{Transport: importerRoundTripFunc(func(*http.Request) *http.Response {
		requests++
		return nil
	})}
	if err := runner.loadFantasyPros(context.Background(), db); err != nil {
		t.Fatalf("loadFantasyPros() error = %v", err)
	}
	if requests != 0 {
		t.Fatalf("provider request count = %d, want 0", requests)
	}
	for _, table := range []string{"player_adp", "player_rankings", "player_tiers"} {
		if count := rowCount(t, db, table); count != 2 {
			t.Fatalf("%s rows = %d, want 2", table, count)
		}
	}
}

const fantasyprosADPFixture = `{
  "year": 2026,
  "count": 2,
  "last_updated_ts": 1787500000,
  "position_id": "ALL",
  "ranking_type_name": "adp",
  "scoring": "HALF",
  "players": [
    {"player_id": "101", "player_name": "Provider QB", "player_position_id": "QB", "player_team_id": "ARI", "rank_ave": 10},
    {"player_id": "102", "player_name": "Provider RB", "player_position_id": "RB", "player_team_id": "BUF", "rank_ave": 20}
  ]
}`

const fantasyprosECRFixture = `{
  "year": 2026,
  "count": 2,
  "last_updated_ts": 1787500000,
  "position_id": "ALL",
  "ranking_type_name": "draft",
  "scoring": "HALF",
  "players": [
    {"player_id": "101", "player_name": "Provider QB", "player_position_id": "QB", "player_team_id": "ARI", "rank_ecr": 9, "pos_rank": "QB1", "tier": 2, "rank_min": 7, "rank_max": 12, "rank_std": 1.5},
    {"player_id": "102", "player_name": "Provider RB", "player_position_id": "RB", "player_team_id": "BUF", "rank_ecr": 19, "pos_rank": "RB9", "tier": 3, "rank_min": 14, "rank_max": 25, "rank_std": 3.5}
  ]
}`

func fantasyProsDatasets() (fantasypros.ADPDataset, fantasypros.ECRDataset) {
	updatedAt := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	return fantasypros.ADPDataset{
			UpdatedAt: updatedAt,
			Rankings: []fantasypros.ADPRanking{
				{FantasyProsID: "101", Name: "Provider QB", Position: "QB", ADP: 10},
				{FantasyProsID: "102", Name: "Provider RB", Position: "RB", ADP: 20},
			},
		}, fantasypros.ECRDataset{
			UpdatedAt: updatedAt,
			Rankings: []fantasypros.ExpertRanking{
				{FantasyProsID: "101", Name: "Provider QB", Position: "QB", OverallRank: 9, PositionRank: 1, Tier: 2, RankMin: 7, RankMax: 12, RankStdDev: 1.5},
				{FantasyProsID: "102", Name: "Provider RB", Position: "RB", OverallRank: 19, PositionRank: 9, Tier: 3, RankMin: 14, RankMax: 25, RankStdDev: 3.5},
			},
		}
}
