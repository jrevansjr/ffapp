package nflverse

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

const testWeeklyHeader = "player_id,player_display_name,position_group,team,season,week,season_type,passing_yards,passing_tds,passing_interceptions,passing_2pt_conversions,carries,rushing_yards,rushing_tds,rushing_2pt_conversions,receptions,targets,receiving_yards,receiving_tds,receiving_2pt_conversions,fumbles_lost_total\n"

func TestParseWeeklyStatsFiltersAndUsesNamedColumns(t *testing.T) {
	body := testWeeklyHeader +
		"00-001,Quarter Back,QB,LA,2025,1,REG,250,2,1,0,3,12,0,0,0,0,0,0,0,1\n" +
		"00-002,Post Season,RB,BUF,2025,1,POST,0,0,0,0,10,50,1,0,0,0,0,0,0,0\n" +
		"00-003,Defender,DL,ARI,2025,1,REG,0,0,0,0,0,0,0,0,0,0,0,0,0,0\n"
	dataset, err := ParseWeeklyStats([]byte(body))
	if err != nil {
		t.Fatalf("ParseWeeklyStats() error = %v", err)
	}
	if dataset.SourceRows != 3 || len(dataset.Rows) != 1 {
		t.Fatalf("dataset = %#v", dataset)
	}
	stat := dataset.Rows[0]
	if stat.GSISID != "00-001" || stat.Team != "LA" || stat.PassingYards != 250 || stat.PassingTouchdowns != 2 || stat.FumblesLost != 1 {
		t.Fatalf("stat = %#v", stat)
	}
}

func TestParsePlayerIDsIgnoresMissingIDs(t *testing.T) {
	body := "name,gsis_id,sleeper_id,fantasypros_id\nMapped,00-001,1001,2001\nFantasyPros Only,NA,1002,2002\nMissing IDs,NA,1003,NA\nMissing Sleeper,00-003,NA,2003\n"
	dataset, err := ParsePlayerIDs([]byte(body))
	if err != nil {
		t.Fatalf("ParsePlayerIDs() error = %v", err)
	}
	if dataset.SourceRows != 4 || len(dataset.Rows) != 2 || dataset.Rows[0].SleeperID != "1001" ||
		dataset.Rows[0].FantasyProsID != "2001" || dataset.Rows[1].FantasyProsID != "2002" {
		t.Fatalf("dataset = %#v", dataset)
	}
}

func TestParseWeeklyStatsRequiresContractColumns(t *testing.T) {
	if _, err := ParseWeeklyStats([]byte("player_id,season\n00-001,2025\n")); err == nil || !strings.Contains(err.Error(), "missing required column") {
		t.Fatalf("ParseWeeklyStats() error = %v, want missing-column error", err)
	}
}

func TestClientRejectsHTTPFailure(t *testing.T) {
	client := NewClient()
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) *http.Response {
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Status:     "502 Bad Gateway",
			Body:       io.NopCloser(strings.NewReader("unavailable")),
			Header:     make(http.Header),
		}
	})}
	if _, _, err := client.FetchWeeklyStats(context.Background()); err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("FetchWeeklyStats() error = %v, want status error", err)
	}
}

type roundTripFunc func(*http.Request) *http.Response

func (roundTrip roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request), nil
}
