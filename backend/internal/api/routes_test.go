package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jrevansjr/ffapp/backend/internal/database"
)

func TestHealth(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	recorder := httptest.NewRecorder()

	NewRouter(nil, nil).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusOK)
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}

	var response healthResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Status != "ok" {
		t.Fatalf("status = %q, want ok", response.Status)
	}
	if corsHeader := recorder.Header().Get("Access-Control-Allow-Origin"); corsHeader != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want no CORS header", corsHeader)
	}
}

func TestPlayerAndTeamEndpoints(t *testing.T) {
	db, router := newTestRouter(t)

	teamsRecorder := serveRequest(router, http.MethodGet, "/api/nfl-teams", "")
	if teamsRecorder.Code != http.StatusOK {
		t.Fatalf("GET /api/nfl-teams status = %d, want 200", teamsRecorder.Code)
	}
	var teams []nflTeamResponse
	decodeResponse(t, teamsRecorder, &teams)
	if len(teams) != 2 {
		t.Fatalf("NFL team count = %d, want 2", len(teams))
	}

	playersRecorder := serveRequest(router, http.MethodGet, "/api/players?position=qb", "")
	if playersRecorder.Code != http.StatusOK {
		t.Fatalf("GET /api/players status = %d, want 200", playersRecorder.Code)
	}
	var players []playerListResponse
	decodeResponse(t, playersRecorder, &players)
	if len(players) != 1 {
		t.Fatalf("QB response count = %d, want 1", len(players))
	}
	if players[0].Age == nil || players[0].Season == nil ||
		players[0].Season.AverageFantasyPoints == nil || players[0].Season.TargetsPerGame == nil ||
		players[0].Draft.AggregateADP == nil || players[0].Draft.ECR == nil ||
		players[0].Draft.PositionRank == nil || players[0].Draft.RankStdDev == nil ||
		players[0].Projections == nil || players[0].Projections.PassingYards == nil {
		t.Fatal("player list response is missing display data")
	}
	team := players[0].NFLTeam.Abbreviation
	teamRecorder := serveRequest(router, http.MethodGet, "/api/players?team="+team, "")
	var teammates []playerListResponse
	decodeResponse(t, teamRecorder, &teammates)
	if len(teammates) == 0 {
		t.Fatalf("team %s response is empty", team)
	}
	for _, teammate := range teammates {
		if teammate.NFLTeam == nil || teammate.NFLTeam.Abbreviation != team {
			t.Fatalf("team filter returned %#v, want team %s", teammate.NFLTeam, team)
		}
	}

	if _, err := database.UpdateSettings(context.Background(), db, database.EditableSettings{
		SleeperDraftID: "fixture-draft", PollingEnabled: true, PollingInterval: 2000,
	}); err != nil {
		t.Fatalf("activate sample draft: %v", err)
	}
	availableRecorder := serveRequest(router, http.MethodGet, "/api/players?available_only=true", "")
	var available []playerListResponse
	decodeResponse(t, availableRecorder, &available)
	if len(available) != 3 {
		t.Fatalf("available response count = %d, want 3", len(available))
	}

	var playerID int64
	if err := db.QueryRow(`SELECT id FROM players ORDER BY id LIMIT 1`).Scan(&playerID); err != nil {
		t.Fatalf("load player id: %v", err)
	}
	detailRecorder := serveRequest(
		router, http.MethodGet, "/api/players/"+strconvFormatInt(playerID), "",
	)
	if detailRecorder.Code != http.StatusOK {
		t.Fatalf("GET player detail status = %d, want 200", detailRecorder.Code)
	}
	var detail playerDetailResponse
	decodeResponse(t, detailRecorder, &detail)
	if len(detail.Weekly) != 2 {
		t.Fatalf("weekly response count = %d, want 2", len(detail.Weekly))
	}
	if detail.WeeklySummary.Average == nil || detail.Player.ProviderIDs.Sportradar == nil ||
		detail.Player.ProviderIDs.GSIS == nil ||
		detail.Weekly[0].PassingYards == 0 || detail.Projections == nil ||
		detail.Projections.PassingTouchdowns == nil {
		t.Fatal("player detail is missing summary, provider identity, or passing yards")
	}
}

func TestPlayerDetailHandlesMissingValues(t *testing.T) {
	db, router := newTestRouter(t)
	result, err := db.Exec(`
		INSERT INTO players (first_name, last_name, position, active)
		VALUES (?, ?, ?, 1)
	`, "No", "Reference Data", "WR")
	if err != nil {
		t.Fatalf("insert sparse player: %v", err)
	}
	playerID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("sparse player id: %v", err)
	}

	recorder := serveRequest(router, http.MethodGet, "/api/players/"+strconvFormatInt(playerID), "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("sparse player status = %d, want 200", recorder.Code)
	}
	var response playerDetailResponse
	decodeResponse(t, recorder, &response)
	if response.Player.NFLTeam != nil || response.Player.Age != nil || response.Season != nil ||
		response.Draft.AggregateADP != nil || response.Draft.ECR != nil || response.Draft.Tier != nil ||
		response.Projections != nil || response.Odds.Touchdowns != nil || response.WeeklySummary.Average != nil {
		t.Fatal("missing optional values should be encoded as null")
	}
	if response.Weekly == nil || len(response.Weekly) != 0 {
		t.Fatalf("weekly = %#v, want non-nil empty array", response.Weekly)
	}
}

func TestSettingsEndpointsValidateAndPersist(t *testing.T) {
	db, router := newTestRouter(t)
	const syncTime = "2026-08-20T12:00:00Z"
	if _, err := db.Exec(`UPDATE app_settings SET players_synced_at = ? WHERE id = 1`, syncTime); err != nil {
		t.Fatalf("set player sync timestamp: %v", err)
	}

	body := `{
		"sleeper_username":"  learner  ",
		"sleeper_league_id":" league-1 ",
		"sleeper_draft_id":" draft-1 ",
		"polling_enabled":false,
		"polling_interval_ms":1500
	}`
	recorder := serveRequest(router, http.MethodPut, "/api/settings", body)
	if recorder.Code != http.StatusOK {
		t.Fatalf("PUT /api/settings status = %d, want 200; body = %s", recorder.Code, recorder.Body)
	}
	var saved settingsResponse
	decodeResponse(t, recorder, &saved)
	if saved.SleeperUsername != "learner" || saved.SleeperLeagueID != "league-1" ||
		saved.SleeperDraftID != "draft-1" || saved.PollingEnabled || saved.PollingInterval != 1500 {
		t.Fatalf("saved settings = %#v, want trimmed request values", saved)
	}
	if saved.PlayersSyncedAt == nil || *saved.PlayersSyncedAt != syncTime {
		t.Fatalf("players_synced_at = %#v, want preserved %s", saved.PlayersSyncedAt, syncTime)
	}

	getRecorder := serveRequest(router, http.MethodGet, "/api/settings", "")
	var loaded settingsResponse
	decodeResponse(t, getRecorder, &loaded)
	if loaded.SleeperUsername != "learner" || loaded.PollingInterval != 1500 {
		t.Fatalf("persisted settings = %#v", loaded)
	}

	invalidBodies := []string{
		`{"sleeper_username":"x"}`,
		`{"sleeper_username":"","sleeper_league_id":"","sleeper_draft_id":"","polling_enabled":true,"polling_interval_ms":100}`,
		`{"sleeper_username":"","sleeper_league_id":"","sleeper_draft_id":"","polling_enabled":true,"polling_interval_ms":2000,"token":"never"}`,
		`{"sleeper_username":"","sleeper_league_id":"","sleeper_draft_id":"","polling_enabled":true,"polling_interval_ms":2000} {}`,
	}
	for _, invalidBody := range invalidBodies {
		recorder := serveRequest(router, http.MethodPut, "/api/settings", invalidBody)
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("invalid settings status = %d, want 400; body = %s", recorder.Code, invalidBody)
		}
	}
}

func TestSettingsUpdateNotifiesDraftPoller(t *testing.T) {
	db, _ := newTestRouter(t)
	var notified database.Settings
	router := NewRouter(db, func(settings database.Settings) { notified = settings })
	body := `{
		"sleeper_username":"learner",
		"sleeper_league_id":"league-1",
		"sleeper_draft_id":"draft-1",
		"polling_enabled":true,
		"polling_interval_ms":2500
	}`
	recorder := serveRequest(router, http.MethodPut, "/api/settings", body)
	if recorder.Code != http.StatusOK {
		t.Fatalf("PUT /api/settings status = %d, want 200", recorder.Code)
	}
	if notified.SleeperDraftID != "draft-1" || notified.PollingInterval != 2500 {
		t.Fatalf("poller notification = %#v", notified)
	}
}

func TestDraftStateEndpointUsesPersistedSnapshot(t *testing.T) {
	db, router := newTestRouter(t)

	unconfigured := serveRequest(router, http.MethodGet, "/api/draft/state", "")
	if unconfigured.Code != http.StatusOK {
		t.Fatalf("unconfigured draft-state status = %d, want 200", unconfigured.Code)
	}
	var empty draftStateResponse
	decodeResponse(t, unconfigured, &empty)
	if empty.Status != "not_configured" || empty.Stale || len(empty.Picks) != 0 {
		t.Fatalf("unconfigured draft state = %#v", empty)
	}

	if _, err := database.UpdateSettings(context.Background(), db, database.EditableSettings{
		SleeperDraftID: "api-draft", PollingEnabled: true, PollingInterval: 2000,
	}); err != nil {
		t.Fatalf("activate API draft: %v", err)
	}
	if _, err := database.SyncDraftPicks(context.Background(), db, "api-draft", "", []database.DraftPickInput{
		{PickNumber: 1, SleeperPlayerID: "fixture-qb", FirstName: "Alex", LastName: "Alpha"},
		{PickNumber: 2, SleeperPlayerID: "unknown", FirstName: "Unknown", LastName: "Rookie", Position: "WR"},
		{PickNumber: 3, SleeperPlayerID: "BUF", FirstName: "Buffalo", LastName: "Bills", Position: "DEF"},
		{PickNumber: 4, SleeperPlayerID: "kicker", FirstName: "Sample", LastName: "Kicker", Position: "K"},
	}); err != nil {
		t.Fatalf("sync API draft fixture: %v", err)
	}

	recorder := serveRequest(router, http.MethodGet, "/api/draft/state", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /api/draft/state status = %d, want 200", recorder.Code)
	}
	var response draftStateResponse
	decodeResponse(t, recorder, &response)
	if response.Status != "current" || response.Stale || response.LastSyncedAt == nil ||
		len(response.Picks) != 4 || len(response.TakenPlayerIDs) != 1 ||
		response.TakenPlayerIDs[0] != 1 || len(response.UnknownSleeperPlayerIDs) != 1 ||
		response.UnknownSleeperPlayerIDs[0] != "unknown" {
		t.Fatalf("draft state response = %#v", response)
	}
	if err := database.RecordDraftSyncFailure(context.Background(), db, "api-draft", ""); err != nil {
		t.Fatalf("record API draft failure: %v", err)
	}
	staleRecorder := serveRequest(router, http.MethodGet, "/api/draft/state", "")
	var stale draftStateResponse
	decodeResponse(t, staleRecorder, &stale)
	if stale.Status != "stale" || !stale.Stale || len(stale.Picks) != 4 || stale.Message == "" {
		t.Fatalf("stale draft state response = %#v", stale)
	}
}

func TestManualPickEndpointsCreateAndUndoActiveDraftPicks(t *testing.T) {
	db, router := newTestRouter(t)

	invalidRequests := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/api/draft/manual-picks", body: `{}`},
		{method: http.MethodPost, path: "/api/draft/manual-picks", body: `{"player_id":0}`},
		{method: http.MethodPost, path: "/api/draft/manual-picks", body: `{"player_id":2,"extra":true}`},
		{method: http.MethodPost, path: "/api/draft/manual-picks", body: `{"player_id":2} {}`},
		{method: http.MethodDelete, path: "/api/draft/manual-picks/not-a-number"},
		{method: http.MethodDelete, path: "/api/draft/manual-picks/0"},
	}
	for _, request := range invalidRequests {
		recorder := serveRequest(router, request.method, request.path, request.body)
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("%s %s status = %d, want 400", request.method, request.path, recorder.Code)
		}
	}

	unconfigured := serveRequest(
		router,
		http.MethodPost,
		"/api/draft/manual-picks",
		`{"player_id":2}`,
	)
	if unconfigured.Code != http.StatusConflict {
		t.Fatalf("unconfigured manual pick status = %d, want 409", unconfigured.Code)
	}
	if _, err := database.UpdateSettings(context.Background(), db, database.EditableSettings{
		SleeperDraftID: "fixture-draft", PollingEnabled: false, PollingInterval: 2000,
	}); err != nil {
		t.Fatalf("activate fixture draft: %v", err)
	}

	createdRecorder := serveRequest(
		router,
		http.MethodPost,
		"/api/draft/manual-picks",
		`{"player_id":2}`,
	)
	if createdRecorder.Code != http.StatusCreated {
		t.Fatalf(
			"POST manual pick status = %d, want 201; body = %s",
			createdRecorder.Code,
			createdRecorder.Body,
		)
	}
	var created draftStateResponse
	decodeResponse(t, createdRecorder, &created)
	if len(created.Picks) != 2 || len(created.TakenPlayerIDs) != 2 {
		t.Fatalf("state after manual pick = %#v", created)
	}
	var manualPickID, officialPickID int64
	for _, pick := range created.Picks {
		switch pick.Source {
		case "manual":
			manualPickID = pick.ID
		case "sleeper":
			officialPickID = pick.ID
		}
	}
	if manualPickID == 0 || officialPickID == 0 {
		t.Fatalf("manual/official pick IDs = %d/%d", manualPickID, officialPickID)
	}

	duplicate := serveRequest(
		router,
		http.MethodPost,
		"/api/draft/manual-picks",
		`{"player_id":2}`,
	)
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate manual pick status = %d, want 409", duplicate.Code)
	}
	missingPlayer := serveRequest(
		router,
		http.MethodPost,
		"/api/draft/manual-picks",
		`{"player_id":999999}`,
	)
	if missingPlayer.Code != http.StatusNotFound {
		t.Fatalf("missing-player manual pick status = %d, want 404", missingPlayer.Code)
	}
	officialDelete := serveRequest(
		router,
		http.MethodDelete,
		"/api/draft/manual-picks/"+strconvFormatInt(officialPickID),
		"",
	)
	if officialDelete.Code != http.StatusNotFound {
		t.Fatalf("official-pick delete status = %d, want 404", officialDelete.Code)
	}

	deletedRecorder := serveRequest(
		router,
		http.MethodDelete,
		"/api/draft/manual-picks/"+strconvFormatInt(manualPickID),
		"",
	)
	if deletedRecorder.Code != http.StatusOK {
		t.Fatalf("DELETE manual pick status = %d, want 200", deletedRecorder.Code)
	}
	var deleted draftStateResponse
	decodeResponse(t, deletedRecorder, &deleted)
	if len(deleted.Picks) != 1 || len(deleted.TakenPlayerIDs) != 1 ||
		deleted.Picks[0].Source != "sleeper" {
		t.Fatalf("state after manual undo = %#v", deleted)
	}
	repeatedDelete := serveRequest(
		router,
		http.MethodDelete,
		"/api/draft/manual-picks/"+strconvFormatInt(manualPickID),
		"",
	)
	if repeatedDelete.Code != http.StatusNotFound {
		t.Fatalf("repeated manual-pick delete status = %d, want 404", repeatedDelete.Code)
	}
}

func TestPlayerEndpointValidationAndNotFound(t *testing.T) {
	_, router := newTestRouter(t)
	for _, path := range []string{
		"/api/players?available_only=sometimes",
		"/api/players/not-a-number",
		"/api/players/0",
	} {
		recorder := serveRequest(router, http.MethodGet, path, "")
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("GET %s status = %d, want 400", path, recorder.Code)
		}
	}
	if recorder := serveRequest(router, http.MethodGet, "/api/players/999999", ""); recorder.Code != http.StatusNotFound {
		t.Errorf("missing player status = %d, want 404", recorder.Code)
	}
}

func TestCalculateAge(t *testing.T) {
	now := time.Date(2026, time.August, 22, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		birthDate *string
		want      *int
	}{
		{name: "birthday passed", birthDate: stringPointer("2000-08-21"), want: intPointer(26)},
		{name: "birthday upcoming", birthDate: stringPointer("2000-08-23"), want: intPointer(25)},
		{name: "missing", birthDate: nil, want: nil},
		{name: "invalid", birthDate: stringPointer("unknown"), want: nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := calculateAge(test.birthDate, now)
			if !equalIntPointers(got, test.want) {
				t.Fatalf("calculateAge() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestSummarize(t *testing.T) {
	summary := summarize([]float64{5, 1, 7, 3})
	if *summary.Average != 4 || *summary.High != 7 || *summary.Median != 4 || *summary.Low != 1 {
		t.Fatalf("summarize() = %#v, want average/median 4, high 7, low 1", summary)
	}
	if empty := summarize(nil); empty.Average != nil || empty.High != nil || empty.Median != nil || empty.Low != nil {
		t.Fatalf("summarize(nil) = %#v, want null values", empty)
	}
}

func TestSeasonAveragesRequireGamesPlayed(t *testing.T) {
	withGames := newSeasonStatsResponse(&database.PlayerSeasonStats{
		GamesPlayed: 4, FantasyPointsHalfPPR: 50, Targets: 18, RushingAttempts: 27,
	})
	if *withGames.AverageFantasyPoints != 12.5 || *withGames.TargetsPerGame != 4.5 || *withGames.RushingAttemptsPerGame != 6.75 {
		t.Fatalf("season averages = %#v, want 12.5 points, 4.5 targets, and 6.75 rushing attempts", withGames)
	}
	withoutGames := newSeasonStatsResponse(&database.PlayerSeasonStats{})
	if withoutGames.AverageFantasyPoints != nil || withoutGames.TargetsPerGame != nil || withoutGames.RushingAttemptsPerGame != nil {
		t.Fatalf("zero-game averages = %#v, want null", withoutGames)
	}
}

func newTestRouter(t *testing.T) (*sql.DB, http.Handler) {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "draft.db"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	loadAPITestFixture(t, db)
	return db, NewRouter(db, nil)
}

func loadAPITestFixture(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO nfl_teams (id, abbreviation, name) VALUES
			(1, 'ARI', 'Arizona Cardinals'),
			(2, 'BUF', 'Buffalo Bills');
		INSERT INTO players (
			id, sleeper_player_id, first_name, last_name, position, nfl_team_id,
			birth_date, active, years_exp, sportradar_id, gsis_id
		) VALUES
			(1, 'fixture-qb', 'Alex', 'Alpha', 'QB', 1, '1998-01-01', 1, 4, 'sr-qb', 'gsis-qb'),
			(2, 'fixture-rb', 'Blair', 'Beta', 'RB', 1, '1999-01-01', 1, 3, 'sr-rb', 'gsis-rb'),
			(3, 'fixture-wr', 'Casey', 'Gamma', 'WR', 2, '2000-01-01', 1, 2, 'sr-wr', 'gsis-wr'),
			(4, 'fixture-te', 'Devon', 'Theta', 'TE', 2, '2001-01-01', 1, 1, 'sr-te', 'gsis-te');
		INSERT INTO player_season_stats (
			player_id, season, games_played, fantasy_points_half_ppr, passing_yards,
			targets, receptions, rushing_attempts, receiving_yards, rushing_yards,
			receiving_touchdowns, rushing_touchdowns
		) VALUES (1, 2025, 2, 40, 500, 0, 0, 8, 0, 30, 0, 1);
		INSERT INTO player_week_stats (
			player_id, season, week, fantasy_points_half_ppr, passing_yards,
			targets, receptions, rushing_attempts, receiving_yards, rushing_yards,
			receiving_touchdowns, rushing_touchdowns
		) VALUES
			(1, 2025, 1, 18, 220, 0, 0, 3, 0, 10, 0, 0),
			(1, 2025, 2, 22, 280, 0, 0, 5, 0, 20, 0, 1);
		INSERT INTO player_adp (player_id, season, source, adp, updated_at)
		VALUES (1, 2026, 'fantasypros', 12.5, '2026-08-01T00:00:00Z');
		INSERT INTO player_rankings (
			player_id, season, source, overall_rank, position_rank,
			rank_min, rank_max, rank_std_dev, updated_at
		) VALUES (1, 2026, 'fantasypros', 10, 2, 7, 14, 2.5, '2026-08-01T00:00:00Z');
		INSERT INTO player_tiers (player_id, season, source, tier, updated_at)
		VALUES (1, 2026, 'fantasypros', 2, '2026-08-01T00:00:00Z');
		INSERT INTO player_projections (
			player_id, season, source, passing_yards, passing_touchdowns,
			rushing_yards, rushing_touchdowns, updated_at
		) VALUES (1, 2026, 'fantasypros', 4000.5, 30.2, 350.1, 4.5, '2026-08-01T00:00:00Z');
		INSERT INTO odds (season, source, market, player_id, line, captured_at)
		VALUES (2026, 'fixture', 'total_touchdowns', 1, 1.5, '2026-08-01T00:00:00Z');
		INSERT INTO odds (season, source, market, nfl_team_id, line, captured_at)
		VALUES (2026, 'fixture', 'regular_season_wins', 1, 8.5, '2026-08-01T00:00:00Z');
		INSERT INTO drafts (id, sleeper_draft_id, mode, status, created_at, updated_at)
		VALUES (1, 'fixture-draft', 'live', 'mock', '2026-08-01T00:00:00Z', '2026-08-01T00:00:00Z');
		INSERT INTO draft_picks (
			draft_id, pick_number, sleeper_player_id, player_id, source, created_at
		) VALUES (1, 1, 'fixture-qb', 1, 'sleeper', '2026-08-01T00:00:00Z');
	`)
	if err != nil {
		t.Fatalf("load API test fixture: %v", err)
	}
}

func serveRequest(router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func decodeResponse(t *testing.T, recorder *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.NewDecoder(recorder.Body).Decode(target); err != nil {
		t.Fatalf("decode response: %v; body = %s", err, recorder.Body.String())
	}
}

func strconvFormatInt(value int64) string {
	return strconv.FormatInt(value, 10)
}

func stringPointer(value string) *string { return &value }
func intPointer(value int) *int          { return &value }

func equalIntPointers(left, right *int) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}
