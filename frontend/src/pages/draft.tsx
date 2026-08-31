import { useMemo, useState } from "react"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { Link } from "react-router"

import DraftPlayerTable from "@/components/draft-player-table"
import PlayerInspector from "@/components/player-inspector"
import {
  getDraftState,
  getNFLTeams,
  getPlayers,
  markPlayerDrafted,
  undoManualPick,
} from "@/lib/api"
import type { DraftPick, DraftState } from "@/lib/types"
import { cn } from "@/lib/utils"

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

function formatTimestamp(timestamp: string): string {
  const date = new Date(timestamp)
  return Number.isNaN(date.getTime())
    ? "—"
    : date.toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" })
}

function draftStatusPresentation(state: DraftState): {
  badgeClassName: string
  label: string
  message: string
  panelClassName: string
} {
  switch (state.status) {
    case "current":
      return {
        badgeClassName: "bg-emerald-100 text-emerald-900",
        label: "Live sync",
        message: "Sleeper picks are current.",
        panelClassName: "border-emerald-200 bg-emerald-50/40",
      }
    case "syncing":
      return {
        badgeClassName: "bg-blue-100 text-blue-900",
        label: "Syncing",
        message: "Waiting for the first successful Sleeper sync.",
        panelClassName: "border-blue-200 bg-blue-50/40",
      }
    case "stale":
      return {
        badgeClassName: "bg-amber-100 text-amber-950",
        label: "Stale snapshot",
        message: "Sleeper is unavailable; showing the last persisted draft state.",
        panelClassName: "border-amber-300 bg-amber-50/50",
      }
    case "disabled":
      return {
        badgeClassName: "bg-neutral-200 text-neutral-900",
        label: "Polling paused",
        message: "Showing persisted picks. Manual draft actions remain available.",
        panelClassName: "border-neutral-300 bg-neutral-50",
      }
    default:
      return {
        badgeClassName: "bg-neutral-200 text-neutral-900",
        label: "Not configured",
        message: "Save a Sleeper draft ID to start tracking picks.",
        panelClassName: "border-neutral-300 bg-neutral-50",
      }
  }
}

function unknownPickLabel(pick: DraftPick): string {
  const name = [pick.first_name, pick.last_name].filter(Boolean).join(" ")
  return name === "" ? pick.sleeper_player_id : `${name} (${pick.sleeper_player_id})`
}

function pickPlayerLabel(pick: DraftPick): string {
  const name = [pick.first_name, pick.last_name].filter(Boolean).join(" ")
  return name === "" ? `Player ${pick.sleeper_player_id}` : name
}

/** DraftPage coordinates local availability, filters, selection, and inspector data. */
export default function DraftPage() {
  const queryClient = useQueryClient()
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
  const draftState = useQuery({
    queryKey: ["draft-state"],
    queryFn: getDraftState,
    retry: false,
    refetchInterval: 1000,
  })
  const markDrafted = useMutation({
    mutationFn: markPlayerDrafted,
    onSuccess: (state) => {
      queryClient.setQueryData(["draft-state"], state)
      setSelectedPlayerID(null)
    },
  })
  const undoPick = useMutation({
    mutationFn: undoManualPick,
    onSuccess: (state) => queryClient.setQueryData(["draft-state"], state),
  })

  const takenPlayerIDs = draftState.data?.taken_player_ids
  const displayedPlayers = useMemo(() => {
    if (!players.data) return []
    if (!takenPlayerIDs) return players.data
    const taken = new Set(takenPlayerIDs)
    return players.data.map((player) => ({ ...player, is_taken: taken.has(player.id) }))
  }, [players.data, takenPlayerIDs])
  const availablePlayers = useMemo(
    () => displayedPlayers.filter((player) => !player.is_taken),
    [displayedPlayers],
  )
  const inspectedPlayerID =
    selectedPlayerID !== null && availablePlayers.some((player) => player.id === selectedPlayerID)
      ? selectedPlayerID
      : null
  const tiers = useMemo(
    () =>
      Array.from(
        new Set(
          displayedPlayers.flatMap((player) =>
            player.draft.tier === null ? [] : [player.draft.tier],
          ),
        ),
      ).sort((left, right) => left - right),
    [displayedPlayers],
  )
  const filteredPlayers = useMemo(() => {
    const name = filters.name.trim().toLocaleLowerCase()
    return availablePlayers.filter((player) => {
      const fullName = `${player.first_name} ${player.last_name}`.toLocaleLowerCase()
      return (
        (name === "" || fullName.includes(name)) &&
        (filters.position === "" || player.position === filters.position) &&
        (filters.team === "" || player.nfl_team?.abbreviation === filters.team) &&
        (filters.tier === "" || player.draft.tier === Number(filters.tier))
      )
    })
  }, [availablePlayers, filters])

  const takenCount = displayedPlayers.length - availablePlayers.length
  const unknownSleeperIDs = new Set(draftState.data?.unknown_sleeper_player_ids ?? [])
  const unknownPicks =
    draftState.data?.picks.filter((pick) => unknownSleeperIDs.has(pick.sleeper_player_id)) ?? []
  const manualPicks = draftState.data?.picks.filter((pick) => pick.source === "manual") ?? []
  const draftStatus = draftState.data ? draftStatusPresentation(draftState.data) : null

  function selectPlayer(playerID: number) {
    markDrafted.reset()
    setSelectedPlayerID(playerID)
  }

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

      <div
        className={cn(
          "mt-5 rounded-lg border bg-card px-4 py-3 text-sm",
          draftStatus?.panelClassName ?? "border-border",
        )}
      >
        {draftState.isPending && (
          <p className="text-muted-foreground" role="status">
            Loading draft configuration…
          </p>
        )}
        {draftState.isError && !draftState.data && (
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="text-red-700" role="alert">
              {errorMessage(draftState.error, "Could not load draft state.")}
            </p>
            <button
              className="rounded-md border border-border px-3 py-1.5 font-medium hover:bg-muted"
              onClick={() => void draftState.refetch()}
              type="button"
            >
              Try Again
            </button>
          </div>
        )}
        {draftState.data?.status === "not_configured" && (
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <div className="flex flex-wrap items-center gap-2">
                <span className={cn("rounded-full px-2.5 py-1 text-xs font-semibold", draftStatus?.badgeClassName)}>
                  {draftStatus?.label}
                </span>
                <p className="font-medium">No active Sleeper draft configured</p>
              </div>
              <p className="mt-1 text-muted-foreground">
                {draftStatus?.message} All imported players remain available.
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
        {draftState.data && draftState.data.status !== "not_configured" && (
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex flex-wrap items-center gap-3">
              <span
                className={cn(
                  "rounded-full px-2.5 py-1 text-xs font-semibold",
                  draftStatus?.badgeClassName,
                )}
              >
                {draftStatus?.label}
              </span>
              <div>
                <p className="font-medium">
                  Draft <code>{draftState.data.draft_id}</code>
                </p>
                <p className="mt-0.5 text-xs text-muted-foreground">{draftStatus?.message}</p>
              </div>
            </div>
            <div className="text-right text-xs text-muted-foreground">
              <p>{draftState.data.picks.length} recorded picks</p>
              <p>
                {draftState.data.last_synced_at
                  ? `Last sync ${formatTimestamp(draftState.data.last_synced_at)}`
                  : "No successful sync yet"}
              </p>
            </div>
          </div>
        )}
        {draftState.data && draftState.data.unknown_sleeper_player_ids.length > 0 && (
          <div className="mt-3 border-t border-amber-200 pt-3 text-xs text-amber-900">
            <p>
              {draftState.data.unknown_sleeper_player_ids.length} pick
              {draftState.data.unknown_sleeper_player_ids.length === 1 ? " has" : "s have"} an
              unknown local player ID; synchronization is continuing.
            </p>
            <p className="mt-0.5 break-words">
              Unknown mapping: {unknownPicks.map(unknownPickLabel).join(", ")}
            </p>
          </div>
        )}
        {manualPicks.length > 0 && (
          <div className="mt-3 border-t border-border pt-3">
            <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
              Manual picks
            </p>
            <ul className="mt-2 grid gap-2 sm:grid-cols-2">
              {manualPicks.map((pick) => (
                <li
                  className="flex items-center justify-between gap-3 rounded-md bg-muted px-3 py-2"
                  key={pick.id}
                >
                  <div className="min-w-0">
                    <p className="truncate font-medium">
                      #{pick.pick_number} {pickPlayerLabel(pick)}
                    </p>
                    <p className="text-xs text-muted-foreground">
                      {[pick.position, pick.team].filter(Boolean).join(" · ") ||
                        "Player details unavailable"}
                    </p>
                  </div>
                  <button
                    className="shrink-0 rounded-md border border-border bg-card px-2.5 py-1 text-xs font-medium hover:bg-background disabled:cursor-not-allowed disabled:opacity-50"
                    disabled={undoPick.isPending}
                    onClick={() => undoPick.mutate(pick.id)}
                    type="button"
                  >
                    {undoPick.isPending && undoPick.variables === pick.id ? "Undoing…" : "Undo"}
                  </button>
                </li>
              ))}
            </ul>
            {undoPick.isError && (
              <p className="mt-2 text-xs text-red-700" role="alert">
                {errorMessage(undoPick.error, "Could not undo manual pick.")}
              </p>
            )}
          </div>
        )}
        {draftState.isError && draftState.data && (
          <div className="mt-3 border-t border-red-200 pt-3 text-xs text-red-800" role="alert">
            <p className="font-semibold">Browser refresh failed</p>
            <p className="mt-0.5">
              The local API could not be reached; showing the last snapshot held by this browser.
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
                FantasyPros tier
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
              onSelectPlayer={selectPlayer}
              players={filteredPlayers}
              selectedPlayerID={selectedPlayerID}
            />
          </section>

          <PlayerInspector
            canMarkDrafted={
              draftState.data !== undefined && draftState.data.status !== "not_configured"
            }
            isMarkingDrafted={markDrafted.isPending}
            markDraftedError={
              markDrafted.isError
                ? errorMessage(markDrafted.error, "Could not mark player as drafted.")
                : null
            }
            onMarkDrafted={(playerID) => markDrafted.mutate(playerID)}
            playerID={inspectedPlayerID}
          />
        </div>
      )}
    </section>
  )
}
