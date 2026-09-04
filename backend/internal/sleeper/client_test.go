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
				"depth_chart_position": "SWR",
				"depth_chart_order": 2,
				"injury_status": "Questionable",
				"injury_body_part": "Knee",
				"injury_notes": "Limited workload",
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
		player.YearsExp.Value != 4 || player.DepthChartPosition.Value != "SWR" ||
		player.DepthChartOrder.Value != 2 || player.InjuryStatus.Value != "Questionable" ||
		player.InjuryBodyPart.Value != "Knee" || player.InjuryNotes.Value != "Limited workload" ||
		player.RotowireID.Value != "999" || player.GSISID.Valid {
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

func TestFetchDraftPicksParsesLivePickFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/draft/mock-123/picks" {
			t.Fatalf("request path = %q, want /draft/mock-123/picks", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[
			{
				"round": 2,
				"pick_no": 14,
				"draft_slot": 3,
				"roster_id": 7,
				"picked_by": "owner-1",
				"player_id": "known-player",
				"metadata": {
					"first_name": "Test",
					"last_name": "Receiver",
					"position": "WR",
					"team": "BUF"
				}
			}
		]`))
	}))
	defer server.Close()
	client := NewClient()
	client.BaseURL = server.URL
	client.HTTPClient = server.Client()

	picks, err := client.FetchDraftPicks(context.Background(), " mock-123 ")
	if err != nil {
		t.Fatalf("FetchDraftPicks() error = %v", err)
	}
	if len(picks) != 1 {
		t.Fatalf("pick count = %d, want 1", len(picks))
	}
	pick := picks[0]
	if pick.PickNumber != 14 || pick.Round != 2 || pick.DraftSlot != 3 ||
		pick.RosterID.Value != "7" || pick.PickedBy.Value != "owner-1" ||
		pick.SleeperPlayerID.Value != "known-player" || pick.Metadata.Team.Value != "BUF" {
		t.Fatalf("parsed pick = %#v", pick)
	}
}

func TestFetchDraftPicksRejectsInvalidResponses(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "provider error", status: http.StatusNotFound, body: `null`},
		{name: "malformed JSON", status: http.StatusOK, body: `[`},
		{name: "null response", status: http.StatusOK, body: `null`},
		{name: "missing player", status: http.StatusOK, body: `[{"pick_no":1}]`},
		{name: "duplicate pick", status: http.StatusOK, body: `[{"pick_no":1,"player_id":"a"},{"pick_no":1,"player_id":"b"}]`},
		{name: "duplicate player", status: http.StatusOK, body: `[{"pick_no":1,"player_id":"a"},{"pick_no":2,"player_id":"a"}]`},
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
			if _, err := client.FetchDraftPicks(context.Background(), "draft"); err == nil {
				t.Fatal("FetchDraftPicks() error = nil, want failure")
			}
		})
	}
}
