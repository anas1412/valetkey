import { useMemo } from 'react'
import { InteractiveAreaChart, type AreaChartData } from '@/components/ui/interactive-area-chart'
import { Panel } from '@/components/PageHeader'
import { formatCount } from '@/lib/format'
import type { MinuteBucket } from '@/lib/types'

const RANGES: Array<[label: string, minutes: number]> = [['1H', 60], ['6H', 360], ['24H', 1440]]

export function UsageChart({ series, stepMinutes, className }: { series: MinuteBucket[]; stepMinutes: number; className?: string }) {
  const data = useMemo<AreaChartData>(() => {
    const points = series.map((b) => ({ date: b.minute * 60_000, value: b.requests }))
    return Object.fromEntries(RANGES.map(([label, minutes]) => [label, points.slice(-Math.ceil(minutes / stepMinutes))]))
  }, [series, stepMinutes])

  return (
    <Panel className={className}>
      <InteractiveAreaChart
        data={data}
        defaultRange="6H"
        title="Requests"
        height={260}
        formatValue={(n) => `${formatCount(Math.round(n))} requests`}
        formatTick={(n) => formatCount(Math.round(n))}
        formatAxis={(ms) => new Date(ms).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })}
        formatDate={(ms) => `${new Date(ms).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })} · ${stepMinutes} min`}
      />
    </Panel>
  )
}
