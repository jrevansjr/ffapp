// Package morality parses the committed, user-supplied morality-index
// snapshot. It does not calculate scores or contact an external source.
package morality

import (
	"embed"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"strconv"
	"strings"
	"time"
)

const scoreFilename = "ffapp_morality_index_sleeper_adp_le_120.csv"

var (
	//go:embed metadata.json ffapp_morality_index_sleeper_adp_le_120.csv
	embeddedSnapshot embed.FS
)

type metadata struct {
	Source       string `json:"source"`
	SnapshotDate string `json:"snapshot_date"`
	Description  string `json:"description"`
}

// PlayerScore is one manually supplied score keyed by an exact Sleeper player ID.
type PlayerScore struct {
	SleeperPlayerID string
	Score           int
}

// Snapshot contains the subjective score rows and their shared provenance.
type Snapshot struct {
	Source       string
	SnapshotDate string
	Description  string
	Rows         []PlayerScore
}

// LoadSnapshot validates and returns the score snapshot compiled into the data command.
func LoadSnapshot() (Snapshot, error) {
	return ParseSnapshot(embeddedSnapshot)
}

// ParseSnapshot reads metadata and score rows from files. Accepting fs.FS keeps
// parser and rebuild tests fully local.
func ParseSnapshot(files fs.FS) (Snapshot, error) {
	var snapshot Snapshot
	body, err := fs.ReadFile(files, "metadata.json")
	if err != nil {
		return snapshot, fmt.Errorf("read morality metadata: %w", err)
	}
	var meta metadata
	if err := json.Unmarshal(body, &meta); err != nil {
		return snapshot, fmt.Errorf("decode morality metadata: %w", err)
	}
	if strings.TrimSpace(meta.Source) == "" || strings.TrimSpace(meta.Description) == "" {
		return snapshot, fmt.Errorf("morality metadata requires source and description")
	}
	if _, err := time.Parse("2006-01-02", meta.SnapshotDate); err != nil {
		return snapshot, fmt.Errorf("morality snapshot_date must use YYYY-MM-DD: %w", err)
	}

	file, err := files.Open(scoreFilename)
	if err != nil {
		return snapshot, fmt.Errorf("open morality scores: %w", err)
	}
	defer file.Close()
	rows, err := parseScores(file)
	if err != nil {
		return snapshot, err
	}
	if len(rows) == 0 {
		return snapshot, fmt.Errorf("morality snapshot contains no scores")
	}
	return Snapshot{
		Source:       strings.TrimSpace(meta.Source),
		SnapshotDate: meta.SnapshotDate,
		Description:  strings.TrimSpace(meta.Description),
		Rows:         rows,
	}, nil
}

func parseScores(reader io.Reader) ([]PlayerScore, error) {
	csvReader := csv.NewReader(reader)
	records, err := csvReader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read morality scores: %w", err)
	}
	if len(records) == 0 || len(records[0]) != 2 ||
		records[0][0] != "sleeper_player_id" || records[0][1] != "morality_index" {
		return nil, fmt.Errorf("morality scores require sleeper_player_id,morality_index columns")
	}

	rows := make([]PlayerScore, 0, len(records)-1)
	seen := make(map[string]struct{}, len(records)-1)
	for index, record := range records[1:] {
		rowNumber := index + 2
		if len(record) != 2 {
			return nil, fmt.Errorf("morality row %d has %d columns; want 2", rowNumber, len(record))
		}
		sleeperID := strings.TrimSpace(record[0])
		if sleeperID == "" {
			return nil, fmt.Errorf("morality row %d has an empty Sleeper player ID", rowNumber)
		}
		if _, duplicate := seen[sleeperID]; duplicate {
			return nil, fmt.Errorf("morality scores repeat Sleeper player ID %q", sleeperID)
		}
		seen[sleeperID] = struct{}{}
		score, err := strconv.Atoi(strings.TrimSpace(record[1]))
		if err != nil || score < 0 || score > 5 {
			return nil, fmt.Errorf("morality row %d has invalid 0-5 score %q", rowNumber, record[1])
		}
		rows = append(rows, PlayerScore{SleeperPlayerID: sleeperID, Score: score})
	}
	return rows, nil
}
