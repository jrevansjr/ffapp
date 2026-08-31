package importer

import (
	"context"
	"testing"
	"time"

	"github.com/jrevansjr/ffapp/backend/internal/morality"
)

func TestLoadMoralityMatchesExactlyAndIsIdempotent(t *testing.T) {
	db := statsTestDatabase(t)
	importedAt := time.Date(2026, 8, 30, 20, 0, 0, 0, time.UTC)
	snapshot := moralitySnapshotFixture()

	summary, err := LoadMorality(context.Background(), db, snapshot, importedAt)
	if err != nil {
		t.Fatalf("LoadMorality() error = %v", err)
	}
	if summary.MatchedRows != 2 || summary.InsertedRows != 2 || rowCount(t, db, "player_morality_scores") != 2 {
		t.Fatalf("summary = %#v", summary)
	}

	snapshot.Rows[0].Score = 5
	if _, err := LoadMorality(context.Background(), db, snapshot, importedAt.Add(time.Hour)); err != nil {
		t.Fatalf("second LoadMorality() error = %v", err)
	}
	var score int
	if err := db.QueryRow(`
		SELECT scores.score
		FROM player_morality_scores AS scores
		JOIN players ON players.id = scores.player_id
		WHERE players.sleeper_player_id = 'qb-1'
	`).Scan(&score); err != nil {
		t.Fatal(err)
	}
	if score != 5 || rowCount(t, db, "player_morality_scores") != 2 {
		t.Fatalf("QB score = %d; rows = %d", score, rowCount(t, db, "player_morality_scores"))
	}
}

func TestLoadMoralityRejectsUnknownPlayerWithoutChangingData(t *testing.T) {
	db := statsTestDatabase(t)
	importedAt := time.Date(2026, 8, 30, 20, 0, 0, 0, time.UTC)
	if _, err := LoadMorality(context.Background(), db, moralitySnapshotFixture(), importedAt); err != nil {
		t.Fatal(err)
	}
	snapshot := moralitySnapshotFixture()
	snapshot.Rows = append(snapshot.Rows, morality.PlayerScore{SleeperPlayerID: "unknown", Score: 1})
	if _, err := LoadMorality(context.Background(), db, snapshot, importedAt); err == nil {
		t.Fatal("LoadMorality() error = nil, want unknown-ID failure")
	}
	if count := rowCount(t, db, "player_morality_scores"); count != 2 {
		t.Fatalf("preserved morality rows = %d, want 2", count)
	}
}

func TestLoadMoralityRollsBackWriteFailure(t *testing.T) {
	db := statsTestDatabase(t)
	importedAt := time.Date(2026, 8, 30, 20, 0, 0, 0, time.UTC)
	if _, err := LoadMorality(context.Background(), db, moralitySnapshotFixture(), importedAt); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE TRIGGER reject_morality_score
		BEFORE INSERT ON player_morality_scores
		BEGIN SELECT RAISE(ABORT, 'test rejection'); END
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadMorality(context.Background(), db, moralitySnapshotFixture(), importedAt.Add(time.Hour)); err == nil {
		t.Fatal("LoadMorality() error = nil, want trigger failure")
	}
	if count := rowCount(t, db, "player_morality_scores"); count != 2 {
		t.Fatalf("preserved morality rows = %d, want 2", count)
	}
}

func moralitySnapshotFixture() morality.Snapshot {
	return morality.Snapshot{
		Source:       "user_supplied",
		SnapshotDate: "2026-08-30",
		Description:  "test subjective score snapshot",
		Rows: []morality.PlayerScore{
			{SleeperPlayerID: "qb-1", Score: 4},
			{SleeperPlayerID: "rb-1", Score: 3},
		},
	}
}
