import { useEffect, useState } from 'react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  ChartContainer,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
  Tabs,
  TabsList,
  TabsTrigger,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@thyris/ui'
import { Bar, BarChart, CartesianGrid, Legend, XAxis, YAxis } from 'recharts'
import { getAnalytics, DEFAULT_ANALYTICS_RANGE } from '../api/analytics'
import { ApiError } from '../api/client'
import type { AnalyticsNamedCount, AnalyticsRange, AnalyticsReport } from '../types/analytics'
import { ANALYTICS_RANGES, blockRate, toChartRows } from '../utils/analytics'

type LoadState = 'loading' | 'success' | 'empty' | 'error' | 'unauthorized'

const requestsConfig = {
  allowed: { label: 'Allowed', color: 'hsl(var(--chart-2))' },
  blocked: { label: 'Blocked', color: 'hsl(var(--destructive))' },
}

const countConfig = {
  total: { label: 'Detections', color: 'hsl(var(--chart-2))' },
}

function TopList({ title, description, items, emptyText }: {
  title: string
  description: string
  items: AnalyticsNamedCount[]
  emptyText: string
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent>
        {items.length === 0 ? (
          <p className="info-message">{emptyText}</p>
        ) : (
          <ChartContainer
            config={countConfig}
            className="w-full"
            style={{ height: Math.max(items.length * 36 + 24, 120) }}
          >
            <BarChart data={items} layout="vertical" margin={{ left: 8, right: 16 }}>
              <CartesianGrid horizontal={false} strokeDasharray="3 3" />
              <XAxis type="number" allowDecimals={false} tickLine={false} axisLine={false} />
              <YAxis dataKey="name" type="category" width={110} tickLine={false} axisLine={false} />
              <ChartTooltip content={<ChartTooltipContent hideLabel />} />
              <Bar dataKey="total" fill="var(--color-total)" radius={[0, 4, 4, 0]} barSize={16} />
            </BarChart>
          </ChartContainer>
        )}
      </CardContent>
    </Card>
  )
}

export default function Analytics() {
  const [range, setRange] = useState<AnalyticsRange>(DEFAULT_ANALYTICS_RANGE)
  const [report, setReport] = useState<AnalyticsReport | null>(null)
  const [loadState, setLoadState] = useState<LoadState>('loading')

  useEffect(() => {
    let cancelled = false

    async function load() {
      setLoadState('loading')
      try {
        const data = await getAnalytics(range)
        if (cancelled) return
        setReport(data)
        setLoadState(data.totals.requests === 0 ? 'empty' : 'success')
      } catch (err) {
        if (cancelled) return
        if (err instanceof ApiError && (err.status === 401 || err.status === 403)) {
          setLoadState('unauthorized')
        } else {
          setLoadState('error')
        }
      }
    }

    load()
    return () => {
      cancelled = true
    }
  }, [range])

  const rows = report ? toChartRows(report.requests, report.bucket) : []
  const hasData = loadState === 'success' || loadState === 'empty'

  return (
    <div>
      <div className="page-header">
        <h1 className="page-title">Analytics</h1>
        <p className="page-subtitle">Request volume, PII detections and guardrail activity over time</p>
      </div>

      <Tabs value={range} onValueChange={(value) => setRange(value as AnalyticsRange)} className="mb-6">
        <TabsList>
          {ANALYTICS_RANGES.map((r) => (
            <TabsTrigger key={r.value} value={r.value}>
              {r.label}
            </TabsTrigger>
          ))}
        </TabsList>
      </Tabs>

      {loadState === 'loading' && <p className="info-message">Loading analytics...</p>}

      {loadState === 'unauthorized' && (
        <p className="info-message">
          You don't have permission to view this data. Contact your administrator if you believe this is an error.
        </p>
      )}

      {loadState === 'error' && (
        <p className="info-message">
          Unable to load analytics. Please check your connection and try again.
        </p>
      )}

      {hasData && report && (
        <>
          <div className="mb-6 grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <Card>
              <CardHeader className="pb-2">
                <CardDescription>Total Requests</CardDescription>
              </CardHeader>
              <CardContent>
                <CardTitle className="font-mono text-3xl">{report.totals.requests}</CardTitle>
              </CardContent>
            </Card>
            <Card>
              <CardHeader className="pb-2">
                <CardDescription>Allowed</CardDescription>
              </CardHeader>
              <CardContent>
                <CardTitle className="font-mono text-3xl">{report.totals.allowed}</CardTitle>
              </CardContent>
            </Card>
            <Card>
              <CardHeader className="pb-2">
                <CardDescription>Blocked</CardDescription>
              </CardHeader>
              <CardContent>
                <CardTitle className="font-mono text-3xl">{report.totals.blocked}</CardTitle>
              </CardContent>
            </Card>
            <Card>
              <CardHeader className="pb-2">
                <CardDescription>Block Rate</CardDescription>
              </CardHeader>
              <CardContent>
                <CardTitle className="font-mono text-3xl">
                  {blockRate(report.totals.blocked, report.totals.requests)}
                </CardTitle>
              </CardContent>
            </Card>
          </div>

          {loadState === 'empty' ? (
            <p className="info-message">
              No requests recorded in this period yet. Data appears here as requests are processed through the gateway.
            </p>
          ) : (
            <>
              <Card className="mb-6">
                <CardHeader>
                  <CardTitle className="text-base">Requests over time</CardTitle>
                  <CardDescription>
                    {report.bucket === 'hour'
                      ? 'One bar per hour, in your local time.'
                      : 'One bar per day. Days are UTC.'}{' '}
                    The latest bar is still in progress.
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <ChartContainer config={requestsConfig} className="h-72 w-full">
                    <BarChart data={rows} margin={{ left: 0, right: 8 }}>
                      <CartesianGrid vertical={false} strokeDasharray="3 3" />
                      <XAxis dataKey="label" tickLine={false} axisLine={false} minTickGap={24} />
                      <YAxis allowDecimals={false} tickLine={false} axisLine={false} width={40} />
                      <ChartTooltip content={<ChartTooltipContent />} />
                      <Legend content={<ChartLegendContent />} />
                      <Bar dataKey="allowed" stackId="req" fill="var(--color-allowed)" stroke="hsl(var(--card))" strokeWidth={1} />
                      <Bar dataKey="blocked" stackId="req" fill="var(--color-blocked)" stroke="hsl(var(--card))" strokeWidth={1} radius={[3, 3, 0, 0]} />
                    </BarChart>
                  </ChartContainer>
                </CardContent>
              </Card>

              <div className="mb-6 grid grid-cols-1 gap-4 lg:grid-cols-2">
                <TopList
                  title="PII by category"
                  description="Detections per category (top 10)."
                  items={report.pii}
                  emptyText="No PII detected in this period."
                />
                <TopList
                  title="Most triggered guardrails"
                  description="Times a guardrail failed (top 10)."
                  items={report.guardrails}
                  emptyText="No guardrail failures in this period."
                />
              </div>

              <details className="mb-6">
                <summary className="cursor-pointer text-sm font-medium">View data as table</summary>
                <Table className="data-table mt-3">
                  <TableHeader>
                    <TableRow>
                      <TableHead scope="col">{report.bucket === 'hour' ? 'Hour' : 'Day (UTC)'}</TableHead>
                      <TableHead scope="col">Total</TableHead>
                      <TableHead scope="col">Allowed</TableHead>
                      <TableHead scope="col">Blocked</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {[...rows].reverse().map((r) => (
                      <TableRow key={r.time}>
                        <TableCell data-label="Period">{r.label}</TableCell>
                        <TableCell data-label="Total">{r.total}</TableCell>
                        <TableCell data-label="Allowed">{r.allowed}</TableCell>
                        <TableCell data-label="Blocked">{r.blocked}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </details>
            </>
          )}
        </>
      )}
    </div>
  )
}
