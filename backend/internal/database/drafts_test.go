package database

import (
	"context"
	"path/filepath"
	"testing"
)

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
