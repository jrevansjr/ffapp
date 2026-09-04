package api

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jrevansjr/ffapp/backend/internal/database"
)

type playerTeamResponse struct {
	ID           int64  `json:"id"`
	Abbreviation string `json:"abbreviation"`
	Name         string `json:"name"`
}

// The player response types intentionally mirror the UI's data needs instead
// of exposing rows from normalized draft, stats, and odds tables.
type playerDraftResponse struct {
	AggregateADP *float64 `json:"aggregate_adp"`
	ECR          *int     `json:"ecr"`
	PositionRank *int     `json:"position_rank"`
	Tier         *int     `json:"tier"`
	RankMin      *int     `json:"rank_min"`
	RankMax      *int     `json:"rank_max"`
	RankStdDev   *float64 `json:"rank_std_dev"`
}

type seasonStatsResponse struct {
	Season                 int      `json:"season"`
	GamesPlayed            int      `json:"games_played"`
	FantasyPointsHalfPPR   float64  `json:"fantasy_points_half_ppr"`
	AverageFantasyPoints   *float64 `json:"average_fantasy_points"`
	PassingYards           int      `json:"passing_yards"`
	Targets                int      `json:"targets"`
	TargetsPerGame         *float64 `json:"targets_per_game"`
	Receptions             int      `json:"receptions"`
	RushingAttempts        int      `json:"rushing_attempts"`
	RushingAttemptsPerGame *float64 `json:"rushing_attempts_per_game"`
	ReceivingYards         int      `json:"receiving_yards"`
	RushingYards           int      `json:"rushing_yards"`
	ReceivingTouchdowns    int      `json:"receiving_touchdowns"`
	RushingTouchdowns      int      `json:"rushing_touchdowns"`
}

type playerProjectionsResponse struct {
	Season              int      `json:"season"`
	Source              string   `json:"source"`
	PassingYards        *float64 `json:"passing_yards"`
	PassingTouchdowns   *float64 `json:"passing_touchdowns"`
	RushingYards        *float64 `json:"rushing_yards"`
	RushingTouchdowns   *float64 `json:"rushing_touchdowns"`
	ReceivingYards      *float64 `json:"receiving_yards"`
	ReceivingTouchdowns *float64 `json:"receiving_touchdowns"`
	UpdatedAt           string   `json:"updated_at"`
}

type playerListResponse struct {
	ID                 int64                      `json:"id"`
	SleeperPlayerID    *string                    `json:"sleeper_player_id"`
	FirstName          string                     `json:"first_name"`
	LastName           string                     `json:"last_name"`
	Position           string                     `json:"position"`
	NFLTeam            *playerTeamResponse        `json:"nfl_team"`
	Age                *int                       `json:"age"`
	Number             *int                       `json:"number"`
	YearsExp           *int                       `json:"years_exp"`
	DepthChartPosition *string                    `json:"depth_chart_position"`
	DepthChartOrder    *int                       `json:"depth_chart_order"`
	InjuryStatus       *string                    `json:"injury_status"`
	Draft              playerDraftResponse        `json:"draft"`
	Season             *seasonStatsResponse       `json:"season"`
	Projections        *playerProjectionsResponse `json:"projections"`
	IsTaken            bool                       `json:"is_taken"`
}

type providerIDsResponse struct {
	GSIS        *string `json:"gsis"`
	FantasyPros *string `json:"fantasypros"`
	ESPN        *string `json:"espn"`
	Sportradar  *string `json:"sportradar"`
	Rotowire    *string `json:"rotowire"`
	Rotoworld   *string `json:"rotoworld"`
	Yahoo       *string `json:"yahoo"`
	FantasyData *string `json:"fantasy_data"`
	Stats       *string `json:"stats"`
}

type playerProfileResponse struct {
	ID                    int64               `json:"id"`
	SleeperPlayerID       *string             `json:"sleeper_player_id"`
	FirstName             string              `json:"first_name"`
	LastName              string              `json:"last_name"`
	Position              string              `json:"position"`
	NFLTeam               *playerTeamResponse `json:"nfl_team"`
	BirthDate             *string             `json:"birth_date"`
	Age                   *int                `json:"age"`
	Active                bool                `json:"active"`
	Status                *string             `json:"status"`
	Number                *int                `json:"number"`
	College               *string             `json:"college"`
	Height                *string             `json:"height"`
	Weight                *string             `json:"weight"`
	BirthCountry          *string             `json:"birth_country"`
	YearsExp              *int                `json:"years_exp"`
	DepthChartPosition    *string             `json:"depth_chart_position"`
	DepthChartOrder       *int                `json:"depth_chart_order"`
	InjuryStatus          *string             `json:"injury_status"`
	InjuryBodyPart        *string             `json:"injury_body_part"`
	InjuryNotes           *string             `json:"injury_notes"`
	InjuryStartDate       *string             `json:"injury_start_date"`
	PracticeParticipation *string             `json:"practice_participation"`
	SleeperDataUpdatedAt  *string             `json:"sleeper_data_updated_at"`
	ProviderIDs           providerIDsResponse `json:"provider_ids"`
	IsTaken               bool                `json:"is_taken"`
}

type oddsLineResponse struct {
	Season     int      `json:"season"`
	Source     string   `json:"source"`
	Market     string   `json:"market"`
	Line       float64  `json:"line"`
	OverPrice  *float64 `json:"over_price"`
	UnderPrice *float64 `json:"under_price"`
	CapturedAt string   `json:"captured_at"`
}

type playerOddsResponse struct {
	PassingYards        *oddsLineResponse `json:"passing_yards"`
	PassingTouchdowns   *oddsLineResponse `json:"passing_touchdowns"`
	RushingYards        *oddsLineResponse `json:"rushing_yards"`
	RushingTouchdowns   *oddsLineResponse `json:"rushing_touchdowns"`
	ReceivingYards      *oddsLineResponse `json:"receiving_yards"`
	ReceivingTouchdowns *oddsLineResponse `json:"receiving_touchdowns"`
	TeamWins            *oddsLineResponse `json:"team_wins"`
}

type playerMoralityResponse struct {
	Score          int    `json:"score"`
	Source         string `json:"source"`
	SnapshotDate   string `json:"snapshot_date"`
	ScaleMinimum   int    `json:"scale_minimum"`
	ScaleMaximum   int    `json:"scale_maximum"`
	HigherIsBetter bool   `json:"higher_is_better"`
}

type weeklyStatsResponse struct {
	Season               int     `json:"season"`
	Week                 int     `json:"week"`
	FantasyPointsHalfPPR float64 `json:"fantasy_points_half_ppr"`
	PassingYards         int     `json:"passing_yards"`
	Targets              int     `json:"targets"`
	Receptions           int     `json:"receptions"`
	RushingAttempts      int     `json:"rushing_attempts"`
	ReceivingYards       int     `json:"receiving_yards"`
	RushingYards         int     `json:"rushing_yards"`
	ReceivingTouchdowns  int     `json:"receiving_touchdowns"`
	RushingTouchdowns    int     `json:"rushing_touchdowns"`
}

type weeklySummaryResponse struct {
	Average *float64 `json:"average"`
	High    *float64 `json:"high"`
	Median  *float64 `json:"median"`
	Low     *float64 `json:"low"`
}

type playerDetailResponse struct {
	Player        playerProfileResponse      `json:"player"`
	Season        *seasonStatsResponse       `json:"season"`
	Draft         playerDraftResponse        `json:"draft"`
	Projections   *playerProjectionsResponse `json:"projections"`
	Odds          playerOddsResponse         `json:"odds"`
	Morality      *playerMoralityResponse    `json:"morality"`
	SeasonTeams   []playerTeamResponse       `json:"season_teams"`
	Weekly        []weeklyStatsResponse      `json:"weekly"`
	WeeklySummary weeklySummaryResponse      `json:"weekly_summary"`
}

// handlePlayers returns the compact Overview/Draft Day dataset in one request.
// Filters are normalized at the HTTP boundary before reaching SQL.
func (h handler) handlePlayers(w http.ResponseWriter, r *http.Request) {
	filters, validationMessage := playerFiltersFromRequest(r)
	if validationMessage != "" {
		writeError(w, http.StatusBadRequest, validationMessage)
		return
	}

	players, err := database.ListPlayers(r.Context(), h.db, filters)
	if err != nil {
		log.Printf("list players: %v", err)
		writeError(w, http.StatusInternalServerError, "could not load players")
		return
	}
	response := make([]playerListResponse, 0, len(players))
	now := time.Now().UTC()
	for _, player := range players {
		response = append(response, newPlayerListResponse(player, now))
	}
	writeJSON(w, http.StatusOK, response)
}

// handlePlayer returns the richer inspector payload, including weekly history
// and its server-calculated fantasy-point summary.
func (h handler) handlePlayer(w http.ResponseWriter, r *http.Request) {
	playerID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || playerID <= 0 {
		writeError(w, http.StatusBadRequest, "player id must be a positive integer")
		return
	}

	detail, err := database.GetPlayer(r.Context(), h.db, playerID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "player not found")
		return
	}
	if err != nil {
		log.Printf("get player %d: %v", playerID, err)
		writeError(w, http.StatusInternalServerError, "could not load player")
		return
	}
	writeJSON(w, http.StatusOK, newPlayerDetailResponse(detail, time.Now().UTC()))
}

// playerFiltersFromRequest accepts case-insensitive position/team values while
// keeping available_only strict enough to catch malformed URLs.
func playerFiltersFromRequest(r *http.Request) (database.PlayerFilters, string) {
	filters := database.PlayerFilters{
		Position: strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("position"))),
		Team:     strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("team"))),
	}
	if value := strings.TrimSpace(r.URL.Query().Get("available_only")); value != "" {
		switch strings.ToLower(value) {
		case "true":
			filters.AvailableOnly = true
		case "false":
			filters.AvailableOnly = false
		default:
			return database.PlayerFilters{}, "available_only must be true or false"
		}
	}
	return filters, ""
}

func newPlayerListResponse(player database.PlayerListItem, now time.Time) playerListResponse {
	return playerListResponse{
		ID:                 player.ID,
		SleeperPlayerID:    player.SleeperPlayerID,
		FirstName:          player.FirstName,
		LastName:           player.LastName,
		Position:           player.Position,
		NFLTeam:            newPlayerTeamResponse(player.NFLTeam),
		Age:                calculateAge(player.BirthDate, now),
		Number:             player.Number,
		YearsExp:           player.YearsExp,
		DepthChartPosition: player.DepthChartPosition,
		DepthChartOrder:    player.DepthChartOrder,
		InjuryStatus:       player.InjuryStatus,
		Draft:              newPlayerDraftResponse(player.Draft),
		Season:             newSeasonStatsResponse(player.Season),
		Projections:        newPlayerProjectionsResponse(player.Projections),
		IsTaken:            player.IsTaken,
	}
}

// newPlayerDetailResponse keeps nullable storage values nullable in JSON and
// guarantees weekly is an empty array, rather than null, when no rows exist.
func newPlayerDetailResponse(detail database.PlayerDetail, now time.Time) playerDetailResponse {
	weekly := make([]weeklyStatsResponse, 0, len(detail.Weekly))
	points := make([]float64, 0, len(detail.Weekly))
	for _, week := range detail.Weekly {
		weekly = append(weekly, weeklyStatsResponse{
			Season:               week.Season,
			Week:                 week.Week,
			FantasyPointsHalfPPR: week.FantasyPointsHalfPPR,
			PassingYards:         week.PassingYards,
			Targets:              week.Targets,
			Receptions:           week.Receptions,
			RushingAttempts:      week.RushingAttempts,
			ReceivingYards:       week.ReceivingYards,
			RushingYards:         week.RushingYards,
			ReceivingTouchdowns:  week.ReceivingTouchdowns,
			RushingTouchdowns:    week.RushingTouchdowns,
		})
		points = append(points, week.FantasyPointsHalfPPR)
	}

	return playerDetailResponse{
		Player: playerProfileResponse{
			ID:                    detail.Player.ID,
			SleeperPlayerID:       detail.Player.SleeperPlayerID,
			FirstName:             detail.Player.FirstName,
			LastName:              detail.Player.LastName,
			Position:              detail.Player.Position,
			NFLTeam:               newPlayerTeamResponse(detail.Player.NFLTeam),
			BirthDate:             detail.Player.BirthDate,
			Age:                   calculateAge(detail.Player.BirthDate, now),
			Active:                detail.Player.Active,
			Status:                detail.Player.Status,
			Number:                detail.Player.Number,
			College:               detail.Player.College,
			Height:                detail.Player.Height,
			Weight:                detail.Player.Weight,
			BirthCountry:          detail.Player.BirthCountry,
			YearsExp:              detail.Player.YearsExp,
			DepthChartPosition:    detail.Player.DepthChartPosition,
			DepthChartOrder:       detail.Player.DepthChartOrder,
			InjuryStatus:          detail.Player.InjuryStatus,
			InjuryBodyPart:        detail.Player.InjuryBodyPart,
			InjuryNotes:           detail.Player.InjuryNotes,
			InjuryStartDate:       detail.Player.InjuryStartDate,
			PracticeParticipation: detail.Player.PracticeParticipation,
			SleeperDataUpdatedAt:  detail.Player.SleeperDataUpdatedAt,
			ProviderIDs: providerIDsResponse{
				GSIS:        detail.Player.ProviderIDs.GSIS,
				FantasyPros: detail.Player.ProviderIDs.FantasyPros,
				ESPN:        detail.Player.ProviderIDs.ESPN,
				Sportradar:  detail.Player.ProviderIDs.Sportradar,
				Rotowire:    detail.Player.ProviderIDs.Rotowire,
				Rotoworld:   detail.Player.ProviderIDs.Rotoworld,
				Yahoo:       detail.Player.ProviderIDs.Yahoo,
				FantasyData: detail.Player.ProviderIDs.FantasyData,
				Stats:       detail.Player.ProviderIDs.Stats,
			},
			IsTaken: detail.IsTaken,
		},
		Season:        newSeasonStatsResponse(detail.Season),
		Draft:         newPlayerDraftResponse(detail.Draft),
		Projections:   newPlayerProjectionsResponse(detail.Projections),
		Odds:          newPlayerOddsResponse(detail.Odds),
		Morality:      newPlayerMoralityResponse(detail.Morality),
		SeasonTeams:   newPlayerTeamResponses(detail.SeasonTeams),
		Weekly:        weekly,
		WeeklySummary: summarize(points),
	}
}

func newPlayerMoralityResponse(score *database.PlayerMoralityScore) *playerMoralityResponse {
	if score == nil {
		return nil
	}
	return &playerMoralityResponse{
		Score:          score.Score,
		Source:         score.Source,
		SnapshotDate:   score.SnapshotDate,
		ScaleMinimum:   0,
		ScaleMaximum:   5,
		HigherIsBetter: true,
	}
}

func newPlayerProjectionsResponse(projections *database.PlayerProjections) *playerProjectionsResponse {
	if projections == nil {
		return nil
	}
	return &playerProjectionsResponse{
		Season:              projections.Season,
		Source:              projections.Source,
		PassingYards:        projections.PassingYards,
		PassingTouchdowns:   projections.PassingTouchdowns,
		RushingYards:        projections.RushingYards,
		RushingTouchdowns:   projections.RushingTouchdowns,
		ReceivingYards:      projections.ReceivingYards,
		ReceivingTouchdowns: projections.ReceivingTouchdowns,
		UpdatedAt:           projections.UpdatedAt,
	}
}

func newPlayerTeamResponse(team *database.NFLTeam) *playerTeamResponse {
	if team == nil {
		return nil
	}
	return &playerTeamResponse{ID: team.ID, Abbreviation: team.Abbreviation, Name: team.Name}
}

func newPlayerTeamResponses(teams []database.NFLTeam) []playerTeamResponse {
	response := make([]playerTeamResponse, 0, len(teams))
	for _, team := range teams {
		response = append(response, playerTeamResponse{
			ID: team.ID, Abbreviation: team.Abbreviation, Name: team.Name,
		})
	}
	return response
}

// newSeasonStatsResponse derives per-game presentation values only when a
// non-zero games-played denominator makes them meaningful.
func newSeasonStatsResponse(stats *database.PlayerSeasonStats) *seasonStatsResponse {
	if stats == nil {
		return nil
	}
	var averageFantasyPoints, targetsPerGame, rushingAttemptsPerGame *float64
	if stats.GamesPlayed > 0 {
		average := stats.FantasyPointsHalfPPR / float64(stats.GamesPlayed)
		targetAverage := float64(stats.Targets) / float64(stats.GamesPlayed)
		rushingAttemptAverage := float64(stats.RushingAttempts) / float64(stats.GamesPlayed)
		averageFantasyPoints = &average
		targetsPerGame = &targetAverage
		rushingAttemptsPerGame = &rushingAttemptAverage
	}
	return &seasonStatsResponse{
		Season:                 stats.Season,
		GamesPlayed:            stats.GamesPlayed,
		FantasyPointsHalfPPR:   stats.FantasyPointsHalfPPR,
		AverageFantasyPoints:   averageFantasyPoints,
		PassingYards:           stats.PassingYards,
		Targets:                stats.Targets,
		TargetsPerGame:         targetsPerGame,
		Receptions:             stats.Receptions,
		RushingAttempts:        stats.RushingAttempts,
		RushingAttemptsPerGame: rushingAttemptsPerGame,
		ReceivingYards:         stats.ReceivingYards,
		RushingYards:           stats.RushingYards,
		ReceivingTouchdowns:    stats.ReceivingTouchdowns,
		RushingTouchdowns:      stats.RushingTouchdowns,
	}
}

func newPlayerDraftResponse(data database.PlayerDraftData) playerDraftResponse {
	return playerDraftResponse{
		AggregateADP: data.AggregateADP,
		ECR:          data.ECR,
		PositionRank: data.PositionRank,
		Tier:         data.Tier,
		RankMin:      data.RankMin,
		RankMax:      data.RankMax,
		RankStdDev:   data.RankStdDev,
	}
}

func newPlayerOddsResponse(odds database.PlayerOdds) playerOddsResponse {
	return playerOddsResponse{
		PassingYards:        newOddsLineResponse(odds.PassingYards),
		PassingTouchdowns:   newOddsLineResponse(odds.PassingTouchdowns),
		RushingYards:        newOddsLineResponse(odds.RushingYards),
		RushingTouchdowns:   newOddsLineResponse(odds.RushingTouchdowns),
		ReceivingYards:      newOddsLineResponse(odds.ReceivingYards),
		ReceivingTouchdowns: newOddsLineResponse(odds.ReceivingTouchdowns),
		TeamWins:            newOddsLineResponse(odds.TeamWins),
	}
}

func newOddsLineResponse(line *database.OddsLine) *oddsLineResponse {
	if line == nil {
		return nil
	}
	return &oddsLineResponse{
		Season: line.Season, Source: line.Source, Market: line.Market, Line: line.Line,
		OverPrice: line.OverPrice, UnderPrice: line.UnderPrice, CapturedAt: line.CapturedAt,
	}
}

// calculateAge derives display age without storing a season-specific age on
// the player record. Invalid or missing dates remain unknown.
func calculateAge(birthDate *string, now time.Time) *int {
	if birthDate == nil {
		return nil
	}
	birth, err := time.Parse("2006-01-02", *birthDate)
	if err != nil || birth.After(now) {
		return nil
	}
	age := now.Year() - birth.Year()
	if now.Month() < birth.Month() || (now.Month() == birth.Month() && now.Day() < birth.Day()) {
		age--
	}
	return &age
}

// summarize calculates the four chart summary values in the API so every
// frontend chart uses the same definition, including an averaged even median.
func summarize(values []float64) weeklySummaryResponse {
	if len(values) == 0 {
		return weeklySummaryResponse{}
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	var total float64
	for _, value := range sorted {
		total += value
	}
	average := total / float64(len(sorted))
	low := sorted[0]
	high := sorted[len(sorted)-1]
	middle := len(sorted) / 2
	median := sorted[middle]
	if len(sorted)%2 == 0 {
		median = (sorted[middle-1] + sorted[middle]) / 2
	}
	return weeklySummaryResponse{
		Average: &average,
		High:    &high,
		Median:  &median,
		Low:     &low,
	}
}
