import { describe, expect, it } from 'vitest'
import { blockRate, toChartRows } from './analytics'

describe('blockRate', () => {
  it('is 0% without traffic', () => {
    expect(blockRate(0, 0)).toBe('0%')
  })
  it('rounds to one decimal below 10% and to whole numbers above', () => {
    expect(blockRate(1, 30)).toBe('3.3%')
    expect(blockRate(4, 7)).toBe('57%')
    expect(blockRate(5, 5)).toBe('100%')
  })
})

describe('toChartRows', () => {
  it('splits total into allowed and blocked', () => {
    const rows = toChartRows([{ time: '2026-10-11T10:00:00Z', total: 7, blocked: 4 }], 'hour')
    expect(rows[0].allowed).toBe(3)
    expect(rows[0].blocked).toBe(4)
    expect(rows[0].total).toBe(7)
  })
  it('never returns a negative allowed count', () => {
    const rows = toChartRows([{ time: '2026-10-11T10:00:00Z', total: 1, blocked: 2 }], 'day')
    expect(rows[0].allowed).toBe(0)
  })
})
