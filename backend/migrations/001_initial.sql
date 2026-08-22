-- +goose Up

-- Initial normalized storage for player identity, season/source reference data,
-- application settings, and per-draft picks. Availability is intentionally
-- derived from draft_picks; it is never stored as a player property.

CREATE TABLE nfl_teams (
    id INTEGER PRIMARY KEY,
    abbreviation TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL
);

CREATE TABLE players (
    id INTEGER PRIMARY KEY,
    sleeper_player_id TEXT UNIQUE,
    first_name TEXT NOT NULL,
    last_name TEXT NOT NULL,
    position TEXT NOT NULL,
    nfl_team_id INTEGER,
    birth_date TEXT,
    active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0, 1)),
    FOREIGN KEY (nfl_team_id) REFERENCES nfl_teams (id)
);

CREATE INDEX idx_players_position ON players (position);
CREATE INDEX idx_players_nfl_team_id ON players (nfl_team_id);

CREATE TABLE player_season_stats (
    player_id INTEGER NOT NULL,
    season INTEGER NOT NULL,
    games_played INTEGER NOT NULL,
    fantasy_points_half_ppr REAL NOT NULL,
    targets INTEGER NOT NULL,
    receptions INTEGER NOT NULL,
    rushing_attempts INTEGER NOT NULL,
    receiving_yards INTEGER NOT NULL,
    rushing_yards INTEGER NOT NULL,
    receiving_touchdowns INTEGER NOT NULL,
    rushing_touchdowns INTEGER NOT NULL,
    PRIMARY KEY (player_id, season),
    FOREIGN KEY (player_id) REFERENCES players (id) ON DELETE CASCADE
);

CREATE TABLE player_week_stats (
    player_id INTEGER NOT NULL,
    season INTEGER NOT NULL,
    week INTEGER NOT NULL,
    fantasy_points_half_ppr REAL NOT NULL,
    targets INTEGER NOT NULL,
    receptions INTEGER NOT NULL,
    rushing_attempts INTEGER NOT NULL,
    receiving_yards INTEGER NOT NULL,
    rushing_yards INTEGER NOT NULL,
    receiving_touchdowns INTEGER NOT NULL,
    rushing_touchdowns INTEGER NOT NULL,
    PRIMARY KEY (player_id, season, week),
    FOREIGN KEY (player_id) REFERENCES players (id) ON DELETE CASCADE
);

CREATE TABLE player_adp (
    player_id INTEGER NOT NULL,
    season INTEGER NOT NULL,
    source TEXT NOT NULL,
    adp REAL NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (player_id, season, source),
    FOREIGN KEY (player_id) REFERENCES players (id) ON DELETE CASCADE
);

CREATE TABLE player_tiers (
    player_id INTEGER NOT NULL,
    season INTEGER NOT NULL,
    source TEXT NOT NULL,
    tier INTEGER NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (player_id, season, source),
    FOREIGN KEY (player_id) REFERENCES players (id) ON DELETE CASCADE
);

-- Odds share one table because both player and NFL-team markets carry the same
-- season/source/capture metadata. The check keeps each row tied to one subject.
CREATE TABLE odds (
    id INTEGER PRIMARY KEY,
    season INTEGER NOT NULL,
    source TEXT NOT NULL,
    market TEXT NOT NULL,
    player_id INTEGER,
    nfl_team_id INTEGER,
    line REAL NOT NULL,
    over_price REAL,
    under_price REAL,
    captured_at TEXT NOT NULL,
    CHECK (
        (player_id IS NOT NULL AND nfl_team_id IS NULL)
        OR (player_id IS NULL AND nfl_team_id IS NOT NULL)
    ),
    FOREIGN KEY (player_id) REFERENCES players (id) ON DELETE CASCADE,
    FOREIGN KEY (nfl_team_id) REFERENCES nfl_teams (id) ON DELETE CASCADE
);

-- SQLite permits repeated NULLs in unique indexes, so partial indexes provide
-- stable upsert identities separately for player and NFL-team markets.
CREATE UNIQUE INDEX idx_odds_player_unique
    ON odds (player_id, season, source, market)
    WHERE player_id IS NOT NULL;
CREATE UNIQUE INDEX idx_odds_nfl_team_unique
    ON odds (nfl_team_id, season, source, market)
    WHERE nfl_team_id IS NOT NULL;
CREATE INDEX idx_odds_player_market ON odds (player_id, season, market);
CREATE INDEX idx_odds_nfl_team_market ON odds (nfl_team_id, season, market);

-- Drafts own event metadata. Their picks preserve the raw Sleeper player ID
-- even when player_id cannot be mapped to local identity data.
CREATE TABLE drafts (
    id INTEGER PRIMARY KEY,
    sleeper_draft_id TEXT UNIQUE,
    sleeper_league_id TEXT,
    mode TEXT NOT NULL CHECK (mode IN ('live', 'manual')),
    status TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE draft_picks (
    id INTEGER PRIMARY KEY,
    draft_id INTEGER NOT NULL,
    pick_number INTEGER NOT NULL,
    round INTEGER,
    draft_slot INTEGER,
    roster_id TEXT,
    picked_by TEXT,
    sleeper_player_id TEXT NOT NULL,
    player_id INTEGER,
    source TEXT NOT NULL CHECK (source IN ('sleeper', 'manual')),
    created_at TEXT NOT NULL,
    UNIQUE (draft_id, pick_number),
    UNIQUE (draft_id, sleeper_player_id),
    FOREIGN KEY (draft_id) REFERENCES drafts (id) ON DELETE CASCADE,
    FOREIGN KEY (player_id) REFERENCES players (id) ON DELETE SET NULL
);

-- Application configuration is a singleton populated by database.Open after
-- migrations complete; later Admin updates retain the same row.
CREATE TABLE app_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    sleeper_username TEXT NOT NULL DEFAULT '',
    sleeper_league_id TEXT NOT NULL DEFAULT '',
    sleeper_draft_id TEXT NOT NULL DEFAULT '',
    polling_enabled INTEGER NOT NULL DEFAULT 1 CHECK (polling_enabled IN (0, 1)),
    polling_interval_ms INTEGER NOT NULL DEFAULT 2000 CHECK (polling_interval_ms > 0),
    players_synced_at TEXT,
    updated_at TEXT NOT NULL
);

-- +goose Down

DROP TABLE IF EXISTS app_settings;
DROP TABLE IF EXISTS draft_picks;
DROP TABLE IF EXISTS drafts;
DROP INDEX IF EXISTS idx_odds_nfl_team_market;
DROP INDEX IF EXISTS idx_odds_player_market;
DROP INDEX IF EXISTS idx_odds_nfl_team_unique;
DROP INDEX IF EXISTS idx_odds_player_unique;
DROP TABLE IF EXISTS odds;
DROP TABLE IF EXISTS player_tiers;
DROP TABLE IF EXISTS player_adp;
DROP TABLE IF EXISTS player_week_stats;
DROP TABLE IF EXISTS player_season_stats;
DROP INDEX IF EXISTS idx_players_nfl_team_id;
DROP INDEX IF EXISTS idx_players_position;
DROP TABLE IF EXISTS players;
DROP TABLE IF EXISTS nfl_teams;
