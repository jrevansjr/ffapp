-- +goose Up

-- FantasyPros draft rankings complement market ADP with expert consensus and
-- disagreement. Position rank is numeric because the player's position is
-- already stored on players and numeric sorting is useful in the UI.
CREATE TABLE player_rankings (
    player_id INTEGER NOT NULL,
    season INTEGER NOT NULL,
    source TEXT NOT NULL,
    overall_rank INTEGER NOT NULL CHECK (overall_rank > 0),
    position_rank INTEGER NOT NULL CHECK (position_rank > 0),
    rank_min INTEGER NOT NULL CHECK (rank_min > 0),
    rank_max INTEGER NOT NULL CHECK (rank_max > 0),
    rank_std_dev REAL NOT NULL CHECK (rank_std_dev >= 0),
    updated_at TEXT NOT NULL,
    PRIMARY KEY (player_id, season, source),
    FOREIGN KEY (player_id) REFERENCES players (id) ON DELETE CASCADE
);

-- +goose Down

DROP TABLE IF EXISTS player_rankings;
