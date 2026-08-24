package importer

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jrevansjr/ffapp/backend/internal/sleeper"
)

var fantasyPositions = map[string]struct{}{
	"QB": {},
	"RB": {},
	"WR": {},
	"TE": {},
}

// PlayerSummary describes the observable result of one idempotent player load.
type PlayerSummary struct {
	Fetched       int
	Eligible      int
	Inserted      int
	Updated       int
	Deactivated   int
	Skipped       int
	UnmappedTeams int
}

// LoadPlayers reconciles active fantasy-position players from one validated
// Sleeper response. Missing former players are retained but marked inactive.
func LoadPlayers(
	ctx context.Context,
	db *sql.DB,
	players sleeper.PlayersResponse,
	importedAt time.Time,
) (PlayerSummary, error) {
	summary := PlayerSummary{Fetched: len(players)}
	candidates := eligiblePlayers(players, &summary)
	if len(candidates) == 0 {
		return summary, fmt.Errorf("Sleeper response contains no active QB/RB/WR/TE players")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return summary, fmt.Errorf("begin player import: %w", err)
	}
	defer tx.Rollback()

	teamIDs, err := loadTeamIDs(ctx, tx)
	if err != nil {
		return summary, err
	}
	existingActive, err := loadExistingPlayerActivity(ctx, tx)
	if err != nil {
		return summary, err
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE players
		SET active = 0
		WHERE sleeper_player_id IS NOT NULL AND position IN ('QB', 'RB', 'WR', 'TE')
	`); err != nil {
		return summary, fmt.Errorf("deactivate prior Sleeper players: %w", err)
	}

	for sleeperID, player := range candidates {
		teamID := nullableTeamID(player.Team, teamIDs)
		if player.Team.Valid && teamID == nil {
			summary.UnmappedTeams++
		}
		_, existed := existingActive[sleeperID]
		if existed {
			summary.Updated++
		} else {
			summary.Inserted++
		}
		if err := upsertPlayer(ctx, tx, sleeperID, player, teamID); err != nil {
			return summary, err
		}
	}
	for sleeperID, wasActive := range existingActive {
		if _, found := candidates[sleeperID]; wasActive && !found {
			summary.Deactivated++
		}
	}

	timestamp := importedAt.UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `
		UPDATE app_settings
		SET players_synced_at = ?, updated_at = ?
		WHERE id = 1
	`, timestamp, timestamp); err != nil {
		return summary, fmt.Errorf("record player import time: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return summary, fmt.Errorf("commit player import: %w", err)
	}
	return summary, nil
}

func eligiblePlayers(
	players sleeper.PlayersResponse,
	summary *PlayerSummary,
) sleeper.PlayersResponse {
	eligible := make(sleeper.PlayersResponse)
	for mapID, player := range players {
		position := strings.ToUpper(strings.TrimSpace(player.Position.Value))
		if !player.Active {
			continue
		}
		if _, allowed := fantasyPositions[position]; !allowed {
			continue
		}
		sleeperID := strings.TrimSpace(mapID)
		if sleeperID == "" && player.PlayerID.Valid {
			sleeperID = strings.TrimSpace(player.PlayerID.Value)
		}
		firstName, lastName := playerNames(player)
		if sleeperID == "" || firstName == "" || lastName == "" {
			summary.Skipped++
			continue
		}
		player.FirstName = sleeper.StringValue{Value: firstName, Valid: true}
		player.LastName = sleeper.StringValue{Value: lastName, Valid: true}
		player.Position = sleeper.StringValue{Value: position, Valid: true}
		eligible[sleeperID] = player
	}
	summary.Eligible = len(eligible)
	return eligible
}

func playerNames(player sleeper.Player) (string, string) {
	firstName := strings.TrimSpace(player.FirstName.Value)
	lastName := strings.TrimSpace(player.LastName.Value)
	if firstName != "" && lastName != "" {
		return firstName, lastName
	}
	parts := strings.Fields(player.FullName.Value)
	if len(parts) < 2 {
		return firstName, lastName
	}
	if firstName == "" {
		firstName = parts[0]
	}
	if lastName == "" {
		lastName = strings.Join(parts[1:], " ")
	}
	return firstName, lastName
}

func loadTeamIDs(ctx context.Context, tx *sql.Tx) (map[string]int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, abbreviation FROM nfl_teams`)
	if err != nil {
		return nil, fmt.Errorf("load NFL teams for player import: %w", err)
	}
	defer rows.Close()
	ids := make(map[string]int64)
	for rows.Next() {
		var id int64
		var abbreviation string
		if err := rows.Scan(&id, &abbreviation); err != nil {
			return nil, fmt.Errorf("scan NFL team for player import: %w", err)
		}
		ids[abbreviation] = id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read NFL teams for player import: %w", err)
	}
	return ids, nil
}

func loadExistingPlayerActivity(ctx context.Context, tx *sql.Tx) (map[string]bool, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT sleeper_player_id, active
		FROM players
		WHERE sleeper_player_id IS NOT NULL AND position IN ('QB', 'RB', 'WR', 'TE')
	`)
	if err != nil {
		return nil, fmt.Errorf("load existing Sleeper players: %w", err)
	}
	defer rows.Close()
	existing := make(map[string]bool)
	for rows.Next() {
		var sleeperID string
		var active int
		if err := rows.Scan(&sleeperID, &active); err != nil {
			return nil, fmt.Errorf("scan existing Sleeper player: %w", err)
		}
		existing[sleeperID] = active == 1
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read existing Sleeper players: %w", err)
	}
	return existing, nil
}

func nullableTeamID(team sleeper.StringValue, ids map[string]int64) any {
	if !team.Valid {
		return nil
	}
	abbreviation := strings.ToUpper(strings.TrimSpace(team.Value))
	if abbreviation == "" {
		return nil
	}
	id, ok := ids[abbreviation]
	if !ok {
		return nil
	}
	return id
}

func upsertPlayer(
	ctx context.Context,
	tx *sql.Tx,
	sleeperID string,
	player sleeper.Player,
	teamID any,
) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO players (
			sleeper_player_id, first_name, last_name, position, nfl_team_id,
			birth_date, active, status, number, college, height, weight,
			birth_country, years_exp, depth_chart_position, depth_chart_order,
			injury_status, injury_start_date, practice_participation, espn_id,
			sportradar_id, rotowire_id, rotoworld_id, yahoo_id, fantasy_data_id,
			stats_id, gsis_id
		) VALUES (
			?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
		)
		ON CONFLICT (sleeper_player_id) DO UPDATE SET
			first_name = excluded.first_name,
			last_name = excluded.last_name,
			position = excluded.position,
			nfl_team_id = excluded.nfl_team_id,
			birth_date = excluded.birth_date,
			active = 1,
			status = excluded.status,
			number = excluded.number,
			college = excluded.college,
			height = excluded.height,
			weight = excluded.weight,
			birth_country = excluded.birth_country,
			years_exp = excluded.years_exp,
			depth_chart_position = excluded.depth_chart_position,
			depth_chart_order = excluded.depth_chart_order,
			injury_status = excluded.injury_status,
			injury_start_date = excluded.injury_start_date,
			practice_participation = excluded.practice_participation,
			espn_id = excluded.espn_id,
			sportradar_id = excluded.sportradar_id,
			rotowire_id = excluded.rotowire_id,
			rotoworld_id = excluded.rotoworld_id,
			yahoo_id = excluded.yahoo_id,
			fantasy_data_id = excluded.fantasy_data_id,
			stats_id = excluded.stats_id,
			gsis_id = COALESCE(excluded.gsis_id, players.gsis_id)
	`,
		sleeperID,
		player.FirstName.Value,
		player.LastName.Value,
		player.Position.Value,
		teamID,
		nullableString(player.BirthDate),
		nullableString(player.Status),
		nullableInt(player.Number),
		nullableString(player.College),
		nullableString(player.Height),
		nullableString(player.Weight),
		nullableString(player.BirthCountry),
		nullableInt(player.YearsExp),
		nullableString(player.DepthChartPosition),
		nullableInt(player.DepthChartOrder),
		nullableString(player.InjuryStatus),
		nullableString(player.InjuryStartDate),
		nullableString(player.PracticeParticipation),
		nullableString(player.ESPNID),
		nullableString(player.SportradarID),
		nullableString(player.RotowireID),
		nullableString(player.RotoworldID),
		nullableString(player.YahooID),
		nullableString(player.FantasyDataID),
		nullableString(player.StatsID),
		nullableString(player.GSISID),
	)
	if err != nil {
		return fmt.Errorf("upsert Sleeper player %s: %w", sleeperID, err)
	}
	return nil
}

func nullableString(value sleeper.StringValue) any {
	if !value.Valid || strings.TrimSpace(value.Value) == "" {
		return nil
	}
	return strings.TrimSpace(value.Value)
}

func nullableInt(value sleeper.IntValue) any {
	if !value.Valid {
		return nil
	}
	return value.Value
}
