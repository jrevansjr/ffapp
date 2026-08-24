package importer

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jrevansjr/ffapp/backend/internal/nflverse"
)

func TestFantasyPointsHalfPPR(t *testing.T) {
	tests := []struct {
		name string
		stat nflverse.WeeklyStat
		want float64
	}{
		{
			name: "quarterback scoring",
			stat: nflverse.WeeklyStat{
				PassingYards: 250, PassingTouchdowns: 2, PassingInterceptions: 1,
				RushingYards: 20, RushingTouchdowns: 1, PassingTwoPointConversions: 1,
			},
			want: 26,
		},
		{
			name: "half PPR skill player",
			stat: nflverse.WeeklyStat{
				RushingAttempts: 10, RushingYards: 50, RushingTouchdowns: 1,
				Targets: 8, Receptions: 6, ReceivingYards: 70,
				ReceivingTouchdowns: 1, ReceivingTwoPointConversions: 1, FumblesLost: 1,
			},
			want: 27,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := FantasyPointsHalfPPR(test.stat); got != test.want {
				t.Fatalf("FantasyPointsHalfPPR() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestLoadStatsMatchesBackfillsAggregatesAndIsIdempotent(t *testing.T) {
	db := statsTestDatabase(t)
	if _, err := db.Exec(`UPDATE players SET gsis_id = NULL WHERE sleeper_player_id = 'rb-1'`); err != nil {
		t.Fatal(err)
	}
	weekly := completeWeeklyFixture()
	// A duplicate provider row must combine into the same player/week record.
	weekly.Rows = append(weekly.Rows, nflverse.WeeklyStat{
		GSISID: "gsis-rb-1", PlayerName: "Not The Local Name", Position: "RB",
		Season: 2025, Week: 1, Receptions: 1, Targets: 1, ReceivingYards: 5,
	})
	weekly.Rows = append(weekly.Rows, nflverse.WeeklyStat{
		GSISID: "unknown-high", PlayerName: "Unknown Star", Position: "WR",
		Season: 2025, Week: 1, Receptions: 10, Targets: 12, ReceivingYards: 150, ReceivingTouchdowns: 2,
	})
	weekly.SourceRows = len(weekly.Rows)
	crosswalk := nflverse.PlayerIDDataset{
		SourceRows: 2,
		Rows: []nflverse.PlayerID{
			{SleeperID: "rb-1", GSISID: "gsis-rb-1", Name: "A deliberately wrong name"},
			{SleeperID: "wr-1", GSISID: "different-gsis", Name: "Conflict"},
		},
	}

	first, err := LoadStats(context.Background(), db, weekly, crosswalk)
	if err != nil {
		t.Fatalf("LoadStats() error = %v", err)
	}
	if first.GSISBackfills != 1 || first.IdentityConflicts != 1 || first.MatchedRows != 36 ||
		first.UnmatchedRows != 1 || first.WeeklyRows != 36 || first.SeasonRows != 2 {
		t.Fatalf("summary = %#v", first)
	}
	if len(first.Unmatched) != 1 || first.Unmatched[0].GSISID != "unknown-high" {
		t.Fatalf("unmatched = %#v", first.Unmatched)
	}
	var gsisID string
	if err := db.QueryRow(`SELECT gsis_id FROM players WHERE sleeper_player_id = 'rb-1'`).Scan(&gsisID); err != nil {
		t.Fatal(err)
	}
	if gsisID != "gsis-rb-1" {
		t.Fatalf("backfilled GSIS ID = %q", gsisID)
	}
	var games, targets, receptions int
	var points float64
	if err := db.QueryRow(`
		SELECT games_played, fantasy_points_half_ppr, targets, receptions
		FROM player_season_stats AS stats
		JOIN players ON players.id = stats.player_id
		WHERE players.sleeper_player_id = 'rb-1' AND stats.season = 2025
	`).Scan(&games, &points, &targets, &receptions); err != nil {
		t.Fatal(err)
	}
	if games != 18 || points != 28 || targets != 19 || receptions != 19 {
		t.Fatalf("RB season totals = games %d points %.2f targets %d receptions %d", games, points, targets, receptions)
	}
	second, err := LoadStats(context.Background(), db, weekly, crosswalk)
	if err != nil {
		t.Fatalf("second LoadStats() error = %v", err)
	}
	if second.GSISBackfills != 0 || rowCount(t, db, "player_week_stats") != 36 || rowCount(t, db, "player_season_stats") != 2 {
		t.Fatalf("second summary = %#v", second)
	}
}

func TestLoadStatsSkipsAmbiguousCrosswalkAndNeverNameMatches(t *testing.T) {
	db := statsTestDatabase(t)
	if _, err := db.Exec(`UPDATE players SET gsis_id = NULL WHERE sleeper_player_id IN ('rb-1', 'te-1')`); err != nil {
		t.Fatal(err)
	}
	crosswalk := nflverse.PlayerIDDataset{Rows: []nflverse.PlayerID{
		{SleeperID: "rb-1", GSISID: "first", Name: "Test RB"},
		{SleeperID: "rb-1", GSISID: "second", Name: "Test RB"},
	}}
	weekly := completeWeeklyFixture()
	for index := range weekly.Rows {
		if weekly.Rows[index].GSISID == "gsis-rb-1" {
			weekly.Rows[index].GSISID = "first"
			weekly.Rows[index].PlayerName = "Test TE"
		}
	}
	summary, err := LoadStats(context.Background(), db, weekly, crosswalk)
	if err != nil {
		t.Fatalf("LoadStats() error = %v", err)
	}
	if summary.AmbiguousMappings != 1 || summary.GSISBackfills != 0 || summary.MatchedRows != 18 {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestLoadStatsRollsBackReplacementOnWriteFailure(t *testing.T) {
	db := statsTestDatabase(t)
	weekly := completeWeeklyFixture()
	if _, err := LoadStats(context.Background(), db, weekly, nflverse.PlayerIDDataset{}); err != nil {
		t.Fatal(err)
	}
	before := rowCount(t, db, "player_week_stats")
	if _, err := db.Exec(`
		CREATE TRIGGER reject_weekly_stats
		BEFORE INSERT ON player_week_stats
		WHEN NEW.season = 2025
		BEGIN
			SELECT RAISE(ABORT, 'test rejection');
		END
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadStats(context.Background(), db, weekly, nflverse.PlayerIDDataset{}); err == nil {
		t.Fatal("LoadStats() error = nil, want trigger failure")
	}
	if after := rowCount(t, db, "player_week_stats"); after != before {
		t.Fatalf("weekly rows after rollback = %d, want %d", after, before)
	}
}

func TestStatsRefreshPreservesPriorCacheWhenEitherProviderFails(t *testing.T) {
	root := t.TempDir()
	statsBody := weeklyCSVFixture()
	crosswalkBody := []byte("sleeper_id,gsis_id,name\nqb-1,gsis-qb-1,Test QB\n")
	firstFetch := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	runner := NewRunner(root+"/draft.db", io.Discard)
	runner.CacheDir = root + "/cache"
	runner.MinimumWeeklyStats = 1
	runner.Now = func() time.Time { return firstFetch }
	runner.NFLVerseClient.WeeklyStatsURL = "https://provider.test/stats"
	runner.NFLVerseClient.PlayerIDsURL = "https://provider.test/ids"
	failCrosswalk := false
	runner.NFLVerseClient.HTTPClient = &http.Client{Transport: importerRoundTripFunc(func(request *http.Request) *http.Response {
		status := http.StatusOK
		body := statsBody
		if request.URL.Path == "/ids" {
			body = crosswalkBody
			if failCrosswalk {
				status = http.StatusBadGateway
			}
		}
		return &http.Response{
			StatusCode: status,
			Status:     http.StatusText(status),
			Body:       io.NopCloser(strings.NewReader(string(body))),
			Header:     make(http.Header),
		}
	})}
	if _, _, _, source, err := runner.statsSource(context.Background(), true); err != nil || source != "providers" {
		t.Fatalf("first statsSource() = source %q, error %v", source, err)
	}
	_, _, cachedAt, source, err := runner.statsSource(context.Background(), false)
	if err != nil || source != "cache" || !cachedAt.Equal(firstFetch) {
		t.Fatalf("cached statsSource() = time %v source %q error %v", cachedAt, source, err)
	}

	failCrosswalk = true
	runner.Now = func() time.Time { return firstFetch.Add(time.Hour) }
	if _, _, _, _, err := runner.statsSource(context.Background(), true); err == nil {
		t.Fatal("failed refresh error = nil")
	}
	_, _, cachedAt, source, err = runner.statsSource(context.Background(), false)
	if err != nil || source != "cache" || !cachedAt.Equal(firstFetch) {
		t.Fatalf("cache after failed refresh = time %v source %q error %v", cachedAt, source, err)
	}
}

func statsTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	db := openTestDatabase(t)
	ctx := context.Background()
	if _, err := LoadTeams(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPlayers(ctx, db, fixturePlayers(), time.Now()); err != nil {
		t.Fatal(err)
	}
	return db
}

func completeWeeklyFixture() nflverse.WeeklyDataset {
	dataset := nflverse.WeeklyDataset{}
	for week := 1; week <= 18; week++ {
		dataset.Rows = append(dataset.Rows,
			nflverse.WeeklyStat{
				GSISID: "gsis-qb-1", PlayerName: "Test QB", Position: "QB", Season: 2025, Week: week,
				PassingYards: 250, PassingTouchdowns: 2, PassingInterceptions: 1,
			},
			nflverse.WeeklyStat{
				GSISID: "gsis-rb-1", PlayerName: "Test RB", Position: "RB", Season: 2025, Week: week,
				RushingAttempts: 10, RushingYards: 10, Receptions: 1, Targets: 1,
			},
		)
	}
	dataset.SourceRows = len(dataset.Rows)
	return dataset
}

func weeklyCSVFixture() []byte {
	var body strings.Builder
	body.WriteString("player_id,player_display_name,position_group,season,week,season_type,passing_yards,passing_tds,passing_interceptions,passing_2pt_conversions,carries,rushing_yards,rushing_tds,rushing_2pt_conversions,receptions,targets,receiving_yards,receiving_tds,receiving_2pt_conversions,fumbles_lost_total\n")
	for week := 1; week <= 18; week++ {
		body.WriteString("gsis-qb-1,Test QB,QB,2025,")
		body.WriteString(strconv.Itoa(week))
		body.WriteString(",REG,250,2,1,0,0,0,0,0,0,0,0,0,0,0\n")
	}
	return []byte(body.String())
}

type importerRoundTripFunc func(*http.Request) *http.Response

func (roundTrip importerRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request), nil
}
