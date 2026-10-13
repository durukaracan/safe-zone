interface TooltipItem {
  name?: string | number
  value?: number | string | ReadonlyArray<number | string>
  color?: string
}

interface ChartTooltipBoxProps {
  active?: boolean
  label?: string | number
  payload?: ReadonlyArray<TooltipItem>
  // When true the label (axis value) is not shown, e.g. for horizontal category bars.
  hideLabel?: boolean
}

/**
 * Plain tooltip body for recharts. The one in @thyris/ui needs a Radix
 * TooltipProvider and crashes the page without it, so charts here use this.
 */
export default function ChartTooltipBox({ active, label, payload, hideLabel }: ChartTooltipBoxProps) {
  if (!active || !payload || payload.length === 0) return null

  return (
    <div className="grid min-w-[8rem] gap-1.5 rounded-lg border border-border/50 bg-background px-2.5 py-1.5 text-xs shadow-xl">
      {!hideLabel && label !== undefined && <div className="font-medium">{label}</div>}
      {payload.map((item) => (
        <div key={String(item.name)} className="flex items-center justify-between gap-4">
          <span className="flex items-center gap-1.5 text-muted-foreground">
            <span className="h-2 w-2 shrink-0 rounded-[2px]" style={{ backgroundColor: item.color }} />
            {item.name}
          </span>
          <span className="font-mono font-medium tabular-nums text-foreground">
            {typeof item.value === 'number' ? item.value.toLocaleString() : String(item.value ?? '-')}
          </span>
        </div>
      ))}
    </div>
  )
}
