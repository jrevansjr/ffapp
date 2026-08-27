-- +goose Up

-- FantasyPros is the approved source gateway for ADP and likely the next tier
-- import. Persisting its player ID makes both joins exact and reusable.
ALTER TABLE players ADD COLUMN fantasypros_id TEXT;

CREATE UNIQUE INDEX idx_players_fantasypros_id
    ON players (fantasypros_id)
    WHERE fantasypros_id IS NOT NULL;

-- +goose Down

DROP INDEX IF EXISTS idx_players_fantasypros_id;
ALTER TABLE players DROP COLUMN fantasypros_id;
