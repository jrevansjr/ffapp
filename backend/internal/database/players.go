package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Availability belongs to a configured draft, not to the player row. Keeping
// the expression shared prevents list and detail queries from drifting.
const takenPlayerExpression = `EXISTS (
	SELECT 1
	FROM app_settings AS settings
	JOIN drafts ON drafts.sleeper_draft_id = settings.sleeper_draft_id
	JOIN draft_picks ON draft_picks.draft_id = drafts.id
	WHERE settings.id = 1
		AND settings.sleeper_draft_id <> ''
		AND (
			draft_picks.player_id = players.id
			OR (
				players.sleeper_player_id IS NOT NULL
				AND draft_picks.sleeper_player_id = players.sleeper_player_id
			)
		)
)`

// PlayerFilters controls the optional filters supported by GET /api/players.
type PlayerFilters struct {
	Position      string
	Team          string
	AvailableOnly bool
}

// PlayerListItem contains the compact identity and reference data needed by
// Overview and Draft Day without exposing normalized database rows.
type PlayerListItem struct {
	ID                 int64
	SleeperPlayerID    *string
	FirstName          string
	LastName           string
	Position           string
	NFLTeam            *NFLTeam
	BirthDate          *string
	Number             *int
	YearsExp           *int
	DepthChartPosition *string
	DepthChartOrder    *int
	InjuryStatus       *string
	Draft              PlayerDraftData
	Season             *PlayerSeasonStats
	Projections        *PlayerProjections
	IsTaken            bool
}

// ProviderIDs contains external identities retained from approved providers
// and exact crosswalks.
type ProviderIDs struct {
	GSIS        *string
	FantasyPros *string
	ESPN        *string
	Sportradar  *string
	Rotowire    *string
	Rotoworld   *string
	Yahoo       *string
	FantasyData *string
	Stats       *string
}

// PlayerProfile is the durable local identity and Sleeper profile for a player.
type PlayerProfile struct {
	ID                    int64
	SleeperPlayerID       *string
	FirstName             string
	LastName              string
	Position              string
	NFLTeam               *NFLTeam
	BirthDate             *string
	Active                bool
	Status                *string
	Number                *int
	College               *string
	Height                *string
	Weight                *string
	BirthCountry          *string
	YearsExp              *int
	DepthChartPosition    *string
	DepthChartOrder       *int
	InjuryStatus          *string
	InjuryStartDate       *string
	PracticeParticipation *string
	ProviderIDs           ProviderIDs
}

// PlayerSeasonStats stores one historical season's totals.
type PlayerSeasonStats struct {
	Season               int
	GamesPlayed          int
	FantasyPointsHalfPPR float64
	PassingYards         int
	Targets              int
	Receptions           int
	RushingAttempts      int
	ReceivingYards       int
	RushingYards         int
	ReceivingTouchdowns  int
	RushingTouchdowns    int
}

// PlayerWeekStats stores one week's historical totals.
type PlayerWeekStats struct {
	Season               int
	Week                 int
	FantasyPointsHalfPPR float64
	PassingYards         int
	Targets              int
	Receptions           int
	RushingAttempts      int
	ReceivingYards       int
	RushingYards         int
	ReceivingTouchdowns  int
	RushingTouchdowns    int
}

// PlayerDraftData combines the FantasyPros fields used for quick draft-day
// comparison. Every value remains nullable when the provider has no exact ID
// match for a local player.
type PlayerDraftData struct {
	AggregateADP *float64
	ECR          *int
	PositionRank *int
	Tier         *int
	RankMin      *int
	RankMax      *int
	RankStdDev   *float64
}

// PlayerProjections stores FantasyPros' preseason volume forecast. Fields are
// nullable because FantasyPros publishes only statistics relevant to a
// player's position.
type PlayerProjections struct {
	Season              int
	Source              string
	PassingYards        *float64
	PassingTouchdowns   *float64
	RushingYards        *float64
	RushingTouchdowns   *float64
	ReceivingYards      *float64
	ReceivingTouchdowns *float64
	UpdatedAt           string
}

// OddsLine is one player- or NFL-team-specific betting market snapshot.
type OddsLine struct {
	Season     int
	Source     string
	Market     string
	Line       float64
	OverPrice  *float64
	UnderPrice *float64
	CapturedAt string
}

// PlayerOdds groups position-relevant player futures with the player's NFL-team win total.
type PlayerOdds struct {
	PassingYards        *OddsLine
	PassingTouchdowns   *OddsLine
	RushingYards        *OddsLine
	RushingTouchdowns   *OddsLine
	ReceivingYards      *OddsLine
	ReceivingTouchdowns *OddsLine
	TeamWins            *OddsLine
}

// PlayerMoralityScore is one user-supplied subjective score and its provenance.
type PlayerMoralityScore struct {
	Score        int
	Source       string
	SnapshotDate string
	ImportedAt   string
}

// PlayerDetail combines normalized player data for the detail API response.
type PlayerDetail struct {
	Player      PlayerProfile
	IsTaken     bool
	Season      *PlayerSeasonStats
	Draft       PlayerDraftData
	Projections *PlayerProjections
	Odds        PlayerOdds
	Morality    *PlayerMoralityScore
	Weekly      []PlayerWeekStats
}

// ListPlayers returns active players with optional exact position/team filters.
// Availability is derived from the currently configured draft.
func ListPlayers(ctx context.Context, db *sql.DB, filters PlayerFilters) ([]PlayerListItem, error) {
	query := `
		SELECT
			players.id,
			players.sleeper_player_id,
			players.first_name,
			players.last_name,
			players.position,
			nfl_teams.id,
			nfl_teams.abbreviation,
			nfl_teams.name,
			players.birth_date,
			players.number,
			players.years_exp,
			players.depth_chart_position,
			players.depth_chart_order,
			players.injury_status,
			aggregate_adp.adp,
			draft_rankings.overall_rank,
			draft_rankings.position_rank,
			fantasypros_tiers.tier,
			draft_rankings.rank_min,
			draft_rankings.rank_max,
			draft_rankings.rank_std_dev,
			season_stats.season,
			season_stats.games_played,
			season_stats.fantasy_points_half_ppr,
			season_stats.passing_yards,
			season_stats.targets,
			season_stats.receptions,
			season_stats.rushing_attempts,
			season_stats.receiving_yards,
			season_stats.rushing_yards,
			season_stats.receiving_touchdowns,
			season_stats.rushing_touchdowns,
			projections.season,
			projections.source,
			projections.passing_yards,
			projections.passing_touchdowns,
			projections.rushing_yards,
			projections.rushing_touchdowns,
			projections.receiving_yards,
			projections.receiving_touchdowns,
			projections.updated_at,
			` + takenPlayerExpression + ` AS is_taken
		FROM players
		LEFT JOIN nfl_teams ON nfl_teams.id = players.nfl_team_id
		LEFT JOIN player_season_stats AS season_stats
			ON season_stats.player_id = players.id AND season_stats.season = 2025
		LEFT JOIN player_adp AS aggregate_adp
			ON aggregate_adp.player_id = players.id
			AND aggregate_adp.season = 2026
			AND aggregate_adp.source = 'fantasypros'
		LEFT JOIN player_rankings AS draft_rankings
			ON draft_rankings.player_id = players.id
			AND draft_rankings.season = 2026
			AND draft_rankings.source = 'fantasypros'
		LEFT JOIN player_tiers AS fantasypros_tiers
			ON fantasypros_tiers.player_id = players.id
			AND fantasypros_tiers.season = 2026
			AND fantasypros_tiers.source = 'fantasypros'
		LEFT JOIN player_projections AS projections
			ON projections.player_id = players.id
			AND projections.season = 2026
			AND projections.source = 'fantasypros'
		WHERE players.active = 1
	`
	args := make([]any, 0, 2)
	if filters.Position != "" {
		query += " AND players.position = ?"
		args = append(args, filters.Position)
	}
	if filters.Team != "" {
		query += " AND nfl_teams.abbreviation = ?"
		args = append(args, filters.Team)
	}
	if filters.AvailableOnly {
		query += " AND NOT " + takenPlayerExpression
	}
	query += " ORDER BY players.last_name, players.first_name, players.id"

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list players: %w", err)
	}
	defer rows.Close()

	players := make([]PlayerListItem, 0)
	for rows.Next() {
		player, err := scanPlayerListItem(rows)
		if err != nil {
			return nil, err
		}
		players = append(players, player)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read players: %w", err)
	}
	return players, nil
}

func scanPlayerListItem(rows *sql.Rows) (PlayerListItem, error) {
	var (
		player                                                         PlayerListItem
		sleeperID, teamAbbreviation, teamName, birthDate               sql.NullString
		depthChartPosition, injuryStatus                               sql.NullString
		teamID, number, yearsExp, depthChartOrder, season, gamesPlayed sql.NullInt64
		passingYards, targets, receptions, rushingAttempts             sql.NullInt64
		receivingYards, rushingYards, receivingTDs, rushingTDs         sql.NullInt64
		ecr, positionRank, tier, rankMin, rankMax                      sql.NullInt64
		aggregateADP, rankStdDev, fantasyPoints                        sql.NullFloat64
		projectionSeason                                               sql.NullInt64
		projectionSource, projectionUpdatedAt                          sql.NullString
		passingProjection, passingTDProjection                         sql.NullFloat64
		rushingProjection, rushingTDProjection                         sql.NullFloat64
		receivingProjection, receivingTDProjection                     sql.NullFloat64
		isTaken                                                        int
	)
	if err := rows.Scan(
		&player.ID,
		&sleeperID,
		&player.FirstName,
		&player.LastName,
		&player.Position,
		&teamID,
		&teamAbbreviation,
		&teamName,
		&birthDate,
		&number,
		&yearsExp,
		&depthChartPosition,
		&depthChartOrder,
		&injuryStatus,
		&aggregateADP,
		&ecr,
		&positionRank,
		&tier,
		&rankMin,
		&rankMax,
		&rankStdDev,
		&season,
		&gamesPlayed,
		&fantasyPoints,
		&passingYards,
		&targets,
		&receptions,
		&rushingAttempts,
		&receivingYards,
		&rushingYards,
		&receivingTDs,
		&rushingTDs,
		&projectionSeason,
		&projectionSource,
		&passingProjection,
		&passingTDProjection,
		&rushingProjection,
		&rushingTDProjection,
		&receivingProjection,
		&receivingTDProjection,
		&projectionUpdatedAt,
		&isTaken,
	); err != nil {
		return PlayerListItem{}, fmt.Errorf("scan player list item: %w", err)
	}

	player.SleeperPlayerID = nullStringPointer(sleeperID)
	player.NFLTeam = nullableNFLTeam(teamID, teamAbbreviation, teamName)
	player.BirthDate = nullStringPointer(birthDate)
	player.Number = nullIntPointer(number)
	player.YearsExp = nullIntPointer(yearsExp)
	player.DepthChartPosition = nullStringPointer(depthChartPosition)
	player.DepthChartOrder = nullIntPointer(depthChartOrder)
	player.InjuryStatus = nullStringPointer(injuryStatus)
	player.Draft = PlayerDraftData{
		AggregateADP: nullFloatPointer(aggregateADP),
		ECR:          nullIntPointer(ecr),
		PositionRank: nullIntPointer(positionRank),
		Tier:         nullIntPointer(tier),
		RankMin:      nullIntPointer(rankMin),
		RankMax:      nullIntPointer(rankMax),
		RankStdDev:   nullFloatPointer(rankStdDev),
	}
	player.IsTaken = isTaken == 1
	if projectionSeason.Valid {
		player.Projections = &PlayerProjections{
			Season: int(projectionSeason.Int64), Source: projectionSource.String,
			PassingYards:        nullFloatPointer(passingProjection),
			PassingTouchdowns:   nullFloatPointer(passingTDProjection),
			RushingYards:        nullFloatPointer(rushingProjection),
			RushingTouchdowns:   nullFloatPointer(rushingTDProjection),
			ReceivingYards:      nullFloatPointer(receivingProjection),
			ReceivingTouchdowns: nullFloatPointer(receivingTDProjection),
			UpdatedAt:           projectionUpdatedAt.String,
		}
	}
	if season.Valid {
		player.Season = &PlayerSeasonStats{
			Season:               int(season.Int64),
			GamesPlayed:          int(gamesPlayed.Int64),
			FantasyPointsHalfPPR: fantasyPoints.Float64,
			PassingYards:         int(passingYards.Int64),
			Targets:              int(targets.Int64),
			Receptions:           int(receptions.Int64),
			RushingAttempts:      int(rushingAttempts.Int64),
			ReceivingYards:       int(receivingYards.Int64),
			RushingYards:         int(rushingYards.Int64),
			ReceivingTouchdowns:  int(receivingTDs.Int64),
			RushingTouchdowns:    int(rushingTDs.Int64),
		}
	}
	return player, nil
}

// GetPlayer returns one player and all reference data used by the inspector.
func GetPlayer(ctx context.Context, db *sql.DB, playerID int64) (PlayerDetail, error) {
	detail, err := loadPlayerProfile(ctx, db, playerID)
	if err != nil {
		return PlayerDetail{}, err
	}
	if detail.Season, err = loadPlayerSeason(ctx, db, playerID); err != nil {
		return PlayerDetail{}, err
	}
	if detail.Draft, err = loadPlayerDraftData(ctx, db, playerID); err != nil {
		return PlayerDetail{}, err
	}
	if detail.Projections, err = loadPlayerProjections(ctx, db, playerID); err != nil {
		return PlayerDetail{}, err
	}
	if detail.Odds, err = loadPlayerOdds(ctx, db, playerID, detail.Player.NFLTeam); err != nil {
		return PlayerDetail{}, err
	}
	if detail.Morality, err = loadPlayerMorality(ctx, db, playerID); err != nil {
		return PlayerDetail{}, err
	}
	if detail.Weekly, err = loadPlayerWeeks(ctx, db, playerID); err != nil {
		return PlayerDetail{}, err
	}
	return detail, nil
}

func loadPlayerMorality(ctx context.Context, db *sql.DB, playerID int64) (*PlayerMoralityScore, error) {
	var score PlayerMoralityScore
	err := db.QueryRowContext(ctx, `
		SELECT score, source, snapshot_date, imported_at
		FROM player_morality_scores
		WHERE player_id = ? AND source = 'user_supplied'
	`, playerID).Scan(&score.Score, &score.Source, &score.SnapshotDate, &score.ImportedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get player morality score: %w", err)
	}
	return &score, nil
}

func loadPlayerProjections(ctx context.Context, db *sql.DB, playerID int64) (*PlayerProjections, error) {
	var (
		projection                                         PlayerProjections
		passingYards, passingTDs, rushingYards, rushingTDs sql.NullFloat64
		receivingYards, receivingTDs                       sql.NullFloat64
	)
	err := db.QueryRowContext(ctx, `
		SELECT season, source, passing_yards, passing_touchdowns,
			rushing_yards, rushing_touchdowns, receiving_yards,
			receiving_touchdowns, updated_at
		FROM player_projections
		WHERE player_id = ? AND season = 2026 AND source = 'fantasypros'
	`, playerID).Scan(
		&projection.Season, &projection.Source, &passingYards, &passingTDs,
		&rushingYards, &rushingTDs, &receivingYards, &receivingTDs, &projection.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get player projections: %w", err)
	}
	projection.PassingYards = nullFloatPointer(passingYards)
	projection.PassingTouchdowns = nullFloatPointer(passingTDs)
	projection.RushingYards = nullFloatPointer(rushingYards)
	projection.RushingTouchdowns = nullFloatPointer(rushingTDs)
	projection.ReceivingYards = nullFloatPointer(receivingYards)
	projection.ReceivingTouchdowns = nullFloatPointer(receivingTDs)
	return &projection, nil
}

func loadPlayerProfile(ctx context.Context, db *sql.DB, playerID int64) (PlayerDetail, error) {
	var (
		detail                                                 PlayerDetail
		sleeperID, teamAbbreviation, teamName, birthDate       sql.NullString
		status, college, height, weight, birthCountry          sql.NullString
		depthPosition, injuryStatus, injuryStart, practice     sql.NullString
		espnID, sportradarID, rotowireID, rotoworldID, yahooID sql.NullString
		fantasyDataID, statsID, gsisID, fantasyProsID          sql.NullString
		teamID, number, yearsExp, depthOrder                   sql.NullInt64
		active, isTaken                                        int
	)
	err := db.QueryRowContext(ctx, `
		SELECT
			players.id,
			players.sleeper_player_id,
			players.first_name,
			players.last_name,
			players.position,
			nfl_teams.id,
			nfl_teams.abbreviation,
			nfl_teams.name,
			players.birth_date,
			players.active,
			players.status,
			players.number,
			players.college,
			players.height,
			players.weight,
			players.birth_country,
			players.years_exp,
			players.depth_chart_position,
			players.depth_chart_order,
			players.injury_status,
			players.injury_start_date,
			players.practice_participation,
			players.espn_id,
			players.sportradar_id,
			players.rotowire_id,
			players.rotoworld_id,
			players.yahoo_id,
			players.fantasy_data_id,
			players.stats_id,
			players.gsis_id,
			players.fantasypros_id,
			`+takenPlayerExpression+` AS is_taken
		FROM players
		LEFT JOIN nfl_teams ON nfl_teams.id = players.nfl_team_id
		WHERE players.id = ?
	`, playerID).Scan(
		&detail.Player.ID,
		&sleeperID,
		&detail.Player.FirstName,
		&detail.Player.LastName,
		&detail.Player.Position,
		&teamID,
		&teamAbbreviation,
		&teamName,
		&birthDate,
		&active,
		&status,
		&number,
		&college,
		&height,
		&weight,
		&birthCountry,
		&yearsExp,
		&depthPosition,
		&depthOrder,
		&injuryStatus,
		&injuryStart,
		&practice,
		&espnID,
		&sportradarID,
		&rotowireID,
		&rotoworldID,
		&yahooID,
		&fantasyDataID,
		&statsID,
		&gsisID,
		&fantasyProsID,
		&isTaken,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return PlayerDetail{}, sql.ErrNoRows
		}
		return PlayerDetail{}, fmt.Errorf("get player profile: %w", err)
	}

	detail.Player.SleeperPlayerID = nullStringPointer(sleeperID)
	detail.Player.NFLTeam = nullableNFLTeam(teamID, teamAbbreviation, teamName)
	detail.Player.BirthDate = nullStringPointer(birthDate)
	detail.Player.Active = active == 1
	detail.Player.Status = nullStringPointer(status)
	detail.Player.Number = nullIntPointer(number)
	detail.Player.College = nullStringPointer(college)
	detail.Player.Height = nullStringPointer(height)
	detail.Player.Weight = nullStringPointer(weight)
	detail.Player.BirthCountry = nullStringPointer(birthCountry)
	detail.Player.YearsExp = nullIntPointer(yearsExp)
	detail.Player.DepthChartPosition = nullStringPointer(depthPosition)
	detail.Player.DepthChartOrder = nullIntPointer(depthOrder)
	detail.Player.InjuryStatus = nullStringPointer(injuryStatus)
	detail.Player.InjuryStartDate = nullStringPointer(injuryStart)
	detail.Player.PracticeParticipation = nullStringPointer(practice)
	detail.Player.ProviderIDs = ProviderIDs{
		GSIS:        nullStringPointer(gsisID),
		FantasyPros: nullStringPointer(fantasyProsID),
		ESPN:        nullStringPointer(espnID),
		Sportradar:  nullStringPointer(sportradarID),
		Rotowire:    nullStringPointer(rotowireID),
		Rotoworld:   nullStringPointer(rotoworldID),
		Yahoo:       nullStringPointer(yahooID),
		FantasyData: nullStringPointer(fantasyDataID),
		Stats:       nullStringPointer(statsID),
	}
	detail.IsTaken = isTaken == 1
	return detail, nil
}

func loadPlayerSeason(ctx context.Context, db *sql.DB, playerID int64) (*PlayerSeasonStats, error) {
	var stats PlayerSeasonStats
	err := db.QueryRowContext(ctx, `
		SELECT
			season,
			games_played,
			fantasy_points_half_ppr,
			passing_yards,
			targets,
			receptions,
			rushing_attempts,
			receiving_yards,
			rushing_yards,
			receiving_touchdowns,
			rushing_touchdowns
		FROM player_season_stats
		WHERE player_id = ? AND season = 2025
	`, playerID).Scan(
		&stats.Season,
		&stats.GamesPlayed,
		&stats.FantasyPointsHalfPPR,
		&stats.PassingYards,
		&stats.Targets,
		&stats.Receptions,
		&stats.RushingAttempts,
		&stats.ReceivingYards,
		&stats.RushingYards,
		&stats.ReceivingTouchdowns,
		&stats.RushingTouchdowns,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get player season stats: %w", err)
	}
	return &stats, nil
}

func loadPlayerDraftData(ctx context.Context, db *sql.DB, playerID int64) (PlayerDraftData, error) {
	var (
		data                                      PlayerDraftData
		aggregateADP, rankStdDev                  sql.NullFloat64
		ecr, positionRank, tier, rankMin, rankMax sql.NullInt64
	)
	err := db.QueryRowContext(ctx, `
		SELECT
			aggregate_adp.adp,
			draft_rankings.overall_rank,
			draft_rankings.position_rank,
			fantasypros_tiers.tier,
			draft_rankings.rank_min,
			draft_rankings.rank_max,
			draft_rankings.rank_std_dev
		FROM players
		LEFT JOIN player_adp AS aggregate_adp
			ON aggregate_adp.player_id = players.id
			AND aggregate_adp.season = 2026
			AND aggregate_adp.source = 'fantasypros'
		LEFT JOIN player_rankings AS draft_rankings
			ON draft_rankings.player_id = players.id
			AND draft_rankings.season = 2026
			AND draft_rankings.source = 'fantasypros'
		LEFT JOIN player_tiers AS fantasypros_tiers
			ON fantasypros_tiers.player_id = players.id
			AND fantasypros_tiers.season = 2026
			AND fantasypros_tiers.source = 'fantasypros'
		WHERE players.id = ?
	`, playerID).Scan(
		&aggregateADP, &ecr, &positionRank, &tier, &rankMin, &rankMax, &rankStdDev,
	)
	if err != nil {
		return PlayerDraftData{}, fmt.Errorf("get player draft data: %w", err)
	}
	data.AggregateADP = nullFloatPointer(aggregateADP)
	data.ECR = nullIntPointer(ecr)
	data.PositionRank = nullIntPointer(positionRank)
	data.Tier = nullIntPointer(tier)
	data.RankMin = nullIntPointer(rankMin)
	data.RankMax = nullIntPointer(rankMax)
	data.RankStdDev = nullFloatPointer(rankStdDev)
	return data, nil
}

func loadPlayerOdds(
	ctx context.Context,
	db *sql.DB,
	playerID int64,
	team *NFLTeam,
) (PlayerOdds, error) {
	var odds PlayerOdds
	var err error
	if odds.PassingYards, err = loadOddsLine(ctx, db, "player_id", playerID, "passing_yards"); err != nil {
		return PlayerOdds{}, err
	}
	if odds.PassingTouchdowns, err = loadOddsLine(ctx, db, "player_id", playerID, "passing_touchdowns"); err != nil {
		return PlayerOdds{}, err
	}
	if odds.RushingYards, err = loadOddsLine(ctx, db, "player_id", playerID, "rushing_yards"); err != nil {
		return PlayerOdds{}, err
	}
	if odds.RushingTouchdowns, err = loadOddsLine(ctx, db, "player_id", playerID, "rushing_touchdowns"); err != nil {
		return PlayerOdds{}, err
	}
	if odds.ReceivingYards, err = loadOddsLine(ctx, db, "player_id", playerID, "receiving_yards"); err != nil {
		return PlayerOdds{}, err
	}
	if odds.ReceivingTouchdowns, err = loadOddsLine(ctx, db, "player_id", playerID, "receiving_touchdowns"); err != nil {
		return PlayerOdds{}, err
	}
	if team != nil {
		odds.TeamWins, err = loadOddsLine(ctx, db, "nfl_team_id", team.ID, "regular_season_wins")
		if err != nil {
			return PlayerOdds{}, err
		}
	}
	return odds, nil
}

func loadOddsLine(
	ctx context.Context,
	db *sql.DB,
	subjectColumn string,
	subjectID int64,
	market string,
) (*OddsLine, error) {
	// subjectColumn is never user input: callers select one of the two schema
	// columns above, while subjectID and market remain parameterized values.
	var (
		line       OddsLine
		overPrice  sql.NullFloat64
		underPrice sql.NullFloat64
	)
	query := `
		SELECT season, source, market, line, over_price, under_price, captured_at
		FROM odds
		WHERE ` + subjectColumn + ` = ?
			AND season = 2026
			AND source = 'sportsbook_consensus'
			AND market = ?
		ORDER BY captured_at DESC, source
		LIMIT 1
	`
	err := db.QueryRowContext(ctx, query, subjectID, market).Scan(
		&line.Season,
		&line.Source,
		&line.Market,
		&line.Line,
		&overPrice,
		&underPrice,
		&line.CapturedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get %s odds: %w", market, err)
	}
	line.OverPrice = nullFloatPointer(overPrice)
	line.UnderPrice = nullFloatPointer(underPrice)
	return &line, nil
}

func loadPlayerWeeks(ctx context.Context, db *sql.DB, playerID int64) ([]PlayerWeekStats, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT
			season,
			week,
			fantasy_points_half_ppr,
			passing_yards,
			targets,
			receptions,
			rushing_attempts,
			receiving_yards,
			rushing_yards,
			receiving_touchdowns,
			rushing_touchdowns
		FROM player_week_stats
		WHERE player_id = ? AND season = 2025
		ORDER BY week
	`, playerID)
	if err != nil {
		return nil, fmt.Errorf("get player weekly stats: %w", err)
	}
	defer rows.Close()

	weekly := make([]PlayerWeekStats, 0)
	for rows.Next() {
		var stats PlayerWeekStats
		if err := rows.Scan(
			&stats.Season,
			&stats.Week,
			&stats.FantasyPointsHalfPPR,
			&stats.PassingYards,
			&stats.Targets,
			&stats.Receptions,
			&stats.RushingAttempts,
			&stats.ReceivingYards,
			&stats.RushingYards,
			&stats.ReceivingTouchdowns,
			&stats.RushingTouchdowns,
		); err != nil {
			return nil, fmt.Errorf("scan player weekly stats: %w", err)
		}
		weekly = append(weekly, stats)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read player weekly stats: %w", err)
	}
	return weekly, nil
}

func nullableNFLTeam(id sql.NullInt64, abbreviation, name sql.NullString) *NFLTeam {
	if !id.Valid {
		return nil
	}
	return &NFLTeam{ID: id.Int64, Abbreviation: abbreviation.String, Name: name.String}
}

func nullStringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func nullIntPointer(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	result := int(value.Int64)
	return &result
}

func nullFloatPointer(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	return floatPointer(value.Float64)
}

func floatPointer(value float64) *float64 {
	return &value
}
