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
		players[0].Draft.RankStdDev == nil || players[0].Projections == nil ||
		players[0].Projections.PassingYards == nil {
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
	if detail.Player.InjuryBodyPart == nil || *detail.Player.InjuryBodyPart != "Knee" ||
		detail.Player.InjuryNotes == nil || *detail.Player.InjuryNotes != "Limited workload" ||
		detail.Player.SleeperDataUpdatedAt == nil || *detail.Player.SleeperDataUpdatedAt != "2026-09-03T12:00:00Z" {
		t.Fatalf("detail Sleeper status = %#v", detail.Player)
	}
	if len(detail.SeasonTeams) != 2 || detail.SeasonTeams[0].Abbreviation != "ARI" ||
		detail.SeasonTeams[1].Abbreviation != "BUF" {
		t.Fatalf("detail season teams = %#v, want ARI then BUF", detail.SeasonTeams)
	}
	if detail.Weekly[0].PassingYards == 0 {
		t.Fatal("detail weekly passing yards = 0, want seeded QB yards")
	}
	if detail.Projections == nil || detail.Projections.PassingTouchdowns == nil {
		t.Fatal("detail projections are missing")
	}
	if detail.Odds.PassingYards == nil || detail.Odds.PassingTouchdowns == nil ||
		detail.Odds.RushingYards == nil || detail.Odds.RushingTouchdowns == nil ||
		detail.Odds.TeamWins == nil || detail.Odds.ReceivingYards != nil {
		t.Fatalf("detail odds = %#v, want QB markets and team wins only", detail.Odds)
	}
	if detail.Morality == nil || detail.Morality.Score != 4 || detail.Morality.SnapshotDate != "2026-08-30" {
		t.Fatalf("detail morality = %#v, want supplied score", detail.Morality)
	}
}
