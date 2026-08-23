import { useMemo, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { Link } from "react-router"

import DraftPlayerTable from "@/components/draft-player-table"
import PlayerInspector from "@/components/player-inspector"
import { getNFLTeams, getPlayers, getSettings } from "@/lib/api"

const positions = ["QB", "RB", "WR", "TE"]
const controlClassName =
  "mt-1.5 block h-9 w-full rounded-md border border-border bg-card px-2.5 text-sm outline-none focus:border-neutral-400 focus:ring-2 focus:ring-neutral-200 disabled:cursor-not-allowed disabled:opacity-60"

interface DraftFilters {
  name: string
  position: string
  team: string
  tier: string
}

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message !== "" ? error.message : fallback
}

/** DraftPage coordinates local availability, filters, selection, and inspector data. */
export default function DraftPage() {
  const [filters, setFilters] = useState<DraftFilters>({
    name: "",
    position: "",
    team: "",
    tier: "",
  })
  const [selectedPlayerID, setSelectedPlayerID] = useState<number | null>(null)
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
  const settings = useQuery({
    queryKey: ["settings"],
    queryFn: getSettings,
    retry: false,
    staleTime: Infinity,
  })

  const availablePlayers = useMemo(
    () => (players.data ?? []).filter((player) => !player.is_taken),
    [players.data],
  )
  const tiers = useMemo(
    () =>
      Array.from(
        new Set(
          (players.data ?? []).flatMap((player) => (player.tier === null ? [] : [player.tier])),
        ),
      ).sort((left, right) => left - right),
    [players.data],
  )
  const filteredPlayers = useMemo(() => {
    const name = filters.name.trim().toLocaleLowerCase()
    return availablePlayers.filter((player) => {
      const fullName = `${player.first_name} ${player.last_name}`.toLocaleLowerCase()
      return (
        (name === "" || fullName.includes(name)) &&
        (filters.position === "" || player.position === filters.position) &&
        (filters.team === "" || player.nfl_team?.abbreviation === filters.team) &&
        (filters.tier === "" || player.tier === Number(filters.tier))
      )
    })
  }, [availablePlayers, filters])

  const takenCount = (players.data?.length ?? 0) - availablePlayers.length
  const draftID = settings.data?.sleeper_draft_id ?? ""

  return (
    <section aria-labelledby="draft-heading">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold" id="draft-heading">
            Draft Day
          </h1>
          <p className="mt-2 text-sm text-muted-foreground">
            Compare available players using persisted local draft and reference data.
          </p>
        </div>
        {players.data && (
          <p className="text-sm tabular-nums text-muted-foreground">
            {availablePlayers.length} available · {takenCount} taken
          </p>
        )}
      </div>

      <div className="mt-5 rounded-lg border border-border bg-card px-4 py-3 text-sm">
        {settings.isPending && (
          <p className="text-muted-foreground" role="status">
            Loading draft configuration…
          </p>
        )}
        {settings.isError && (
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-red-700" role="alert">
              {errorMessage(settings.error, "Could not load draft configuration.")}
            </p>
            <button
              className="rounded-md border border-border px-3 py-1.5 font-medium hover:bg-muted"
              onClick={() => void settings.refetch()}
              type="button"
            >
              Try Again
            </button>
          </div>
        )}
        {settings.data && draftID === "" && (
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <p className="font-medium">No active Sleeper draft configured.</p>
              <p className="mt-1 text-muted-foreground">
                All imported players are available. Live Sleeper synchronization arrives in M7.
              </p>
            </div>
            <Link
              className="rounded-md border border-border px-3 py-1.5 font-medium hover:bg-muted"
              to="/admin"
            >
              Open Admin
            </Link>
          </div>
        )}
        {settings.data && draftID !== "" && (
          <div className="flex flex-wrap items-center gap-3">
            <span className="rounded-full bg-muted px-2.5 py-1 text-xs font-semibold">Local data</span>
            <p>
              Draft <code>{draftID}</code> is configured. Live Sleeper synchronization is not active yet.
            </p>
          </div>
        )}
      </div>

      {players.isPending && (
        <p
          className="mt-4 rounded-lg border border-border bg-card px-4 py-10 text-center text-sm text-muted-foreground"
          role="status"
        >
          Loading available players from the local database…
        </p>
      )}

      {players.isError && (
        <div className="mt-4 rounded-lg border border-red-200 bg-red-50 p-4 text-sm">
          <p className="text-red-800" role="alert">
            {errorMessage(players.error, "Could not load players from the local database.")}
          </p>
          <p className="mt-1 text-red-700">This page does not contact Sleeper.</p>
          <button
            className="mt-3 rounded-md border border-red-300 bg-white px-3 py-1.5 font-medium text-red-800 hover:bg-red-100"
            onClick={() => void players.refetch()}
            type="button"
          >
            Try Again
          </button>
        </div>
      )}

      {players.data && (
        <div className="mt-4 grid items-start gap-4 xl:grid-cols-[minmax(0,1.1fr)_minmax(440px,0.9fr)]">
          <section
            className="min-w-0 rounded-lg border border-border bg-muted/30 p-3"
            aria-labelledby="available-heading"
          >
            <div className="flex flex-wrap items-end justify-between gap-3">
              <div>
                <h2 className="font-semibold" id="available-heading">
                  Available Players
                </h2>
                <p className="mt-1 text-xs tabular-nums text-muted-foreground">
                  {filteredPlayers.length === availablePlayers.length
                    ? `${availablePlayers.length} players`
                    : `${filteredPlayers.length} of ${availablePlayers.length} players`}
                </p>
              </div>
              {nflTeams.isError && (
                <button
                  className="text-xs font-medium underline"
                  onClick={() => void nflTeams.refetch()}
                  type="button"
                >
                  Retry team filter
                </button>
              )}
            </div>

            <div className="my-3 grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
              <label className="text-xs font-medium" htmlFor="draft-name-filter">
                Player search
                <input
                  className={controlClassName}
                  id="draft-name-filter"
                  onChange={(event) =>
                    setFilters((current) => ({ ...current, name: event.target.value }))
                  }
                  placeholder="Name"
                  type="search"
                  value={filters.name}
                />
              </label>
              <label className="text-xs font-medium" htmlFor="draft-position-filter">
                Position
                <select
                  className={controlClassName}
                  id="draft-position-filter"
                  onChange={(event) =>
                    setFilters((current) => ({ ...current, position: event.target.value }))
                  }
                  value={filters.position}
                >
                  <option value="">All</option>
                  {positions.map((position) => (
                    <option key={position}>{position}</option>
                  ))}
                </select>
              </label>
              <label className="text-xs font-medium" htmlFor="draft-team-filter">
                NFL team
                <select
                  className={controlClassName}
                  disabled={nflTeams.isPending || nflTeams.isError}
                  id="draft-team-filter"
                  onChange={(event) =>
                    setFilters((current) => ({ ...current, team: event.target.value }))
                  }
                  value={filters.team}
                >
                  <option value="">
                    {nflTeams.isPending ? "Loading…" : nflTeams.isError ? "Unavailable" : "All"}
                  </option>
                  {nflTeams.data?.map((team) => (
                    <option key={team.id} value={team.abbreviation}>
                      {team.abbreviation}
                    </option>
                  ))}
                </select>
              </label>
              <label className="text-xs font-medium" htmlFor="draft-tier-filter">
                Position tier
                <select
                  className={controlClassName}
                  id="draft-tier-filter"
                  onChange={(event) =>
                    setFilters((current) => ({ ...current, tier: event.target.value }))
                  }
                  value={filters.tier}
                >
                  <option value="">All</option>
                  {tiers.map((tier) => (
                    <option key={tier} value={tier}>
                      Tier {tier}
                    </option>
                  ))}
                </select>
              </label>
            </div>

            <DraftPlayerTable
              emptyMessage={
                availablePlayers.length === 0
                  ? "No available players."
                  : "No players match these filters."
              }
              onSelectPlayer={setSelectedPlayerID}
              players={filteredPlayers}
              selectedPlayerID={selectedPlayerID}
            />
          </section>

          <PlayerInspector playerID={selectedPlayerID} />
        </div>
      )}
    </section>
  )
}
