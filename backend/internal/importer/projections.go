package importer

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/jrevansjr/ffapp/backend/internal/fantasypros"
)

const minimumRealProjectionRows = 500

// ProjectionSummary describes exact FantasyPros-ID coverage for one import.
type ProjectionSummary struct {
	SourceRows    int
	MatchedRows   int
	UnmatchedRows int
	InsertedRows  int
	UpdatedAt     string
	Unmatched     []ProjectionUnmatchedPlayer
}

// ProjectionUnmatchedPlayer is a provider forecast without a local exact ID.
type ProjectionUnmatchedPlayer struct {
	FantasyProsID string
	Name          string
	Position      string
	Team          string
}

type mappedProjection struct {
	PlayerID            int64
	PassingYards        *float64
	PassingTouchdowns   *float64
	RushingYards        *float64
	RushingTouchdowns   *float64
	ReceivingYards      *float64
	ReceivingTouchdowns *float64
	UpdatedAt           string
}

// LoadProjections transactionally replaces FantasyPros 2026 projections.
// Matching uses only the stable provider ID already stored on players.
func LoadProjections(
	ctx context.Context,
	db *sql.DB,
	dataset fantasypros.ProjectionDataset,
) (ProjectionSummary, error) {
	return loadProjectionsWithThreshold(ctx, db, dataset, 1)
}

func loadProjectionsWithThreshold(
	ctx context.Context,
	db *sql.DB,
	dataset fantasypros.ProjectionDataset,
	minimumRows int,
) (ProjectionSummary, error) {
	var summary ProjectionSummary
	if dataset.UpdatedAt.IsZero() || len(dataset.Projections) == 0 {
		return summary, fmt.Errorf("FantasyPros projections have no timestamp or players")
	}
	localByFantasyPros, err := loadActiveFantasyProsIDs(ctx, db)
	if err != nil {
		return summary, err
	}
	summary.SourceRows = len(dataset.Projections)
	summary.UpdatedAt = dataset.UpdatedAt.UTC().Format(time.RFC3339)
	mapped := make([]mappedProjection, 0, len(dataset.Projections))
	for _, projection := range dataset.Projections {
		playerID, found := localByFantasyPros[projection.FantasyProsID]
		if !found {
			summary.Unmatched = append(summary.Unmatched, ProjectionUnmatchedPlayer{
				FantasyProsID: projection.FantasyProsID,
				Name:          projection.Name,
				Position:      projection.Position,
				Team:          projection.Team,
			})
			continue
		}
		mapped = append(mapped, mappedProjection{
			PlayerID:            playerID,
			PassingYards:        projection.PassingYards,
			PassingTouchdowns:   projection.PassingTouchdowns,
			RushingYards:        projection.RushingYards,
			RushingTouchdowns:   projection.RushingTouchdowns,
			ReceivingYards:      projection.ReceivingYards,
			ReceivingTouchdowns: projection.ReceivingTouchdowns,
			UpdatedAt:           summary.UpdatedAt,
		})
	}
	summary.MatchedRows = len(mapped)
	summary.UnmatchedRows = summary.SourceRows - summary.MatchedRows
	summary.InsertedRows = summary.MatchedRows
	sort.Slice(summary.Unmatched, func(i, j int) bool {
		if summary.Unmatched[i].Position != summary.Unmatched[j].Position {
			return summary.Unmatched[i].Position < summary.Unmatched[j].Position
		}
		return summary.Unmatched[i].Name < summary.Unmatched[j].Name
	})
	if summary.MatchedRows < minimumRows {
		return summary, fmt.Errorf(
			"only %d FantasyPros projection rows matched active local players; refusing to replace below the safety threshold of %d",
			summary.MatchedRows,
			minimumRows,
		)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return summary, fmt.Errorf("begin projections import: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(
		ctx,
		`DELETE FROM player_projections WHERE season = ? AND source = 'fantasypros'`,
		fantasypros.Season,
	); err != nil {
		return summary, fmt.Errorf("clear FantasyPros projections: %w", err)
	}
	statement, err := tx.PrepareContext(ctx, `
		INSERT INTO player_projections (
			player_id, season, source, passing_yards, passing_touchdowns,
			rushing_yards, rushing_touchdowns, receiving_yards,
			receiving_touchdowns, updated_at
		) VALUES (?, ?, 'fantasypros', ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return summary, fmt.Errorf("prepare projection insert: %w", err)
	}
	defer statement.Close()
	for _, row := range mapped {
		if _, err := statement.ExecContext(
			ctx,
			row.PlayerID,
			fantasypros.Season,
			nullableProjection(row.PassingYards),
			nullableProjection(row.PassingTouchdowns),
			nullableProjection(row.RushingYards),
			nullableProjection(row.RushingTouchdowns),
			nullableProjection(row.ReceivingYards),
			nullableProjection(row.ReceivingTouchdowns),
			row.UpdatedAt,
		); err != nil {
			return summary, fmt.Errorf("insert projections for player %d: %w", row.PlayerID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return summary, fmt.Errorf("commit projections import: %w", err)
	}
	return summary, nil
}

func loadActiveFantasyProsIDs(ctx context.Context, db *sql.DB) (map[string]int64, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, fantasypros_id
		FROM players
		WHERE active = 1
			AND position IN ('QB', 'RB', 'WR', 'TE')
			AND fantasypros_id IS NOT NULL
	`)
	if err != nil {
		return nil, fmt.Errorf("load player IDs for projections: %w", err)
	}
	defer rows.Close()
	players := make(map[string]int64)
	for rows.Next() {
		var playerID int64
		var fantasyProsID string
		if err := rows.Scan(&playerID, &fantasyProsID); err != nil {
			return nil, fmt.Errorf("scan player ID for projections: %w", err)
		}
		if existingPlayerID, duplicate := players[fantasyProsID]; duplicate {
			return nil, fmt.Errorf(
				"FantasyPros ID %s belongs to active players %d and %d; refusing ambiguous projection mapping",
				fantasyProsID,
				existingPlayerID,
				playerID,
			)
		}
		players[fantasyProsID] = playerID
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read player IDs for projections: %w", err)
	}
	if len(players) == 0 {
		return nil, fmt.Errorf("no active players have FantasyPros IDs; load FantasyPros draft data first")
	}
	return players, nil
}

func nullableProjection(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}
