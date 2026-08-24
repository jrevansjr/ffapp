package importer

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/jrevansjr/ffapp/backend/internal/database"
	"github.com/jrevansjr/ffapp/backend/internal/nflverse"
	"github.com/jrevansjr/ffapp/backend/internal/sleeper"
)

// Runner coordinates the data CLI while keeping provider retrieval separate
// from transactional database loaders.
type Runner struct {
	DBPath             string
	CacheDir           string
	SleeperClient      *sleeper.Client
	NFLVerseClient     *nflverse.Client
	MinimumPlayerCount int
	MinimumWeeklyStats int
	MinimumSeasonStats int
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
		NFLVerseClient:     nflverse.NewClient(),
		MinimumPlayerCount: minimumRealPlayerCount,
		MinimumWeeklyStats: minimumRealWeeklyStatRows,
		MinimumSeasonStats: minimumRealSeasonStatRows,
		Now:                time.Now,
		Output:             output,
	}
}

// Load imports one implemented dataset into the current database. Refresh only
// affects stats, where it bypasses the durable provider cache.
func (runner *Runner) Load(ctx context.Context, dataset string, refresh bool) error {
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
	case "stats":
		return runner.loadStats(ctx, db, refresh)
	default:
		return fmt.Errorf("unknown dataset %q; supported datasets are teams, players, and stats", dataset)
	}
}

// Build loads every dataset implemented through the current milestone in
// foreign-key order, reusing validated caches when available.
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
	if err := runner.loadPlayers(ctx, db); err != nil {
		return err
	}
	return runner.loadStats(ctx, db, false)
}

func (runner *Runner) loadStats(ctx context.Context, db *sql.DB, refresh bool) error {
	weekly, playerIDs, fetchedAt, source, err := runner.statsSource(ctx, refresh)
	if err != nil {
		return err
	}
	if len(weekly.Rows) < runner.MinimumWeeklyStats {
		return fmt.Errorf(
			"nflverse data contains %d relevant rows; refusing to replace stats below the safety threshold of %d",
			len(weekly.Rows),
			runner.MinimumWeeklyStats,
		)
	}
	summary, err := loadStatsWithThresholds(
		ctx,
		db,
		weekly,
		playerIDs,
		runner.MinimumWeeklyStats,
		runner.MinimumSeasonStats,
	)
	if err != nil {
		return err
	}
	fmt.Fprintf(
		runner.Output,
		"stats: source=%s fetched_at=%s source_rows=%d relevant=%d aggregated=%d crosswalk_source_rows=%d crosswalk_usable=%d matched=%d unmatched=%d gsis_backfills=%d conflicts=%d ambiguous=%d weekly_inserted=%d season_inserted=%d\n",
		source,
		fetchedAt.UTC().Format(time.RFC3339),
		summary.SourceRows,
		summary.RelevantRows,
		summary.AggregatedRows,
		summary.CrosswalkSourceRows,
		summary.CrosswalkRows,
		summary.MatchedRows,
		summary.UnmatchedRows,
		summary.GSISBackfills,
		summary.IdentityConflicts,
		summary.AmbiguousMappings,
		summary.WeeklyRows,
		summary.SeasonRows,
	)
	for _, player := range summary.Unmatched {
		fmt.Fprintf(
			runner.Output,
			"stats: unmatched gsis_id=%s player=%q position=%s half_ppr=%.2f\n",
			player.GSISID,
			player.Name,
			player.Position,
			player.FantasyPoints,
		)
	}
	for _, issue := range summary.IdentityIssues {
		fmt.Fprintf(
			runner.Output,
			"stats: identity_issue sleeper_id=%s local_gsis=%s crosswalk_gsis=%s reason=%q\n",
			issue.SleeperID,
			issue.LocalGSIS,
			issue.CrosswalkGSIS,
			issue.Reason,
		)
	}
	return nil
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

func (runner *Runner) statsSource(
	ctx context.Context,
	refresh bool,
) (nflverse.WeeklyDataset, nflverse.PlayerIDDataset, time.Time, string, error) {
	if !refresh {
		weekly, playerIDs, fetchedAt, err := readStatsCache(runner.CacheDir)
		if err == nil {
			return weekly, playerIDs, fetchedAt, "cache", nil
		}
	}
	weekly, statsBody, err := runner.NFLVerseClient.FetchWeeklyStats(ctx)
	if err != nil {
		return nflverse.WeeklyDataset{}, nflverse.PlayerIDDataset{}, time.Time{}, "", err
	}
	playerIDs, crosswalkBody, err := runner.NFLVerseClient.FetchPlayerIDs(ctx)
	if err != nil {
		return nflverse.WeeklyDataset{}, nflverse.PlayerIDDataset{}, time.Time{}, "", err
	}
	if len(weekly.Rows) < runner.MinimumWeeklyStats {
		return nflverse.WeeklyDataset{}, nflverse.PlayerIDDataset{}, time.Time{}, "", fmt.Errorf(
			"nflverse data contains %d relevant rows; refusing to cache stats below the safety threshold of %d",
			len(weekly.Rows),
			runner.MinimumWeeklyStats,
		)
	}
	if _, err := aggregateWeeklyStats(weekly.Rows); err != nil {
		return nflverse.WeeklyDataset{}, nflverse.PlayerIDDataset{}, time.Time{}, "", fmt.Errorf("validate downloaded nflverse stats: %w", err)
	}
	fetchedAt := runner.Now().UTC()
	if err := writeStatsCache(runner.CacheDir, statsBody, crosswalkBody, fetchedAt); err != nil {
		return nflverse.WeeklyDataset{}, nflverse.PlayerIDDataset{}, time.Time{}, "", err
	}
	return weekly, playerIDs, fetchedAt, "providers", nil
}
