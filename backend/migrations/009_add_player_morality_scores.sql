-- +goose Up

-- Morality scores are manually supplied subjective labels. Keeping them in a
-- separate source-dated table avoids treating the score as player identity or
-- as an application-generated recommendation.
CREATE TABLE player_morality_scores (
    player_id INTEGER NOT NULL,
    source TEXT NOT NULL,
    score INTEGER NOT NULL CHECK (score BETWEEN 0 AND 5),
    snapshot_date TEXT NOT NULL,
    imported_at TEXT NOT NULL,
    PRIMARY KEY (player_id, source),
    FOREIGN KEY (player_id) REFERENCES players (id) ON DELETE CASCADE
);

-- +goose Down

DROP TABLE IF EXISTS player_morality_scores;
