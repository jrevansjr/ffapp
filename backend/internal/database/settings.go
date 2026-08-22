package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Settings is the persistent singleton configuration returned by the API.
type Settings struct {
	SleeperUsername string
	SleeperLeagueID string
	SleeperDraftID  string
	PollingEnabled  bool
	PollingInterval int
	PlayersSyncedAt *string
	UpdatedAt       string
}

// EditableSettings contains the settings a user may change through Admin.
type EditableSettings struct {
	SleeperUsername string
	SleeperLeagueID string
	SleeperDraftID  string
	PollingEnabled  bool
	PollingInterval int
}

// GetSettings returns the id = 1 settings row established during database open.
func GetSettings(ctx context.Context, db *sql.DB) (Settings, error) {
	var (
		settings        Settings
		pollingEnabled  int
		playersSyncedAt sql.NullString
	)
	err := db.QueryRowContext(ctx, `
		SELECT
			sleeper_username,
			sleeper_league_id,
			sleeper_draft_id,
			polling_enabled,
			polling_interval_ms,
			players_synced_at,
			updated_at
		FROM app_settings
		WHERE id = 1
	`).Scan(
		&settings.SleeperUsername,
		&settings.SleeperLeagueID,
		&settings.SleeperDraftID,
		&pollingEnabled,
		&settings.PollingInterval,
		&playersSyncedAt,
		&settings.UpdatedAt,
	)
	if err != nil {
		return Settings{}, fmt.Errorf("get settings: %w", err)
	}
	settings.PollingEnabled = pollingEnabled == 1
	settings.PlayersSyncedAt = nullStringPointer(playersSyncedAt)
	return settings, nil
}

// UpdateSettings replaces user-editable settings, preserves sync metadata, and
// returns the saved row with a new UTC update timestamp.
func UpdateSettings(ctx context.Context, db *sql.DB, input EditableSettings) (Settings, error) {
	updatedAt := time.Now().UTC().Format(time.RFC3339)
	_, err := db.ExecContext(ctx, `
		UPDATE app_settings
		SET
			sleeper_username = ?,
			sleeper_league_id = ?,
			sleeper_draft_id = ?,
			polling_enabled = ?,
			polling_interval_ms = ?,
			updated_at = ?
		WHERE id = 1
	`,
		input.SleeperUsername,
		input.SleeperLeagueID,
		input.SleeperDraftID,
		boolInt(input.PollingEnabled),
		input.PollingInterval,
		updatedAt,
	)
	if err != nil {
		return Settings{}, fmt.Errorf("update settings: %w", err)
	}
	return GetSettings(ctx, db)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
