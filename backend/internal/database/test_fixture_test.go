package database

import (
	"database/sql"
	"testing"
)

// loadPlayerQueryFixture creates only the rows needed to exercise database
// reads. It is deliberately test-only and never acts as application seed data.
func loadPlayerQueryFixture(t *testing.T, db *sql.DB) {
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
		INSERT INTO odds (
			season, source, market, player_id, line, captured_at
		) VALUES (2026, 'fixture', 'total_touchdowns', 1, 1.5, '2026-08-01T00:00:00Z');
		INSERT INTO odds (
			season, source, market, nfl_team_id, line, captured_at
		) VALUES (2026, 'fixture', 'regular_season_wins', 1, 8.5, '2026-08-01T00:00:00Z');

		INSERT INTO drafts (
			id, sleeper_draft_id, mode, status, created_at, updated_at
		) VALUES (1, 'fixture-draft', 'live', 'mock', '2026-08-01T00:00:00Z', '2026-08-01T00:00:00Z');
		INSERT INTO draft_picks (
			draft_id, pick_number, sleeper_player_id, player_id, source, created_at
		) VALUES (1, 1, 'fixture-qb', 1, 'sleeper', '2026-08-01T00:00:00Z');
	`)
	if err != nil {
		t.Fatalf("load player query fixture: %v", err)
	}
}
