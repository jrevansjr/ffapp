import {
  createColumnHelper,
  createSortedRowModel,
  flexRender,
  rowSortingFeature,
  sortFn_basic,
  sortFn_text,
  tableFeatures,
  useTable,
  type SortDirection,
} from "@tanstack/react-table"

import { cn } from "@/lib/utils"
import type { PlayerListItem } from "@/lib/types"

interface OverviewColumnMeta {
  numeric?: boolean
  sticky?: boolean
}

const overviewTableFeatures = tableFeatures({
  rowSortingFeature,
  sortedRowModel: createSortedRowModel(),
  sortFns: {
    basic: sortFn_basic,
    text: sortFn_text,
  },
  columnMeta: {} as OverviewColumnMeta,
})

const columnHelper = createColumnHelper<typeof overviewTableFeatures, PlayerListItem>()

function formatNumber(value: number | undefined, fractionDigits: number): string {
  return value === undefined ? "—" : value.toFixed(fractionDigits)
}

function formatWholeNumber(value: number | undefined): string {
  return value === undefined ? "—" : String(value)
}

const columns = columnHelper.columns([
  columnHelper.accessor((player) => `${player.first_name} ${player.last_name}`, {
    id: "name",
    header: "Name",
    sortFn: "text",
    sortDescFirst: false,
    meta: { sticky: true },
    cell: ({ getValue, row }) => (
      <span className={cn("font-medium", row.original.is_taken && "line-through")}>
        {getValue()}
        {row.original.is_taken && <span className="sr-only"> (Taken)</span>}
      </span>
    ),
  }),
  columnHelper.accessor("position", {
    header: "Position",
    sortFn: "text",
    sortDescFirst: false,
  }),
  columnHelper.accessor((player) => player.nfl_team?.abbreviation, {
    id: "team",
    header: "NFL Team",
    sortFn: "text",
    sortDescFirst: false,
    sortUndefined: "last",
    cell: ({ getValue }) => getValue() ?? "—",
  }),
  columnHelper.accessor((player) => player.years_exp ?? undefined, {
    id: "yearsExp",
    header: "Years Experience",
    sortFn: "basic",
    sortDescFirst: false,
    sortUndefined: "last",
    meta: { numeric: true },
    cell: ({ getValue }) => formatWholeNumber(getValue()),
  }),
  columnHelper.accessor((player) => player.adp.fantasypros ?? undefined, {
    id: "fantasyProsADP",
    header: "FantasyPros Aggregate ADP",
    sortFn: "basic",
    sortDescFirst: false,
    sortUndefined: "last",
    meta: { numeric: true },
    cell: ({ getValue }) => formatNumber(getValue(), 1),
  }),
  columnHelper.accessor((player) => player.adp.sleeper ?? undefined, {
    id: "sleeperADP",
    header: "Sleeper ADP",
    sortFn: "basic",
    sortDescFirst: false,
    sortUndefined: "last",
    meta: { numeric: true },
    cell: ({ getValue }) => formatNumber(getValue(), 1),
  }),
  columnHelper.accessor((player) => player.adp.underdog ?? undefined, {
    id: "underdogADP",
    header: "Underdog ADP",
    sortFn: "basic",
    sortDescFirst: false,
    sortUndefined: "last",
    meta: { numeric: true },
    cell: ({ getValue }) => formatNumber(getValue(), 1),
  }),
  columnHelper.accessor((player) => player.tier ?? undefined, {
    id: "tier",
    header: "2026 Projected Position Tier",
    sortFn: "basic",
    sortDescFirst: false,
    sortUndefined: "last",
    meta: { numeric: true },
    cell: ({ getValue }) => formatWholeNumber(getValue()),
  }),
  columnHelper.accessor((player) => player.season?.fantasy_points_half_ppr, {
    id: "fantasyPoints",
    header: "2025 Total Fantasy Points",
    sortFn: "basic",
    sortDescFirst: false,
    sortUndefined: "last",
    meta: { numeric: true },
    cell: ({ getValue }) => formatNumber(getValue(), 1),
  }),
  columnHelper.accessor((player) => player.season?.games_played, {
    id: "gamesPlayed",
    header: "2025 Games Played",
    sortFn: "basic",
    sortDescFirst: false,
    sortUndefined: "last",
    meta: { numeric: true },
    cell: ({ getValue }) => formatWholeNumber(getValue()),
  }),
  columnHelper.accessor((player) => player.season?.average_fantasy_points ?? undefined, {
    id: "averageFantasyPoints",
    header: "2025 Average Fantasy Points",
    sortFn: "basic",
    sortDescFirst: false,
    sortUndefined: "last",
    meta: { numeric: true },
    cell: ({ getValue }) => formatNumber(getValue(), 1),
  }),
  columnHelper.accessor((player) => player.season?.targets_per_game ?? undefined, {
    id: "targetsPerGame",
    header: "2025 Targets Per Game",
    sortFn: "basic",
    sortDescFirst: false,
    sortUndefined: "last",
    meta: { numeric: true },
    cell: ({ getValue }) => formatNumber(getValue(), 1),
  }),
  columnHelper.accessor((player) => player.season?.rushing_attempts_per_game ?? undefined, {
    id: "rushingAttemptsPerGame",
    header: "2025 Rushing Attempts Per Game",
    sortFn: "basic",
    sortDescFirst: false,
    sortUndefined: "last",
    meta: { numeric: true },
    cell: ({ getValue }) => formatNumber(getValue(), 1),
  }),
  columnHelper.accessor((player) => player.odds.touchdown_line ?? undefined, {
    id: "touchdownLine",
    header: "2026 Vegas TD O/U",
    sortFn: "basic",
    sortDescFirst: false,
    sortUndefined: "last",
    meta: { numeric: true },
    cell: ({ getValue }) => formatNumber(getValue(), 1),
  }),
  columnHelper.accessor((player) => player.odds.team_win_line ?? undefined, {
    id: "teamWinLine",
    header: "2026 Vegas Team Win O/U",
    sortFn: "basic",
    sortDescFirst: false,
    sortUndefined: "last",
    meta: { numeric: true },
    cell: ({ getValue }) => formatNumber(getValue(), 1),
  }),
])

function ariaSort(direction: false | SortDirection): "none" | "ascending" | "descending" {
  if (direction === "asc") return "ascending"
  if (direction === "desc") return "descending"
  return "none"
}

/** PlayerOverviewTable owns client-side sorting and dense table presentation. */
export default function PlayerOverviewTable({ players }: { players: PlayerListItem[] }) {
  const playerTable = useTable({
    features: overviewTableFeatures,
    columns,
    data: players,
    getRowId: (player) => String(player.id),
    enableMultiSort: false,
    enableSortingRemoval: false,
    initialState: {
      sorting: [{ id: "fantasyProsADP", desc: false }],
    },
  })

  return (
    <div className="overflow-x-auto rounded-lg border border-border bg-card">
      <table className="min-w-[1640px] w-full border-collapse text-left text-xs">
        <thead className="bg-muted text-muted-foreground">
          {playerTable.getHeaderGroups().map((headerGroup) => (
            <tr key={headerGroup.id}>
              {headerGroup.headers.map((header) => {
                const sorted = header.column.getIsSorted()
                const meta = header.column.columnDef.meta
                return (
                  <th
                    aria-sort={ariaSort(sorted)}
                    className={cn(
                      "border-b border-border px-3 py-2 font-semibold",
                      meta?.numeric && "text-right",
                      meta?.sticky && "sticky left-0 z-20 min-w-44 bg-muted",
                    )}
                    key={header.id}
                    scope="col"
                  >
                    <button
                      className={cn(
                        "inline-flex w-full items-center gap-1 text-left hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2",
                        meta?.numeric && "justify-end text-right",
                      )}
                      onClick={header.column.getToggleSortingHandler()}
                      type="button"
                    >
                      <span>{flexRender(header.column.columnDef.header, header.getContext())}</span>
                      <span aria-hidden="true" className="w-3 shrink-0">
                        {sorted === "asc" ? "↑" : sorted === "desc" ? "↓" : ""}
                      </span>
                    </button>
                  </th>
                )
              })}
            </tr>
          ))}
        </thead>
        <tbody>
          {playerTable.getRowModel().rows.length === 0 ? (
            <tr>
              <td className="px-4 py-8 text-center text-sm text-muted-foreground" colSpan={columns.length}>
                No players match these filters.
              </td>
            </tr>
          ) : (
            playerTable.getRowModel().rows.map((row) => (
              <tr
                className={cn(
                  "border-b border-border last:border-b-0",
                  row.original.is_taken && "bg-muted/60 text-muted-foreground",
                )}
                data-taken={row.original.is_taken ? "true" : undefined}
                key={row.id}
              >
                {row.getAllCells().map((cell) => {
                  const meta = cell.column.columnDef.meta
                  return (
                    <td
                      className={cn(
                        "whitespace-nowrap px-3 py-2",
                        meta?.numeric && "text-right tabular-nums",
                        meta?.sticky &&
                          "sticky left-0 z-10 min-w-44 bg-card",
                        meta?.sticky && row.original.is_taken && "bg-neutral-100",
                      )}
                      key={cell.id}
                    >
                      {flexRender(cell.column.columnDef.cell, cell.getContext())}
                    </td>
                  )
                })}
              </tr>
            ))
          )}
        </tbody>
      </table>
    </div>
  )
}
