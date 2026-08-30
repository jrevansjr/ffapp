package fantasypros

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

type projectionResponse struct {
	Season    json.RawMessage    `json:"season"`
	Week      json.RawMessage    `json:"week"`
	Count     json.RawMessage    `json:"count"`
	Positions string             `json:"positions"`
	Players   []projectionPlayer `json:"players"`
}

type projectionPlayer struct {
	FantasyProsID json.RawMessage `json:"fpid"`
	Name          string          `json:"name"`
	Position      string          `json:"position_id"`
	Team          string          `json:"team_id"`
	Stats         projectionStats `json:"stats"`
}

type projectionStats struct {
	PassingYards        json.RawMessage `json:"pass_yds"`
	PassingTouchdowns   json.RawMessage `json:"pass_tds"`
	RushingYards        json.RawMessage `json:"rush_yds"`
	RushingTouchdowns   json.RawMessage `json:"rush_tds"`
	ReceivingYards      json.RawMessage `json:"rec_yds"`
	ReceivingTouchdowns json.RawMessage `json:"rec_tds"`
}

// ParseProjections validates the preseason projection response while keeping
// unavailable position statistics nil rather than turning them into zeroes.
func ParseProjections(body []byte, fetchedAt time.Time) (ProjectionDataset, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var response projectionResponse
	if err := decoder.Decode(&response); err != nil {
		return ProjectionDataset{}, fmt.Errorf("decode FantasyPros projections: %w", err)
	}
	season, err := rawInt(response.Season)
	if err != nil || season != Season {
		return ProjectionDataset{}, fmt.Errorf("FantasyPros projections season is %d; want %d", season, Season)
	}
	week, err := rawInt(response.Week)
	if err != nil || week != 0 {
		return ProjectionDataset{}, fmt.Errorf("FantasyPros projections week is %d; want preseason week 0", week)
	}
	count, err := rawInt(response.Count)
	if err != nil {
		return ProjectionDataset{}, fmt.Errorf("FantasyPros projections have an invalid count")
	}
	if count != len(response.Players) {
		return ProjectionDataset{}, fmt.Errorf(
			"FantasyPros projections count is %d but response contains %d players",
			count,
			len(response.Players),
		)
	}
	if !hasProjectionPositions(response.Positions) {
		return ProjectionDataset{}, fmt.Errorf(
			"FantasyPros projections positions are %q; want QB,RB,WR,TE",
			response.Positions,
		)
	}
	if len(response.Players) == 0 {
		return ProjectionDataset{}, fmt.Errorf("FantasyPros projections contain no players")
	}

	dataset := ProjectionDataset{UpdatedAt: fetchedAt.UTC()}
	seen := make(map[string]struct{}, len(response.Players))
	for index, player := range response.Players {
		position := normalizedPosition(player.Position)
		if !isDraftPosition(position) {
			return ProjectionDataset{}, fmt.Errorf(
				"FantasyPros projection player %d has unsupported position %q",
				index,
				player.Position,
			)
		}
		playerID, err := projectionPlayerIdentity(player, index, seen)
		if err != nil {
			return ProjectionDataset{}, err
		}
		projection, err := parseProjectionStats(playerID, position, player.Stats)
		if err != nil {
			return ProjectionDataset{}, err
		}
		projection.FantasyProsID = playerID
		projection.Name = strings.TrimSpace(player.Name)
		projection.Position = position
		projection.Team = strings.ToUpper(strings.TrimSpace(player.Team))
		dataset.Projections = append(dataset.Projections, projection)
	}
	return dataset, nil
}

func hasProjectionPositions(value string) bool {
	positions := make(map[string]struct{})
	for _, position := range strings.Split(value, ",") {
		positions[normalizedPosition(position)] = struct{}{}
	}
	for _, required := range []string{"QB", "RB", "WR", "TE"} {
		if _, found := positions[required]; !found {
			return false
		}
	}
	return true
}

func projectionPlayerIdentity(
	player projectionPlayer,
	index int,
	seen map[string]struct{},
) (string, error) {
	playerID, err := rawString(player.FantasyProsID)
	if err != nil || playerID == "" {
		return "", fmt.Errorf("FantasyPros projection player %d has no fpid", index)
	}
	if _, duplicate := seen[playerID]; duplicate {
		return "", fmt.Errorf("FantasyPros projections repeat fpid %s", playerID)
	}
	seen[playerID] = struct{}{}
	return playerID, nil
}

func parseProjectionStats(
	playerID string,
	position string,
	stats projectionStats,
) (PlayerProjection, error) {
	passingYards, err := optionalProjectionValue(stats.PassingYards)
	if err != nil {
		return PlayerProjection{}, projectionFieldError(playerID, "pass_yds", err)
	}
	passingTouchdowns, err := optionalProjectionValue(stats.PassingTouchdowns)
	if err != nil {
		return PlayerProjection{}, projectionFieldError(playerID, "pass_tds", err)
	}
	rushingYards, err := optionalProjectionValue(stats.RushingYards)
	if err != nil {
		return PlayerProjection{}, projectionFieldError(playerID, "rush_yds", err)
	}
	rushingTouchdowns, err := optionalProjectionValue(stats.RushingTouchdowns)
	if err != nil {
		return PlayerProjection{}, projectionFieldError(playerID, "rush_tds", err)
	}
	receivingYards, err := optionalProjectionValue(stats.ReceivingYards)
	if err != nil {
		return PlayerProjection{}, projectionFieldError(playerID, "rec_yds", err)
	}
	receivingTouchdowns, err := optionalProjectionValue(stats.ReceivingTouchdowns)
	if err != nil {
		return PlayerProjection{}, projectionFieldError(playerID, "rec_tds", err)
	}

	projection := PlayerProjection{
		PassingYards: passingYards, PassingTouchdowns: passingTouchdowns,
		RushingYards: rushingYards, RushingTouchdowns: rushingTouchdowns,
		ReceivingYards: receivingYards, ReceivingTouchdowns: receivingTouchdowns,
	}
	if err := validatePositionProjection(position, projection); err != nil {
		return PlayerProjection{}, fmt.Errorf("FantasyPros projection player %s: %w", playerID, err)
	}
	return positionProjection(position, projection), nil
}

// positionProjection removes provider-supplied zero placeholders for fields
// outside this app's approved position mapping. That keeps an inapplicable
// statistic distinct from a genuine projection of zero.
func positionProjection(position string, projection PlayerProjection) PlayerProjection {
	switch position {
	case "QB":
		projection.ReceivingYards = nil
		projection.ReceivingTouchdowns = nil
	case "RB":
		projection.PassingYards = nil
		projection.PassingTouchdowns = nil
	case "WR", "TE":
		projection.PassingYards = nil
		projection.PassingTouchdowns = nil
		projection.RushingYards = nil
		projection.RushingTouchdowns = nil
	}
	return projection
}

func optionalProjectionValue(raw json.RawMessage) (*float64, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	value, err := rawFloat(raw)
	if err != nil || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return nil, fmt.Errorf("value is not a nonnegative finite number")
	}
	return &value, nil
}

func projectionFieldError(playerID, field string, err error) error {
	return fmt.Errorf("FantasyPros projection player %s %s: %w", playerID, field, err)
}

func validatePositionProjection(position string, projection PlayerProjection) error {
	require := func(fields map[string]*float64) error {
		for name, value := range fields {
			if value == nil {
				return fmt.Errorf("position %s is missing %s", position, name)
			}
		}
		return nil
	}
	switch position {
	case "QB":
		return require(map[string]*float64{
			"pass_yds": projection.PassingYards, "pass_tds": projection.PassingTouchdowns,
			"rush_yds": projection.RushingYards, "rush_tds": projection.RushingTouchdowns,
		})
	case "RB":
		return require(map[string]*float64{
			"rush_yds": projection.RushingYards, "rush_tds": projection.RushingTouchdowns,
			"rec_yds": projection.ReceivingYards, "rec_tds": projection.ReceivingTouchdowns,
		})
	case "WR", "TE":
		return require(map[string]*float64{
			"rec_yds": projection.ReceivingYards, "rec_tds": projection.ReceivingTouchdowns,
		})
	default:
		return fmt.Errorf("unsupported position %s", position)
	}
}
