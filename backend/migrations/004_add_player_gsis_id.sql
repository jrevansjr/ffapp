-- +goose Up

-- GSIS is nflverse's primary player identifier. Sleeper supplies it for many
-- players, so retaining it now gives the stats importer an exact join in M6.2.
ALTER TABLE players ADD COLUMN gsis_id TEXT;

CREATE UNIQUE INDEX idx_players_gsis_id
    ON players (gsis_id)
    WHERE gsis_id IS NOT NULL;

-- +goose Down

DROP INDEX IF EXISTS idx_players_gsis_id;
ALTER TABLE players DROP COLUMN gsis_id;
