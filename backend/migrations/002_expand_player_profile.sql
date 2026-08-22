-- +goose Up

-- Sleeper profile fields retained for draft-day context and potential future
-- cross-provider matching. All additions are nullable so existing players are
-- preserved and incomplete upstream profiles remain valid.
ALTER TABLE players ADD COLUMN status TEXT;
ALTER TABLE players ADD COLUMN number INTEGER;
ALTER TABLE players ADD COLUMN college TEXT;
ALTER TABLE players ADD COLUMN height TEXT;
ALTER TABLE players ADD COLUMN weight TEXT;
ALTER TABLE players ADD COLUMN birth_country TEXT;
ALTER TABLE players ADD COLUMN years_exp INTEGER;
ALTER TABLE players ADD COLUMN depth_chart_position TEXT;
ALTER TABLE players ADD COLUMN depth_chart_order INTEGER;
ALTER TABLE players ADD COLUMN injury_status TEXT;
ALTER TABLE players ADD COLUMN injury_start_date TEXT;
ALTER TABLE players ADD COLUMN practice_participation TEXT;
ALTER TABLE players ADD COLUMN espn_id TEXT;
ALTER TABLE players ADD COLUMN sportradar_id TEXT;
ALTER TABLE players ADD COLUMN rotowire_id TEXT;
ALTER TABLE players ADD COLUMN rotoworld_id TEXT;
ALTER TABLE players ADD COLUMN yahoo_id TEXT;
ALTER TABLE players ADD COLUMN fantasy_data_id TEXT;
ALTER TABLE players ADD COLUMN stats_id TEXT;

-- +goose Down

ALTER TABLE players DROP COLUMN stats_id;
ALTER TABLE players DROP COLUMN fantasy_data_id;
ALTER TABLE players DROP COLUMN yahoo_id;
ALTER TABLE players DROP COLUMN rotoworld_id;
ALTER TABLE players DROP COLUMN rotowire_id;
ALTER TABLE players DROP COLUMN sportradar_id;
ALTER TABLE players DROP COLUMN espn_id;
ALTER TABLE players DROP COLUMN practice_participation;
ALTER TABLE players DROP COLUMN injury_start_date;
ALTER TABLE players DROP COLUMN injury_status;
ALTER TABLE players DROP COLUMN depth_chart_order;
ALTER TABLE players DROP COLUMN depth_chart_position;
ALTER TABLE players DROP COLUMN years_exp;
ALTER TABLE players DROP COLUMN birth_country;
ALTER TABLE players DROP COLUMN weight;
ALTER TABLE players DROP COLUMN height;
ALTER TABLE players DROP COLUMN college;
ALTER TABLE players DROP COLUMN number;
ALTER TABLE players DROP COLUMN status;
