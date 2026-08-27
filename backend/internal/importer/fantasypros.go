package importer

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jrevansjr/ffapp/backend/internal/fantasypros"
	"github.com/jrevansjr/ffapp/backend/internal/nflverse"
)

const minimumRealFantasyProsRows = 100

// FantasyProsSummary describes exact-ID matching and the three tables replaced
// together from the two approved provider responses.
type FantasyProsSummary struct {
	FantasyProsBackfills int
	IdentityConflicts    int
	AmbiguousMappings    int
	IdentityIssues       []FantasyProsIdentityIssue
	ADP                  FantasyProsDatasetSummary
	ECR                  FantasyProsDatasetSummary
}

// FantasyProsDatasetSummary records local coverage for one provider response.
type FantasyProsDatasetSummary struct {
	Dataset       fantasypros.DatasetName
	SourceRows    int
	MatchedRows   int
	UnmatchedRows int
	InsertedRows  int
	UpdatedAt     string
	Unmatched     []FantasyProsUnmatchedPlayer
}

// FantasyProsUnmatchedPlayer is a high-priority provider row without a local
// exact FantasyPros ID match. Value is ADP or ECR according to Dataset.
type FantasyProsUnmatchedPlayer struct {
	FantasyProsID string
	Name          string
	Position      string
	Value         float64
}

// FantasyProsIdentityIssue explains a crosswalk value deliberately left
// unapplied rather than guessed.
type FantasyProsIdentityIssue struct {
	SleeperID              string
	LocalFantasyProsID     string
	CrosswalkFantasyProsID string
	Reason                 string
}

type fantasyProsLocalPlayer struct {
	ID            int64
	SleeperID     string
	FantasyProsID string
}

type mappedADP struct {
	PlayerID  int64
	ADP       float64
	UpdatedAt string
}

type mappedECR struct {
	PlayerID     int64
	OverallRank  int
	PositionRank int
	Tier         int
	RankMin      int
	RankMax      int
	RankStdDev   float64
	UpdatedAt    string
}

// LoadFantasyPros exactly reconciles provider IDs and transactionally replaces
// 2026 Aggregate ADP, Draft ECR/disagreement, and FantasyPros tiers.
func LoadFantasyPros(
	ctx context.Context,
	db *sql.DB,
	adp fantasypros.ADPDataset,
	ecr fantasypros.ECRDataset,
	playerIDs nflverse.PlayerIDDataset,
) (FantasyProsSummary, error) {
	return loadFantasyProsWithThreshold(ctx, db, adp, ecr, playerIDs, 1)
}

func loadFantasyProsWithThreshold(
	ctx context.Context,
	db *sql.DB,
	adp fantasypros.ADPDataset,
	ecr fantasypros.ECRDataset,
	playerIDs nflverse.PlayerIDDataset,
	minimumRows int,
) (FantasyProsSummary, error) {
	var summary FantasyProsSummary
	if adp.UpdatedAt.IsZero() || len(adp.Rankings) == 0 {
		return summary, fmt.Errorf("FantasyPros ADP has no timestamp or rankings")
	}
	if ecr.UpdatedAt.IsZero() || len(ecr.Rankings) == 0 {
		return summary, fmt.Errorf("FantasyPros ECR has no timestamp or rankings")
	}
	players, err := loadFantasyProsLocalPlayers(ctx, db)
	if err != nil {
		return summary, err
	}
	localByFantasyPros, backfills, matchSummary := matchFantasyProsIDs(players, playerIDs.Rows)
	summary.FantasyProsBackfills = len(backfills)
	summary.IdentityConflicts = matchSummary.conflicts
	summary.AmbiguousMappings = matchSummary.ambiguous
	summary.IdentityIssues = matchSummary.issues

	mappedADPs, adpSummary := mapADP(adp, localByFantasyPros)
	mappedECRs, ecrSummary := mapECR(ecr, localByFantasyPros)
	summary.ADP = adpSummary
	summary.ECR = ecrSummary
	for _, dataset := range []FantasyProsDatasetSummary{adpSummary, ecrSummary} {
		if dataset.MatchedRows < minimumRows {
			return summary, fmt.Errorf(
				"only %d FantasyPros %s rows matched local players; refusing to replace below the safety threshold of %d",
				dataset.MatchedRows,
				dataset.Dataset,
				minimumRows,
			)
		}
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return summary, fmt.Errorf("begin FantasyPros import: %w", err)
	}
	defer tx.Rollback()
	for playerID, fantasyProsID := range backfills {
		if _, err := tx.ExecContext(ctx, `UPDATE players SET fantasypros_id = ? WHERE id = ?`, fantasyProsID, playerID); err != nil {
			return summary, fmt.Errorf("backfill FantasyPros ID %s: %w", fantasyProsID, err)
		}
	}
	for table := range map[string]struct{}{
		"player_adp": {}, "player_rankings": {}, "player_tiers": {},
	} {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE season = ? AND source = 'fantasypros'`, fantasypros.Season); err != nil {
			return summary, fmt.Errorf("clear FantasyPros rows from %s: %w", table, err)
		}
	}
	if err := insertADP(ctx, tx, mappedADPs); err != nil {
		return summary, err
	}
	if err := insertECRAndTiers(ctx, tx, mappedECRs); err != nil {
		return summary, err
	}
	if err := tx.Commit(); err != nil {
		return summary, fmt.Errorf("commit FantasyPros import: %w", err)
	}
	return summary, nil
}

func mapADP(
	dataset fantasypros.ADPDataset,
	localByFantasyPros map[string]int64,
) ([]mappedADP, FantasyProsDatasetSummary) {
	updatedAt := dataset.UpdatedAt.UTC().Format(time.RFC3339)
	summary := FantasyProsDatasetSummary{
		Dataset: fantasypros.DatasetADP, SourceRows: len(dataset.Rankings), UpdatedAt: updatedAt,
	}
	mapped := make([]mappedADP, 0, len(dataset.Rankings))
	for _, ranking := range dataset.Rankings {
		playerID, found := localByFantasyPros[ranking.FantasyProsID]
		if !found {
			summary.Unmatched = append(summary.Unmatched, FantasyProsUnmatchedPlayer{
				FantasyProsID: ranking.FantasyProsID, Name: ranking.Name,
				Position: ranking.Position, Value: ranking.ADP,
			})
			continue
		}
		mapped = append(mapped, mappedADP{PlayerID: playerID, ADP: ranking.ADP, UpdatedAt: updatedAt})
		summary.MatchedRows++
	}
	finishDatasetSummary(&summary)
	return mapped, summary
}

func mapECR(
	dataset fantasypros.ECRDataset,
	localByFantasyPros map[string]int64,
) ([]mappedECR, FantasyProsDatasetSummary) {
	updatedAt := dataset.UpdatedAt.UTC().Format(time.RFC3339)
	summary := FantasyProsDatasetSummary{
		Dataset: fantasypros.DatasetECR, SourceRows: len(dataset.Rankings), UpdatedAt: updatedAt,
	}
	mapped := make([]mappedECR, 0, len(dataset.Rankings))
	for _, ranking := range dataset.Rankings {
		playerID, found := localByFantasyPros[ranking.FantasyProsID]
		if !found {
			summary.Unmatched = append(summary.Unmatched, FantasyProsUnmatchedPlayer{
				FantasyProsID: ranking.FantasyProsID, Name: ranking.Name,
				Position: ranking.Position, Value: float64(ranking.OverallRank),
			})
			continue
		}
		mapped = append(mapped, mappedECR{
			PlayerID: playerID, OverallRank: ranking.OverallRank,
			PositionRank: ranking.PositionRank, Tier: ranking.Tier,
			RankMin: ranking.RankMin, RankMax: ranking.RankMax,
			RankStdDev: ranking.RankStdDev, UpdatedAt: updatedAt,
		})
		summary.MatchedRows++
	}
	finishDatasetSummary(&summary)
	return mapped, summary
}

func finishDatasetSummary(summary *FantasyProsDatasetSummary) {
	summary.UnmatchedRows = summary.SourceRows - summary.MatchedRows
	summary.InsertedRows = summary.MatchedRows
	sort.Slice(summary.Unmatched, func(i, j int) bool {
		return summary.Unmatched[i].Value < summary.Unmatched[j].Value
	})
	if len(summary.Unmatched) > 20 {
		summary.Unmatched = summary.Unmatched[:20]
	}
}

func insertADP(ctx context.Context, tx *sql.Tx, rows []mappedADP) error {
	statement, err := tx.PrepareContext(ctx, `
		INSERT INTO player_adp (player_id, season, source, adp, updated_at)
		VALUES (?, ?, 'fantasypros', ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare Aggregate ADP insert: %w", err)
	}
	defer statement.Close()
	for _, row := range rows {
		if _, err := statement.ExecContext(ctx, row.PlayerID, fantasypros.Season, row.ADP, row.UpdatedAt); err != nil {
			return fmt.Errorf("insert Aggregate ADP for player %d: %w", row.PlayerID, err)
		}
	}
	return nil
}

func insertECRAndTiers(ctx context.Context, tx *sql.Tx, rows []mappedECR) error {
	rankingStatement, err := tx.PrepareContext(ctx, `
		INSERT INTO player_rankings (
			player_id, season, source, overall_rank, position_rank,
			rank_min, rank_max, rank_std_dev, updated_at
		) VALUES (?, ?, 'fantasypros', ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare ECR insert: %w", err)
	}
	defer rankingStatement.Close()
	tierStatement, err := tx.PrepareContext(ctx, `
		INSERT INTO player_tiers (player_id, season, source, tier, updated_at)
		VALUES (?, ?, 'fantasypros', ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare FantasyPros tier insert: %w", err)
	}
	defer tierStatement.Close()
	for _, row := range rows {
		if _, err := rankingStatement.ExecContext(
			ctx, row.PlayerID, fantasypros.Season, row.OverallRank, row.PositionRank,
			row.RankMin, row.RankMax, row.RankStdDev, row.UpdatedAt,
		); err != nil {
			return fmt.Errorf("insert ECR for player %d: %w", row.PlayerID, err)
		}
		if _, err := tierStatement.ExecContext(
			ctx, row.PlayerID, fantasypros.Season, row.Tier, row.UpdatedAt,
		); err != nil {
			return fmt.Errorf("insert tier for player %d: %w", row.PlayerID, err)
		}
	}
	return nil
}

func loadFantasyProsLocalPlayers(ctx context.Context, db *sql.DB) ([]fantasyProsLocalPlayer, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, sleeper_player_id, COALESCE(fantasypros_id, '')
		FROM players
		WHERE sleeper_player_id IS NOT NULL AND position IN ('QB', 'RB', 'WR', 'TE')
		ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("load local players for FantasyPros import: %w", err)
	}
	defer rows.Close()
	var players []fantasyProsLocalPlayer
	for rows.Next() {
		var player fantasyProsLocalPlayer
		if err := rows.Scan(&player.ID, &player.SleeperID, &player.FantasyProsID); err != nil {
			return nil, fmt.Errorf("scan local player for FantasyPros import: %w", err)
		}
		players = append(players, player)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read local players for FantasyPros import: %w", err)
	}
	if len(players) == 0 {
		return nil, fmt.Errorf("no local fantasy players exist; load players before FantasyPros data")
	}
	return players, nil
}

type fantasyProsIdentityMatchSummary struct {
	conflicts int
	ambiguous int
	issues    []FantasyProsIdentityIssue
}

func matchFantasyProsIDs(
	players []fantasyProsLocalPlayer,
	crosswalk []nflverse.PlayerID,
) (map[string]int64, map[int64]string, fantasyProsIdentityMatchSummary) {
	crossBySleeper := make(map[string]string)
	ambiguousSleeper := make(map[string]struct{})
	fantasyProsSleepers := make(map[string]map[string]struct{})
	for _, identity := range crosswalk {
		sleeperID := strings.TrimSpace(identity.SleeperID)
		fantasyProsID := strings.TrimSpace(identity.FantasyProsID)
		if sleeperID == "" || fantasyProsID == "" {
			continue
		}
		if prior, found := crossBySleeper[sleeperID]; found && prior != fantasyProsID {
			ambiguousSleeper[sleeperID] = struct{}{}
		} else {
			crossBySleeper[sleeperID] = fantasyProsID
		}
		if fantasyProsSleepers[fantasyProsID] == nil {
			fantasyProsSleepers[fantasyProsID] = make(map[string]struct{})
		}
		fantasyProsSleepers[fantasyProsID][sleeperID] = struct{}{}
	}

	localByFantasyPros := make(map[string]int64)
	for _, player := range players {
		if player.FantasyProsID != "" {
			localByFantasyPros[player.FantasyProsID] = player.ID
		}
	}
	backfills := make(map[int64]string)
	var summary fantasyProsIdentityMatchSummary
	for _, player := range players {
		crossID := crossBySleeper[player.SleeperID]
		if player.FantasyProsID != "" {
			if _, ambiguous := ambiguousSleeper[player.SleeperID]; ambiguous {
				summary.ambiguous++
				summary.issues = append(summary.issues, FantasyProsIdentityIssue{
					SleeperID: player.SleeperID, LocalFantasyProsID: player.FantasyProsID,
					CrosswalkFantasyProsID: crossID, Reason: "ambiguous crosswalk rows",
				})
			} else if crossID != "" && crossID != player.FantasyProsID {
				summary.conflicts++
				summary.issues = append(summary.issues, FantasyProsIdentityIssue{
					SleeperID: player.SleeperID, LocalFantasyProsID: player.FantasyProsID,
					CrosswalkFantasyProsID: crossID, Reason: "local FantasyPros ID differs from crosswalk",
				})
			}
			continue
		}
		if crossID == "" {
			continue
		}
		if _, ambiguous := ambiguousSleeper[player.SleeperID]; ambiguous || len(fantasyProsSleepers[crossID]) != 1 {
			summary.ambiguous++
			summary.issues = append(summary.issues, FantasyProsIdentityIssue{
				SleeperID: player.SleeperID, CrosswalkFantasyProsID: crossID,
				Reason: "crosswalk Sleeper or FantasyPros ID is not unique",
			})
			continue
		}
		if owner, occupied := localByFantasyPros[crossID]; occupied && owner != player.ID {
			summary.conflicts++
			summary.issues = append(summary.issues, FantasyProsIdentityIssue{
				SleeperID: player.SleeperID, CrosswalkFantasyProsID: crossID,
				Reason: "crosswalk FantasyPros ID is already assigned locally",
			})
			continue
		}
		backfills[player.ID] = crossID
		localByFantasyPros[crossID] = player.ID
	}
	return localByFantasyPros, backfills, summary
}
