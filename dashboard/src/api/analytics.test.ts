import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { getAnalytics } from './analytics'
import { ApiError } from './client'

const mockReport = {
  range: '24h',
  bucket: 'hour',
  from: '2026-10-10T16:00:00Z',
  to: '2026-10-11T16:00:00Z',
  totals: { requests: 7, blocked: 4, allowed: 3 },
  requests: [{ time: '2026-10-10T16:00:00Z', total: 7, blocked: 4 }],
  pii: [{ name: 'EMAIL', total: 2 }],
  guardrails: [{ name: 'NUMERIC', total: 2 }],
}

describe('analytics API', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn())
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('getAnalytics requests /dashboard/analytics with the chosen range and returns the report', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(new Response(JSON.stringify(mockReport), { status: 200 }))

    const result = await getAnalytics('24h')

    expect(result).toEqual(mockReport)
    expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining('/dashboard/analytics?range=24h'),
      expect.anything(),
    )
  })

  it('getAnalytics uses the 7d window when no range is given', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(new Response(JSON.stringify(mockReport), { status: 200 }))

    await getAnalytics()

    expect(fetch).toHaveBeenCalledWith(
      expect.stringContaining('/dashboard/analytics?range=7d'),
      expect.anything(),
    )
  })

  it('getAnalytics throws an ApiError with the status when the session is missing', async () => {
    vi.mocked(fetch).mockResolvedValueOnce(new Response('Not authenticated', { status: 401 }))

    await expect(getAnalytics('30d')).rejects.toBeInstanceOf(ApiError)
    vi.mocked(fetch).mockResolvedValueOnce(new Response('Not authenticated', { status: 401 }))
    await expect(getAnalytics('30d')).rejects.toMatchObject({ status: 401 })
  })
})
