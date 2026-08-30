-- +goose Up

-- Sync metadata belongs to the draft so old draft history remains inspectable
-- when the active draft setting changes.
ALTER TABLE drafts ADD COLUMN last_synced_at TEXT;
ALTER TABLE drafts ADD COLUMN last_sync_error TEXT;

-- Pick metadata lets unknown Sleeper player IDs remain useful in the UI while
-- preserving the nullable exact mapping to the local players table.
ALTER TABLE draft_picks ADD COLUMN player_first_name TEXT;
ALTER TABLE draft_picks ADD COLUMN player_last_name TEXT;
ALTER TABLE draft_picks ADD COLUMN player_position TEXT;
ALTER TABLE draft_picks ADD COLUMN player_team TEXT;

-- +goose Down

ALTER TABLE draft_picks DROP COLUMN player_team;
ALTER TABLE draft_picks DROP COLUMN player_position;
ALTER TABLE draft_picks DROP COLUMN player_last_name;
ALTER TABLE draft_picks DROP COLUMN player_first_name;
ALTER TABLE drafts DROP COLUMN last_sync_error;
ALTER TABLE drafts DROP COLUMN last_synced_at;
