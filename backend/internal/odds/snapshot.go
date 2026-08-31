// Package odds parses the committed sportsbook-consensus snapshot. It does
// not contact sportsbooks or calculate consensus values from the raw quotes.
package odds

import (
	"embed"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"strconv"
	"strings"
	"time"
)

const (
	MarketPassingYards        = "passing_yards"
	MarketPassingTouchdowns   = "passing_touchdowns"
	MarketRushingYards        = "rushing_yards"
	MarketRushingTouchdowns   = "rushing_touchdowns"
	MarketReceivingYards      = "receiving_yards"
	MarketReceivingTouchdowns = "receiving_touchdowns"
	MarketRegularSeasonWins   = "regular_season_wins"
)

var (
	//go:embed metadata.json nfl_2026_*.csv
	embeddedSnapshot embed.FS

	playerFiles = []struct {
		Name   string
		Market string
	}{
		{Name: "nfl_2026_pass_yards.csv", Market: MarketPassingYards},
		{Name: "nfl_2026_pass_td.csv", Market: MarketPassingTouchdowns},
		{Name: "nfl_2026_rush_yards.csv", Market: MarketRushingYards},
		{Name: "nfl_2026_rush_td.csv", Market: MarketRushingTouchdowns},
		{Name: "nfl_2026_rec_yards.csv", Market: MarketReceivingYards},
		{Name: "nfl_2026_rec_td.csv", Market: MarketReceivingTouchdowns},
	}
)

type metadata struct {
	Season      int    `json:"season"`
	Source      string `json:"source"`
	CapturedAt  string `json:"captured_at"`
	Description string `json:"description"`
}

// PlayerLine is one supplied consensus for a named player and market.
type PlayerLine struct {
	Name   string
	Team   string
	Market string
	Line   float64
}

// TeamLine is one supplied consensus for an NFL team and market.
type TeamLine struct {
	Team   string
	Market string
	Line   float64
}

// Snapshot contains parsed consensus rows and their shared provenance. Rows
// without a consensus remain counted but are not represented as zero-valued lines.
type Snapshot struct {
	Season          int
	Source          string
	CapturedAt      time.Time
	Description     string
	SourceRows      int
	NoConsensusRows int
	PlayerLines     []PlayerLine
	TeamLines       []TeamLine
}

// LoadSnapshot validates and returns the CSV snapshot compiled into the data command.
func LoadSnapshot() (Snapshot, error) {
	return ParseSnapshot(embeddedSnapshot)
}

// ParseSnapshot loads the required metadata and seven CSV files from a file
// system. The fs parameter keeps complete rebuild tests offline and small.
func ParseSnapshot(files fs.FS) (Snapshot, error) {
	var snapshot Snapshot
	body, err := fs.ReadFile(files, "metadata.json")
	if err != nil {
		return snapshot, fmt.Errorf("read odds metadata: %w", err)
	}
	var meta metadata
	if err := json.Unmarshal(body, &meta); err != nil {
		return snapshot, fmt.Errorf("decode odds metadata: %w", err)
	}
	if meta.Season <= 0 || meta.Source == "" || meta.Description == "" {
		return snapshot, fmt.Errorf("odds metadata requires season, source, and description")
	}
	capturedAt, err := time.Parse(time.RFC3339, meta.CapturedAt)
	if err != nil || capturedAt.Location() != time.UTC {
		return snapshot, fmt.Errorf("odds captured_at must be UTC RFC3339")
	}
	snapshot.Season = meta.Season
	snapshot.Source = meta.Source
	snapshot.CapturedAt = capturedAt
	snapshot.Description = meta.Description

	for _, spec := range playerFiles {
		file, err := files.Open(spec.Name)
		if err != nil {
			return Snapshot{}, fmt.Errorf("open odds file %s: %w", spec.Name, err)
		}
		lines, sourceRows, skippedRows, parseErr := parsePlayerCSV(file, spec.Name, spec.Market)
		_ = file.Close()
		if parseErr != nil {
			return Snapshot{}, parseErr
		}
		snapshot.PlayerLines = append(snapshot.PlayerLines, lines...)
		snapshot.SourceRows += sourceRows
		snapshot.NoConsensusRows += skippedRows
	}

	file, err := files.Open("nfl_2026_win_totals.csv")
	if err != nil {
		return Snapshot{}, fmt.Errorf("open team win-total file: %w", err)
	}
	lines, sourceRows, skippedRows, parseErr := parseTeamCSV(file, "nfl_2026_win_totals.csv")
	_ = file.Close()
	if parseErr != nil {
		return Snapshot{}, parseErr
	}
	snapshot.TeamLines = lines
	snapshot.SourceRows += sourceRows
	snapshot.NoConsensusRows += skippedRows
	if len(snapshot.PlayerLines)+len(snapshot.TeamLines) == 0 {
		return Snapshot{}, fmt.Errorf("odds snapshot contains no consensus lines")
	}
	return snapshot, nil
}

// PositionAllowed reports whether a market belongs to a player's position.
func PositionAllowed(market, position string) bool {
	switch market {
	case MarketPassingYards, MarketPassingTouchdowns:
		return position == "QB"
	case MarketRushingYards, MarketRushingTouchdowns:
		return position == "QB" || position == "RB"
	case MarketReceivingYards, MarketReceivingTouchdowns:
		return position == "RB" || position == "WR" || position == "TE"
	default:
		return false
	}
}

func parsePlayerCSV(reader io.Reader, filename, market string) ([]PlayerLine, int, int, error) {
	records, header, err := readCSV(reader, filename)
	if err != nil {
		return nil, 0, 0, err
	}
	playerColumn, err := requiredColumn(header, "Player", filename)
	if err != nil {
		return nil, 0, 0, err
	}
	teamColumn, err := requiredColumn(header, "Team", filename)
	if err != nil {
		return nil, 0, 0, err
	}
	lineColumn, err := requiredColumn(header, "Consensus_Line", filename)
	if err != nil {
		return nil, 0, 0, err
	}

	lines := make([]PlayerLine, 0, len(records))
	seen := make(map[string]struct{}, len(records))
	skipped := 0
	for index, record := range records {
		row := index + 2
		name := strings.TrimSpace(record[playerColumn])
		team := strings.TrimSpace(record[teamColumn])
		if name == "" || team == "" {
			return nil, 0, 0, fmt.Errorf("%s row %d requires player and team", filename, row)
		}
		key := name + "\x00" + team
		if _, duplicate := seen[key]; duplicate {
			return nil, 0, 0, fmt.Errorf("%s repeats player %q on team %s", filename, name, team)
		}
		seen[key] = struct{}{}
		value := strings.TrimSpace(record[lineColumn])
		if value == "" {
			skipped++
			continue
		}
		line, err := strconv.ParseFloat(value, 64)
		if err != nil || line <= 0 {
			return nil, 0, 0, fmt.Errorf("%s row %d has invalid consensus line %q", filename, row, value)
		}
		lines = append(lines, PlayerLine{Name: name, Team: team, Market: market, Line: line})
	}
	return lines, len(records), skipped, nil
}

func parseTeamCSV(reader io.Reader, filename string) ([]TeamLine, int, int, error) {
	records, header, err := readCSV(reader, filename)
	if err != nil {
		return nil, 0, 0, err
	}
	teamColumn, err := requiredColumn(header, "Team", filename)
	if err != nil {
		return nil, 0, 0, err
	}
	lineColumn, err := requiredColumn(header, "Consensus_Win_Total", filename)
	if err != nil {
		return nil, 0, 0, err
	}

	lines := make([]TeamLine, 0, len(records))
	seen := make(map[string]struct{}, len(records))
	skipped := 0
	for index, record := range records {
		row := index + 2
		team := strings.TrimSpace(record[teamColumn])
		if team == "" {
			return nil, 0, 0, fmt.Errorf("%s row %d requires team", filename, row)
		}
		if _, duplicate := seen[team]; duplicate {
			return nil, 0, 0, fmt.Errorf("%s repeats team %q", filename, team)
		}
		seen[team] = struct{}{}
		value := strings.TrimSpace(record[lineColumn])
		if value == "" {
			skipped++
			continue
		}
		line, err := strconv.ParseFloat(value, 64)
		if err != nil || line <= 0 {
			return nil, 0, 0, fmt.Errorf("%s row %d has invalid consensus line %q", filename, row, value)
		}
		lines = append(lines, TeamLine{Team: team, Market: MarketRegularSeasonWins, Line: line})
	}
	return lines, len(records), skipped, nil
}

func readCSV(reader io.Reader, filename string) ([][]string, map[string]int, error) {
	csvReader := csv.NewReader(reader)
	records, err := csvReader.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", filename, err)
	}
	if len(records) < 2 {
		return nil, nil, fmt.Errorf("%s contains no data rows", filename)
	}
	header := make(map[string]int, len(records[0]))
	for index, value := range records[0] {
		name := strings.TrimSpace(value)
		if name == "" {
			return nil, nil, fmt.Errorf("%s contains an empty header", filename)
		}
		if _, duplicate := header[name]; duplicate {
			return nil, nil, fmt.Errorf("%s repeats header %q", filename, name)
		}
		header[name] = index
	}
	return records[1:], header, nil
}

func requiredColumn(header map[string]int, name, filename string) (int, error) {
	column, found := header[name]
	if !found {
		return 0, fmt.Errorf("%s is missing required column %s", filename, name)
	}
	return column, nil
}
