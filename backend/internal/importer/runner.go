package importer

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/jrevansjr/ffapp/backend/internal/database"
	"github.com/jrevansjr/ffapp/backend/internal/sleeper"
)

// Runner coordinates the data CLI while keeping provider retrieval separate
// from transactional database loaders.
type Runner struct {
	DBPath             string
	CacheDir           string
	SleeperClient      *sleeper.Client
	MinimumPlayerCount int
	Now                func() time.Time
	Output             io.Writer
}

// NewRunner builds a data runner using DB_PATH's directory for ignored caches
// and backups.
func NewRunner(dbPath string, output io.Writer) *Runner {
	if dbPath == "" {
		dbPath = database.DefaultPath
	}
	return &Runner{
		DBPath:             dbPath,
		CacheDir:           filepath.Join(filepath.Dir(dbPath), "import-cache"),
		SleeperClient:      sleeper.NewClient(),
		MinimumPlayerCount: minimumRealPlayerCount,
		Now:                time.Now,
		Output:             output,
	}
}

// Load imports one M6.1 dataset into the current database.
func (runner *Runner) Load(ctx context.Context, dataset string) error {
	db, err := database.Open(runner.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	switch dataset {
	case "teams":
		return runner.loadTeams(ctx, db)
	case "players":
		if err := runner.loadTeams(ctx, db); err != nil {
			return err
		}
		return runner.loadPlayers(ctx, db)
	default:
		return fmt.Errorf("unknown dataset %q; M6.1 supports teams and players", dataset)
	}
}

// Build loads every dataset implemented through the current milestone in
// foreign-key order. M6.1 deliberately registers only teams and players.
func (runner *Runner) Build(ctx context.Context) error {
	return runner.buildAt(ctx, runner.DBPath)
}

func (runner *Runner) buildAt(ctx context.Context, dbPath string) error {
	db, err := database.Open(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := runner.loadTeams(ctx, db); err != nil {
		return err
	}
	return runner.loadPlayers(ctx, db)
}

func (runner *Runner) loadTeams(ctx context.Context, db *sql.DB) error {
	count, err := LoadTeams(ctx, db)
	if err != nil {
		return err
	}
	fmt.Fprintf(runner.Output, "teams: loaded %d canonical NFL teams\n", count)
	return nil
}

func (runner *Runner) loadPlayers(ctx context.Context, db *sql.DB) error {
	players, fetchedAt, source, err := runner.playerSource(ctx)
	if err != nil {
		return err
	}
	validationSummary := PlayerSummary{}
	eligiblePlayers(players, &validationSummary)
	if validationSummary.Eligible < runner.MinimumPlayerCount {
		return fmt.Errorf(
			"Sleeper response contains %d eligible players; refusing to replace data below the safety threshold of %d",
			validationSummary.Eligible,
			runner.MinimumPlayerCount,
		)
	}
	summary, err := LoadPlayers(ctx, db, players, fetchedAt)
	if err != nil {
		return err
	}
	fmt.Fprintf(
		runner.Output,
		"players: source=%s fetched=%d eligible=%d inserted=%d updated=%d deactivated=%d skipped=%d unmapped_teams=%d\n",
		source,
		summary.Fetched,
		summary.Eligible,
		summary.Inserted,
		summary.Updated,
		summary.Deactivated,
		summary.Skipped,
		summary.UnmappedTeams,
	)
	return nil
}

func (runner *Runner) playerSource(
	ctx context.Context,
) (sleeper.PlayersResponse, time.Time, string, error) {
	now := runner.Now().UTC()
	cachedPlayers, cachedAt, _, cacheErr := readPlayerCache(runner.CacheDir)
	if cacheErr == nil {
		age := now.Sub(cachedAt)
		if age >= 0 && age < playerCacheMaxAge {
			return cachedPlayers, cachedAt, "cache", nil
		}
	}

	players, body, err := runner.SleeperClient.FetchPlayers(ctx)
	if err != nil {
		if cacheErr == nil {
			fmt.Fprintf(
				runner.Output,
				"players: warning: refresh failed (%v); using stale cache from %s\n",
				err,
				cachedAt.UTC().Format(time.RFC3339),
			)
			return cachedPlayers, cachedAt, "stale-cache", nil
		}
		return nil, time.Time{}, "", err
	}
	if err := writePlayerCache(runner.CacheDir, body, now); err != nil {
		return nil, time.Time{}, "", err
	}
	return players, now, "sleeper", nil
}
