package sleeper

import (
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
)

// Client retrieves public Sleeper data for explicit import commands.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
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
