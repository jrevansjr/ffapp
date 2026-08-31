package importer

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/jrevansjr/ffapp/backend/internal/database"
	"github.com/jrevansjr/ffapp/backend/internal/fantasypros"
	"github.com/jrevansjr/ffapp/backend/internal/nflverse"
	"github.com/jrevansjr/ffapp/backend/internal/odds"
	"github.com/jrevansjr/ffapp/backend/internal/sleeper"
)

// Runner coordinates the data CLI while keeping provider retrieval separate
// from transactional database loaders.
type Runner struct {
	DBPath                 string
	CacheDir               string
	SleeperClient          *sleeper.Client
	NFLVerseClient         *nflverse.Client
	FantasyProsClient      *fantasypros.Client
	MinimumPlayerCount     int
	MinimumWeeklyStats     int
	MinimumSeasonStats     int
	MinimumFantasyProsRows int
	MinimumProjectionRows  int
	MinimumOddsRows        int
	OddsSnapshot           *odds.Snapshot
	Now                    func() time.Time
	Output                 io.Writer
}

// NewRunner builds a data runner using DB_PATH's directory for ignored caches
// and backups.
func NewRunner(dbPath string, output io.Writer) *Runner {
	if dbPath == "" {
		dbPath = database.DefaultPath
	}
	return &Runner{
		DBPath:                 dbPath,
		CacheDir:               filepath.Join(filepath.Dir(dbPath), "import-cache"),
		SleeperClient:          sleeper.NewClient(),
		NFLVerseClient:         nflverse.NewClient(),
		FantasyProsClient:      fantasypros.NewClient(os.Getenv("FANTASYPROS_API_KEY")),
		MinimumPlayerCount:     minimumRealPlayerCount,
		MinimumWeeklyStats:     minimumRealWeeklyStatRows,
		MinimumSeasonStats:     minimumRealSeasonStatRows,
		MinimumFantasyProsRows: minimumRealFantasyProsRows,
		MinimumProjectionRows:  minimumRealProjectionRows,
		MinimumOddsRows:        minimumRealOddsRows,
		Now:                    time.Now,
		Output:                 output,
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
	case "fantasypros":
		if refresh {
			return fmt.Errorf("FantasyPros refreshes require a separately approved command for each dataset")
		}
		return runner.loadFantasyPros(ctx, db)
	case "projections":
		return runner.loadProjections(ctx, db)
	case "odds":
		return runner.loadOdds(ctx, db)
	default:
		return fmt.Errorf("unknown dataset %q; supported datasets are teams, players, stats, fantasypros, projections, and odds", dataset)
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
	if err := runner.loadStats(ctx, db, false); err != nil {
		return err
	}
	if err := runner.loadFantasyPros(ctx, db); err != nil {
		return err
	}
	if err := runner.loadProjections(ctx, db); err != nil {
		return err
	}
	return runner.loadOdds(ctx, db)
}

func (runner *Runner) loadOdds(ctx context.Context, db *sql.DB) error {
	var snapshot odds.Snapshot
	if runner.OddsSnapshot != nil {
		snapshot = *runner.OddsSnapshot
	} else {
		var err error
		snapshot, err = odds.LoadSnapshot()
		if err != nil {
			return err
		}
	}
	summary, err := loadOddsWithThreshold(ctx, db, snapshot, runner.MinimumOddsRows)
	if err != nil {
		return err
	}
	fmt.Fprintf(
		runner.Output,
		"odds: source=%s source_rows=%d consensus=%d no_consensus=%d matched=%d unmatched=%d position_mismatches=%d inserted=%d captured_at=%s\n",
		snapshot.Source,
		summary.SourceRows,
		summary.ConsensusRows,
		summary.SkippedNoConsensus,
		summary.MatchedRows,
		summary.UnmatchedRows,
		summary.PositionMismatchRows,
		summary.InsertedRows,
		summary.CapturedAt,
	)
	for _, issue := range summary.Issues {
		fmt.Fprintf(
			runner.Output,
			"odds: skipped reason=%s market=%s subject=%q team=%s position=%s\n",
			issue.Reason,
			issue.Market,
			issue.Subject,
			issue.Team,
			issue.Position,
		)
	}
	return nil
}

// RefreshFantasyPros performs exactly one authenticated provider request after
// the CLI caller has obtained approval, then atomically caches that response.
// It deliberately does not alter the database.
func (runner *Runner) RefreshFantasyPros(ctx context.Context, name fantasypros.DatasetName) error {
	var body []byte
	var rowCount int
	var err error
	switch name {
	case fantasypros.DatasetADP:
		var dataset fantasypros.ADPDataset
		dataset, body, err = runner.FantasyProsClient.FetchADP(ctx)
		rowCount = len(dataset.Rankings)
	case fantasypros.DatasetECR:
		var dataset fantasypros.ECRDataset
		dataset, body, err = runner.FantasyProsClient.FetchECR(ctx)
		rowCount = len(dataset.Rankings)
	case fantasypros.DatasetProjections:
		var dataset fantasypros.ProjectionDataset
		dataset, body, err = runner.FantasyProsClient.FetchProjections(ctx)
		rowCount = len(dataset.Projections)
	default:
		return fmt.Errorf("unknown FantasyPros dataset %q; supported datasets are adp, ecr, and projections", name)
	}
	if err != nil {
		return err
	}
	minimumRows := runner.MinimumFantasyProsRows
	rowLabel := "rankings"
	if name == fantasypros.DatasetProjections {
		minimumRows = runner.MinimumProjectionRows
		rowLabel = "projections"
	}
	if rowCount < minimumRows {
		return fmt.Errorf(
			"FantasyPros %s response contains %d %s; refusing to cache below the safety threshold of %d",
			name,
			rowCount,
			rowLabel,
			minimumRows,
		)
	}
	fetchedAt := runner.Now().UTC()
	if err := writeFantasyProsCache(runner.CacheDir, string(name), body, fetchedAt); err != nil {
		return err
	}
	fmt.Fprintf(
		runner.Output,
		"fantasypros: refreshed dataset=%s fetched_at=%s rows=%d\n",
		name,
		fetchedAt.Format(time.RFC3339),
		rowCount,
	)
	return nil
}

func (runner *Runner) loadProjections(ctx context.Context, db *sql.DB) error {
	body, fetchedAt, err := readFantasyProsCache(
		runner.CacheDir,
		string(fantasypros.DatasetProjections),
	)
	if err != nil {
		return fmt.Errorf(
			"read cached FantasyPros projections: %w; run the separately approved projections refresh first",
			err,
		)
	}
	dataset, err := fantasypros.ParseProjections(body, fetchedAt)
	if err != nil {
		return fmt.Errorf("parse cached FantasyPros projections: %w", err)
	}
	if len(dataset.Projections) < runner.MinimumProjectionRows {
		return fmt.Errorf(
			"FantasyPros response contains %d projections; refusing to load below the safety threshold of %d",
			len(dataset.Projections),
			runner.MinimumProjectionRows,
		)
	}
	summary, err := loadProjectionsWithThreshold(
		ctx,
		db,
		dataset,
		runner.MinimumProjectionRows,
	)
	if err != nil {
		return err
	}
	fmt.Fprintf(
		runner.Output,
		"projections: source=fantasypros source_rows=%d matched=%d unmatched=%d inserted=%d updated_at=%s\n",
		summary.SourceRows,
		summary.MatchedRows,
		summary.UnmatchedRows,
		summary.InsertedRows,
		summary.UpdatedAt,
	)
	for _, player := range summary.Unmatched {
		fmt.Fprintf(
			runner.Output,
			"projections: unmatched fantasypros_id=%s player=%q position=%s team=%s\n",
			player.FantasyProsID,
			player.Name,
			player.Position,
			player.Team,
		)
	}
	return nil
}

func (runner *Runner) loadFantasyPros(ctx context.Context, db *sql.DB) error {
	adpBody, adpFetchedAt, err := readFantasyProsCache(runner.CacheDir, string(fantasypros.DatasetADP))
	if err != nil {
		return fmt.Errorf("read cached FantasyPros ADP: %w; run the separately approved ADP refresh first", err)
	}
	adp, err := fantasypros.ParseADP(adpBody, adpFetchedAt)
	if err != nil {
		return fmt.Errorf("parse cached FantasyPros ADP: %w", err)
	}
	ecrBody, ecrFetchedAt, err := readFantasyProsCache(runner.CacheDir, string(fantasypros.DatasetECR))
	if err != nil {
		return fmt.Errorf("read cached FantasyPros ECR: %w; run the separately approved ECR refresh first", err)
	}
	ecr, err := fantasypros.ParseECR(ecrBody, ecrFetchedAt)
	if err != nil {
		return fmt.Errorf("parse cached FantasyPros ECR: %w", err)
	}
	playerIDs, err := readCachedPlayerIDs(runner.CacheDir)
	if err != nil {
		return fmt.Errorf("read cached player-ID crosswalk for FantasyPros: %w; load stats first", err)
	}
	summary, err := loadFantasyProsWithThreshold(
		ctx, db, adp, ecr, playerIDs, runner.MinimumFantasyProsRows,
	)
	if err != nil {
		return err
	}
	fmt.Fprintf(
		runner.Output,
		"fantasypros: id_backfills=%d conflicts=%d ambiguous=%d\n",
		summary.FantasyProsBackfills,
		summary.IdentityConflicts,
		summary.AmbiguousMappings,
	)
	for _, dataset := range []FantasyProsDatasetSummary{summary.ADP, summary.ECR} {
		fmt.Fprintf(
			runner.Output,
			"fantasypros: dataset=%s source_rows=%d matched=%d unmatched=%d inserted=%d updated_at=%s\n",
			dataset.Dataset,
			dataset.SourceRows,
			dataset.MatchedRows,
			dataset.UnmatchedRows,
			dataset.InsertedRows,
			dataset.UpdatedAt,
		)
		for _, player := range dataset.Unmatched {
			fmt.Fprintf(
				runner.Output,
				"fantasypros: unmatched dataset=%s fantasypros_id=%s player=%q position=%s value=%.2f\n",
				dataset.Dataset,
				player.FantasyProsID,
				player.Name,
				player.Position,
				player.Value,
			)
		}
	}
	for _, issue := range summary.IdentityIssues {
		fmt.Fprintf(
			runner.Output,
			"fantasypros: identity_issue sleeper_id=%s local_fantasypros_id=%s crosswalk_fantasypros_id=%s reason=%q\n",
			issue.SleeperID,
			issue.LocalFantasyProsID,
			issue.CrosswalkFantasyProsID,
			issue.Reason,
		)
	}
	return nil
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
