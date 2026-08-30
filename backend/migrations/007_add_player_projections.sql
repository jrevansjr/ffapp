-- +goose Up
-- FantasyPros preseason projections are forecasts, not historical results or
-- sportsbook lines, so they live in their own season/source table.
CREATE TABLE player_projections (
    player_id INTEGER NOT NULL,
    season INTEGER NOT NULL,
    source TEXT NOT NULL,
    passing_yards REAL,
    passing_touchdowns REAL,
    rushing_yards REAL,
    rushing_touchdowns REAL,
    receiving_yards REAL,
    receiving_touchdowns REAL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (player_id, season, source),
    FOREIGN KEY (player_id) REFERENCES players (id) ON DELETE CASCADE
);

CREATE INDEX idx_player_projections_season_source
    ON player_projections (season, source);

-- +goose Down
DROP INDEX IF EXISTS idx_player_projections_season_source;
DROP TABLE IF EXISTS player_projections;
