import { useMemo } from 'react'
import { InteractiveAreaChart, type AreaChartData } from '@/components/ui/interactive-area-chart'
import { Panel } from '@/components/PageHeader'
import { formatCount } from '@/lib/format'
import type { HistoryRange, MinuteBucket } from '@/lib/types'

// Within a 24 h series the chart offers zoom levels; 7 and 30 days are shown whole.
const DAY_ZOOM: Array<[label: string, minutes: number]> = [['1H', 60], ['6H', 360], ['24H', 1440]]

const time = (ms: number) => new Date(ms).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })
const day = (ms: number) => new Date(ms).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })

export function UsageChart({ series, stepMinutes, range, className }: {
  series: MinuteBucket[]
  stepMinutes: number
  range: HistoryRange
  className?: string
}) {
  const daily = range === '24h'
  const data = useMemo<AreaChartData>(() => {
    const points = series.map((b) => ({ date: b.minute * 60_000, value: b.requests }))
    if (!daily) return { [range.toUpperCase()]: points }
    return Object.fromEntries(DAY_ZOOM.map(([label, minutes]) => [label, points.slice(-Math.ceil(minutes / stepMinutes))]))
  }, [series, stepMinutes, range, daily])

  const step = stepMinutes >= 60 ? `${stepMinutes / 60} h` : `${stepMinutes} min`

  return (
    <Panel className={className}>
      <InteractiveAreaChart
        key={range}
        data={data}
        defaultRange={daily ? '6H' : range.toUpperCase()}
        title="Requests"
        height={260}
        formatValue={(n) => `${formatCount(Math.round(n))} requests`}
        formatTick={(n) => formatCount(Math.round(n))}
        formatAxis={daily ? time : day}
        formatDate={(ms) => `${daily ? time(ms) : `${day(ms)}, ${time(ms)}`} · ${step}`}
      />
    </Panel>
  )
}
