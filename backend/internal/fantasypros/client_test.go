package fantasypros

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

const adpFixture = `{
  "year": "2026",
  "count": 2,
  "last_updated_ts": 1787500000,
  "position_id": "ALL",
  "ranking_type_name": "adp",
  "scoring": "HALF",
  "players": [
    {"player_id": 101, "player_name": "Alpha QB", "player_position_id": "QB", "player_team_id": "ARI", "rank_ave": "12.5"},
    {"player_id": "102", "player_name": "Beta RB", "player_position_id": "rb", "player_team_id": "BUF", "rank_ave": 24}
  ]
}`

const ecrFixture = `{
  "year": 2026,
  "count": 2,
  "last_updated_ts": 1787500000,
  "position_id": "ALL",
  "ranking_type_name": "draft",
  "scoring": "HALF",
  "players": [
    {"player_id": "101", "player_name": "Alpha QB", "player_position_id": "QB", "player_team_id": "ARI", "rank_ecr": 10, "pos_rank": "QB2", "tier": 2, "rank_min": "7", "rank_max": "14", "rank_std": "2.5"},
    {"player_id": 102, "player_name": "Beta RB", "player_position_id": "RB", "player_team_id": "BUF", "rank_ecr": "20", "pos_rank": "RB10", "tier": "3", "rank_min": 15, "rank_max": 27, "rank_std": 3.25}
  ]
}`

func TestParseADPAndECRAcceptProviderNumberFormats(t *testing.T) {
	fetchedAt := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	adp, err := ParseADP([]byte(adpFixture), fetchedAt)
	if err != nil {
		t.Fatalf("ParseADP() error = %v", err)
	}
	if len(adp.Rankings) != 2 || adp.Rankings[0].FantasyProsID != "101" ||
		adp.Rankings[0].ADP != 12.5 || adp.Rankings[1].Position != "RB" {
		t.Fatalf("ADP dataset = %#v", adp)
	}
	if adp.UpdatedAt.Equal(fetchedAt) {
		t.Fatalf("ADP timestamp = %v, want provider timestamp", adp.UpdatedAt)
	}

	ecr, err := ParseECR([]byte(ecrFixture), fetchedAt)
	if err != nil {
		t.Fatalf("ParseECR() error = %v", err)
	}
	first := ecr.Rankings[0]
	if len(ecr.Rankings) != 2 || first.OverallRank != 10 || first.PositionRank != 2 ||
		first.Tier != 2 || first.RankMin != 7 || first.RankMax != 14 || first.RankStdDev != 2.5 {
		t.Fatalf("ECR dataset = %#v", ecr)
	}
}

func TestParseADPRejectsInvalidResponse(t *testing.T) {
	tests := []string{
		`{"year":2026,"position_id":"ALL","ranking_type_name":"ADP","scoring":"HALF","players":[{"rank_ave":1}]}`,
		`{"year":2026,"position_id":"ALL","ranking_type_name":"ADP","scoring":"HALF","players":[{"player_id":1,"rank_ave":0}]}`,
		`{"year":2026,"position_id":"ALL","ranking_type_name":"ADP","scoring":"HALF","players":[{"player_id":1,"rank_ave":1},{"player_id":"1","rank_ave":2}]}`,
		`{"year":2025,"position_id":"ALL","ranking_type_name":"ADP","scoring":"HALF","players":[{"player_id":1,"rank_ave":1}]}`,
		`{"year":2026,"count":2,"position_id":"ALL","ranking_type_name":"ADP","scoring":"HALF","players":[{"player_id":1,"rank_ave":1}]}`,
		`{"year":2026,"position_id":"ALL","ranking_type_name":"ADP","scoring":"PPR","players":[{"player_id":1,"rank_ave":1}]}`,
	}
	for _, body := range tests {
		if _, err := ParseADP([]byte(body), time.Now()); err == nil {
			t.Fatalf("ParseADP(%s) error = nil, want validation failure", body)
		}
	}
}

func TestParseECRRejectsInvalidDraftFields(t *testing.T) {
	tests := []string{
		`{"year":2026,"position_id":"ALL","ranking_type_name":"DRAFT","scoring":"HALF","players":[{"player_id":1,"player_position_id":"RB","rank_ecr":0,"pos_rank":"RB1","tier":1,"rank_min":1,"rank_max":2,"rank_std":1}]}`,
		`{"year":2026,"position_id":"ALL","ranking_type_name":"DRAFT","scoring":"HALF","players":[{"player_id":1,"player_position_id":"RB","rank_ecr":1,"pos_rank":"WR1","tier":1,"rank_min":1,"rank_max":2,"rank_std":1}]}`,
		`{"year":2026,"position_id":"ALL","ranking_type_name":"DRAFT","scoring":"HALF","players":[{"player_id":1,"player_position_id":"RB","rank_ecr":1,"pos_rank":"RB1","tier":1,"rank_min":3,"rank_max":2,"rank_std":1}]}`,
		`{"year":2026,"position_id":"ALL","ranking_type_name":"DRAFT","scoring":"HALF","players":[{"player_id":1,"player_position_id":"RB","rank_ecr":1,"pos_rank":"RB1","tier":1,"rank_min":1,"rank_max":2,"rank_std":-1}]}`,
	}
	for _, body := range tests {
		if _, err := ParseECR([]byte(body), time.Now()); err == nil {
			t.Fatalf("ParseECR(%s) error = nil, want validation failure", body)
		}
	}
}

func TestParseADPIgnoresPositionsOutsideTheAppPlayerPool(t *testing.T) {
	body := `{
		"year":2026,"count":2,"position_id":"ALL","ranking_type_name":"ADP","scoring":"HALF",
		"players":[
			{"player_id":1,"player_position_id":"RB","rank_ave":1},
			{"player_id":2,"player_position_id":"K","rank_ave":2}
		]
	}`
	dataset, err := ParseADP([]byte(body), time.Now())
	if err != nil {
		t.Fatalf("ParseADP() error = %v", err)
	}
	if len(dataset.Rankings) != 1 || dataset.Rankings[0].Position != "RB" {
		t.Fatalf("rankings = %#v, want only RB", dataset.Rankings)
	}
}

func TestFetchMethodsSendKeyOnlyInHeaderAndOneDatasetPerRequest(t *testing.T) {
	requests := 0
	client := NewClient("secret-test-key")
	client.BaseURL = "https://provider.test"
	client.Now = func() time.Time { return time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC) }
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) *http.Response {
		requests++
		if request.Header.Get("x-api-key") != "secret-test-key" {
			t.Errorf("x-api-key = %q", request.Header.Get("x-api-key"))
		}
		if request.URL.Query().Get("position") != "ALL" || request.URL.Query().Get("scoring") != "HALF" {
			t.Errorf("query = %s", request.URL.RawQuery)
		}
		switch request.URL.Query().Get("type") {
		case "ADP":
			return response(http.StatusOK, adpFixture)
		case "DRAFT":
			return response(http.StatusOK, ecrFixture)
		default:
			t.Fatalf("unexpected ranking type %q", request.URL.Query().Get("type"))
			return nil
		}
	})}
	if _, _, err := client.FetchADP(context.Background()); err != nil {
		t.Fatalf("FetchADP() error = %v", err)
	}
	if _, _, err := client.FetchECR(context.Background()); err != nil {
		t.Fatalf("FetchECR() error = %v", err)
	}
	if requests != 2 {
		t.Fatalf("request count = %d, want 2", requests)
	}
}

func TestMissingKeyMakesNoRequest(t *testing.T) {
	requests := 0
	client := NewClient("")
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) *http.Response {
		requests++
		return response(http.StatusOK, adpFixture)
	})}
	if _, _, err := client.FetchADP(context.Background()); err == nil {
		t.Fatal("FetchADP() error = nil, want missing-key failure")
	}
	if requests != 0 {
		t.Fatalf("request count = %d, want 0", requests)
	}
}

type roundTripFunc func(*http.Request) *http.Response

func (roundTrip roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request), nil
}

func response(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}
