import { request } from './client'
import type { AnalyticsRange, AnalyticsReport } from '../types/analytics'

/** Default window, the same one the backend uses when no range is sent. */
export const DEFAULT_ANALYTICS_RANGE: AnalyticsRange = '7d'

/**
 * Hourly and daily request counts, PII detections by category and the most
 * triggered guardrails for the given window.
 */
export async function getAnalytics(range: AnalyticsRange = DEFAULT_ANALYTICS_RANGE): Promise<AnalyticsReport> {
  return request<AnalyticsReport>(`/dashboard/analytics?range=${encodeURIComponent(range)}`)
}
