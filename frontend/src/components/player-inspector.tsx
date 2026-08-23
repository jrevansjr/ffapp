import { useQuery } from "@tanstack/react-query"

import WeeklyTrendChart, { type WeeklySeries } from "@/components/weekly-trend-chart"
import { getPlayer } from "@/lib/api"
import type { PlayerDetail } from "@/lib/types"

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
  playerID: number | null
}

/** PlayerInspector loads one selected player's local detail and decision signals. */
export default function PlayerInspector({ playerID }: PlayerInspectorProps) {
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
            Select an available player to compare weekly consistency, usage, ADP, and odds.
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
            <p className="text-xs text-muted-foreground">Position tier</p>
            <p className="font-semibold tabular-nums">{formatWhole(detail.data.tier?.tier)}</p>
          </div>
        </div>

        <dl className="mt-4 grid grid-cols-3 gap-2">
          <Metric label="Aggregate ADP" value={formatDecimal(detail.data.adp.fantasypros)} />
          <Metric label="Sleeper ADP" value={formatDecimal(detail.data.adp.sleeper)} />
          <Metric label="Underdog ADP" value={formatDecimal(detail.data.adp.underdog)} />
        </dl>
      </section>

      <section className="rounded-lg border border-border bg-card p-4" aria-labelledby="weekly-summary-heading">
        <h3 className="text-sm font-semibold" id="weekly-summary-heading">
          2025 Half-PPR Weekly Range
        </h3>
        <dl className="mt-3 grid grid-cols-4 gap-2">
          <Metric label="Average" value={formatDecimal(detail.data.weekly_summary.average)} />
          <Metric label="High" value={formatDecimal(detail.data.weekly_summary.high)} />
          <Metric label="Median" value={formatDecimal(detail.data.weekly_summary.median)} />
          <Metric label="Low" value={formatDecimal(detail.data.weekly_summary.low)} />
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

      <section className="rounded-lg border border-border bg-card p-4" aria-labelledby="player-context-heading">
        <h3 className="text-sm font-semibold" id="player-context-heading">
          Season and Market Context
        </h3>
        <dl className="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-3">
          <Metric label="Games played" value={formatWhole(detail.data.season?.games_played)} />
          <Metric label="Receptions" value={formatWhole(detail.data.season?.receptions)} />
          <Metric label="Targets/game" value={formatDecimal(detail.data.season?.targets_per_game)} />
          <Metric
            label="Rush attempts/game"
            value={formatDecimal(detail.data.season?.rushing_attempts_per_game)}
          />
          <Metric label="TD O/U" value={formatDecimal(detail.data.odds.touchdowns?.line)} />
          <Metric label="Team wins O/U" value={formatDecimal(detail.data.odds.team_wins?.line)} />
        </dl>
      </section>
    </aside>
  )
}
