package database

import (
	"context"
	"database/sql"
	"fmt"
)

// NFLTeam is the local identity used by players and team-level reference data.
type NFLTeam struct {
	ID           int64
	Abbreviation string
	Name         string
}

// ListNFLTeams returns every stored NFL team ordered by abbreviation.
func ListNFLTeams(ctx context.Context, db *sql.DB) ([]NFLTeam, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, abbreviation, name
		FROM nfl_teams
		ORDER BY abbreviation
	`)
	if err != nil {
		return nil, fmt.Errorf("list NFL teams: %w", err)
	}
	defer rows.Close()

	teams := make([]NFLTeam, 0)
	for rows.Next() {
		var team NFLTeam
		if err := rows.Scan(&team.ID, &team.Abbreviation, &team.Name); err != nil {
			return nil, fmt.Errorf("scan NFL team: %w", err)
		}
		teams = append(teams, team)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read NFL teams: %w", err)
	}
	return teams, nil
}
