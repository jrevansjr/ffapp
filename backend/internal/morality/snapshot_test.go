package morality

import (
	"testing"
	"testing/fstest"
)

func TestLoadSnapshot(t *testing.T) {
	snapshot, err := LoadSnapshot()
	if err != nil {
		t.Fatalf("LoadSnapshot() error = %v", err)
	}
	if snapshot.Source != "user_supplied" || snapshot.SnapshotDate != "2026-08-30" || len(snapshot.Rows) != 116 {
		t.Fatalf("snapshot = source %q date %q rows %d", snapshot.Source, snapshot.SnapshotDate, len(snapshot.Rows))
	}
}

func TestParseSnapshotRejectsInvalidRows(t *testing.T) {
	tests := []struct {
		name string
		csv  string
	}{
		{name: "duplicate player", csv: "sleeper_player_id,morality_index\n1,4\n1,5\n"},
		{name: "score too high", csv: "sleeper_player_id,morality_index\n1,6\n"},
		{name: "non-integer score", csv: "sleeper_player_id,morality_index\n1,good\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			files := fstest.MapFS{
				"metadata.json": &fstest.MapFile{Data: []byte(`{"source":"user_supplied","snapshot_date":"2026-08-30","description":"test"}`)},
				scoreFilename:   &fstest.MapFile{Data: []byte(test.csv)},
			}
			if _, err := ParseSnapshot(files); err == nil {
				t.Fatal("ParseSnapshot() error = nil, want validation failure")
			}
		})
	}
}
