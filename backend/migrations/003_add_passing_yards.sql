-- +goose Up

-- Passing yards support the position-specific quarterback trend chart. The
-- default preserves existing season and weekly rows until their source data is
-- refreshed or the sample seed is run again.
ALTER TABLE player_season_stats ADD COLUMN passing_yards INTEGER NOT NULL DEFAULT 0;
ALTER TABLE player_week_stats ADD COLUMN passing_yards INTEGER NOT NULL DEFAULT 0;

-- +goose Down

ALTER TABLE player_week_stats DROP COLUMN passing_yards;
ALTER TABLE player_season_stats DROP COLUMN passing_yards;
