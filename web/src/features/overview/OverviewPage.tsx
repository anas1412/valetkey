import { useEffect, useState } from 'react'
import { StatCard } from '@/components/ui/stat-card'
import { AnimatedTabs } from '@/components/ui/animated-tabs'
import { Badge } from '@/components/ui/badge'
import { PageHeader } from '@/components/PageHeader'
import { usePoll } from '@/hooks/use-poll'
import { api } from '@/lib/api'
import { formatUSD, formatCount, timeAgo } from '@/lib/format'
import type { Route } from '@/hooks/use-hash-route'
import type { HistoryRange, MinuteBucket } from '@/lib/types'
import { GettingStarted } from './GettingStarted'
import { UsageChart } from './UsageChart'
import { TrafficShare } from './TrafficShare'
import { ProviderHealth } from './ProviderHealth'
import { Loader } from '@/components/Loader'

const GUIDE = 'https://anas.cyberia.tn/valetkey/guide/usage-history'
const RANGES: Array<{ id: HistoryRange; label: string }> = [
  { id: '24h', label: '24 hours' },
  { id: '7d', label: '7 days' },
  { id: '30d', label: '30 days' },
]

/** Sparkline points: buckets merged to at least one hour each, with a label per point. */
function sparkline(series: MinuteBucket[], stepMinutes: number, pick: (b: MinuteBucket) => number) {
  const per = Math.max(1, Math.round(60 / stepMinutes))
  const values: number[] = []
  const labels: string[] = []
  for (let i = 0; i < series.length; i += per) {
    const slice = series.slice(i, i + per)
    values.push(slice.reduce((s, b) => s + pick(b), 0))
    labels.push(new Date(slice[0].minute * 60_000).toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' }))
  }
  return { values, labels }
}

export function OverviewPage({ onNavigate }: { onNavigate: (r: Route) => void }) {
  const [range, setRange] = useState<HistoryRange>('24h')
  const { data, error, reload } = usePoll(() => api.overview(range), 15_000)
  useEffect(() => void reload(), [range, reload])
  if (!data) return error ? <p className="text-sm text-rose-500">{error}</p> : <Loader />

  const { totals, series, stepMinutes, counts } = data
  const fromProm = data.source === 'prometheus'
  const tokens = totals.inputTokens + totals.outputTokens
  const errorRate = totals.requests ? (totals.errors / totals.requests) * 100 : 0
  const period = fromProm ? RANGES.find((r) => r.id === data.range)!.label.replace(/^/, 'last ') : 'since pod start'
  const spark = (pick: (b: MinuteBucket) => number) => sparkline(series, stepMinutes, pick)
  const requestsSpark = spark((b) => b.requests)

  return (
    <>
      <PageHeader
        title="Overview"
        description={fromProm
          ? <>Usage from Prometheus, kept across pod restarts. <Badge tone="success" dot>Prometheus</Badge></>
          : <>Traffic since this pod started {timeAgo(data.startedAt)}; it resets on restart.{' '}
              <a href={GUIDE} target="_blank" rel="noreferrer" className="underline underline-offset-2 hover:text-text-primary">Keep history with Prometheus</a></>}
        actions={fromProm ? <AnimatedTabs tabs={RANGES} activeTab={range} onChange={(id) => setRange(id as HistoryRange)} renderContent={false} variant="pill" /> : undefined}
      />

      {data.warning && (
        <div className="mb-6 rounded-lg border border-amber-500/30 bg-amber-500/10 px-4 py-3 text-sm text-text-secondary">
          {data.warning}
        </div>
      )}

      {(counts.channels === 0 || counts.groups === 0 || counts.keys === 0) && (
        <GettingStarted counts={counts} onNavigate={onNavigate} />
      )}

      <div className="mb-6 grid gap-4 grid-cols-1 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard label="Requests" value={totals.requests} trend={requestsSpark.values}
          trendLabels={requestsSpark.labels} period={period} format={formatCount} showDelta={false} />
        <StatCard label="Tokens" value={tokens} trend={spark((b) => b.inputTokens + b.outputTokens).values}
          trendLabels={requestsSpark.labels} period={`${formatCount(totals.inputTokens)} in · ${formatCount(totals.outputTokens)} out`} format={formatCount} showDelta={false} />
        <StatCard label="Error rate" value={errorRate} goodWhen="down"
          trend={spark((b) => b.errors).values} trendLabels={requestsSpark.labels}
          period={`${formatCount(totals.errors)} failed requests`} format={(n) => `${n.toFixed(1)}%`} showDelta={false} />
        <StatCard label="Spend" value={data.cost} trend={spark((b) => b.cost).values} trendLabels={requestsSpark.labels}
          period={`${period}, ${counts.activeKeys} of ${counts.keys} keys active`} format={formatUSD} showDelta={false} />
      </div>

      <div className="grid gap-6 lg:grid-cols-3">
        <UsageChart series={series} stepMinutes={stepMinutes} range={data.range} className="lg:col-span-2" />
        <TrafficShare share={data.channelShare} subtitle={fromProm ? period : 'Last 500 requests'} />
      </div>

      <ProviderHealth channels={data.channels} onNavigate={onNavigate} />
    </>
  )
}
