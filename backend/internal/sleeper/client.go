package sleeper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://api.sleeper.app/v1"
	maxPlayerBytes = 32 << 20
	maxPicksBytes  = 4 << 20
)

// Client retrieves public Sleeper data for explicit player imports and the one
// managed live draft poller.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// FetchDraftPicks retrieves the current ordered pick list for one draft. It is
// intentionally separate from player imports because the live poller calls
// this small endpoint repeatedly while the application is running.
func (client *Client) FetchDraftPicks(ctx context.Context, draftID string) ([]DraftPick, error) {
	draftID = strings.TrimSpace(draftID)
	if draftID == "" {
		return nil, fmt.Errorf("Sleeper draft ID is required")
	}

	baseURL := strings.TrimRight(client.BaseURL, "/")
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		baseURL+"/draft/"+draftID+"/picks",
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("create Sleeper draft-picks request: %w", err)
	}
	response, err := client.HTTPClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch Sleeper draft picks: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxPicksBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read Sleeper draft-picks response: %w", err)
	}
	if len(body) > maxPicksBytes {
		return nil, fmt.Errorf("Sleeper draft-picks response exceeds %d bytes", maxPicksBytes)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Sleeper draft-picks request returned %s", response.Status)
	}
	if bytes.Equal(bytes.TrimSpace(body), []byte("null")) {
		return nil, fmt.Errorf("Sleeper draft-picks response is null")
	}

	var picks []DraftPick
	if err := json.Unmarshal(body, &picks); err != nil {
		return nil, fmt.Errorf("decode Sleeper draft-picks response: %w", err)
	}
	seenPickNumbers := make(map[int]struct{}, len(picks))
	seenPlayerIDs := make(map[string]struct{}, len(picks))
	for _, pick := range picks {
		if pick.PickNumber <= 0 || !pick.SleeperPlayerID.Valid {
			return nil, fmt.Errorf("Sleeper draft-picks response contains an invalid pick")
		}
		if _, exists := seenPickNumbers[pick.PickNumber]; exists {
			return nil, fmt.Errorf("Sleeper draft-picks response repeats pick number %d", pick.PickNumber)
		}
		if _, exists := seenPlayerIDs[pick.SleeperPlayerID.Value]; exists {
			return nil, fmt.Errorf("Sleeper draft-picks response repeats player %s", pick.SleeperPlayerID.Value)
		}
		seenPickNumbers[pick.PickNumber] = struct{}{}
		seenPlayerIDs[pick.SleeperPlayerID.Value] = struct{}{}
	}
	if picks == nil {
		picks = make([]DraftPick, 0)
	}
	return picks, nil
}

// NewClient returns a Sleeper client with a bounded request timeout.
func NewClient() *Client {
	return &Client{
		BaseURL:    defaultBaseURL,
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// FetchPlayers downloads and validates Sleeper's complete NFL player map. It
// returns both parsed players and the raw response used by the local cache.
func (client *Client) FetchPlayers(ctx context.Context) (PlayersResponse, []byte, error) {
	baseURL := strings.TrimRight(client.BaseURL, "/")
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/players/nfl", nil)
	if err != nil {
		return nil, nil, fmt.Errorf("create Sleeper players request: %w", err)
	}
	response, err := client.HTTPClient.Do(request)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch Sleeper players: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxPlayerBytes+1))
	if err != nil {
		return nil, nil, fmt.Errorf("read Sleeper players response: %w", err)
	}
	if len(body) > maxPlayerBytes {
		return nil, nil, fmt.Errorf("Sleeper players response exceeds %d bytes", maxPlayerBytes)
	}
	if response.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("Sleeper players request returned %s", response.Status)
	}
	players, err := ParsePlayers(body)
	if err != nil {
		return nil, nil, err
	}
	return players, body, nil
}

// ParsePlayers decodes a cached or freshly downloaded player map.
func ParsePlayers(body []byte) (PlayersResponse, error) {
	var players PlayersResponse
	if err := json.Unmarshal(body, &players); err != nil {
		return nil, fmt.Errorf("decode Sleeper players response: %w", err)
	}
	if len(players) == 0 {
		return nil, fmt.Errorf("Sleeper players response is empty")
	}
	return players, nil
}
