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

	NewRouter(nil).ServeHTTP(recorder, request)

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
	db, router := newSeededTestRouter(t)

	teamsRecorder := serveRequest(router, http.MethodGet, "/api/nfl-teams", "")
	if teamsRecorder.Code != http.StatusOK {
		t.Fatalf("GET /api/nfl-teams status = %d, want 200", teamsRecorder.Code)
	}
	var teams []nflTeamResponse
	decodeResponse(t, teamsRecorder, &teams)
	if len(teams) != 32 {
		t.Fatalf("NFL team count = %d, want 32", len(teams))
	}

	playersRecorder := serveRequest(router, http.MethodGet, "/api/players?position=qb", "")
	if playersRecorder.Code != http.StatusOK {
		t.Fatalf("GET /api/players status = %d, want 200", playersRecorder.Code)
	}
	var players []playerListResponse
	decodeResponse(t, playersRecorder, &players)
	if len(players) != 12 {
		t.Fatalf("QB response count = %d, want 12", len(players))
	}
	if players[0].Age == nil || players[0].Season == nil ||
		players[0].Season.AverageFantasyPoints == nil || players[0].Season.TargetsPerGame == nil ||
		players[0].ADP.FantasyPros == nil {
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
		SleeperDraftID: "sample-draft-2026", PollingEnabled: true, PollingInterval: 2000,
	}); err != nil {
		t.Fatalf("activate sample draft: %v", err)
	}
	availableRecorder := serveRequest(router, http.MethodGet, "/api/players?available_only=true", "")
	var available []playerListResponse
	decodeResponse(t, availableRecorder, &available)
	if len(available) != 54 {
		t.Fatalf("available response count = %d, want 54", len(available))
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
	if len(detail.Weekly) != 8 {
		t.Fatalf("weekly response count = %d, want 8", len(detail.Weekly))
	}
	if detail.WeeklySummary.Average == nil || detail.Player.ProviderIDs.Sportradar == nil ||
		detail.Weekly[0].PassingYards == 0 {
		t.Fatal("player detail is missing summary, provider identity, or passing yards")
	}
}

func TestPlayerDetailHandlesMissingValues(t *testing.T) {
	db, router := newSeededTestRouter(t)
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
		response.ADP.FantasyPros != nil || response.Tier != nil ||
		response.Odds.Touchdowns != nil || response.WeeklySummary.Average != nil {
		t.Fatal("missing optional values should be encoded as null")
	}
	if response.Weekly == nil || len(response.Weekly) != 0 {
		t.Fatalf("weekly = %#v, want non-nil empty array", response.Weekly)
	}
}

func TestSettingsEndpointsValidateAndPersist(t *testing.T) {
	db, router := newSeededTestRouter(t)
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

func TestPlayerEndpointValidationAndNotFound(t *testing.T) {
	_, router := newSeededTestRouter(t)
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

func newSeededTestRouter(t *testing.T) (*sql.DB, http.Handler) {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "draft.db"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.SeedSampleData(context.Background(), db); err != nil {
		t.Fatalf("seed test database: %v", err)
	}
	return db, NewRouter(db)
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
