package nflverse

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	defaultWeeklyStatsURL = "https://github.com/nflverse/nflverse-data/releases/download/stats_player/stats_player_week_2025.csv"
	defaultPlayerIDsURL   = "https://raw.githubusercontent.com/dynastyprocess/data/master/files/db_playerids.csv"
	maximumStatsBytes     = 64 << 20
	maximumPlayerIDsBytes = 16 << 20
)

// Client downloads the two public CSV sources required by the stats importer.
// URLs and HTTPClient are exported to keep provider behavior testable locally.
type Client struct {
	WeeklyStatsURL string
	PlayerIDsURL   string
	HTTPClient     *http.Client
}

// NewClient returns a client with the pinned M6.2 dataset URLs and a timeout
// generous enough for the larger nflverse release asset.
func NewClient() *Client {
	return &Client{
		WeeklyStatsURL: defaultWeeklyStatsURL,
		PlayerIDsURL:   defaultPlayerIDsURL,
		HTTPClient:     &http.Client{Timeout: 5 * time.Minute},
	}
}

// FetchWeeklyStats downloads and validates the nflverse weekly CSV. The raw
// bytes are returned so the caller can cache the exact parsed source.
func (client *Client) FetchWeeklyStats(ctx context.Context) (WeeklyDataset, []byte, error) {
	body, err := client.get(ctx, client.WeeklyStatsURL, maximumStatsBytes)
	if err != nil {
		return WeeklyDataset{}, nil, fmt.Errorf("fetch nflverse weekly stats: %w", err)
	}
	dataset, err := ParseWeeklyStats(body)
	if err != nil {
		return WeeklyDataset{}, nil, err
	}
	return dataset, body, nil
}

// FetchPlayerIDs downloads and validates the DynastyProcess player-ID
// crosswalk. The crosswalk is public and requires no API key.
func (client *Client) FetchPlayerIDs(ctx context.Context) (PlayerIDDataset, []byte, error) {
	body, err := client.get(ctx, client.PlayerIDsURL, maximumPlayerIDsBytes)
	if err != nil {
		return PlayerIDDataset{}, nil, fmt.Errorf("fetch DynastyProcess player IDs: %w", err)
	}
	dataset, err := ParsePlayerIDs(body)
	if err != nil {
		return PlayerIDDataset{}, nil, err
	}
	return dataset, body, nil
}

func (client *Client) get(ctx context.Context, url string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	response, err := client.HTTPClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s returned %s", url, response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("response exceeds %d bytes", limit)
	}
	return body, nil
}

var weeklyColumns = []string{
	"player_id", "player_display_name", "position_group", "team", "season", "week", "season_type",
	"passing_yards", "passing_tds", "passing_interceptions", "passing_2pt_conversions",
	"carries", "rushing_yards", "rushing_tds", "rushing_2pt_conversions",
	"receptions", "targets", "receiving_yards", "receiving_tds",
	"receiving_2pt_conversions", "fumbles_lost_total",
}

// ParseWeeklyStats parses regular-season QB/RB/WR/TE scoring inputs from an
// nflverse CSV. Column names, rather than column positions, define the contract.
func ParseWeeklyStats(body []byte) (WeeklyDataset, error) {
	reader := csv.NewReader(bytes.NewReader(body))
	header, err := reader.Read()
	if err != nil {
		return WeeklyDataset{}, fmt.Errorf("read nflverse header: %w", err)
	}
	columns, err := requireColumns(header, weeklyColumns)
	if err != nil {
		return WeeklyDataset{}, fmt.Errorf("validate nflverse header: %w", err)
	}
	reader.FieldsPerRecord = len(header)
	reader.ReuseRecord = true

	dataset := WeeklyDataset{}
	for line := 2; ; line++ {
		record, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return WeeklyDataset{}, fmt.Errorf("read nflverse row %d: %w", line, readErr)
		}
		dataset.SourceRows++
		season, err := csvInt(record[columns["season"]])
		if err != nil {
			return WeeklyDataset{}, fmt.Errorf("parse nflverse row %d season: %w", line, err)
		}
		position := strings.ToUpper(strings.TrimSpace(record[columns["position_group"]]))
		if season != Season || strings.TrimSpace(record[columns["season_type"]]) != "REG" || !fantasyPosition(position) {
			continue
		}

		stat := WeeklyStat{
			GSISID:     cleanValue(record[columns["player_id"]]),
			PlayerName: cleanValue(record[columns["player_display_name"]]),
			Position:   position,
			Team:       strings.ToUpper(cleanValue(record[columns["team"]])),
			Season:     season,
		}
		if stat.GSISID == "" {
			return WeeklyDataset{}, fmt.Errorf("nflverse row %d has no player_id", line)
		}
		if stat.Team == "" {
			return WeeklyDataset{}, fmt.Errorf("nflverse row %d has no team", line)
		}
		integerFields := []struct {
			column string
			target *int
		}{
			{"week", &stat.Week},
			{"passing_yards", &stat.PassingYards},
			{"passing_tds", &stat.PassingTouchdowns},
			{"passing_interceptions", &stat.PassingInterceptions},
			{"passing_2pt_conversions", &stat.PassingTwoPointConversions},
			{"carries", &stat.RushingAttempts},
			{"rushing_yards", &stat.RushingYards},
			{"rushing_tds", &stat.RushingTouchdowns},
			{"rushing_2pt_conversions", &stat.RushingTwoPointConversions},
			{"receptions", &stat.Receptions},
			{"targets", &stat.Targets},
			{"receiving_yards", &stat.ReceivingYards},
			{"receiving_tds", &stat.ReceivingTouchdowns},
			{"receiving_2pt_conversions", &stat.ReceivingTwoPointConversions},
			{"fumbles_lost_total", &stat.FumblesLost},
		}
		for _, field := range integerFields {
			value, err := csvInt(record[columns[field.column]])
			if err != nil {
				return WeeklyDataset{}, fmt.Errorf("parse nflverse row %d %s: %w", line, field.column, err)
			}
			*field.target = value
		}
		if stat.Week < 1 || stat.Week > 18 {
			return WeeklyDataset{}, fmt.Errorf("nflverse row %d has invalid regular-season week %d", line, stat.Week)
		}
		dataset.Rows = append(dataset.Rows, stat)
	}
	if len(dataset.Rows) == 0 {
		return WeeklyDataset{}, fmt.Errorf("nflverse CSV contains no %d regular-season fantasy-position rows", Season)
	}
	return dataset, nil
}

// ParsePlayerIDs reads the exact external IDs used by implemented importers
// from the DynastyProcess crosswalk. Names remain diagnostic only.
func ParsePlayerIDs(body []byte) (PlayerIDDataset, error) {
	reader := csv.NewReader(bytes.NewReader(body))
	header, err := reader.Read()
	if err != nil {
		return PlayerIDDataset{}, fmt.Errorf("read player-ID header: %w", err)
	}
	columns, err := requireColumns(header, []string{"sleeper_id", "gsis_id", "fantasypros_id", "name"})
	if err != nil {
		return PlayerIDDataset{}, fmt.Errorf("validate player-ID header: %w", err)
	}
	reader.FieldsPerRecord = len(header)
	reader.ReuseRecord = true
	dataset := PlayerIDDataset{}
	for line := 2; ; line++ {
		record, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return PlayerIDDataset{}, fmt.Errorf("read player-ID row %d: %w", line, readErr)
		}
		dataset.SourceRows++
		row := PlayerID{
			SleeperID:     cleanValue(record[columns["sleeper_id"]]),
			GSISID:        cleanValue(record[columns["gsis_id"]]),
			FantasyProsID: cleanValue(record[columns["fantasypros_id"]]),
			Name:          cleanValue(record[columns["name"]]),
		}
		if row.SleeperID != "" && (row.GSISID != "" || row.FantasyProsID != "") {
			dataset.Rows = append(dataset.Rows, row)
		}
	}
	if len(dataset.Rows) == 0 {
		return PlayerIDDataset{}, fmt.Errorf("player-ID CSV contains no usable Sleeper mappings")
	}
	return dataset, nil
}

func requireColumns(header, required []string) (map[string]int, error) {
	columns := make(map[string]int, len(header))
	for index, name := range header {
		columns[strings.TrimSpace(name)] = index
	}
	for _, name := range required {
		if _, found := columns[name]; !found {
			return nil, fmt.Errorf("missing required column %q", name)
		}
	}
	return columns, nil
}

func csvInt(raw string) (int, error) {
	value := cleanValue(raw)
	if value == "" {
		return 0, nil
	}
	if integer, err := strconv.Atoi(value); err == nil {
		return integer, nil
	}
	decimal, err := strconv.ParseFloat(value, 64)
	if err != nil || decimal != float64(int(decimal)) {
		return 0, fmt.Errorf("%q is not an integer", raw)
	}
	return int(decimal), nil
}

func cleanValue(value string) string {
	value = strings.TrimSpace(value)
	if strings.EqualFold(value, "NA") {
		return ""
	}
	return value
}

func fantasyPosition(position string) bool {
	switch position {
	case "QB", "RB", "WR", "TE":
		return true
	default:
		return false
	}
}
