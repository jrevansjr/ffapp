package database

import (
	"context"
	"path/filepath"
	"testing"
)

func TestPlayerQueriesAndDerivedAvailability(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "draft.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	ctx := context.Background()
	loadPlayerQueryFixture(t, db)

	players, err := ListPlayers(ctx, db, PlayerFilters{})
	if err != nil {
		t.Fatalf("ListPlayers() error = %v", err)
	}
	if len(players) != 4 {
		t.Fatalf("player count = %d, want 4", len(players))
	}
	if players[0].Season == nil || players[0].Draft.AggregateADP == nil ||
		players[0].Draft.ECR == nil || players[0].Draft.Tier == nil ||
		players[0].Draft.RankStdDev == nil {
		t.Fatal("first player is missing seeded summary data")
	}
	for _, player := range players {
		if player.IsTaken {
			t.Fatal("player is taken before a draft is configured")
		}
	}

	quarterbacks, err := ListPlayers(ctx, db, PlayerFilters{Position: "QB"})
	if err != nil {
		t.Fatalf("ListPlayers(QB) error = %v", err)
	}
	if len(quarterbacks) != 1 {
		t.Fatalf("QB count = %d, want 1", len(quarterbacks))
	}

	if _, err := UpdateSettings(ctx, db, EditableSettings{
		SleeperDraftID: "fixture-draft", PollingEnabled: true, PollingInterval: 2000,
	}); err != nil {
		t.Fatalf("activate sample draft: %v", err)
	}
	available, err := ListPlayers(ctx, db, PlayerFilters{AvailableOnly: true})
	if err != nil {
		t.Fatalf("ListPlayers(available) error = %v", err)
	}
	if len(available) != 3 {
		t.Fatalf("available player count = %d, want 3", len(available))
	}

	var takenID int64
	if err := db.QueryRow(`
		SELECT player_id FROM draft_picks ORDER BY pick_number LIMIT 1
	`).Scan(&takenID); err != nil {
		t.Fatalf("load taken player id: %v", err)
	}
	detail, err := GetPlayer(ctx, db, takenID)
	if err != nil {
		t.Fatalf("GetPlayer() error = %v", err)
	}
	if !detail.IsTaken {
		t.Fatal("sample-picked player is_taken = false, want true")
	}
	if len(detail.Weekly) != 2 {
		t.Fatalf("weekly row count = %d, want 2", len(detail.Weekly))
	}
	if detail.Player.ProviderIDs.Sportradar == nil {
		t.Fatal("detail Sportradar ID = nil, want fixture ID")
	}
	if detail.Player.ProviderIDs.GSIS == nil {
		t.Fatal("detail GSIS ID = nil, want fixture ID")
	}
	if detail.Weekly[0].PassingYards == 0 {
		t.Fatal("detail weekly passing yards = 0, want seeded QB yards")
	}
}
