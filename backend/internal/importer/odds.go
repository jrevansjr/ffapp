package importer

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/jrevansjr/ffapp/backend/internal/odds"
)

const minimumRealOddsRows = 250

// OddsSummary describes how the committed consensus snapshot mapped to local subjects.
type OddsSummary struct {
	SourceRows           int
	ConsensusRows        int
	SkippedNoConsensus   int
	MatchedRows          int
	UnmatchedRows        int
	PositionMismatchRows int
	InsertedRows         int
	CapturedAt           string
	Issues               []OddsIssue
}

// OddsIssue is one supplied line that was not imported without an exact match.
type OddsIssue struct {
	Subject  string
	Team     string
	Market   string
	Position string
	Reason   string
}

type localOddsPlayer struct {
	ID       int64
	Position string
}

type mappedOddsLine struct {
	PlayerID *int64
	TeamID   *int64
	Market   string
	Line     float64
}

type oddsPlayerKey struct {
	Name string
	Team string
}

// LoadOdds transactionally replaces one supplied season/source snapshot.
// Player identity requires exact full-name and NFL-team matches.
func LoadOdds(
	ctx context.Context,
	db *sql.DB,
	snapshot odds.Snapshot,
) (OddsSummary, error) {
	return loadOddsWithThreshold(ctx, db, snapshot, 1)
}

func loadOddsWithThreshold(
	ctx context.Context,
	db *sql.DB,
	snapshot odds.Snapshot,
	minimumRows int,
) (OddsSummary, error) {
	summary := OddsSummary{
		SourceRows:         snapshot.SourceRows,
		ConsensusRows:      len(snapshot.PlayerLines) + len(snapshot.TeamLines),
		SkippedNoConsensus: snapshot.NoConsensusRows,
	}
	if snapshot.Season <= 0 || snapshot.Source == "" || snapshot.CapturedAt.IsZero() {
		return summary, fmt.Errorf("odds snapshot requires season, source, and capture time")
	}
	if summary.ConsensusRows == 0 || summary.SourceRows != summary.ConsensusRows+summary.SkippedNoConsensus {
		return summary, fmt.Errorf("odds snapshot row counts are inconsistent")
	}
	if snapshot.CapturedAt.Location() != time.UTC {
		return summary, fmt.Errorf("odds snapshot capture time must be UTC")
	}
	summary.CapturedAt = snapshot.CapturedAt.Format(time.RFC3339)

	players, err := loadOddsPlayers(ctx, db)
	if err != nil {
		return summary, err
	}
	teams, err := loadOddsTeams(ctx, db)
	if err != nil {
		return summary, err
	}
	mapped := make([]mappedOddsLine, 0, summary.ConsensusRows)
	for _, line := range snapshot.PlayerLines {
		player, found := players[oddsPlayerKey{Name: line.Name, Team: line.Team}]
		if !found {
			summary.UnmatchedRows++
			summary.Issues = append(summary.Issues, OddsIssue{
				Subject: line.Name, Team: line.Team, Market: line.Market, Reason: "unmatched",
			})
			continue
		}
		if !odds.PositionAllowed(line.Market, player.Position) {
			summary.PositionMismatchRows++
			summary.Issues = append(summary.Issues, OddsIssue{
				Subject: line.Name, Team: line.Team, Market: line.Market,
				Position: player.Position, Reason: "position_mismatch",
			})
			continue
		}
		playerID := player.ID
		mapped = append(mapped, mappedOddsLine{
			PlayerID: &playerID, Market: line.Market, Line: line.Line,
		})
	}
	for _, line := range snapshot.TeamLines {
		teamID, found := teams[line.Team]
		if !found {
			summary.UnmatchedRows++
			summary.Issues = append(summary.Issues, OddsIssue{
				Subject: line.Team, Market: line.Market, Reason: "unmatched",
			})
			continue
		}
		id := teamID
		mapped = append(mapped, mappedOddsLine{
			TeamID: &id, Market: line.Market, Line: line.Line,
		})
	}
	summary.MatchedRows = len(mapped)
	summary.InsertedRows = len(mapped)
	sort.Slice(summary.Issues, func(i, j int) bool {
		if summary.Issues[i].Market != summary.Issues[j].Market {
			return summary.Issues[i].Market < summary.Issues[j].Market
		}
		return summary.Issues[i].Subject < summary.Issues[j].Subject
	})
	if summary.MatchedRows < minimumRows {
		return summary, fmt.Errorf(
			"only %d sportsbook consensus rows matched local subjects; refusing to replace below the safety threshold of %d",
			summary.MatchedRows,
			minimumRows,
		)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return summary, fmt.Errorf("begin odds import: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(
		ctx,
		`DELETE FROM odds WHERE season = ? AND source = ?`,
		snapshot.Season,
		snapshot.Source,
	); err != nil {
		return summary, fmt.Errorf("clear sportsbook consensus odds: %w", err)
	}
	statement, err := tx.PrepareContext(ctx, `
		INSERT INTO odds (
			season, source, market, player_id, nfl_team_id, line, captured_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return summary, fmt.Errorf("prepare odds insert: %w", err)
	}
	defer statement.Close()
	for _, line := range mapped {
		if _, err := statement.ExecContext(
			ctx,
			snapshot.Season,
			snapshot.Source,
			line.Market,
			nullableOddsID(line.PlayerID),
			nullableOddsID(line.TeamID),
			line.Line,
			summary.CapturedAt,
		); err != nil {
			return summary, fmt.Errorf("insert %s odds: %w", line.Market, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return summary, fmt.Errorf("commit odds import: %w", err)
	}
	return summary, nil
}

func loadOddsPlayers(ctx context.Context, db *sql.DB) (map[oddsPlayerKey]localOddsPlayer, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT players.id, players.first_name, players.last_name,
			players.position, nfl_teams.abbreviation
		FROM players
		JOIN nfl_teams ON nfl_teams.id = players.nfl_team_id
		WHERE players.active = 1 AND players.position IN ('QB', 'RB', 'WR', 'TE')
	`)
	if err != nil {
		return nil, fmt.Errorf("load players for odds matching: %w", err)
	}
	defer rows.Close()
	players := make(map[oddsPlayerKey]localOddsPlayer)
	for rows.Next() {
		var id int64
		var firstName, lastName, position, team string
		if err := rows.Scan(&id, &firstName, &lastName, &position, &team); err != nil {
			return nil, fmt.Errorf("scan player for odds matching: %w", err)
		}
		key := oddsPlayerKey{Name: firstName + " " + lastName, Team: team}
		if existing, duplicate := players[key]; duplicate {
			return nil, fmt.Errorf(
				"players %d and %d share odds identity %q on %s",
				existing.ID,
				id,
				key.Name,
				key.Team,
			)
		}
		players[key] = localOddsPlayer{ID: id, Position: position}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read players for odds matching: %w", err)
	}
	return players, nil
}

func loadOddsTeams(ctx context.Context, db *sql.DB) (map[string]int64, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, name FROM nfl_teams`)
	if err != nil {
		return nil, fmt.Errorf("load NFL teams for odds matching: %w", err)
	}
	defer rows.Close()
	teams := make(map[string]int64)
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("scan NFL team for odds matching: %w", err)
		}
		if existing, duplicate := teams[name]; duplicate {
			return nil, fmt.Errorf("NFL teams %d and %d share odds identity %q", existing, id, name)
		}
		teams[name] = id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read NFL teams for odds matching: %w", err)
	}
	return teams, nil
}

func nullableOddsID(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}
