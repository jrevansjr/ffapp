package importer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jrevansjr/ffapp/backend/internal/nflverse"
	"github.com/jrevansjr/ffapp/backend/internal/sleeper"
)

const playerCacheMaxAge = 24 * time.Hour

type playerCacheMetadata struct {
	FetchedAt string `json:"fetched_at"`
}

type statsCacheMetadata struct {
	FetchedAt     string `json:"fetched_at"`
	StatsFile     string `json:"stats_file"`
	CrosswalkFile string `json:"crosswalk_file"`
}

type fantasyProsCacheMetadata struct {
	Dataset   string `json:"dataset"`
	FetchedAt string `json:"fetched_at"`
	DataFile  string `json:"data_file"`
}

func readPlayerCache(cacheDir string) (sleeper.PlayersResponse, time.Time, []byte, error) {
	body, err := os.ReadFile(filepath.Join(cacheDir, "sleeper-players.json"))
	if err != nil {
		return nil, time.Time{}, nil, err
	}
	metadataBody, err := os.ReadFile(filepath.Join(cacheDir, "sleeper-players.metadata.json"))
	if err != nil {
		return nil, time.Time{}, nil, err
	}
	var metadata playerCacheMetadata
	if err := json.Unmarshal(metadataBody, &metadata); err != nil {
		return nil, time.Time{}, nil, fmt.Errorf("decode player cache metadata: %w", err)
	}
	fetchedAt, err := time.Parse(time.RFC3339, metadata.FetchedAt)
	if err != nil {
		return nil, time.Time{}, nil, fmt.Errorf("parse player cache timestamp: %w", err)
	}
	players, err := sleeper.ParsePlayers(body)
	if err != nil {
		return nil, time.Time{}, nil, fmt.Errorf("parse cached Sleeper players: %w", err)
	}
	return players, fetchedAt, body, nil
}

func writePlayerCache(cacheDir string, body []byte, fetchedAt time.Time) error {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return fmt.Errorf("create player cache directory: %w", err)
	}
	metadata, err := json.MarshalIndent(playerCacheMetadata{
		FetchedAt: fetchedAt.UTC().Format(time.RFC3339),
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode player cache metadata: %w", err)
	}
	metadata = append(metadata, '\n')
	if err := writeFileAtomically(filepath.Join(cacheDir, "sleeper-players.json"), body); err != nil {
		return err
	}
	if err := writeFileAtomically(
		filepath.Join(cacheDir, "sleeper-players.metadata.json"), metadata,
	); err != nil {
		return err
	}
	return nil
}

// readStatsCache follows an atomic metadata pointer to a pair of previously
// validated provider files. Historical data has no automatic expiry.
func readStatsCache(
	cacheDir string,
) (nflverse.WeeklyDataset, nflverse.PlayerIDDataset, time.Time, error) {
	metadataBody, err := os.ReadFile(filepath.Join(cacheDir, "stats-2025.metadata.json"))
	if err != nil {
		return nflverse.WeeklyDataset{}, nflverse.PlayerIDDataset{}, time.Time{}, err
	}
	var metadata statsCacheMetadata
	if err := json.Unmarshal(metadataBody, &metadata); err != nil {
		return nflverse.WeeklyDataset{}, nflverse.PlayerIDDataset{}, time.Time{}, fmt.Errorf("decode stats cache metadata: %w", err)
	}
	fetchedAt, err := time.Parse(time.RFC3339, metadata.FetchedAt)
	if err != nil {
		return nflverse.WeeklyDataset{}, nflverse.PlayerIDDataset{}, time.Time{}, fmt.Errorf("parse stats cache timestamp: %w", err)
	}
	if filepath.Base(metadata.StatsFile) != metadata.StatsFile ||
		filepath.Base(metadata.CrosswalkFile) != metadata.CrosswalkFile {
		return nflverse.WeeklyDataset{}, nflverse.PlayerIDDataset{}, time.Time{}, fmt.Errorf("stats cache metadata contains an invalid filename")
	}
	statsBody, err := os.ReadFile(filepath.Join(cacheDir, metadata.StatsFile))
	if err != nil {
		return nflverse.WeeklyDataset{}, nflverse.PlayerIDDataset{}, time.Time{}, err
	}
	crosswalkBody, err := os.ReadFile(filepath.Join(cacheDir, metadata.CrosswalkFile))
	if err != nil {
		return nflverse.WeeklyDataset{}, nflverse.PlayerIDDataset{}, time.Time{}, err
	}
	weekly, err := nflverse.ParseWeeklyStats(statsBody)
	if err != nil {
		return nflverse.WeeklyDataset{}, nflverse.PlayerIDDataset{}, time.Time{}, fmt.Errorf("parse cached nflverse stats: %w", err)
	}
	playerIDs, err := nflverse.ParsePlayerIDs(crosswalkBody)
	if err != nil {
		return nflverse.WeeklyDataset{}, nflverse.PlayerIDDataset{}, time.Time{}, fmt.Errorf("parse cached player IDs: %w", err)
	}
	return weekly, playerIDs, fetchedAt, nil
}

// readCachedPlayerIDs reads only the identity half of the stats cache so a
// FantasyPros load does not need to parse the much larger weekly-stat file.
func readCachedPlayerIDs(cacheDir string) (nflverse.PlayerIDDataset, error) {
	metadataBody, err := os.ReadFile(filepath.Join(cacheDir, "stats-2025.metadata.json"))
	if err != nil {
		return nflverse.PlayerIDDataset{}, err
	}
	var metadata statsCacheMetadata
	if err := json.Unmarshal(metadataBody, &metadata); err != nil {
		return nflverse.PlayerIDDataset{}, fmt.Errorf("decode stats cache metadata: %w", err)
	}
	if filepath.Base(metadata.CrosswalkFile) != metadata.CrosswalkFile {
		return nflverse.PlayerIDDataset{}, fmt.Errorf("stats cache metadata contains an invalid crosswalk filename")
	}
	body, err := os.ReadFile(filepath.Join(cacheDir, metadata.CrosswalkFile))
	if err != nil {
		return nflverse.PlayerIDDataset{}, err
	}
	playerIDs, err := nflverse.ParsePlayerIDs(body)
	if err != nil {
		return nflverse.PlayerIDDataset{}, fmt.Errorf("parse cached player IDs: %w", err)
	}
	return playerIDs, nil
}

// writeStatsCache writes immutable, generation-named source files before
// atomically moving the metadata pointer. A failed refresh therefore leaves the
// previous complete cache generation readable.
func writeStatsCache(
	cacheDir string,
	statsBody []byte,
	crosswalkBody []byte,
	fetchedAt time.Time,
) error {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return fmt.Errorf("create stats cache directory: %w", err)
	}
	generation := fetchedAt.UTC().Format("20060102T150405.000000000Z")
	statsFile := "nflverse-player-stats-week-2025-" + generation + ".csv"
	crosswalkFile := "dynastyprocess-player-ids-" + generation + ".csv"
	if err := writeFileAtomically(filepath.Join(cacheDir, statsFile), statsBody); err != nil {
		return err
	}
	if err := writeFileAtomically(filepath.Join(cacheDir, crosswalkFile), crosswalkBody); err != nil {
		return err
	}
	metadata, err := json.MarshalIndent(statsCacheMetadata{
		FetchedAt:     fetchedAt.UTC().Format(time.RFC3339),
		StatsFile:     statsFile,
		CrosswalkFile: crosswalkFile,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode stats cache metadata: %w", err)
	}
	metadata = append(metadata, '\n')
	if err := writeFileAtomically(filepath.Join(cacheDir, "stats-2025.metadata.json"), metadata); err != nil {
		return err
	}
	return nil
}

func readFantasyProsCache(cacheDir string, dataset string) ([]byte, time.Time, error) {
	metadataPath := filepath.Join(cacheDir, "fantasypros-2026-"+dataset+".metadata.json")
	metadataBody, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, time.Time{}, err
	}
	var metadata fantasyProsCacheMetadata
	if err := json.Unmarshal(metadataBody, &metadata); err != nil {
		return nil, time.Time{}, fmt.Errorf("decode FantasyPros %s cache metadata: %w", dataset, err)
	}
	if metadata.Dataset != dataset || filepath.Base(metadata.DataFile) != metadata.DataFile {
		return nil, time.Time{}, fmt.Errorf("FantasyPros %s cache metadata is inconsistent", dataset)
	}
	fetchedAt, err := time.Parse(time.RFC3339, metadata.FetchedAt)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("parse FantasyPros %s cache timestamp: %w", dataset, err)
	}
	body, err := os.ReadFile(filepath.Join(cacheDir, metadata.DataFile))
	if err != nil {
		return nil, time.Time{}, err
	}
	return body, fetchedAt, nil
}

func writeFantasyProsCache(cacheDir, dataset string, body []byte, fetchedAt time.Time) error {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return fmt.Errorf("create FantasyPros cache directory: %w", err)
	}
	generation := fetchedAt.UTC().Format("20060102T150405.000000000Z")
	dataFile := "fantasypros-2026-" + dataset + "-" + generation + ".json"
	if err := writeFileAtomically(filepath.Join(cacheDir, dataFile), body); err != nil {
		return err
	}
	metadata, err := json.MarshalIndent(fantasyProsCacheMetadata{
		Dataset: dataset, FetchedAt: fetchedAt.UTC().Format(time.RFC3339), DataFile: dataFile,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode FantasyPros %s cache metadata: %w", dataset, err)
	}
	metadata = append(metadata, '\n')
	return writeFileAtomically(
		filepath.Join(cacheDir, "fantasypros-2026-"+dataset+".metadata.json"),
		metadata,
	)
}

func writeFileAtomically(path string, data []byte) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".import-cache-*")
	if err != nil {
		return fmt.Errorf("create temporary cache file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary cache file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary cache file: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace cache file: %w", err)
	}
	return nil
}
