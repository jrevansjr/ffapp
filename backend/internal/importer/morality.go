package importer

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jrevansjr/ffapp/backend/internal/morality"
)

const minimumRealMoralityRows = 116

// MoralitySummary describes one exact-ID import of the user-supplied score snapshot.
type MoralitySummary struct {
	Source       string
	SnapshotDate string
	SourceRows   int
	MatchedRows  int
	InsertedRows int
	ImportedAt   string
}

type mappedMoralityScore struct {
	PlayerID int64
	Score    int
}

// LoadMorality validates and transactionally replaces one subjective score source.
func LoadMorality(
	ctx context.Context,
	db *sql.DB,
	snapshot morality.Snapshot,
	importedAt time.Time,
) (MoralitySummary, error) {
	return loadMoralityWithThreshold(ctx, db, snapshot, importedAt, 1)
}

func loadMoralityWithThreshold(
	ctx context.Context,
	db *sql.DB,
	snapshot morality.Snapshot,
	importedAt time.Time,
	minimumRows int,
) (MoralitySummary, error) {
	summary := MoralitySummary{
		Source:       strings.TrimSpace(snapshot.Source),
		SnapshotDate: snapshot.SnapshotDate,
		SourceRows:   len(snapshot.Rows),
		ImportedAt:   importedAt.UTC().Format(time.RFC3339),
	}
	if summary.Source == "" || summary.SnapshotDate == "" {
		return summary, fmt.Errorf("morality snapshot requires source and snapshot date")
	}
	if _, err := time.Parse("2006-01-02", summary.SnapshotDate); err != nil {
		return summary, fmt.Errorf("morality snapshot date must use YYYY-MM-DD: %w", err)
	}
	if len(snapshot.Rows) < minimumRows {
		return summary, fmt.Errorf(
			"morality snapshot contains %d rows; refusing to replace below the safety threshold of %d",
			len(snapshot.Rows), minimumRows,
		)
	}

	playerIDs, err := loadSleeperPlayerIDs(ctx, db)
	if err != nil {
		return summary, err
	}
	mapped := make([]mappedMoralityScore, 0, len(snapshot.Rows))
	seen := make(map[string]struct{}, len(snapshot.Rows))
	for _, row := range snapshot.Rows {
		sleeperID := strings.TrimSpace(row.SleeperPlayerID)
		if sleeperID == "" || row.Score < 0 || row.Score > 5 {
			return summary, fmt.Errorf("morality snapshot contains an invalid player ID or 0-5 score")
		}
		if _, duplicate := seen[sleeperID]; duplicate {
			return summary, fmt.Errorf("morality snapshot repeats Sleeper player ID %q", sleeperID)
		}
		seen[sleeperID] = struct{}{}
		playerID, matched := playerIDs[sleeperID]
		if !matched {
			return summary, fmt.Errorf("morality snapshot has unknown Sleeper player ID %q", sleeperID)
		}
		mapped = append(mapped, mappedMoralityScore{PlayerID: playerID, Score: row.Score})
	}
	summary.MatchedRows = len(mapped)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return summary, fmt.Errorf("begin morality import: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM player_morality_scores WHERE source = ?`, summary.Source); err != nil {
		return summary, fmt.Errorf("clear morality scores: %w", err)
	}
	for _, row := range mapped {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO player_morality_scores (
				player_id, source, score, snapshot_date, imported_at
			) VALUES (?, ?, ?, ?, ?)
		`, row.PlayerID, summary.Source, row.Score, summary.SnapshotDate, summary.ImportedAt); err != nil {
			return summary, fmt.Errorf("insert morality score: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return summary, fmt.Errorf("commit morality import: %w", err)
	}
	summary.InsertedRows = len(mapped)
	return summary, nil
}

func loadSleeperPlayerIDs(ctx context.Context, db *sql.DB) (map[string]int64, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, sleeper_player_id
		FROM players
		WHERE sleeper_player_id IS NOT NULL
	`)
	if err != nil {
		return nil, fmt.Errorf("load player IDs for morality import: %w", err)
	}
	defer rows.Close()
	players := make(map[string]int64)
	for rows.Next() {
		var id int64
		var sleeperID string
		if err := rows.Scan(&id, &sleeperID); err != nil {
			return nil, fmt.Errorf("scan player ID for morality import: %w", err)
		}
		players[sleeperID] = id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read player IDs for morality import: %w", err)
	}
	return players, nil
}
