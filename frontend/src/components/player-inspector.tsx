import { useQuery } from "@tanstack/react-query"

import WeeklyTrendChart, { type WeeklySeries } from "@/components/weekly-trend-chart"
import { getPlayer } from "@/lib/api"
import type { OddsLine, PlayerDetail } from "@/lib/types"

function formatDecimal(value: number | null | undefined): string {
  return value === null || value === undefined ? "—" : value.toFixed(1)
}

function formatWhole(value: number | null | undefined): string {
  return value === null || value === undefined ? "—" : String(value)
}

function errorMessage(error: unknown): string {
  return error instanceof Error && error.message !== ""
    ? error.message
    : "Could not load player details."
}

// The second chart changes by position so it shows the most useful weekly
// volume signal without growing into separate position-specific inspectors.
function usageChart(detail: PlayerDetail): { title: string; series: WeeklySeries[] } {
  switch (detail.player.position) {
    case "QB":
      return {
        title: "Passing Yards by Week",
        series: [{ dataKey: "passing_yards", label: "Passing yards", color: "#0f766e" }],
      }
    case "RB":
      return {
        title: "Weekly Opportunity",
        series: [
          { dataKey: "rushing_attempts", label: "Rush attempts", color: "#0f766e" },
          { dataKey: "targets", label: "Targets", color: "#c2410c", yAxis: "right" },
        ],
      }
    default:
      return {
        title: "Targets by Week",
        series: [{ dataKey: "targets", label: "Targets", color: "#0f766e" }],
      }
  }
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border border-border bg-background px-3 py-2">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="mt-1 font-semibold tabular-nums">{value}</dd>
    </div>
  )
}

interface PlayerInspectorProps {
  canMarkDrafted: boolean
  isMarkingDrafted: boolean
  markDraftedError: string | null
  onMarkDrafted: (playerID: number) => void
  playerID: number | null
}

function projectionMetrics(detail: PlayerDetail): Array<{ label: string; value: number | null }> {
  const projections = detail.projections
  switch (detail.player.position) {
    case "QB":
      return [
        { label: "Passing yards", value: projections?.passing_yards ?? null },
        { label: "Passing TDs", value: projections?.passing_touchdowns ?? null },
        { label: "Rushing yards", value: projections?.rushing_yards ?? null },
        { label: "Rushing TDs", value: projections?.rushing_touchdowns ?? null },
      ]
    case "RB":
      return [
        { label: "Rushing yards", value: projections?.rushing_yards ?? null },
        { label: "Rushing TDs", value: projections?.rushing_touchdowns ?? null },
        { label: "Receiving yards", value: projections?.receiving_yards ?? null },
        { label: "Receiving TDs", value: projections?.receiving_touchdowns ?? null },
      ]
    default:
      return [
        { label: "Receiving yards", value: projections?.receiving_yards ?? null },
        { label: "Receiving TDs", value: projections?.receiving_touchdowns ?? null },
      ]
  }
}

function oddsMetrics(detail: PlayerDetail): Array<{ label: string; value: number | null }> {
  const odds = detail.odds
  const teamWins = { label: "Team wins O/U", value: odds.team_wins?.line ?? null }
  switch (detail.player.position) {
    case "QB":
      return [
        { label: "Pass yards O/U", value: odds.passing_yards?.line ?? null },
        { label: "Pass TDs O/U", value: odds.passing_touchdowns?.line ?? null },
        { label: "Rush yards O/U", value: odds.rushing_yards?.line ?? null },
        { label: "Rush TDs O/U", value: odds.rushing_touchdowns?.line ?? null },
        teamWins,
      ]
    case "RB":
      return [
        { label: "Rush yards O/U", value: odds.rushing_yards?.line ?? null },
        { label: "Rush TDs O/U", value: odds.rushing_touchdowns?.line ?? null },
        { label: "Rec yards O/U", value: odds.receiving_yards?.line ?? null },
        { label: "Rec TDs O/U", value: odds.receiving_touchdowns?.line ?? null },
        teamWins,
      ]
    default:
      return [
        { label: "Rec yards O/U", value: odds.receiving_yards?.line ?? null },
        { label: "Rec TDs O/U", value: odds.receiving_touchdowns?.line ?? null },
        teamWins,
      ]
  }
}

function oddsProvenance(detail: PlayerDetail): OddsLine | null {
  return (
    Object.values(detail.odds).find((line): line is OddsLine => line !== null) ?? null
  )
}

function formatCapturedAt(timestamp: string): string {
  const date = new Date(timestamp)
  return Number.isNaN(date.getTime())
    ? timestamp
    : date.toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" })
}

/** PlayerInspector loads one selected player's local detail and exposes the manual draft fallback. */
export default function PlayerInspector({
  canMarkDrafted,
  isMarkingDrafted,
  markDraftedError,
  onMarkDrafted,
  playerID,
}: PlayerInspectorProps) {
  const detail = useQuery({
    queryKey: ["player", playerID],
    queryFn: () => getPlayer(playerID as number),
    enabled: playerID !== null,
    retry: false,
    staleTime: Infinity,
  })

  if (playerID === null) {
    return (
      <aside className="flex min-h-80 items-center justify-center rounded-lg border border-dashed border-border bg-card p-8 text-center">
        <div>
          <h2 className="font-semibold">Player Inspector</h2>
          <p className="mt-2 text-sm text-muted-foreground">
            Select an available player to compare weekly consistency, usage, ADP, and projections.
          </p>
        </div>
      </aside>
    )
  }

  if (detail.isPending) {
    return (
      <aside className="rounded-lg border border-border bg-card p-6 text-sm text-muted-foreground" role="status">
        Loading player details from the local database…
      </aside>
    )
  }

  if (detail.isError) {
    return (
      <aside className="rounded-lg border border-red-200 bg-red-50 p-5 text-sm">
        <p className="text-red-800" role="alert">
          {errorMessage(detail.error)}
        </p>
        <button
          className="mt-3 rounded-md border border-red-300 bg-white px-3 py-1.5 font-medium text-red-800 hover:bg-red-100"
          onClick={() => void detail.refetch()}
          type="button"
        >
          Try Again
        </button>
      </aside>
    )
  }

  const player = detail.data.player
  const usage = usageChart(detail.data)
  const oddsSource = oddsProvenance(detail.data)

  return (
    <aside className="space-y-4 xl:sticky xl:top-4" aria-labelledby="inspector-heading">
      <section className="rounded-lg border border-border bg-card p-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
              {player.position} · {player.nfl_team?.abbreviation ?? "No NFL team"}
            </p>
            <h2 className="mt-1 text-xl font-semibold" id="inspector-heading">
              {player.first_name} {player.last_name}
            </h2>
          </div>
          <div className="rounded-md bg-muted px-3 py-2 text-center">
            <p className="text-xs text-muted-foreground">FantasyPros tier</p>
            <p className="font-semibold tabular-nums">{formatWhole(detail.data.draft.tier)}</p>
          </div>
        </div>

        <dl className="mt-4 grid grid-cols-3 gap-2">
          <Metric label="Aggregate ADP" value={formatDecimal(detail.data.draft.aggregate_adp)} />
          <Metric label="Overall ECR" value={formatWhole(detail.data.draft.ecr)} />
          <Metric
            label="Position rank"
            value={
              detail.data.draft.position_rank === null
                ? "—"
                : `${player.position}${detail.data.draft.position_rank}`
            }
          />
        </dl>

        <div className="mt-4 border-t border-border pt-4">
          <button
            className="w-full rounded-md bg-neutral-900 px-4 py-2 text-sm font-semibold text-white hover:bg-neutral-700 disabled:cursor-not-allowed disabled:opacity-50"
            disabled={!canMarkDrafted || isMarkingDrafted}
            onClick={() => onMarkDrafted(player.id)}
            type="button"
          >
            {isMarkingDrafted ? "Marking Drafted…" : "Mark Drafted"}
          </button>
          {!canMarkDrafted && (
            <p className="mt-2 text-xs text-muted-foreground">
              Save a Sleeper draft ID in Admin before recording manual picks.
            </p>
          )}
          {markDraftedError && (
            <p className="mt-2 text-xs text-red-700" role="alert">
              {markDraftedError}
            </p>
          )}
        </div>
      </section>

      <section className="rounded-lg border border-border bg-card p-4" aria-labelledby="expert-range-heading">
        <h3 className="text-sm font-semibold" id="expert-range-heading">
          Expert Ranking Range
        </h3>
        <p className="mt-1 text-xs text-muted-foreground">
          Min and max are the highest and lowest submitted ranks; standard deviation summarizes disagreement.
        </p>
        <dl className="mt-3 grid grid-cols-3 gap-2">
          <Metric label="Minimum" value={formatWhole(detail.data.draft.rank_min)} />
          <Metric label="Maximum" value={formatWhole(detail.data.draft.rank_max)} />
          <Metric label="Std dev" value={formatDecimal(detail.data.draft.rank_std_dev)} />
        </dl>
      </section>

      <section className="rounded-lg border border-border bg-card p-4" aria-labelledby="weekly-summary-heading">
        <h3 className="text-sm font-semibold" id="weekly-summary-heading">
          2025 Performance
        </h3>
        <p className="mt-1 text-xs text-muted-foreground">Half-PPR results and weekly range.</p>
        <dl className="mt-3 grid grid-cols-4 gap-2">
          <Metric label="Average" value={formatDecimal(detail.data.weekly_summary.average)} />
          <Metric label="High" value={formatDecimal(detail.data.weekly_summary.high)} />
          <Metric label="Median" value={formatDecimal(detail.data.weekly_summary.median)} />
          <Metric label="Low" value={formatDecimal(detail.data.weekly_summary.low)} />
        </dl>
        <dl className="mt-2 grid grid-cols-2 gap-2 sm:grid-cols-4">
          <Metric label="Games played" value={formatWhole(detail.data.season?.games_played)} />
          <Metric label="Receptions" value={formatWhole(detail.data.season?.receptions)} />
          <Metric label="Targets/game" value={formatDecimal(detail.data.season?.targets_per_game)} />
          <Metric
            label="Rush attempts/game"
            value={formatDecimal(detail.data.season?.rushing_attempts_per_game)}
          />
        </dl>
      </section>

      {detail.data.weekly.length === 0 ? (
        <p className="rounded-lg border border-border bg-card px-4 py-8 text-center text-sm text-muted-foreground">
          No weekly data available.
        </p>
      ) : (
        <div className="grid gap-4 2xl:grid-cols-2">
          <WeeklyTrendChart
            average={detail.data.weekly_summary.average}
            data={detail.data.weekly}
            series={[
              { dataKey: "fantasy_points_half_ppr", label: "Fantasy points", color: "#1d4ed8" },
            ]}
            title="Fantasy Points by Week"
          />
          <WeeklyTrendChart data={detail.data.weekly} series={usage.series} title={usage.title} />
        </div>
      )}

      <section className="rounded-lg border border-border bg-card p-4" aria-labelledby="projection-heading">
        <h3 className="text-sm font-semibold" id="projection-heading">
          2026 FantasyPros Projections
        </h3>
        <p className="mt-1 text-xs text-muted-foreground">
          Preseason forecasts, kept separate from historical results and sportsbook odds.
        </p>
        <dl className="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-4">
          {projectionMetrics(detail.data).map((metric) => (
            <Metric key={metric.label} label={metric.label} value={formatDecimal(metric.value)} />
          ))}
        </dl>
      </section>

      <section className="rounded-lg border border-border bg-card p-4" aria-labelledby="odds-heading">
        <h3 className="text-sm font-semibold" id="odds-heading">
          2026 Sportsbook Consensus
        </h3>
        <p className="mt-1 text-xs text-muted-foreground">
          Supplied season lines, kept separate from FantasyPros projections.
          {oddsSource && ` Captured ${formatCapturedAt(oddsSource.captured_at)}.`}
        </p>
        <dl className="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-3">
          {oddsMetrics(detail.data).map((metric) => (
            <Metric key={metric.label} label={metric.label} value={formatDecimal(metric.value)} />
          ))}
        </dl>
      </section>
    </aside>
  )
}
