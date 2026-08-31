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
  unit: string
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
  const leftSeries = series.find((item) => item.yAxis !== "right")
  const rightSeries = series.find((item) => item.yAxis === "right")

  return (
    <section className="rounded-lg border border-border bg-card p-3" aria-label={title}>
      <h3 className="text-sm font-semibold">{title}</h3>
      <div className="mt-1 flex flex-wrap gap-x-3 text-[11px] text-muted-foreground" aria-hidden="true">
        {leftSeries && <span style={{ color: leftSeries.color }}>Left: {leftSeries.unit}</span>}
        {rightSeries && <span style={{ color: rightSeries.color }}>Right: {rightSeries.unit}</span>}
      </div>
      <div className="mt-1 h-56 w-full">
        <ResponsiveContainer height="100%" width="100%">
          <LineChart accessibilityLayer data={data} margin={{ top: 12, right: 8, bottom: 8, left: 0 }}>
            <CartesianGrid stroke="#e5e5e5" strokeDasharray="3 3" vertical={false} />
            <XAxis
              axisLine={{ stroke: "#d4d4d4" }}
              dataKey="week"
              interval="preserveStartEnd"
              minTickGap={8}
              tick={{ fill: "#737373", fontSize: 11 }}
              tickFormatter={(week) => `W${week}`}
              tickLine={false}
            />
            <YAxis
              allowDecimals
              axisLine={false}
              domain={[0, "auto"]}
              tick={{ fill: leftSeries?.color ?? "#737373", fontSize: 11 }}
              tickLine={false}
              width={38}
              yAxisId="left"
            />
            {hasRightAxis && (
              <YAxis
                allowDecimals
                axisLine={false}
                domain={[0, "auto"]}
                orientation="right"
                tick={{ fill: rightSeries?.color ?? "#737373", fontSize: 11 }}
                tickLine={false}
                width={34}
                yAxisId="right"
              />
            )}
            <Tooltip
              contentStyle={{
                backgroundColor: "#ffffff",
                borderColor: "#d4d4d4",
                borderRadius: "0.375rem",
                boxShadow: "0 1px 3px rgb(0 0 0 / 0.1)",
                fontSize: "0.75rem",
              }}
              cursor={{ stroke: "#a3a3a3", strokeDasharray: "3 3" }}
              formatter={(value) =>
                typeof value === "number" ? value.toFixed(1) : value
              }
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
