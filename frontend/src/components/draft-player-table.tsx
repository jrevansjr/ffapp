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
import type { KeyboardEvent } from "react"

import type { PlayerListItem } from "@/lib/types"
import { cn } from "@/lib/utils"

interface DraftColumnMeta {
  numeric?: boolean
  sticky?: boolean
}

const draftTableFeatures = tableFeatures({
  rowSortingFeature,
  sortedRowModel: createSortedRowModel(),
  sortFns: { basic: sortFn_basic, text: sortFn_text },
  columnMeta: {} as DraftColumnMeta,
})

const columnHelper = createColumnHelper<typeof draftTableFeatures, PlayerListItem>()

function formatADP(value: number | undefined): string {
  return value === undefined ? "—" : value.toFixed(1)
}

function formatDepth(player: PlayerListItem): string {
  if (player.depth_chart_position !== null && player.depth_chart_order !== null) {
    return `${player.depth_chart_position} ${player.depth_chart_order}`
  }
  if (player.depth_chart_position !== null) return player.depth_chart_position
  if (player.depth_chart_order !== null) return `Order ${player.depth_chart_order}`
  return "—"
}

const columns = columnHelper.columns([
  columnHelper.accessor((player) => `${player.first_name} ${player.last_name}`, {
    id: "name",
    header: "Name",
    sortFn: "text",
    sortDescFirst: false,
    meta: { sticky: true },
  }),
  columnHelper.accessor("position", {
    header: "Pos",
    sortFn: "text",
    sortDescFirst: false,
  }),
  columnHelper.accessor((player) => player.nfl_team?.abbreviation, {
    id: "team",
    header: "Team",
    sortFn: "text",
    sortDescFirst: false,
    sortUndefined: "last",
    cell: ({ getValue }) => getValue() ?? "—",
  }),
  columnHelper.accessor((player) => player.depth_chart_order ?? undefined, {
    id: "depth",
    header: "Depth",
    sortFn: (left, right) => {
      const orderDifference =
        (left.original.depth_chart_order ?? Number.MAX_SAFE_INTEGER) -
        (right.original.depth_chart_order ?? Number.MAX_SAFE_INTEGER)
      if (orderDifference !== 0) return orderDifference
      return (left.original.depth_chart_position ?? "").localeCompare(
        right.original.depth_chart_position ?? "",
      )
    },
    sortDescFirst: false,
    sortUndefined: "last",
    meta: { numeric: true },
    cell: ({ row }) => formatDepth(row.original),
  }),
  columnHelper.accessor((player) => player.draft.tier ?? undefined, {
    id: "tier",
    header: "Tier",
    sortFn: "basic",
    sortDescFirst: false,
    sortUndefined: "last",
    meta: { numeric: true },
    cell: ({ getValue }) => getValue() ?? "—",
  }),
  columnHelper.accessor((player) => player.draft.ecr ?? undefined, {
    id: "ecr",
    header: "ECR",
    sortFn: "basic",
    sortDescFirst: false,
    sortUndefined: "last",
    meta: { numeric: true },
    cell: ({ getValue }) => getValue() ?? "—",
  }),
  columnHelper.accessor((player) => player.draft.position_rank ?? undefined, {
    id: "positionRank",
    header: "Pos Rank",
    sortFn: "basic",
    sortDescFirst: false,
    sortUndefined: "last",
    meta: { numeric: true },
    cell: ({ getValue, row }) =>
      getValue() === undefined ? "—" : `${row.original.position}${getValue()}`,
  }),
  columnHelper.accessor((player) => player.draft.aggregate_adp ?? undefined, {
    id: "fantasyProsADP",
    header: "Aggregate ADP",
    sortFn: "basic",
    sortDescFirst: false,
    sortUndefined: "last",
    meta: { numeric: true },
    cell: ({ getValue }) => formatADP(getValue()),
  }),
])

function ariaSort(direction: false | SortDirection): "none" | "ascending" | "descending" {
  if (direction === "asc") return "ascending"
  if (direction === "desc") return "descending"
  return "none"
}

interface DraftPlayerTableProps {
  emptyMessage: string
  onSelectPlayer: (playerID: number) => void
  players: PlayerListItem[]
  selectedPlayerID: number | null
}

/** DraftPlayerTable owns sorting and accessible row selection for available players. */
export default function DraftPlayerTable({
  emptyMessage,
  onSelectPlayer,
  players,
  selectedPlayerID,
}: DraftPlayerTableProps) {
  const playerTable = useTable({
    features: draftTableFeatures,
    columns,
    data: players,
    getRowId: (player) => String(player.id),
    enableMultiSort: false,
    enableSortingRemoval: false,
    initialState: { sorting: [{ id: "fantasyProsADP", desc: false }] },
  })

  function selectWithKeyboard(event: KeyboardEvent<HTMLTableRowElement>, playerID: number) {
    if (event.key !== "Enter" && event.key !== " ") return
    event.preventDefault()
    onSelectPlayer(playerID)
  }

  return (
    <div className="max-h-[calc(100vh-20rem)] min-h-80 overflow-auto rounded-lg border border-border bg-card">
      <table className="w-full min-w-[760px] border-collapse text-left text-xs">
        <caption className="sr-only">
          Available fantasy football players. Select a row to open its player inspector.
        </caption>
        <thead className="sticky top-0 z-10 bg-muted text-muted-foreground">
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
                      meta?.sticky && "sticky left-0 z-20 min-w-40 bg-muted shadow-[1px_0_0_0_var(--border)]",
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
              <td className="px-4 py-10 text-center text-sm text-muted-foreground" colSpan={columns.length}>
                {emptyMessage}
              </td>
            </tr>
          ) : (
            playerTable.getRowModel().rows.map((row) => {
              const selected = row.original.id === selectedPlayerID
              return (
                <tr
                  aria-selected={selected}
                  className={cn(
                    "cursor-pointer border-b border-border outline-none last:border-b-0 hover:bg-muted/70 focus-visible:bg-muted focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-neutral-400",
                    selected && "bg-neutral-200 hover:bg-neutral-200 focus-visible:bg-neutral-200",
                  )}
                  key={row.id}
                  onClick={() => onSelectPlayer(row.original.id)}
                  onKeyDown={(event) => selectWithKeyboard(event, row.original.id)}
                  tabIndex={0}
                >
                  {row.getAllCells().map((cell) => (
                    <td
                      className={cn(
                        "whitespace-nowrap px-3 py-2.5",
                        cell.column.columnDef.meta?.numeric && "text-right tabular-nums",
                        cell.column.columnDef.meta?.sticky &&
                          "sticky left-0 z-[1] min-w-40 bg-card font-medium shadow-[1px_0_0_0_var(--border)]",
                        cell.column.columnDef.meta?.sticky && selected && "bg-neutral-200",
                      )}
                      key={cell.id}
                    >
                      {flexRender(cell.column.columnDef.cell, cell.getContext())}
                    </td>
                  ))}
                </tr>
              )
            })
          )}
        </tbody>
      </table>
    </div>
  )
}
