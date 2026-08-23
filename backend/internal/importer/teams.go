// Package importer owns explicit, repeatable writes of external reference data
// into SQLite. It is called only by cmd/data, never by the web server.
package importer

import (
	"context"
	"database/sql"
	"fmt"
)

// NFLTeamSeed is the reviewed identity data that does not need an external
// provider. Seasonal team values belong in their source-specific tables.
type NFLTeamSeed struct {
	Abbreviation string
	Name         string
}

// NFLTeams is the canonical 32-team list used to resolve Sleeper abbreviations.
var NFLTeams = []NFLTeamSeed{
	{Abbreviation: "ARI", Name: "Arizona Cardinals"},
	{Abbreviation: "ATL", Name: "Atlanta Falcons"},
	{Abbreviation: "BAL", Name: "Baltimore Ravens"},
	{Abbreviation: "BUF", Name: "Buffalo Bills"},
	{Abbreviation: "CAR", Name: "Carolina Panthers"},
	{Abbreviation: "CHI", Name: "Chicago Bears"},
	{Abbreviation: "CIN", Name: "Cincinnati Bengals"},
	{Abbreviation: "CLE", Name: "Cleveland Browns"},
	{Abbreviation: "DAL", Name: "Dallas Cowboys"},
	{Abbreviation: "DEN", Name: "Denver Broncos"},
	{Abbreviation: "DET", Name: "Detroit Lions"},
	{Abbreviation: "GB", Name: "Green Bay Packers"},
	{Abbreviation: "HOU", Name: "Houston Texans"},
	{Abbreviation: "IND", Name: "Indianapolis Colts"},
	{Abbreviation: "JAX", Name: "Jacksonville Jaguars"},
	{Abbreviation: "KC", Name: "Kansas City Chiefs"},
	{Abbreviation: "LAC", Name: "Los Angeles Chargers"},
	{Abbreviation: "LAR", Name: "Los Angeles Rams"},
	{Abbreviation: "LV", Name: "Las Vegas Raiders"},
	{Abbreviation: "MIA", Name: "Miami Dolphins"},
	{Abbreviation: "MIN", Name: "Minnesota Vikings"},
	{Abbreviation: "NE", Name: "New England Patriots"},
	{Abbreviation: "NO", Name: "New Orleans Saints"},
	{Abbreviation: "NYG", Name: "New York Giants"},
	{Abbreviation: "NYJ", Name: "New York Jets"},
	{Abbreviation: "PHI", Name: "Philadelphia Eagles"},
	{Abbreviation: "PIT", Name: "Pittsburgh Steelers"},
	{Abbreviation: "SEA", Name: "Seattle Seahawks"},
	{Abbreviation: "SF", Name: "San Francisco 49ers"},
	{Abbreviation: "TB", Name: "Tampa Bay Buccaneers"},
	{Abbreviation: "TEN", Name: "Tennessee Titans"},
	{Abbreviation: "WAS", Name: "Washington Commanders"},
}

// LoadTeams upserts the canonical team identities in one transaction.
func LoadTeams(ctx context.Context, db *sql.DB) (int, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin NFL team import: %w", err)
	}
	defer tx.Rollback()

	for _, team := range NFLTeams {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO nfl_teams (abbreviation, name)
			VALUES (?, ?)
			ON CONFLICT (abbreviation) DO UPDATE SET name = excluded.name
		`, team.Abbreviation, team.Name); err != nil {
			return 0, fmt.Errorf("upsert NFL team %s: %w", team.Abbreviation, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit NFL team import: %w", err)
	}
	return len(NFLTeams), nil
}
