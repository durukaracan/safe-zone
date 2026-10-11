/**
 * Shapes of the GET /dashboard/analytics response.
 * See internal/analytics/report.go - Report struct.
 *
 * The backend sends only counts and names: no request text, request ids or
 * IP addresses, so there is nothing sensitive to filter out here.
 */

/** The time windows the backend accepts (query parameter "range"). */
export type AnalyticsRange = '24h' | '7d' | '30d' | '90d'

/** Short ranges are grouped by hour, long ranges by UTC day. */
export type AnalyticsBucket = 'hour' | 'day'

/** One point of the requests-over-time series. */
export interface AnalyticsPoint {
  time: string // ISO 8601, UTC, start of the hour or day
  total: number
  blocked: number
}

/** One entry of a "top" list (PII by category, most triggered guardrails). */
export interface AnalyticsNamedCount {
  name: string
  total: number
}

export interface AnalyticsTotals {
  requests: number
  blocked: number
  allowed: number
}

export interface AnalyticsReport {
  range: AnalyticsRange
  bucket: AnalyticsBucket
  from: string // start of the first point
  to: string // end of the last point (exclusive)
  totals: AnalyticsTotals
  requests: AnalyticsPoint[] // gaps are already filled with zeros
  pii: AnalyticsNamedCount[]
  guardrails: AnalyticsNamedCount[]
}
