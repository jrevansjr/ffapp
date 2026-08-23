import {
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  ReferenceLine,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts"

import type { PlayerWeekStats } from "@/lib/types"

export type WeeklyMetric =
  | "fantasy_points_half_ppr"
  | "passing_yards"
  | "targets"
  | "rushing_attempts"

export interface WeeklySeries {
  color: string
  dataKey: WeeklyMetric
  label: string
  yAxis?: "left" | "right"
}

interface WeeklyTrendChartProps {
  average?: number | null
  data: PlayerWeekStats[]
  series: WeeklySeries[]
  title: string
}

/** WeeklyTrendChart renders one or two comparable weekly signals with tooltips. */
export default function WeeklyTrendChart({ average, data, series, title }: WeeklyTrendChartProps) {
  const hasRightAxis = series.some((item) => item.yAxis === "right")

  return (
    <section className="rounded-lg border border-border bg-card p-3" aria-label={title}>
      <h3 className="text-sm font-semibold">{title}</h3>
      <div className="mt-3 h-52 w-full">
        <ResponsiveContainer height="100%" width="100%">
          <LineChart accessibilityLayer data={data} margin={{ top: 8, right: 4, bottom: 0, left: -12 }}>
            <CartesianGrid stroke="#e5e5e5" strokeDasharray="3 3" vertical={false} />
            <XAxis dataKey="week" tickFormatter={(week) => `W${week}`} tickLine={false} />
            <YAxis allowDecimals yAxisId="left" domain={[0, "auto"]} tickLine={false} />
            {hasRightAxis && (
              <YAxis allowDecimals domain={[0, "auto"]} orientation="right" tickLine={false} yAxisId="right" />
            )}
            <Tooltip
              contentStyle={{ borderColor: "#d4d4d4", borderRadius: "0.375rem", fontSize: "0.75rem" }}
              labelFormatter={(week) => `Week ${week}`}
            />
            {series.length > 1 && <Legend iconType="line" wrapperStyle={{ fontSize: "0.75rem" }} />}
            {average !== null && average !== undefined && (
              <ReferenceLine
                label={{ value: `Avg ${average.toFixed(1)}`, fill: "#737373", fontSize: 10 }}
                stroke="#a3a3a3"
                strokeDasharray="4 4"
                y={average}
                yAxisId="left"
              />
            )}
            {series.map((item) => (
              <Line
                activeDot={{ r: 4 }}
                dataKey={item.dataKey}
                dot={{ r: 2 }}
                isAnimationActive={false}
                key={item.dataKey}
                name={item.label}
                stroke={item.color}
                strokeWidth={2}
                type="linear"
                yAxisId={item.yAxis ?? "left"}
              />
            ))}
          </LineChart>
        </ResponsiveContainer>
      </div>
    </section>
  )
}
