package sleeper

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchPlayersParsesFlexibleProfileValues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/players/nfl" {
			t.Fatalf("request path = %q, want /players/nfl", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"123": {
				"player_id": 123,
				"first_name": "Test",
				"last_name": "Player",
				"position": "WR",
				"active": true,
				"number": "17",
				"years_exp": 4,
				"rotowire_id": 999,
				"gsis_id": null
			}
		}`))
	}))
	defer server.Close()

	client := NewClient()
	client.BaseURL = server.URL
	client.HTTPClient = server.Client()
	players, raw, err := client.FetchPlayers(context.Background())
	if err != nil {
		t.Fatalf("FetchPlayers() error = %v", err)
	}
	player := players["123"]
	if len(raw) == 0 || player.PlayerID.Value != "123" || player.Number.Value != 17 ||
		player.YearsExp.Value != 4 || player.RotowireID.Value != "999" || player.GSISID.Valid {
		t.Fatalf("parsed player = %#v", player)
	}
}

func TestFetchPlayersRejectsProviderErrorsAndEmptyData(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "provider error", status: http.StatusBadGateway, body: `{"error":"down"}`},
		{name: "empty map", status: http.StatusOK, body: `{}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			client := NewClient()
			client.BaseURL = server.URL
			client.HTTPClient = server.Client()
			if _, _, err := client.FetchPlayers(context.Background()); err == nil {
				t.Fatal("FetchPlayers() error = nil, want failure")
			}
		})
	}
}
