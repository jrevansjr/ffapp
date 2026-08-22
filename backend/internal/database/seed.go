package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Stable seasons, sources, and synthetic external IDs make the sample dataset
// deterministic, allowing the seed command to upsert it safely on every run.
const (
	sampleStatsSeason = 2025
	sampleDataSeason  = 2026
	sampleSource      = "sample"
)

type sampleTeam struct {
	Abbreviation string
	Name         string
}

var sampleTeams = []sampleTeam{
	{Abbreviation: "ARI", Name: "Arizona Cardinals"},
	{Abbreviation: "ATL", Name: "Atlanta Falcons"},
	{Abbreviation: "BAL", Name: "Baltimore Ravens"},
	{Abbreviation: "BUF", Name: "Buffalo Bills"},
	{Abbreviation: "CAR", Name: "Carolina Panthers"},
	{Abbreviation: "CHI", Name: "Chicago Bears"},
	{Abbreviation: "CIN", Name: "Cincinnati Bengals"},
	{Abbreviation: "CLE", Name: "Cleveland Browns"},
	{Abbreviation: "DAL", Name: "Dallas Cowboys"},
	{Abbreviation: "DEN", Name: "Denver Broncos"},
	{Abbreviation: "DET", Name: "Detroit Lions"},
	{Abbreviation: "GB", Name: "Green Bay Packers"},
	{Abbreviation: "HOU", Name: "Houston Texans"},
	{Abbreviation: "IND", Name: "Indianapolis Colts"},
	{Abbreviation: "JAX", Name: "Jacksonville Jaguars"},
	{Abbreviation: "KC", Name: "Kansas City Chiefs"},
	{Abbreviation: "LV", Name: "Las Vegas Raiders"},
	{Abbreviation: "LAC", Name: "Los Angeles Chargers"},
	{Abbreviation: "LAR", Name: "Los Angeles Rams"},
	{Abbreviation: "MIA", Name: "Miami Dolphins"},
	{Abbreviation: "MIN", Name: "Minnesota Vikings"},
	{Abbreviation: "NE", Name: "New England Patriots"},
	{Abbreviation: "NO", Name: "New Orleans Saints"},
	{Abbreviation: "NYG", Name: "New York Giants"},
	{Abbreviation: "NYJ", Name: "New York Jets"},
	{Abbreviation: "PHI", Name: "Philadelphia Eagles"},
	{Abbreviation: "PIT", Name: "Pittsburgh Steelers"},
	{Abbreviation: "SF", Name: "San Francisco 49ers"},
	{Abbreviation: "SEA", Name: "Seattle Seahawks"},
	{Abbreviation: "TB", Name: "Tampa Bay Buccaneers"},
	{Abbreviation: "TEN", Name: "Tennessee Titans"},
	{Abbreviation: "WAS", Name: "Washington Commanders"},
}

type positionGroup struct {
	Position string
	Label    string
	Count    int
}

var samplePositionGroups = []positionGroup{
	{Position: "QB", Label: "Quarterback", Count: 12},
	{Position: "RB", Label: "Running Back", Count: 18},
	{Position: "WR", Label: "Wide Receiver", Count: 22},
	{Position: "TE", Label: "Tight End", Count: 8},
}

// seededPlayer carries both local and external identity into sample draft picks.
type seededPlayer struct {
	ID        int64
	SleeperID string
}

// weeklySample represents one generated weekly row and also serves as the
// accumulator used to produce the matching season summary.
type weeklySample struct {
	FantasyPoints       float64
	Targets             int
	Receptions          int
	RushingAttempts     int
	ReceivingYards      int
	RushingYards        int
	ReceivingTouchdowns int
	RushingTouchdowns   int
}

// SeedSampleData loads the complete fictional development dataset in one short
// transaction. Stable natural keys and upserts make repeated calls idempotent;
// a failure rolls back the entire seed operation.
func SeedSampleData(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sample seed: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	now := time.Now().UTC().Format(time.RFC3339)
	teamIDs, err := seedNFLTeams(ctx, tx)
	if err != nil {
		return err
	}
	players, err := seedPlayers(ctx, tx, teamIDs, now)
	if err != nil {
		return err
	}
	if err := seedTeamOdds(ctx, tx, teamIDs, now); err != nil {
		return err
	}
	if err := seedDraft(ctx, tx, players, now); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sample seed: %w", err)
	}
	return nil
}

func seedNFLTeams(ctx context.Context, tx *sql.Tx) (map[string]int64, error) {
	teamIDs := make(map[string]int64, len(sampleTeams))
	for _, team := range sampleTeams {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO nfl_teams (abbreviation, name)
			VALUES (?, ?)
			ON CONFLICT (abbreviation) DO UPDATE SET name = excluded.name
		`, team.Abbreviation, team.Name)
		if err != nil {
			return nil, fmt.Errorf("upsert NFL team %s: %w", team.Abbreviation, err)
		}

		var id int64
		if err := tx.QueryRowContext(ctx,
			"SELECT id FROM nfl_teams WHERE abbreviation = ?", team.Abbreviation,
		).Scan(&id); err != nil {
			return nil, fmt.Errorf("load NFL team %s: %w", team.Abbreviation, err)
		}
		teamIDs[team.Abbreviation] = id
	}
	return teamIDs, nil
}

func seedPlayers(
	ctx context.Context,
	tx *sql.Tx,
	teamIDs map[string]int64,
	now string,
) ([]seededPlayer, error) {
	players := make([]seededPlayer, 0, 60)
	playerNumber := 0

	for _, group := range samplePositionGroups {
		for positionRank := 1; positionRank <= group.Count; positionRank++ {
			playerNumber++
			team := sampleTeams[(playerNumber-1)%len(sampleTeams)]
			sleeperID := fmt.Sprintf("sample-player-%03d", playerNumber)
			birthDate := time.Date(
				1994+(playerNumber%8),
				time.Month(1+(playerNumber%12)),
				1+(playerNumber%28),
				0,
				0,
				0,
				0,
				time.UTC,
			).Format("2006-01-02")

			_, err := tx.ExecContext(ctx, `
				INSERT INTO players (
					sleeper_player_id,
					first_name,
					last_name,
					position,
					nfl_team_id,
					birth_date,
					active
				) VALUES (?, ?, ?, ?, ?, ?, 1)
				ON CONFLICT (sleeper_player_id) DO UPDATE SET
					first_name = excluded.first_name,
					last_name = excluded.last_name,
					position = excluded.position,
					nfl_team_id = excluded.nfl_team_id,
					birth_date = excluded.birth_date,
					active = excluded.active
			`,
				sleeperID,
				"Sample",
				fmt.Sprintf("%s %02d", group.Label, positionRank),
				group.Position,
				teamIDs[team.Abbreviation],
				birthDate,
			)
			if err != nil {
				return nil, fmt.Errorf("upsert sample player %s: %w", sleeperID, err)
			}

			var playerID int64
			if err := tx.QueryRowContext(ctx,
				"SELECT id FROM players WHERE sleeper_player_id = ?", sleeperID,
			).Scan(&playerID); err != nil {
				return nil, fmt.Errorf("load sample player %s: %w", sleeperID, err)
			}

			if err := seedPlayerStats(ctx, tx, playerID, group.Position, playerNumber); err != nil {
				return nil, err
			}
			if err := seedPlayerReferenceData(
				ctx,
				tx,
				playerID,
				playerNumber,
				positionRank,
				now,
			); err != nil {
				return nil, err
			}

			players = append(players, seededPlayer{ID: playerID, SleeperID: sleeperID})
		}
	}

	return players, nil
}

// seedPlayerStats writes eight weekly rows and derives the season row from the
// same values so player summaries remain consistent with weekly detail.
func seedPlayerStats(
	ctx context.Context,
	tx *sql.Tx,
	playerID int64,
	position string,
	playerNumber int,
) error {
	season := weeklySample{}
	for week := 1; week <= 8; week++ {
		stats := makeWeeklySample(position, playerNumber, week)
		season.FantasyPoints += stats.FantasyPoints
		season.Targets += stats.Targets
		season.Receptions += stats.Receptions
		season.RushingAttempts += stats.RushingAttempts
		season.ReceivingYards += stats.ReceivingYards
		season.RushingYards += stats.RushingYards
		season.ReceivingTouchdowns += stats.ReceivingTouchdowns
		season.RushingTouchdowns += stats.RushingTouchdowns

		_, err := tx.ExecContext(ctx, `
			INSERT INTO player_week_stats (
				player_id,
				season,
				week,
				fantasy_points_half_ppr,
				targets,
				receptions,
				rushing_attempts,
				receiving_yards,
				rushing_yards,
				receiving_touchdowns,
				rushing_touchdowns
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (player_id, season, week) DO UPDATE SET
				fantasy_points_half_ppr = excluded.fantasy_points_half_ppr,
				targets = excluded.targets,
				receptions = excluded.receptions,
				rushing_attempts = excluded.rushing_attempts,
				receiving_yards = excluded.receiving_yards,
				rushing_yards = excluded.rushing_yards,
				receiving_touchdowns = excluded.receiving_touchdowns,
				rushing_touchdowns = excluded.rushing_touchdowns
		`,
			playerID,
			sampleStatsSeason,
			week,
			stats.FantasyPoints,
			stats.Targets,
			stats.Receptions,
			stats.RushingAttempts,
			stats.ReceivingYards,
			stats.RushingYards,
			stats.ReceivingTouchdowns,
			stats.RushingTouchdowns,
		)
		if err != nil {
			return fmt.Errorf("upsert weekly stats for player %d week %d: %w", playerID, week, err)
		}
	}

	_, err := tx.ExecContext(ctx, `
		INSERT INTO player_season_stats (
			player_id,
			season,
			games_played,
			fantasy_points_half_ppr,
			targets,
			receptions,
			rushing_attempts,
			receiving_yards,
			rushing_yards,
			receiving_touchdowns,
			rushing_touchdowns
		) VALUES (?, ?, 8, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (player_id, season) DO UPDATE SET
			games_played = excluded.games_played,
			fantasy_points_half_ppr = excluded.fantasy_points_half_ppr,
			targets = excluded.targets,
			receptions = excluded.receptions,
			rushing_attempts = excluded.rushing_attempts,
			receiving_yards = excluded.receiving_yards,
			rushing_yards = excluded.rushing_yards,
			receiving_touchdowns = excluded.receiving_touchdowns,
			rushing_touchdowns = excluded.rushing_touchdowns
	`,
		playerID,
		sampleStatsSeason,
		season.FantasyPoints,
		season.Targets,
		season.Receptions,
		season.RushingAttempts,
		season.ReceivingYards,
		season.RushingYards,
		season.ReceivingTouchdowns,
		season.RushingTouchdowns,
	)
	if err != nil {
		return fmt.Errorf("upsert season stats for player %d: %w", playerID, err)
	}
	return nil
}

// makeWeeklySample produces deterministic, position-shaped values for UI
// development. These values are synthetic fixtures, not projections or advice.
func makeWeeklySample(position string, playerNumber, week int) weeklySample {
	stats := weeklySample{
		FantasyPoints: float64(60+((playerNumber*3+week*5)%140)) / 10,
	}

	switch position {
	case "QB":
		stats.RushingAttempts = 2 + ((playerNumber + week) % 5)
		stats.RushingYards = stats.RushingAttempts * (3 + (week % 3))
		stats.RushingTouchdowns = (playerNumber + week) % 2
	case "RB":
		stats.Targets = 2 + ((playerNumber + week) % 6)
		stats.Receptions = stats.Targets - 1
		stats.RushingAttempts = 9 + ((playerNumber + week*2) % 11)
		stats.ReceivingYards = stats.Receptions * (6 + (week % 4))
		stats.RushingYards = stats.RushingAttempts * (3 + (playerNumber % 3))
		stats.ReceivingTouchdowns = (playerNumber + week) % 2
		stats.RushingTouchdowns = (playerNumber + week + 1) % 2
	case "WR":
		stats.Targets = 5 + ((playerNumber + week) % 7)
		stats.Receptions = stats.Targets - 2
		stats.RushingAttempts = (playerNumber + week) % 3
		stats.ReceivingYards = stats.Receptions * (9 + (week % 5))
		stats.RushingYards = stats.RushingAttempts * 6
		stats.ReceivingTouchdowns = (playerNumber + week) % 2
	case "TE":
		stats.Targets = 3 + ((playerNumber + week) % 6)
		stats.Receptions = stats.Targets - 1
		stats.ReceivingYards = stats.Receptions * (7 + (week % 4))
		stats.ReceivingTouchdowns = (playerNumber + week) % 2
	}

	return stats
}

// seedPlayerReferenceData adds the three draft-year ADPs, an externally labeled
// sample tier, and a player touchdown line without implying recommendation logic.
func seedPlayerReferenceData(
	ctx context.Context,
	tx *sql.Tx,
	playerID int64,
	playerNumber int,
	positionRank int,
	now string,
) error {
	adpSources := []struct {
		Source string
		Offset float64
	}{
		{Source: "fantasypros", Offset: 0},
		{Source: "sleeper", Offset: 2.4},
		{Source: "underdog", Offset: -1.3},
	}
	baseADP := float64(playerNumber*2 - 1)
	for _, source := range adpSources {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO player_adp (player_id, season, source, adp, updated_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (player_id, season, source) DO UPDATE SET
				adp = excluded.adp,
				updated_at = excluded.updated_at
		`, playerID, sampleDataSeason, source.Source, baseADP+source.Offset, now)
		if err != nil {
			return fmt.Errorf("upsert %s ADP for player %d: %w", source.Source, playerID, err)
		}
	}

	tier := 1 + ((positionRank - 1) / 4)
	_, err := tx.ExecContext(ctx, `
		INSERT INTO player_tiers (player_id, season, source, tier, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (player_id, season, source) DO UPDATE SET
			tier = excluded.tier,
			updated_at = excluded.updated_at
	`, playerID, sampleDataSeason, sampleSource, tier, now)
	if err != nil {
		return fmt.Errorf("upsert tier for player %d: %w", playerID, err)
	}

	touchdownLine := 2.5 + (float64(playerNumber%13) * 0.5)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO odds (
			season,
			source,
			market,
			player_id,
			nfl_team_id,
			line,
			over_price,
			under_price,
			captured_at
		) VALUES (?, ?, 'total_touchdowns', ?, NULL, ?, -110, -110, ?)
		ON CONFLICT DO UPDATE SET
			line = excluded.line,
			over_price = excluded.over_price,
			under_price = excluded.under_price,
			captured_at = excluded.captured_at
	`, sampleDataSeason, sampleSource, playerID, touchdownLine, now)
	if err != nil {
		return fmt.Errorf("upsert touchdown odds for player %d: %w", playerID, err)
	}

	return nil
}

func seedTeamOdds(
	ctx context.Context,
	tx *sql.Tx,
	teamIDs map[string]int64,
	now string,
) error {
	for index, team := range sampleTeams {
		winLine := 5.5 + float64(index%7)
		_, err := tx.ExecContext(ctx, `
			INSERT INTO odds (
				season,
				source,
				market,
				player_id,
				nfl_team_id,
				line,
				over_price,
				under_price,
				captured_at
			) VALUES (?, ?, 'regular_season_wins', NULL, ?, ?, -110, -110, ?)
			ON CONFLICT DO UPDATE SET
				line = excluded.line,
				over_price = excluded.over_price,
				under_price = excluded.under_price,
				captured_at = excluded.captured_at
		`, sampleDataSeason, sampleSource, teamIDs[team.Abbreviation], winLine, now)
		if err != nil {
			return fmt.Errorf("upsert win odds for team %s: %w", team.Abbreviation, err)
		}
	}
	return nil
}

// seedDraft creates a manual sample draft with six picks. It intentionally does
// not update app_settings, so fictional data never becomes the active draft.
func seedDraft(ctx context.Context, tx *sql.Tx, players []seededPlayer, now string) error {
	const draftID = "sample-draft-2026"
	_, err := tx.ExecContext(ctx, `
		INSERT INTO drafts (
			sleeper_draft_id,
			sleeper_league_id,
			mode,
			status,
			created_at,
			updated_at
		) VALUES (?, 'sample-league-2026', 'manual', 'sample', ?, ?)
		ON CONFLICT (sleeper_draft_id) DO UPDATE SET
			sleeper_league_id = excluded.sleeper_league_id,
			mode = excluded.mode,
			status = excluded.status,
			updated_at = excluded.updated_at
	`, draftID, now, now)
	if err != nil {
		return fmt.Errorf("upsert sample draft: %w", err)
	}

	var localDraftID int64
	if err := tx.QueryRowContext(ctx,
		"SELECT id FROM drafts WHERE sleeper_draft_id = ?", draftID,
	).Scan(&localDraftID); err != nil {
		return fmt.Errorf("load sample draft: %w", err)
	}

	for index, player := range players[:6] {
		pickNumber := index + 1
		_, err := tx.ExecContext(ctx, `
			INSERT INTO draft_picks (
				draft_id,
				pick_number,
				round,
				draft_slot,
				roster_id,
				picked_by,
				sleeper_player_id,
				player_id,
				source,
				created_at
			) VALUES (?, ?, 1, ?, ?, ?, ?, ?, 'manual', ?)
			ON CONFLICT DO UPDATE SET
				round = excluded.round,
				draft_slot = excluded.draft_slot,
				roster_id = excluded.roster_id,
				picked_by = excluded.picked_by,
				sleeper_player_id = excluded.sleeper_player_id,
				player_id = excluded.player_id,
				source = excluded.source,
				created_at = excluded.created_at
		`,
			localDraftID,
			pickNumber,
			pickNumber,
			fmt.Sprintf("sample-roster-%02d", pickNumber),
			fmt.Sprintf("sample-user-%02d", pickNumber),
			player.SleeperID,
			player.ID,
			now,
		)
		if err != nil {
			return fmt.Errorf("upsert sample draft pick %d: %w", pickNumber, err)
		}
	}
	return nil
}
