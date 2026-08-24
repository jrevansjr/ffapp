package importer

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/jrevansjr/ffapp/backend/internal/nflverse"
)

const (
	minimumRealWeeklyStatRows = 1000
	minimumRealSeasonStatRows = 200
)

// StatsSummary describes source coverage, identity matching, and rows written by
// one stats replacement. Unmatched contains the highest-scoring misses first.
type StatsSummary struct {
	SourceRows          int
	RelevantRows        int
	AggregatedRows      int
	CrosswalkSourceRows int
	CrosswalkRows       int
	GSISBackfills       int
	IdentityConflicts   int
	AmbiguousMappings   int
	MatchedRows         int
	UnmatchedRows       int
	WeeklyRows          int
	SeasonRows          int
	Unmatched           []UnmatchedPlayer
	IdentityIssues      []IdentityIssue
}

// UnmatchedPlayer is a diagnostic for a GSIS player whose weekly rows could not
// be connected to a local player. Points are summed only to rank useful misses.
type UnmatchedPlayer struct {
	GSISID        string
	Name          string
	Position      string
	FantasyPoints float64
}

// IdentityIssue explains a crosswalk mapping that was deliberately not applied.
// LocalGSIS remains authoritative when it conflicts with CrosswalkGSIS.
type IdentityIssue struct {
	SleeperID     string
	LocalGSIS     string
	CrosswalkGSIS string
	Reason        string
}

type weeklyKey struct {
	GSISID string
	Week   int
}

type mappedWeeklyStat struct {
	PlayerID int64
	Stat     nflverse.WeeklyStat
}

type localPlayer struct {
	ID        int64
	SleeperID string
	GSISID    string
}

// FantasyPointsHalfPPR applies the app's fixed common half-PPR formula to raw
// weekly counting stats. It intentionally excludes bonuses and special teams.
func FantasyPointsHalfPPR(stat nflverse.WeeklyStat) float64 {
	points := 0.04*float64(stat.PassingYards) +
		4*float64(stat.PassingTouchdowns) -
		2*float64(stat.PassingInterceptions) +
		0.1*float64(stat.RushingYards+stat.ReceivingYards) +
		6*float64(stat.RushingTouchdowns+stat.ReceivingTouchdowns) +
		0.5*float64(stat.Receptions) -
		2*float64(stat.FumblesLost) +
		2*float64(
			stat.PassingTwoPointConversions+
				stat.RushingTwoPointConversions+
				stat.ReceivingTwoPointConversions,
		)
	return math.Round(points*100) / 100
}

// LoadStats validates a complete 2025 regular season, reconciles GSIS identity,
// and replaces only 2025 weekly and season stats in one transaction. Existing
// stats survive any validation or write failure.
func LoadStats(
	ctx context.Context,
	db *sql.DB,
	weekly nflverse.WeeklyDataset,
	playerIDs nflverse.PlayerIDDataset,
) (StatsSummary, error) {
	return loadStatsWithThresholds(ctx, db, weekly, playerIDs, 1, 1)
}

func loadStatsWithThresholds(
	ctx context.Context,
	db *sql.DB,
	weekly nflverse.WeeklyDataset,
	playerIDs nflverse.PlayerIDDataset,
	minimumWeeklyRows int,
	minimumPlayers int,
) (StatsSummary, error) {
	summary := StatsSummary{
		SourceRows:          weekly.SourceRows,
		RelevantRows:        len(weekly.Rows),
		CrosswalkSourceRows: playerIDs.SourceRows,
		CrosswalkRows:       len(playerIDs.Rows),
	}
	aggregated, err := aggregateWeeklyStats(weekly.Rows)
	if err != nil {
		return summary, err
	}
	summary.AggregatedRows = len(aggregated)

	players, err := loadLocalPlayers(ctx, db)
	if err != nil {
		return summary, err
	}
	matched, backfills, identitySummary := matchWeeklyStats(players, playerIDs.Rows, aggregated)
	summary.GSISBackfills = len(backfills)
	summary.IdentityConflicts = identitySummary.conflicts
	summary.AmbiguousMappings = identitySummary.ambiguous
	summary.IdentityIssues = identitySummary.issues
	summary.MatchedRows = len(matched)
	summary.UnmatchedRows = len(aggregated) - len(matched)
	summary.Unmatched = highestValueUnmatched(aggregated, identitySummary.matchedGSIS)
	if len(matched) < minimumWeeklyRows {
		return summary, fmt.Errorf(
			"only %d nflverse rows matched local players; refusing to replace stats below the safety threshold of %d",
			len(matched),
			minimumWeeklyRows,
		)
	}
	matchedPlayers := make(map[int64]struct{})
	for _, stat := range matched {
		matchedPlayers[stat.PlayerID] = struct{}{}
	}
	if len(matchedPlayers) < minimumPlayers {
		return summary, fmt.Errorf(
			"only %d players matched nflverse rows; refusing to replace stats below the safety threshold of %d",
			len(matchedPlayers),
			minimumPlayers,
		)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return summary, fmt.Errorf("begin stats import: %w", err)
	}
	defer tx.Rollback()
	for playerID, gsisID := range backfills {
		if _, err := tx.ExecContext(ctx, `UPDATE players SET gsis_id = ? WHERE id = ?`, gsisID, playerID); err != nil {
			return summary, fmt.Errorf("backfill GSIS ID %s: %w", gsisID, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM player_season_stats WHERE season = ?`, nflverse.Season); err != nil {
		return summary, fmt.Errorf("clear %d season stats: %w", nflverse.Season, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM player_week_stats WHERE season = ?`, nflverse.Season); err != nil {
		return summary, fmt.Errorf("clear %d weekly stats: %w", nflverse.Season, err)
	}
	statement, err := tx.PrepareContext(ctx, `
		INSERT INTO player_week_stats (
			player_id, season, week, fantasy_points_half_ppr, targets,
			receptions, rushing_attempts, receiving_yards, rushing_yards,
			receiving_touchdowns, rushing_touchdowns, passing_yards
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return summary, fmt.Errorf("prepare weekly stats insert: %w", err)
	}
	defer statement.Close()
	for _, mapped := range matched {
		stat := mapped.Stat
		if _, err := statement.ExecContext(
			ctx,
			mapped.PlayerID,
			stat.Season,
			stat.Week,
			FantasyPointsHalfPPR(stat),
			stat.Targets,
			stat.Receptions,
			stat.RushingAttempts,
			stat.ReceivingYards,
			stat.RushingYards,
			stat.ReceivingTouchdowns,
			stat.RushingTouchdowns,
			stat.PassingYards,
		); err != nil {
			return summary, fmt.Errorf("insert weekly stats for GSIS ID %s week %d: %w", stat.GSISID, stat.Week, err)
		}
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO player_season_stats (
			player_id, season, games_played, fantasy_points_half_ppr, targets,
			receptions, rushing_attempts, receiving_yards, rushing_yards,
			receiving_touchdowns, rushing_touchdowns, passing_yards
		)
		SELECT
			player_id, season, COUNT(*), SUM(fantasy_points_half_ppr), SUM(targets),
			SUM(receptions), SUM(rushing_attempts), SUM(receiving_yards), SUM(rushing_yards),
			SUM(receiving_touchdowns), SUM(rushing_touchdowns), SUM(passing_yards)
		FROM player_week_stats
		WHERE season = ?
		GROUP BY player_id, season
	`, nflverse.Season)
	if err != nil {
		return summary, fmt.Errorf("derive season stats from weekly rows: %w", err)
	}
	seasonRows, err := result.RowsAffected()
	if err != nil {
		return summary, fmt.Errorf("count inserted season stats: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return summary, fmt.Errorf("commit stats import: %w", err)
	}
	summary.WeeklyRows = len(matched)
	summary.SeasonRows = int(seasonRows)
	return summary, nil
}

func aggregateWeeklyStats(rows []nflverse.WeeklyStat) ([]nflverse.WeeklyStat, error) {
	if len(rows) == 0 {
		return nil, fmt.Errorf("weekly stats contain no relevant rows")
	}
	weeks := make(map[int]struct{}, 18)
	aggregated := make(map[weeklyKey]nflverse.WeeklyStat, len(rows))
	for _, stat := range rows {
		if stat.Season != nflverse.Season || stat.Week < 1 || stat.Week > 18 {
			return nil, fmt.Errorf("weekly stats contain season %d week %d; want %d weeks 1-18", stat.Season, stat.Week, nflverse.Season)
		}
		if strings.TrimSpace(stat.GSISID) == "" {
			return nil, fmt.Errorf("weekly stats contain an empty GSIS ID")
		}
		weeks[stat.Week] = struct{}{}
		key := weeklyKey{GSISID: stat.GSISID, Week: stat.Week}
		current, found := aggregated[key]
		if !found {
			aggregated[key] = stat
			continue
		}
		addWeeklyStat(&current, stat)
		aggregated[key] = current
	}
	if len(weeks) != 18 {
		return nil, fmt.Errorf("weekly stats cover %d regular-season weeks; want all 18", len(weeks))
	}
	result := make([]nflverse.WeeklyStat, 0, len(aggregated))
	for _, stat := range aggregated {
		result = append(result, stat)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].GSISID == result[j].GSISID {
			return result[i].Week < result[j].Week
		}
		return result[i].GSISID < result[j].GSISID
	})
	return result, nil
}

func addWeeklyStat(total *nflverse.WeeklyStat, next nflverse.WeeklyStat) {
	total.PassingYards += next.PassingYards
	total.PassingTouchdowns += next.PassingTouchdowns
	total.PassingInterceptions += next.PassingInterceptions
	total.PassingTwoPointConversions += next.PassingTwoPointConversions
	total.RushingAttempts += next.RushingAttempts
	total.RushingYards += next.RushingYards
	total.RushingTouchdowns += next.RushingTouchdowns
	total.RushingTwoPointConversions += next.RushingTwoPointConversions
	total.Receptions += next.Receptions
	total.Targets += next.Targets
	total.ReceivingYards += next.ReceivingYards
	total.ReceivingTouchdowns += next.ReceivingTouchdowns
	total.ReceivingTwoPointConversions += next.ReceivingTwoPointConversions
	total.FumblesLost += next.FumblesLost
}

func loadLocalPlayers(ctx context.Context, db *sql.DB) ([]localPlayer, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, sleeper_player_id, COALESCE(gsis_id, '')
		FROM players
		WHERE sleeper_player_id IS NOT NULL AND position IN ('QB', 'RB', 'WR', 'TE')
		ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("load local players for stats import: %w", err)
	}
	defer rows.Close()
	var players []localPlayer
	for rows.Next() {
		var player localPlayer
		if err := rows.Scan(&player.ID, &player.SleeperID, &player.GSISID); err != nil {
			return nil, fmt.Errorf("scan local player for stats import: %w", err)
		}
		players = append(players, player)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read local players for stats import: %w", err)
	}
	if len(players) == 0 {
		return nil, fmt.Errorf("no local fantasy players exist; load players before stats")
	}
	return players, nil
}

type identityMatchSummary struct {
	conflicts   int
	ambiguous   int
	matchedGSIS map[string]struct{}
	issues      []IdentityIssue
}

func matchWeeklyStats(
	players []localPlayer,
	crosswalk []nflverse.PlayerID,
	weekly []nflverse.WeeklyStat,
) ([]mappedWeeklyStat, map[int64]string, identityMatchSummary) {
	crossBySleeper := make(map[string]string)
	ambiguousSleeper := make(map[string]struct{})
	gsisSleepers := make(map[string]map[string]struct{})
	for _, identity := range crosswalk {
		sleeperID := strings.TrimSpace(identity.SleeperID)
		gsisID := strings.TrimSpace(identity.GSISID)
		if sleeperID == "" || gsisID == "" {
			continue
		}
		if prior, found := crossBySleeper[sleeperID]; found && prior != gsisID {
			ambiguousSleeper[sleeperID] = struct{}{}
		} else {
			crossBySleeper[sleeperID] = gsisID
		}
		if gsisSleepers[gsisID] == nil {
			gsisSleepers[gsisID] = make(map[string]struct{})
		}
		gsisSleepers[gsisID][sleeperID] = struct{}{}
	}

	localByGSIS := make(map[string]int64)
	for _, player := range players {
		if player.GSISID != "" {
			localByGSIS[player.GSISID] = player.ID
		}
	}
	backfills := make(map[int64]string)
	summary := identityMatchSummary{matchedGSIS: make(map[string]struct{})}
	for _, player := range players {
		crossGSIS := crossBySleeper[player.SleeperID]
		if player.GSISID != "" {
			if _, ambiguous := ambiguousSleeper[player.SleeperID]; ambiguous {
				summary.ambiguous++
				summary.issues = append(summary.issues, IdentityIssue{
					SleeperID: player.SleeperID, LocalGSIS: player.GSISID,
					CrosswalkGSIS: crossGSIS, Reason: "ambiguous crosswalk rows",
				})
			} else if crossGSIS != "" && crossGSIS != player.GSISID {
				summary.conflicts++
				summary.issues = append(summary.issues, IdentityIssue{
					SleeperID: player.SleeperID, LocalGSIS: player.GSISID,
					CrosswalkGSIS: crossGSIS, Reason: "local GSIS differs from crosswalk",
				})
			}
			continue
		}
		if crossGSIS == "" {
			continue
		}
		if _, ambiguous := ambiguousSleeper[player.SleeperID]; ambiguous || len(gsisSleepers[crossGSIS]) != 1 {
			summary.ambiguous++
			summary.issues = append(summary.issues, IdentityIssue{
				SleeperID: player.SleeperID, CrosswalkGSIS: crossGSIS,
				Reason: "crosswalk Sleeper or GSIS ID is not unique",
			})
			continue
		}
		if owner, occupied := localByGSIS[crossGSIS]; occupied && owner != player.ID {
			summary.conflicts++
			summary.issues = append(summary.issues, IdentityIssue{
				SleeperID: player.SleeperID, CrosswalkGSIS: crossGSIS,
				Reason: "crosswalk GSIS ID is already assigned locally",
			})
			continue
		}
		backfills[player.ID] = crossGSIS
		localByGSIS[crossGSIS] = player.ID
	}

	matches := make([]mappedWeeklyStat, 0, len(weekly))
	for _, stat := range weekly {
		playerID, found := localByGSIS[stat.GSISID]
		if !found {
			continue
		}
		matches = append(matches, mappedWeeklyStat{PlayerID: playerID, Stat: stat})
		summary.matchedGSIS[stat.GSISID] = struct{}{}
	}
	return matches, backfills, summary
}

func highestValueUnmatched(
	weekly []nflverse.WeeklyStat,
	matchedGSIS map[string]struct{},
) []UnmatchedPlayer {
	byGSIS := make(map[string]UnmatchedPlayer)
	for _, stat := range weekly {
		if _, matched := matchedGSIS[stat.GSISID]; matched {
			continue
		}
		player := byGSIS[stat.GSISID]
		player.GSISID = stat.GSISID
		player.Name = stat.PlayerName
		player.Position = stat.Position
		player.FantasyPoints += FantasyPointsHalfPPR(stat)
		byGSIS[stat.GSISID] = player
	}
	unmatched := make([]UnmatchedPlayer, 0, len(byGSIS))
	for _, player := range byGSIS {
		player.FantasyPoints = math.Round(player.FantasyPoints*100) / 100
		unmatched = append(unmatched, player)
	}
	sort.Slice(unmatched, func(i, j int) bool {
		if unmatched[i].FantasyPoints == unmatched[j].FantasyPoints {
			return unmatched[i].GSISID < unmatched[j].GSISID
		}
		return unmatched[i].FantasyPoints > unmatched[j].FantasyPoints
	})
	if len(unmatched) > 20 {
		unmatched = unmatched[:20]
	}
	return unmatched
}
