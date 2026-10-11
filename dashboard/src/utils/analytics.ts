import type { AnalyticsBucket, AnalyticsPoint, AnalyticsRange } from '../types/analytics'

export const ANALYTICS_RANGES: { value: AnalyticsRange; label: string }[] = [
  { value: '24h', label: 'Last 24 hours' },
  { value: '7d', label: 'Last 7 days' },
  { value: '30d', label: 'Last 30 days' },
  { value: '90d', label: 'Last 90 days' },
]

export interface ChartRow {
  time: string
  label: string
  allowed: number
  blocked: number
  total: number
}

/** Share of blocked requests as a whole percentage string; "0%" when there is no traffic. */
export function blockRate(blocked: number, total: number): string {
  if (total <= 0) return '0%'
  const pct = (blocked / total) * 100
  return `${pct >= 10 ? Math.round(pct) : Math.round(pct * 10) / 10}%`
}

/**
 * Label of one bucket. Hourly buckets are shown in the browser's time zone,
 * daily buckets are cut at UTC midnight, so they are shown in UTC.
 */
export function bucketLabel(iso: string, bucket: AnalyticsBucket): string {
  const date = new Date(iso)
  if (bucket === 'day') {
    return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric', timeZone: 'UTC' })
  }
  const day = date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
  const hour = date.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })
  return `${day} ${hour}`
}

/** Turns the API points into rows for the stacked chart (allowed = total - blocked). */
export function toChartRows(points: AnalyticsPoint[], bucket: AnalyticsBucket): ChartRow[] {
  return points.map((p) => ({
    time: p.time,
    label: bucketLabel(p.time, bucket),
    allowed: Math.max(p.total - p.blocked, 0),
    blocked: p.blocked,
    total: p.total,
  }))
}
