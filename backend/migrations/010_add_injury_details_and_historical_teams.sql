-- +goose Up

-- Sleeper supplies these optional details with the existing injury designation.
ALTER TABLE players ADD COLUMN injury_body_part TEXT;
ALTER TABLE players ADD COLUMN injury_notes TEXT;

-- Historical team affiliation belongs to the source week, not the player's
-- current profile, because a player may represent multiple teams in a season.
ALTER TABLE player_week_stats ADD COLUMN nfl_team_id INTEGER REFERENCES nfl_teams (id);

CREATE INDEX idx_player_week_stats_nfl_team_id
    ON player_week_stats (nfl_team_id, season, week);

-- +goose Down

DROP INDEX IF EXISTS idx_player_week_stats_nfl_team_id;
ALTER TABLE player_week_stats DROP COLUMN nfl_team_id;
ALTER TABLE players DROP COLUMN injury_notes;
ALTER TABLE players DROP COLUMN injury_body_part;
