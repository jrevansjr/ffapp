// Package nflverse retrieves and parses the public datasets used for historical
// weekly player statistics. It contains no database or application policy.
package nflverse

// Season is the historical season loaded by milestone M6.2.
const Season = 2025

// WeeklyStat is the raw scoring input for one GSIS player in one regular-season
// week. Import code may combine duplicate player/week rows before persistence.
type WeeklyStat struct {
	GSISID     string
	PlayerName string
	Position   string
	Team       string
	Season     int
	Week       int

	PassingYards                 int
	PassingTouchdowns            int
	PassingInterceptions         int
	PassingTwoPointConversions   int
	RushingAttempts              int
	RushingYards                 int
	RushingTouchdowns            int
	RushingTwoPointConversions   int
	Receptions                   int
	Targets                      int
	ReceivingYards               int
	ReceivingTouchdowns          int
	ReceivingTwoPointConversions int
	FumblesLost                  int
}

// WeeklyDataset includes relevant parsed rows and the total number of CSV data
// rows, allowing the importer to report and validate source coverage.
type WeeklyDataset struct {
	Rows       []WeeklyStat
	SourceRows int
}

// PlayerID maps the Sleeper identifier used by this app to nflverse's GSIS
// identifier. Name is retained only for diagnostics; it is never used to match.
type PlayerID struct {
	SleeperID     string
	GSISID        string
	FantasyProsID string
	Name          string
}

// PlayerIDDataset includes usable identity rows and the total crosswalk rows.
type PlayerIDDataset struct {
	Rows       []PlayerID
	SourceRows int
}
