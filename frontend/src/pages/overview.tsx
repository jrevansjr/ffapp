import { useMemo, useState } from "react"
import { useQuery } from "@tanstack/react-query"

import PlayerOverviewTable from "@/components/player-overview-table"
import { getNFLTeams, getPlayers } from "@/lib/api"
import type { PlayerFilters } from "@/lib/types"

const positions = ["QB", "RB", "WR", "TE"]
const selectClassName =
  "mt-1.5 block min-w-44 rounded-md border border-border bg-card px-3 py-2 text-sm outline-none focus:border-neutral-400 focus:ring-2 focus:ring-neutral-200 disabled:cursor-not-allowed disabled:opacity-60"

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message !== "" ? error.message : fallback
}

/** OverviewPage loads the persisted pool once and applies both filters locally. */
export default function OverviewPage() {
  const [filters, setFilters] = useState<PlayerFilters>({ position: "", team: "" })
  const players = useQuery({
    queryKey: ["players"],
    queryFn: getPlayers,
    retry: false,
    staleTime: Infinity,
  })
  const nflTeams = useQuery({
    queryKey: ["nfl-teams"],
    queryFn: getNFLTeams,
    retry: false,
    staleTime: Infinity,
  })

  const filteredPlayers = useMemo(() => {
    return (players.data ?? []).filter((player) => {
      const matchesPosition = filters.position === "" || player.position === filters.position
      const matchesTeam = filters.team === "" || player.nfl_team?.abbreviation === filters.team
      return matchesPosition && matchesTeam
    })
  }, [filters, players.data])

  return (
    <section aria-labelledby="overview-heading">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 id="overview-heading" className="text-xl font-semibold">
            Overview
          </h1>
          <p className="mt-2 text-sm text-muted-foreground">
            Compare the player pool persisted in your local database.
          </p>
        </div>
        {players.data && (
          <p className="text-sm tabular-nums text-muted-foreground">
            {filteredPlayers.length === players.data.length
              ? `${players.data.length} players`
              : `${filteredPlayers.length} of ${players.data.length} players`}
          </p>
        )}
      </div>

      <div className="mt-6 flex flex-wrap items-end gap-4">
        <label className="text-sm font-medium" htmlFor="position-filter">
          Position
          <select
            className={selectClassName}
            id="position-filter"
            onChange={(event) =>
              setFilters((current) => ({ ...current, position: event.target.value }))
            }
            value={filters.position}
          >
            <option value="">All positions</option>
            {positions.map((position) => (
              <option key={position} value={position}>
                {position}
              </option>
            ))}
          </select>
        </label>

        <label className="text-sm font-medium" htmlFor="team-filter">
          NFL team
          <select
            className={selectClassName}
            disabled={nflTeams.isPending || nflTeams.isError}
            id="team-filter"
            onChange={(event) =>
              setFilters((current) => ({ ...current, team: event.target.value }))
            }
            value={filters.team}
          >
            <option value="">
              {nflTeams.isPending
                ? "Loading teams…"
                : nflTeams.isError
                  ? "Teams unavailable"
                  : "All NFL teams"}
            </option>
            {nflTeams.data?.map((team) => (
              <option key={team.id} value={team.abbreviation}>
                {team.abbreviation} — {team.name}
              </option>
            ))}
          </select>
        </label>

        {nflTeams.isError && (
          <div className="pb-0.5 text-sm">
            <span className="mr-2 text-red-700">
              {errorMessage(nflTeams.error, "Could not load NFL teams.")}
            </span>
            <button
              className="rounded-md border border-border bg-card px-3 py-1.5 font-medium hover:bg-muted"
              onClick={() => void nflTeams.refetch()}
              type="button"
            >
              Try Again
            </button>
          </div>
        )}
      </div>

      <div className="mt-4">
        {players.isPending && (
          <p className="rounded-lg border border-border bg-card px-4 py-8 text-center text-sm text-muted-foreground" role="status">
            Loading players from the local database…
          </p>
        )}

        {players.isError && (
          <div className="rounded-lg border border-red-200 bg-red-50 p-4 text-sm">
            <p className="text-red-800" role="alert">
              {errorMessage(players.error, "Could not load players from the local database.")}
            </p>
            <p className="mt-1 text-red-700">
              This reads the Go API and SQLite only; it does not contact Sleeper.
            </p>
            <button
              className="mt-3 rounded-md border border-red-300 bg-white px-3 py-1.5 font-medium text-red-800 hover:bg-red-100"
              onClick={() => void players.refetch()}
              type="button"
            >
              Try Again
            </button>
          </div>
        )}

        {players.data && <PlayerOverviewTable players={filteredPlayers} />}
      </div>
    </section>
  )
}
