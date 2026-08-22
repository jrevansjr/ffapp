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
// of exposing rows from the normalized stats, ADP, tier, and odds tables.
type playerADPResponse struct {
	FantasyPros *float64 `json:"fantasypros"`
	Sleeper     *float64 `json:"sleeper"`
	Underdog    *float64 `json:"underdog"`
}

type seasonStatsResponse struct {
	Season               int      `json:"season"`
	GamesPlayed          int      `json:"games_played"`
	FantasyPointsHalfPPR float64  `json:"fantasy_points_half_ppr"`
	AverageFantasyPoints *float64 `json:"average_fantasy_points"`
	Targets              int      `json:"targets"`
	TargetsPerGame       *float64 `json:"targets_per_game"`
	Receptions           int      `json:"receptions"`
	RushingAttempts      int      `json:"rushing_attempts"`
	ReceivingYards       int      `json:"receiving_yards"`
	RushingYards         int      `json:"rushing_yards"`
	ReceivingTouchdowns  int      `json:"receiving_touchdowns"`
	RushingTouchdowns    int      `json:"rushing_touchdowns"`
}

type playerListOddsResponse struct {
	TouchdownLine *float64 `json:"touchdown_line"`
	TeamWinLine   *float64 `json:"team_win_line"`
}

type playerListResponse struct {
	ID                 int64                  `json:"id"`
	SleeperPlayerID    *string                `json:"sleeper_player_id"`
	FirstName          string                 `json:"first_name"`
	LastName           string                 `json:"last_name"`
	Position           string                 `json:"position"`
	NFLTeam            *playerTeamResponse    `json:"nfl_team"`
	Age                *int                   `json:"age"`
	Number             *int                   `json:"number"`
	YearsExp           *int                   `json:"years_exp"`
	DepthChartPosition *string                `json:"depth_chart_position"`
	DepthChartOrder    *int                   `json:"depth_chart_order"`
	InjuryStatus       *string                `json:"injury_status"`
	ADP                playerADPResponse      `json:"adp"`
	Season             *seasonStatsResponse   `json:"season"`
	Odds               playerListOddsResponse `json:"odds"`
	Tier               *int                   `json:"tier"`
	IsTaken            bool                   `json:"is_taken"`
}

type providerIDsResponse struct {
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
	InjuryStartDate       *string             `json:"injury_start_date"`
	PracticeParticipation *string             `json:"practice_participation"`
	ProviderIDs           providerIDsResponse `json:"provider_ids"`
	IsTaken               bool                `json:"is_taken"`
}

type playerTierResponse struct {
	Season    int    `json:"season"`
	Source    string `json:"source"`
	Tier      int    `json:"tier"`
	UpdatedAt string `json:"updated_at"`
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
	Touchdowns *oddsLineResponse `json:"touchdowns"`
	TeamWins   *oddsLineResponse `json:"team_wins"`
}

type weeklyStatsResponse struct {
	Season               int     `json:"season"`
	Week                 int     `json:"week"`
	FantasyPointsHalfPPR float64 `json:"fantasy_points_half_ppr"`
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
	Player        playerProfileResponse `json:"player"`
	Season        *seasonStatsResponse  `json:"season"`
	ADP           playerADPResponse     `json:"adp"`
	Tier          *playerTierResponse   `json:"tier"`
	Odds          playerOddsResponse    `json:"odds"`
	Weekly        []weeklyStatsResponse `json:"weekly"`
	WeeklySummary weeklySummaryResponse `json:"weekly_summary"`
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
		ADP: playerADPResponse{
			FantasyPros: player.FantasyProsADP,
			Sleeper:     player.SleeperADP,
			Underdog:    player.UnderdogADP,
		},
		Season: newSeasonStatsResponse(player.Season),
		Odds: playerListOddsResponse{
			TouchdownLine: player.TouchdownLine,
			TeamWinLine:   player.TeamWinLine,
		},
		Tier:    player.Tier,
		IsTaken: player.IsTaken,
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
			InjuryStartDate:       detail.Player.InjuryStartDate,
			PracticeParticipation: detail.Player.PracticeParticipation,
			ProviderIDs: providerIDsResponse{
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
		Season: newSeasonStatsResponse(detail.Season),
		ADP: playerADPResponse{
			FantasyPros: detail.ADP.FantasyPros,
			Sleeper:     detail.ADP.Sleeper,
			Underdog:    detail.ADP.Underdog,
		},
		Tier:          newPlayerTierResponse(detail.Tier),
		Odds:          newPlayerOddsResponse(detail.Odds),
		Weekly:        weekly,
		WeeklySummary: summarize(points),
	}
}

func newPlayerTeamResponse(team *database.NFLTeam) *playerTeamResponse {
	if team == nil {
		return nil
	}
	return &playerTeamResponse{ID: team.ID, Abbreviation: team.Abbreviation, Name: team.Name}
}

// newSeasonStatsResponse derives per-game presentation values only when a
// non-zero games-played denominator makes them meaningful.
func newSeasonStatsResponse(stats *database.PlayerSeasonStats) *seasonStatsResponse {
	if stats == nil {
		return nil
	}
	var averageFantasyPoints, targetsPerGame *float64
	if stats.GamesPlayed > 0 {
		average := stats.FantasyPointsHalfPPR / float64(stats.GamesPlayed)
		targetAverage := float64(stats.Targets) / float64(stats.GamesPlayed)
		averageFantasyPoints = &average
		targetsPerGame = &targetAverage
	}
	return &seasonStatsResponse{
		Season:               stats.Season,
		GamesPlayed:          stats.GamesPlayed,
		FantasyPointsHalfPPR: stats.FantasyPointsHalfPPR,
		AverageFantasyPoints: averageFantasyPoints,
		Targets:              stats.Targets,
		TargetsPerGame:       targetsPerGame,
		Receptions:           stats.Receptions,
		RushingAttempts:      stats.RushingAttempts,
		ReceivingYards:       stats.ReceivingYards,
		RushingYards:         stats.RushingYards,
		ReceivingTouchdowns:  stats.ReceivingTouchdowns,
		RushingTouchdowns:    stats.RushingTouchdowns,
	}
}

func newPlayerTierResponse(tier *database.PlayerTier) *playerTierResponse {
	if tier == nil {
		return nil
	}
	return &playerTierResponse{
		Season: tier.Season, Source: tier.Source, Tier: tier.Tier, UpdatedAt: tier.UpdatedAt,
	}
}

func newPlayerOddsResponse(odds database.PlayerOdds) playerOddsResponse {
	return playerOddsResponse{
		Touchdowns: newOddsLineResponse(odds.Touchdowns),
		TeamWins:   newOddsLineResponse(odds.TeamWins),
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
