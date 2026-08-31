package odds

import (
	"strings"
	"testing"
)

func TestEmbeddedSnapshotParsesExpectedConsensusRows(t *testing.T) {
	snapshot, err := LoadSnapshot()
	if err != nil {
		t.Fatalf("LoadSnapshot() error = %v", err)
	}
	if snapshot.Season != 2026 || snapshot.Source != "sportsbook_consensus" ||
		snapshot.CapturedAt.Format("2006-01-02T15:04:05Z07:00") != "2026-08-31T01:23:28Z" {
		t.Fatalf("snapshot metadata = %#v", snapshot)
	}
	if snapshot.SourceRows != 338 || snapshot.NoConsensusRows != 36 ||
		len(snapshot.PlayerLines) != 270 || len(snapshot.TeamLines) != 32 {
		t.Fatalf(
			"snapshot counts = source %d, blank %d, player %d, team %d",
			snapshot.SourceRows,
			snapshot.NoConsensusRows,
			len(snapshot.PlayerLines),
			len(snapshot.TeamLines),
		)
	}
}

func TestParsePlayerCSVValidatesRows(t *testing.T) {
	tests := []struct {
		name    string
		csv     string
		want    int
		skipped int
		wantErr bool
	}{
		{name: "valid and blank", csv: "Player,Team,Consensus_Line\nOne Player,ARI,10.5\nTwo Player,BUF,\n", want: 1, skipped: 1},
		{name: "missing header", csv: "Player,Team,Line\nOne Player,ARI,10.5\n", wantErr: true},
		{name: "invalid number", csv: "Player,Team,Consensus_Line\nOne Player,ARI,nope\n", wantErr: true},
		{name: "non-positive", csv: "Player,Team,Consensus_Line\nOne Player,ARI,0\n", wantErr: true},
		{name: "duplicate", csv: "Player,Team,Consensus_Line\nOne Player,ARI,10.5\nOne Player,ARI,11.5\n", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lines, _, skipped, err := parsePlayerCSV(
				strings.NewReader(test.csv),
				"fixture.csv",
				MarketPassingYards,
			)
			if test.wantErr {
				if err == nil {
					t.Fatal("parsePlayerCSV() error = nil")
				}
				return
			}
			if err != nil || len(lines) != test.want || skipped != test.skipped {
				t.Fatalf("parsePlayerCSV() = %d lines, %d skipped, %v", len(lines), skipped, err)
			}
		})
	}
}

func TestParseTeamCSVValidatesRows(t *testing.T) {
	lines, rows, skipped, err := parseTeamCSV(
		strings.NewReader("Team,Consensus_Win_Total\nArizona Cardinals,7.5\nBuffalo Bills,\n"),
		"teams.csv",
	)
	if err != nil || len(lines) != 1 || rows != 2 || skipped != 1 {
		t.Fatalf("parseTeamCSV() = %d lines, %d rows, %d skipped, %v", len(lines), rows, skipped, err)
	}
	if _, _, _, err := parseTeamCSV(
		strings.NewReader("Team,Consensus_Win_Total\nArizona Cardinals,nope\n"),
		"teams.csv",
	); err == nil {
		t.Fatal("parseTeamCSV() invalid number error = nil")
	}
}

func TestPositionAllowed(t *testing.T) {
	tests := []struct {
		market   string
		position string
		want     bool
	}{
		{MarketPassingYards, "QB", true},
		{MarketPassingTouchdowns, "RB", false},
		{MarketRushingYards, "QB", true},
		{MarketRushingTouchdowns, "RB", true},
		{MarketReceivingYards, "TE", true},
		{MarketReceivingTouchdowns, "QB", false},
		{MarketRegularSeasonWins, "WR", false},
	}
	for _, test := range tests {
		if got := PositionAllowed(test.market, test.position); got != test.want {
			t.Errorf("PositionAllowed(%q, %q) = %v, want %v", test.market, test.position, got, test.want)
		}
	}
}
