package database

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"
)

// DraftSyncFailureMessage is safe to display while the poller keeps serving
// the last successfully persisted draft state.
const DraftSyncFailureMessage = "Sleeper is temporarily unavailable; showing the last known draft state."

// DraftPickInput is one validated Sleeper pick ready for transactional merge.
type DraftPickInput struct {
	PickNumber      int
	Round           int
	DraftSlot       int
	RosterID        string
	PickedBy        string
	SleeperPlayerID string
	FirstName       string
	LastName        string
	Position        string
	Team            string
}

// DraftPick is the persisted pick DTO returned as part of local draft state.
type DraftPick struct {
	ID              int64
	PickNumber      int
	Round           *int
	DraftSlot       *int
	RosterID        *string
	PickedBy        *string
	SleeperPlayerID string
	PlayerID        *int64
	Source          string
	PlayerFirstName *string
	PlayerLastName  *string
	PlayerPosition  *string
	PlayerTeam      *string
}

// DraftState combines active settings with the latest locally persisted picks.
// A configured draft may have no database row while its first sync is pending.
type DraftState struct {
	DraftID           string
	Mode              string
	PollingEnabled    bool
	LastSyncedAt      *string
	LastSyncError     *string
	Picks             []DraftPick
	TakenPlayerIDs    []int64
	UnknownSleeperIDs []string
}

type existingDraftPick struct {
	ID              int64
	PickNumber      int
	SleeperPlayerID string
	Source          string
}

// SyncDraftPicks reconciles the latest complete Sleeper response in one short
// transaction. Removed or corrected Sleeper picks disappear, future manual
// conflicts are replaced by official picks, and unknown player IDs are kept.
func SyncDraftPicks(
	ctx context.Context,
	db *sql.DB,
	draftID string,
	leagueID string,
	picks []DraftPickInput,
) (int, error) {
	if err := validateDraftPickInputs(draftID, picks); err != nil {
		return 0, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin draft sync: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC().Format(time.RFC3339)
	localDraftID, err := ensureDraft(ctx, tx, draftID, leagueID, now)
	if err != nil {
		return 0, err
	}

	existing, err := listExistingDraftPicks(ctx, tx, localDraftID)
	if err != nil {
		return 0, err
	}
	officialByPlayer := make(map[string]int, len(picks))
	officialByNumber := make(map[int]string, len(picks))
	for _, pick := range picks {
		officialByPlayer[pick.SleeperPlayerID] = pick.PickNumber
		officialByNumber[pick.PickNumber] = pick.SleeperPlayerID
	}
	for _, pick := range existing {
		officialNumber, playerExists := officialByPlayer[pick.SleeperPlayerID]
		officialPlayer, numberExists := officialByNumber[pick.PickNumber]
		remove := pick.Source == "sleeper" && (!playerExists || officialNumber != pick.PickNumber)
		remove = remove || pick.Source == "manual" && (playerExists || numberExists && officialPlayer != pick.SleeperPlayerID)
		if remove {
			if _, err := tx.ExecContext(ctx, `DELETE FROM draft_picks WHERE id = ?`, pick.ID); err != nil {
				return 0, fmt.Errorf("remove superseded draft pick: %w", err)
			}
		}
	}

	unknownCount := 0
	for _, pick := range picks {
		var playerID sql.NullInt64
		err := tx.QueryRowContext(ctx, `
			SELECT id FROM players WHERE sleeper_player_id = ?
		`, pick.SleeperPlayerID).Scan(&playerID)
		if err != nil && err != sql.ErrNoRows {
			return 0, fmt.Errorf("map Sleeper player %s: %w", pick.SleeperPlayerID, err)
		}
		if err == sql.ErrNoRows {
			playerID = sql.NullInt64{}
			if !isIntentionallyUnsupportedDraftPosition(pick.Position) {
				unknownCount++
			}
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO draft_picks (
				draft_id, pick_number, round, draft_slot, roster_id, picked_by,
				sleeper_player_id, player_id, source, created_at,
				player_first_name, player_last_name, player_position, player_team
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'sleeper', ?, ?, ?, ?, ?)
			ON CONFLICT (draft_id, sleeper_player_id) DO UPDATE SET
				pick_number = excluded.pick_number,
				round = excluded.round,
				draft_slot = excluded.draft_slot,
				roster_id = excluded.roster_id,
				picked_by = excluded.picked_by,
				player_id = excluded.player_id,
				source = 'sleeper',
				player_first_name = excluded.player_first_name,
				player_last_name = excluded.player_last_name,
				player_position = excluded.player_position,
				player_team = excluded.player_team
		`,
			localDraftID,
			pick.PickNumber,
			nullablePositiveInt(pick.Round),
			nullablePositiveInt(pick.DraftSlot),
			nullableText(pick.RosterID),
			nullableText(pick.PickedBy),
			pick.SleeperPlayerID,
			nullableInt64(playerID),
			now,
			nullableText(pick.FirstName),
			nullableText(pick.LastName),
			nullableText(pick.Position),
			nullableText(pick.Team),
		)
		if err != nil {
			return 0, fmt.Errorf("upsert draft pick %d: %w", pick.PickNumber, err)
		}
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE drafts
		SET status = 'active', last_synced_at = ?, last_sync_error = NULL, updated_at = ?
		WHERE id = ?
	`, now, now, localDraftID); err != nil {
		return 0, fmt.Errorf("record successful draft sync: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit draft sync: %w", err)
	}
	return unknownCount, nil
}

// RecordDraftSyncFailure marks one configured draft stale without changing its
// last successful timestamp or any persisted picks.
func RecordDraftSyncFailure(ctx context.Context, db *sql.DB, draftID, leagueID string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin failed draft sync: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC().Format(time.RFC3339)
	localDraftID, err := ensureDraft(ctx, tx, draftID, leagueID, now)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE drafts SET last_sync_error = ?, updated_at = ? WHERE id = ?
	`, DraftSyncFailureMessage, now, localDraftID); err != nil {
		return fmt.Errorf("record failed draft sync: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit failed draft sync: %w", err)
	}
	return nil
}

// GetDraftState loads the active draft named by app_settings and derives local
// taken-player and unknown-Sleeper-ID lists from its persisted picks.
func GetDraftState(ctx context.Context, db *sql.DB) (DraftState, error) {
	settings, err := GetSettings(ctx, db)
	if err != nil {
		return DraftState{}, err
	}
	state := DraftState{
		DraftID:           settings.SleeperDraftID,
		PollingEnabled:    settings.PollingEnabled,
		Picks:             make([]DraftPick, 0),
		TakenPlayerIDs:    make([]int64, 0),
		UnknownSleeperIDs: make([]string, 0),
	}
	if settings.SleeperDraftID == "" {
		return state, nil
	}

	var localDraftID int64
	var lastSyncedAt, lastSyncError sql.NullString
	err = db.QueryRowContext(ctx, `
		SELECT id, mode, last_synced_at, last_sync_error
		FROM drafts WHERE sleeper_draft_id = ?
	`, settings.SleeperDraftID).Scan(&localDraftID, &state.Mode, &lastSyncedAt, &lastSyncError)
	if err == sql.ErrNoRows {
		state.Mode = "live"
		return state, nil
	}
	if err != nil {
		return DraftState{}, fmt.Errorf("load active draft: %w", err)
	}
	state.LastSyncedAt = nullStringPointer(lastSyncedAt)
	state.LastSyncError = nullStringPointer(lastSyncError)

	rows, err := db.QueryContext(ctx, `
		SELECT
			draft_picks.id, draft_picks.pick_number, draft_picks.round,
			draft_picks.draft_slot, draft_picks.roster_id, draft_picks.picked_by,
			draft_picks.sleeper_player_id,
			COALESCE(draft_picks.player_id, mapped_player.id),
			draft_picks.source, draft_picks.player_first_name,
			draft_picks.player_last_name, draft_picks.player_position,
			draft_picks.player_team
		FROM draft_picks
		LEFT JOIN players AS mapped_player
			ON mapped_player.sleeper_player_id = draft_picks.sleeper_player_id
		WHERE draft_picks.draft_id = ?
		ORDER BY draft_picks.pick_number
	`, localDraftID)
	if err != nil {
		return DraftState{}, fmt.Errorf("list active draft picks: %w", err)
	}
	defer rows.Close()

	taken := make(map[int64]struct{})
	unknown := make(map[string]struct{})
	for rows.Next() {
		var pick DraftPick
		var round, draftSlot sql.NullInt64
		var rosterID, pickedBy, firstName, lastName, position, team sql.NullString
		var playerID sql.NullInt64
		if err := rows.Scan(
			&pick.ID, &pick.PickNumber, &round, &draftSlot, &rosterID, &pickedBy,
			&pick.SleeperPlayerID, &playerID, &pick.Source,
			&firstName, &lastName, &position, &team,
		); err != nil {
			return DraftState{}, fmt.Errorf("scan active draft pick: %w", err)
		}
		pick.Round = nullIntPointer(round)
		pick.DraftSlot = nullIntPointer(draftSlot)
		pick.RosterID = nullStringPointer(rosterID)
		pick.PickedBy = nullStringPointer(pickedBy)
		pick.PlayerFirstName = nullStringPointer(firstName)
		pick.PlayerLastName = nullStringPointer(lastName)
		pick.PlayerPosition = nullStringPointer(position)
		pick.PlayerTeam = nullStringPointer(team)
		if playerID.Valid {
			id := playerID.Int64
			pick.PlayerID = &id
			taken[id] = struct{}{}
		} else if !isIntentionallyUnsupportedDraftPosition(position.String) {
			unknown[pick.SleeperPlayerID] = struct{}{}
		}
		state.Picks = append(state.Picks, pick)
	}
	if err := rows.Err(); err != nil {
		return DraftState{}, fmt.Errorf("read active draft picks: %w", err)
	}
	for id := range taken {
		state.TakenPlayerIDs = append(state.TakenPlayerIDs, id)
	}
	for id := range unknown {
		state.UnknownSleeperIDs = append(state.UnknownSleeperIDs, id)
	}
	sort.Slice(state.TakenPlayerIDs, func(i, j int) bool {
		return state.TakenPlayerIDs[i] < state.TakenPlayerIDs[j]
	})
	sort.Strings(state.UnknownSleeperIDs)
	return state, nil
}

// isIntentionallyUnsupportedDraftPosition distinguishes expected unmapped
// kicker/defense picks from missing fantasy-position players worth surfacing.
func isIntentionallyUnsupportedDraftPosition(position string) bool {
	switch strings.ToUpper(strings.TrimSpace(position)) {
	case "K", "DEF", "DST", "D/ST":
		return true
	default:
		return false
	}
}

func ensureDraft(ctx context.Context, tx *sql.Tx, draftID, leagueID, now string) (int64, error) {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO drafts (
			sleeper_draft_id, sleeper_league_id, mode, status, created_at, updated_at
		) VALUES (?, ?, 'live', 'active', ?, ?)
		ON CONFLICT (sleeper_draft_id) DO UPDATE SET
			sleeper_league_id = excluded.sleeper_league_id,
			mode = 'live',
			updated_at = excluded.updated_at
	`, draftID, nullableText(leagueID), now, now)
	if err != nil {
		return 0, fmt.Errorf("ensure active draft: %w", err)
	}
	var id int64
	if err := tx.QueryRowContext(ctx, `
		SELECT id FROM drafts WHERE sleeper_draft_id = ?
	`, draftID).Scan(&id); err != nil {
		return 0, fmt.Errorf("load active draft ID: %w", err)
	}
	return id, nil
}

func listExistingDraftPicks(ctx context.Context, tx *sql.Tx, draftID int64) ([]existingDraftPick, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, pick_number, sleeper_player_id, source
		FROM draft_picks WHERE draft_id = ?
	`, draftID)
	if err != nil {
		return nil, fmt.Errorf("list existing draft picks: %w", err)
	}
	defer rows.Close()
	var picks []existingDraftPick
	for rows.Next() {
		var pick existingDraftPick
		if err := rows.Scan(&pick.ID, &pick.PickNumber, &pick.SleeperPlayerID, &pick.Source); err != nil {
			return nil, fmt.Errorf("scan existing draft pick: %w", err)
		}
		picks = append(picks, pick)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read existing draft picks: %w", err)
	}
	return picks, nil
}

func validateDraftPickInputs(draftID string, picks []DraftPickInput) error {
	if draftID == "" {
		return fmt.Errorf("sync draft picks: draft ID is required")
	}
	seenNumbers := make(map[int]struct{}, len(picks))
	seenPlayers := make(map[string]struct{}, len(picks))
	for _, pick := range picks {
		if pick.PickNumber <= 0 || pick.SleeperPlayerID == "" {
			return fmt.Errorf("sync draft picks: invalid pick")
		}
		if _, exists := seenNumbers[pick.PickNumber]; exists {
			return fmt.Errorf("sync draft picks: duplicate pick number %d", pick.PickNumber)
		}
		if _, exists := seenPlayers[pick.SleeperPlayerID]; exists {
			return fmt.Errorf("sync draft picks: duplicate Sleeper player %s", pick.SleeperPlayerID)
		}
		seenNumbers[pick.PickNumber] = struct{}{}
		seenPlayers[pick.SleeperPlayerID] = struct{}{}
	}
	return nil
}

func nullableText(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullablePositiveInt(value int) any {
	if value <= 0 {
		return nil
	}
	return value
}

func nullableInt64(value sql.NullInt64) any {
	if !value.Valid {
		return nil
	}
	return value.Int64
}
