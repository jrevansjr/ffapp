package fantasypros

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL       = "https://api.fantasypros.com/public/v2/json"
	maximumResponseBytes = 8 << 20
)

// Client calls FantasyPros with a key supplied by local environment only.
// BaseURL, HTTPClient, and Now are configurable for tests.
type Client struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	Now        func() time.Time
}

// NewClient constructs the authenticated provider client without making a
// request. The key is never embedded in URLs or returned errors.
func NewClient(apiKey string) *Client {
	return &Client{
		BaseURL:    defaultBaseURL,
		APIKey:     strings.TrimSpace(apiKey),
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
		Now:        time.Now,
	}
}

// FetchADP makes one approved request for FantasyPros Aggregate ADP.
func (client *Client) FetchADP(ctx context.Context) (ADPDataset, []byte, error) {
	body, fetchedAt, err := client.fetch(ctx, DatasetADP, "ADP")
	if err != nil {
		return ADPDataset{}, nil, err
	}
	dataset, err := ParseADP(body, fetchedAt)
	if err != nil {
		return ADPDataset{}, nil, err
	}
	return dataset, body, nil
}

// FetchECR makes one approved request for FantasyPros half-PPR Draft ECR.
func (client *Client) FetchECR(ctx context.Context) (ECRDataset, []byte, error) {
	body, fetchedAt, err := client.fetch(ctx, DatasetECR, "DRAFT")
	if err != nil {
		return ECRDataset{}, nil, err
	}
	dataset, err := ParseECR(body, fetchedAt)
	if err != nil {
		return ECRDataset{}, nil, err
	}
	return dataset, body, nil
}

func (client *Client) fetch(
	ctx context.Context,
	dataset DatasetName,
	rankingType string,
) ([]byte, time.Time, error) {
	if strings.TrimSpace(client.APIKey) == "" {
		return nil, time.Time{}, fmt.Errorf("FANTASYPROS_API_KEY is required to refresh %s", dataset)
	}
	endpoint, err := url.Parse(strings.TrimRight(client.BaseURL, "/") + "/nfl/" + strconv.Itoa(Season) + "/consensus-rankings")
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("build FantasyPros URL: %w", err)
	}
	query := endpoint.Query()
	query.Set("position", "ALL")
	query.Set("type", rankingType)
	query.Set("scoring", "HALF")
	endpoint.RawQuery = query.Encode()

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("create FantasyPros %s request: %w", dataset, err)
	}
	httpRequest.Header.Set("x-api-key", client.APIKey)
	httpRequest.Header.Set("Accept", "application/json")
	response, err := client.HTTPClient.Do(httpRequest)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("request FantasyPros %s: %w", dataset, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, time.Time{}, fmt.Errorf("FantasyPros %s returned %s", dataset, response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumResponseBytes+1))
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("read FantasyPros %s: %w", dataset, err)
	}
	if len(body) > maximumResponseBytes {
		return nil, time.Time{}, fmt.Errorf("FantasyPros %s exceeds %d bytes", dataset, maximumResponseBytes)
	}
	return body, client.Now().UTC(), nil
}

type rankingResponse struct {
	Year            json.RawMessage `json:"year"`
	Count           int             `json:"count"`
	LastUpdatedUnix json.RawMessage `json:"last_updated_ts"`
	PositionID      string          `json:"position_id"`
	RankingTypeName string          `json:"ranking_type_name"`
	Scoring         string          `json:"scoring"`
	Players         []rankingPlayer `json:"players"`
}

type rankingPlayer struct {
	PlayerID     json.RawMessage `json:"player_id"`
	Name         string          `json:"player_name"`
	Position     string          `json:"player_position_id"`
	Team         string          `json:"player_team_id"`
	RankECR      json.RawMessage `json:"rank_ecr"`
	RankAverage  json.RawMessage `json:"rank_ave"`
	RankMin      json.RawMessage `json:"rank_min"`
	RankMax      json.RawMessage `json:"rank_max"`
	RankStdDev   json.RawMessage `json:"rank_std"`
	PositionRank string          `json:"pos_rank"`
	Tier         json.RawMessage `json:"tier"`
}

// ParseADP validates the stable fields consumed from the Aggregate ADP
// response. Numeric provider fields may be JSON numbers or strings.
func ParseADP(body []byte, fetchedAt time.Time) (ADPDataset, error) {
	response, updatedAt, err := parseResponse(body, DatasetADP, "ADP", fetchedAt)
	if err != nil {
		return ADPDataset{}, err
	}
	dataset := ADPDataset{UpdatedAt: updatedAt}
	seen := make(map[string]struct{}, len(response.Players))
	for index, player := range response.Players {
		position := normalizedPosition(player.Position)
		if !isDraftPosition(position) {
			continue
		}
		playerID, err := playerIdentity(player, index, DatasetADP, seen)
		if err != nil {
			return ADPDataset{}, err
		}
		adp, err := rawFloat(player.RankAverage)
		if err != nil || adp <= 0 || adp > 1000 {
			return ADPDataset{}, fmt.Errorf("FantasyPros ADP player %s has invalid rank_ave", playerID)
		}
		dataset.Rankings = append(dataset.Rankings, ADPRanking{
			FantasyProsID: playerID,
			Name:          strings.TrimSpace(player.Name),
			Position:      position,
			Team:          strings.ToUpper(strings.TrimSpace(player.Team)),
			ADP:           adp,
		})
	}
	if len(dataset.Rankings) == 0 {
		return ADPDataset{}, fmt.Errorf("FantasyPros ADP contains no QB/RB/WR/TE rankings")
	}
	return dataset, nil
}

// ParseECR validates overall Draft ECR, position rank, tier, and expert
// disagreement from one all-position half-PPR response.
func ParseECR(body []byte, fetchedAt time.Time) (ECRDataset, error) {
	response, updatedAt, err := parseResponse(body, DatasetECR, "DRAFT", fetchedAt)
	if err != nil {
		return ECRDataset{}, err
	}
	dataset := ECRDataset{UpdatedAt: updatedAt}
	seen := make(map[string]struct{}, len(response.Players))
	for index, player := range response.Players {
		position := normalizedPosition(player.Position)
		if !isDraftPosition(position) {
			continue
		}
		playerID, err := playerIdentity(player, index, DatasetECR, seen)
		if err != nil {
			return ECRDataset{}, err
		}
		overallRank, err := positiveRawInt(player.RankECR)
		if err != nil {
			return ECRDataset{}, fmt.Errorf("FantasyPros ECR player %s rank_ecr: %w", playerID, err)
		}
		positionRank, err := parsePositionRank(player.PositionRank, position)
		if err != nil {
			return ECRDataset{}, fmt.Errorf("FantasyPros ECR player %s pos_rank: %w", playerID, err)
		}
		tier, err := positiveRawInt(player.Tier)
		if err != nil {
			return ECRDataset{}, fmt.Errorf("FantasyPros ECR player %s tier: %w", playerID, err)
		}
		rankMin, err := positiveRawInt(player.RankMin)
		if err != nil {
			return ECRDataset{}, fmt.Errorf("FantasyPros ECR player %s rank_min: %w", playerID, err)
		}
		rankMax, err := positiveRawInt(player.RankMax)
		if err != nil || rankMax < rankMin {
			return ECRDataset{}, fmt.Errorf("FantasyPros ECR player %s has invalid rank_max", playerID)
		}
		rankStdDev, err := rawFloat(player.RankStdDev)
		if err != nil || rankStdDev < 0 {
			return ECRDataset{}, fmt.Errorf("FantasyPros ECR player %s has invalid rank_std", playerID)
		}
		dataset.Rankings = append(dataset.Rankings, ExpertRanking{
			FantasyProsID: playerID,
			Name:          strings.TrimSpace(player.Name),
			Position:      position,
			Team:          strings.ToUpper(strings.TrimSpace(player.Team)),
			OverallRank:   overallRank,
			PositionRank:  positionRank,
			Tier:          tier,
			RankMin:       rankMin,
			RankMax:       rankMax,
			RankStdDev:    rankStdDev,
		})
	}
	if len(dataset.Rankings) == 0 {
		return ECRDataset{}, fmt.Errorf("FantasyPros ECR contains no QB/RB/WR/TE rankings")
	}
	return dataset, nil
}

func parseResponse(
	body []byte,
	dataset DatasetName,
	rankingType string,
	fetchedAt time.Time,
) (rankingResponse, time.Time, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var response rankingResponse
	if err := decoder.Decode(&response); err != nil {
		return rankingResponse{}, time.Time{}, fmt.Errorf("decode FantasyPros %s: %w", dataset, err)
	}
	year, err := rawInt(response.Year)
	if err != nil || year != Season {
		return rankingResponse{}, time.Time{}, fmt.Errorf("FantasyPros %s season is %d; want %d", dataset, year, Season)
	}
	if !strings.EqualFold(response.PositionID, "ALL") {
		return rankingResponse{}, time.Time{}, fmt.Errorf("FantasyPros %s position is %q; want ALL", dataset, response.PositionID)
	}
	if !strings.EqualFold(response.Scoring, "HALF") {
		return rankingResponse{}, time.Time{}, fmt.Errorf("FantasyPros %s scoring is %q; want HALF", dataset, response.Scoring)
	}
	if !strings.EqualFold(response.RankingTypeName, rankingType) {
		return rankingResponse{}, time.Time{}, fmt.Errorf(
			"FantasyPros %s ranking type is %q; want %s",
			dataset,
			response.RankingTypeName,
			rankingType,
		)
	}
	if response.Count != 0 && response.Count != len(response.Players) {
		return rankingResponse{}, time.Time{}, fmt.Errorf(
			"FantasyPros %s count is %d but response contains %d players",
			dataset,
			response.Count,
			len(response.Players),
		)
	}
	if len(response.Players) == 0 {
		return rankingResponse{}, time.Time{}, fmt.Errorf("FantasyPros %s contains no rankings", dataset)
	}
	updatedAt := fetchedAt.UTC()
	if unix, unixErr := rawInt(response.LastUpdatedUnix); unixErr == nil && unix > 0 {
		updatedAt = time.Unix(int64(unix), 0).UTC()
	}
	return response, updatedAt, nil
}

func playerIdentity(
	player rankingPlayer,
	index int,
	dataset DatasetName,
	seen map[string]struct{},
) (string, error) {
	playerID, err := rawString(player.PlayerID)
	if err != nil || playerID == "" {
		return "", fmt.Errorf("FantasyPros %s player %d has no player_id", dataset, index)
	}
	if _, duplicate := seen[playerID]; duplicate {
		return "", fmt.Errorf("FantasyPros %s repeats player_id %s", dataset, playerID)
	}
	seen[playerID] = struct{}{}
	return playerID, nil
}

func normalizedPosition(position string) string {
	return strings.ToUpper(strings.TrimSpace(position))
}

func isDraftPosition(position string) bool {
	switch position {
	case "QB", "RB", "WR", "TE":
		return true
	default:
		return false
	}
}

func parsePositionRank(value, position string) (int, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if position == "" || !strings.HasPrefix(value, position) {
		return 0, fmt.Errorf("%q does not match position %q", value, position)
	}
	rank, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(value, position)))
	if err != nil || rank <= 0 {
		return 0, fmt.Errorf("%q is not a positive position rank", value)
	}
	return rank, nil
}

func positiveRawInt(raw json.RawMessage) (int, error) {
	value, err := rawInt(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("value is not a positive integer")
	}
	return value, nil
}

func rawString(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", fmt.Errorf("value is missing")
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text), nil
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err != nil {
		return "", fmt.Errorf("value %q is not a string or number", raw)
	}
	return number.String(), nil
}

func rawFloat(raw json.RawMessage) (float64, error) {
	value, err := rawString(raw)
	if err != nil {
		return 0, err
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("%q is not numeric", value)
	}
	return parsed, nil
}

func rawInt(raw json.RawMessage) (int, error) {
	value, err := rawFloat(raw)
	if err != nil || value != float64(int(value)) {
		return 0, fmt.Errorf("value is not an integer")
	}
	return int(value), nil
}
