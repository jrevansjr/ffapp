package database

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestManualPickLifecycleAndDraftIsolation(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "draft.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`
		INSERT INTO nfl_teams (id, abbreviation, name)
		VALUES (1, 'BUF', 'Buffalo Bills');
		INSERT INTO players (
			id, sleeper_player_id, first_name, last_name, position, nfl_team_id
		) VALUES
			(1, 'manual-one', 'Manual', 'One', 'WR', 1),
			(2, 'manual-two', 'Manual', 'Two', 'RB', 1),
			(3, 'official', 'Official', 'Player', 'QB', 1);
	`); err != nil {
		t.Fatalf("insert manual-pick fixture: %v", err)
	}
	settings, err := UpdateSettings(context.Background(), db, EditableSettings{
		SleeperDraftID: "draft-a", PollingEnabled: false, PollingInterval: 2000,
	})
	if err != nil {
		t.Fatalf("activate draft-a: %v", err)
	}
	if _, err := SyncDraftPicks(context.Background(), db, settings.SleeperDraftID, "", []DraftPickInput{
		{PickNumber: 5, SleeperPlayerID: "official", Position: "QB"},
	}); err != nil {
		t.Fatalf("sync official fixture: %v", err)
	}

	first, err := CreateManualPick(context.Background(), db, 1)
	if err != nil {
		t.Fatalf("CreateManualPick(first) error = %v", err)
	}
	if first.PickNumber != 6 || first.Source != "manual" || first.PlayerTeam == nil ||
		*first.PlayerTeam != "BUF" {
		t.Fatalf("first manual pick = %#v", first)
	}
	second, err := CreateManualPick(context.Background(), db, 2)
	if err != nil {
		t.Fatalf("CreateManualPick(second) error = %v", err)
	}
	if second.PickNumber != 7 {
		t.Fatalf("second manual pick number = %d, want 7", second.PickNumber)
	}
	if _, err := CreateManualPick(context.Background(), db, 1); !errors.Is(err, ErrPlayerAlreadyTaken) {
		t.Fatalf("duplicate CreateManualPick() error = %v, want ErrPlayerAlreadyTaken", err)
	}

	deleted, err := DeleteManualPick(context.Background(), db, second.ID)
	if err != nil {
		t.Fatalf("DeleteManualPick() error = %v", err)
	}
	if deleted.ID != second.ID || deleted.SleeperPlayerID != "manual-two" {
		t.Fatalf("deleted manual pick = %#v", deleted)
	}
	state, err := GetDraftState(context.Background(), db)
	if err != nil {
		t.Fatalf("GetDraftState() after undo error = %v", err)
	}
	if len(state.Picks) != 2 || len(state.TakenPlayerIDs) != 2 {
		t.Fatalf("state after undo = %#v", state)
	}
	var officialPickID int64
	if err := db.QueryRow(`
		SELECT draft_picks.id
		FROM draft_picks
		JOIN drafts ON drafts.id = draft_picks.draft_id
		WHERE drafts.sleeper_draft_id = 'draft-a' AND draft_picks.source = 'sleeper'
	`).Scan(&officialPickID); err != nil {
		t.Fatalf("load official pick ID: %v", err)
	}
	if _, err := DeleteManualPick(context.Background(), db, officialPickID); !errors.Is(err, ErrManualPickNotFound) {
		t.Fatalf("delete official pick error = %v, want ErrManualPickNotFound", err)
	}

	if _, err := UpdateSettings(context.Background(), db, EditableSettings{
		SleeperDraftID: "draft-b", PollingEnabled: false, PollingInterval: 2000,
	}); err != nil {
		t.Fatalf("activate draft-b: %v", err)
	}
	if _, err := DeleteManualPick(context.Background(), db, first.ID); !errors.Is(err, ErrManualPickNotFound) {
		t.Fatalf("delete inactive-draft pick error = %v, want ErrManualPickNotFound", err)
	}
	draftBPick, err := CreateManualPick(context.Background(), db, 1)
	if err != nil {
		t.Fatalf("CreateManualPick(draft-b) error = %v", err)
	}
	if draftBPick.PickNumber != 1 {
		t.Fatalf("draft-b pick number = %d, want 1", draftBPick.PickNumber)
	}

	if _, err := UpdateSettings(context.Background(), db, EditableSettings{
		SleeperDraftID: "draft-a", PollingEnabled: false, PollingInterval: 2000,
	}); err != nil {
		t.Fatalf("reactivate draft-a: %v", err)
	}
	if _, err := SyncDraftPicks(context.Background(), db, "draft-a", "", []DraftPickInput{
		{PickNumber: 5, SleeperPlayerID: "official", Position: "QB"},
		{PickNumber: 6, SleeperPlayerID: "manual-one", Position: "WR"},
	}); err != nil {
		t.Fatalf("reconcile official picks: %v", err)
	}
	state, err = GetDraftState(context.Background(), db)
	if err != nil {
		t.Fatalf("GetDraftState() after reconciliation error = %v", err)
	}
	if len(state.Picks) != 2 || state.Picks[1].Source != "sleeper" ||
		state.Picks[1].SleeperPlayerID != "manual-one" {
		t.Fatalf("reconciled state = %#v", state)
	}
}

func TestManualPickValidation(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "draft.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`
		INSERT INTO players (id, first_name, last_name, position)
		VALUES (1, 'No', 'Sleeper ID', 'WR')
	`); err != nil {
		t.Fatalf("insert undraftable player: %v", err)
	}

	if _, err := CreateManualPick(context.Background(), db, 1); !errors.Is(err, ErrDraftNotConfigured) {
		t.Fatalf("unconfigured CreateManualPick() error = %v, want ErrDraftNotConfigured", err)
	}
	if _, err := DeleteManualPick(context.Background(), db, 1); !errors.Is(err, ErrDraftNotConfigured) {
		t.Fatalf("unconfigured DeleteManualPick() error = %v, want ErrDraftNotConfigured", err)
	}
	if _, err := UpdateSettings(context.Background(), db, EditableSettings{
		SleeperDraftID: "draft", PollingEnabled: false, PollingInterval: 2000,
	}); err != nil {
		t.Fatalf("activate draft: %v", err)
	}
	if _, err := CreateManualPick(context.Background(), db, 999); !errors.Is(err, ErrPlayerNotFound) {
		t.Fatalf("missing-player CreateManualPick() error = %v, want ErrPlayerNotFound", err)
	}
	if _, err := CreateManualPick(context.Background(), db, 1); !errors.Is(err, ErrPlayerNotDraftable) {
		t.Fatalf("undraftable CreateManualPick() error = %v, want ErrPlayerNotDraftable", err)
	}
}

func TestSyncDraftPicksIsIdempotentAndReconcilesOfficialState(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "draft.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`
		INSERT INTO players (id, sleeper_player_id, first_name, last_name, position)
		VALUES (1, 'known', 'Known', 'Player', 'WR')
	`); err != nil {
		t.Fatalf("insert known player: %v", err)
	}
	if _, err := UpdateSettings(context.Background(), db, EditableSettings{
		SleeperLeagueID: "league", SleeperDraftID: "draft", PollingEnabled: true, PollingInterval: 2000,
	}); err != nil {
		t.Fatalf("activate draft: %v", err)
	}

	initial := []DraftPickInput{
		{PickNumber: 1, Round: 1, DraftSlot: 1, SleeperPlayerID: "known", FirstName: "Known", LastName: "Player"},
		{PickNumber: 2, Round: 1, DraftSlot: 2, SleeperPlayerID: "unknown", FirstName: "Future", LastName: "Player"},
	}
	unknown, err := SyncDraftPicks(context.Background(), db, "draft", "league", initial)
	if err != nil {
		t.Fatalf("initial SyncDraftPicks() error = %v", err)
	}
	if unknown != 1 {
		t.Fatalf("unknown count = %d, want 1", unknown)
	}
	if _, err := SyncDraftPicks(context.Background(), db, "draft", "league", initial); err != nil {
		t.Fatalf("repeat SyncDraftPicks() error = %v", err)
	}
	if count := countRows(t, db, "draft_picks"); count != 2 {
		t.Fatalf("pick count after repeat = %d, want 2", count)
	}

	if _, err := db.Exec(`
		INSERT INTO draft_picks (
			draft_id, pick_number, sleeper_player_id, source, created_at
		) SELECT id, 3, 'manual-player', 'manual', '2026-08-29T00:00:00Z'
		FROM drafts WHERE sleeper_draft_id = 'draft'
	`); err != nil {
		t.Fatalf("insert future manual pick: %v", err)
	}
	revised := []DraftPickInput{
		{PickNumber: 1, SleeperPlayerID: "known"},
		{PickNumber: 3, SleeperPlayerID: "manual-player", FirstName: "Official", LastName: "Now"},
	}
	if _, err := SyncDraftPicks(context.Background(), db, "draft", "league", revised); err != nil {
		t.Fatalf("revised SyncDraftPicks() error = %v", err)
	}

	state, err := GetDraftState(context.Background(), db)
	if err != nil {
		t.Fatalf("GetDraftState() error = %v", err)
	}
	if len(state.Picks) != 2 || state.Picks[0].SleeperPlayerID != "known" ||
		state.Picks[1].SleeperPlayerID != "manual-player" || state.Picks[1].Source != "sleeper" {
		t.Fatalf("reconciled picks = %#v", state.Picks)
	}
	if len(state.TakenPlayerIDs) != 1 || state.TakenPlayerIDs[0] != 1 {
		t.Fatalf("taken player IDs = %#v, want [1]", state.TakenPlayerIDs)
	}
	if len(state.UnknownSleeperIDs) != 1 || state.UnknownSleeperIDs[0] != "manual-player" {
		t.Fatalf("unknown IDs = %#v, want [manual-player]", state.UnknownSleeperIDs)
	}
	if state.LastSyncedAt == nil || state.LastSyncError != nil {
		t.Fatalf("sync metadata = %#v, %#v", state.LastSyncedAt, state.LastSyncError)
	}

	corrected := []DraftPickInput{
		{PickNumber: 1, SleeperPlayerID: "replacement"},
		{PickNumber: 3, SleeperPlayerID: "manual-player"},
	}
	if _, err := SyncDraftPicks(context.Background(), db, "draft", "league", corrected); err != nil {
		t.Fatalf("corrected SyncDraftPicks() error = %v", err)
	}
	state, err = GetDraftState(context.Background(), db)
	if err != nil {
		t.Fatalf("GetDraftState() after correction error = %v", err)
	}
	if len(state.Picks) != 2 || state.Picks[0].SleeperPlayerID != "replacement" ||
		len(state.TakenPlayerIDs) != 0 {
		t.Fatalf("corrected picks = %#v; taken = %#v", state.Picks, state.TakenPlayerIDs)
	}
}

func TestDraftSyncFailurePreservesLastKnownPicks(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "draft.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	if _, err := UpdateSettings(context.Background(), db, EditableSettings{
		SleeperDraftID: "draft", PollingEnabled: true, PollingInterval: 2000,
	}); err != nil {
		t.Fatalf("activate draft: %v", err)
	}
	if _, err := SyncDraftPicks(context.Background(), db, "draft", "", []DraftPickInput{
		{PickNumber: 1, SleeperPlayerID: "unknown"},
	}); err != nil {
		t.Fatalf("SyncDraftPicks() error = %v", err)
	}
	before, err := GetDraftState(context.Background(), db)
	if err != nil {
		t.Fatalf("GetDraftState() before failure error = %v", err)
	}
	if err := RecordDraftSyncFailure(context.Background(), db, "draft", ""); err != nil {
		t.Fatalf("RecordDraftSyncFailure() error = %v", err)
	}
	after, err := GetDraftState(context.Background(), db)
	if err != nil {
		t.Fatalf("GetDraftState() after failure error = %v", err)
	}
	if len(after.Picks) != 1 || after.LastSyncedAt == nil || before.LastSyncedAt == nil ||
		*after.LastSyncedAt != *before.LastSyncedAt || after.LastSyncError == nil ||
		*after.LastSyncError != DraftSyncFailureMessage {
		t.Fatalf("state after failure = %#v", after)
	}
}

func TestDraftStateDoesNotWarnForUnsupportedPositions(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "draft.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	if _, err := UpdateSettings(context.Background(), db, EditableSettings{
		SleeperDraftID: "draft", PollingEnabled: true, PollingInterval: 2000,
	}); err != nil {
		t.Fatalf("activate draft: %v", err)
	}

	picks := []DraftPickInput{
		{PickNumber: 1, SleeperPlayerID: "unknown-wr", Position: "WR"},
		{PickNumber: 2, SleeperPlayerID: "unknown-k", Position: "K"},
		{PickNumber: 3, SleeperPlayerID: "unknown-def", Position: "DEF"},
		{PickNumber: 4, SleeperPlayerID: "unknown-dst", Position: "dst"},
		{PickNumber: 5, SleeperPlayerID: "unknown-d-slash-st", Position: " D/ST "},
	}
	unknownCount, err := SyncDraftPicks(context.Background(), db, "draft", "", picks)
	if err != nil {
		t.Fatalf("SyncDraftPicks() error = %v", err)
	}
	if unknownCount != 1 {
		t.Fatalf("actionable unknown count = %d, want 1", unknownCount)
	}

	state, err := GetDraftState(context.Background(), db)
	if err != nil {
		t.Fatalf("GetDraftState() error = %v", err)
	}
	if len(state.Picks) != len(picks) {
		t.Fatalf("persisted pick count = %d, want %d", len(state.Picks), len(picks))
	}
	if len(state.UnknownSleeperIDs) != 1 || state.UnknownSleeperIDs[0] != "unknown-wr" {
		t.Fatalf("unknown IDs = %#v, want [unknown-wr]", state.UnknownSleeperIDs)
	}
}

func TestGetDraftStateUsesConfiguredDraftContext(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "draft.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	if _, err := SyncDraftPicks(context.Background(), db, "old", "", []DraftPickInput{
		{PickNumber: 1, SleeperPlayerID: "old-player"},
	}); err != nil {
		t.Fatalf("sync old draft: %v", err)
	}
	if _, err := UpdateSettings(context.Background(), db, EditableSettings{
		SleeperDraftID: "new", PollingEnabled: false, PollingInterval: 2000,
	}); err != nil {
		t.Fatalf("switch active draft: %v", err)
	}
	state, err := GetDraftState(context.Background(), db)
	if err != nil {
		t.Fatalf("GetDraftState() error = %v", err)
	}
	if state.DraftID != "new" || len(state.Picks) != 0 {
		t.Fatalf("active state = %#v, want empty new draft", state)
	}
	if count := countRows(t, db, "drafts"); count != 1 {
		t.Fatalf("stored draft history count = %d, want 1", count)
	}
}
