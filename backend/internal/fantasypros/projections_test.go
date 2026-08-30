package fantasypros

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

const projectionsFixture = `{
  "season":"2026","week":"0","count":"4","positions":"QB,RB,WR,TE","scoring":"STD",
  "players":[
    {"fpid":101,"name":"Alpha QB","position_id":"QB","team_id":"ARI","stats":{"pass_yds":4000.5,"pass_tds":30.2,"rush_yds":350.1,"rush_tds":4.5}},
    {"fpid":"102","name":"Beta RB","position_id":"RB","team_id":"BUF","stats":{"rush_yds":1000.2,"rush_tds":8.1,"rec_yds":450.3,"rec_tds":2.2}},
    {"fpid":103,"name":"Gamma WR","position_id":"WR","team_id":"CAR","stats":{"rush_yds":0,"rush_tds":0,"rec_yds":1100.4,"rec_tds":7.3}},
    {"fpid":104,"name":"Delta TE","position_id":"TE","team_id":"DAL","stats":{"rec_yds":700.6,"rec_tds":5.4}}
  ]
}`

func TestParseProjectionsPreservesPositionSpecificValues(t *testing.T) {
	fetchedAt := time.Date(2026, 8, 30, 0, 49, 31, 0, time.UTC)
	dataset, err := ParseProjections([]byte(projectionsFixture), fetchedAt)
	if err != nil {
		t.Fatalf("ParseProjections() error = %v", err)
	}
	if len(dataset.Projections) != 4 || dataset.Projections[0].FantasyProsID != "101" {
		t.Fatalf("dataset = %#v", dataset)
	}
	if dataset.Projections[0].PassingYards == nil || *dataset.Projections[0].PassingYards != 4000.5 {
		t.Fatalf("QB passing yards = %v", dataset.Projections[0].PassingYards)
	}
	if dataset.Projections[2].PassingYards != nil {
		t.Fatalf("WR passing yards = %v, want nil", dataset.Projections[2].PassingYards)
	}
	if dataset.Projections[2].RushingYards != nil {
		t.Fatalf("WR rushing yards = %v, want nil", dataset.Projections[2].RushingYards)
	}
}

func TestParseProjectionsRejectsUnsafeResponses(t *testing.T) {
	tests := map[string]string{
		"wrong season":    strings.Replace(projectionsFixture, `"season":"2026"`, `"season":"2025"`, 1),
		"regular week":    strings.Replace(projectionsFixture, `"week":"0"`, `"week":"1"`, 1),
		"wrong count":     strings.Replace(projectionsFixture, `"count":"4"`, `"count":"5"`, 1),
		"duplicate ID":    strings.Replace(projectionsFixture, `"fpid":"102"`, `"fpid":"101"`, 1),
		"negative stat":   strings.Replace(projectionsFixture, `"pass_yds":4000.5`, `"pass_yds":-1`, 1),
		"missing QB stat": strings.Replace(projectionsFixture, `"pass_tds":30.2`, `"pass_tds":null`, 1),
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseProjections([]byte(body), time.Now()); err == nil {
				t.Fatal("ParseProjections() error = nil, want validation failure")
			}
		})
	}
}

func TestFetchProjectionsUsesOneAllPositionRequest(t *testing.T) {
	requests := 0
	client := NewClient("secret-test-key")
	client.BaseURL = "https://provider.test"
	client.Now = func() time.Time { return time.Date(2026, 8, 30, 0, 49, 31, 0, time.UTC) }
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) *http.Response {
		requests++
		if request.Header.Get("x-api-key") != "secret-test-key" || strings.Contains(request.URL.String(), "secret-test-key") {
			t.Fatalf("API key was not confined to x-api-key header")
		}
		if request.URL.Path != "/nfl/2026/projections" || request.URL.Query().Get("positions") != "QB:RB:WR:TE" ||
			request.URL.Query().Get("week") != "0" || request.URL.Query().Get("scoring") != "HALF" {
			t.Fatalf("request URL = %s", request.URL.String())
		}
		return response(http.StatusOK, projectionsFixture)
	})}
	if _, _, err := client.FetchProjections(context.Background()); err != nil {
		t.Fatalf("FetchProjections() error = %v", err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
}
