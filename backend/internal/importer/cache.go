package importer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jrevansjr/ffapp/backend/internal/sleeper"
)

const playerCacheMaxAge = 24 * time.Hour

type playerCacheMetadata struct {
	FetchedAt string `json:"fetched_at"`
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
