import { StatCard } from '@/components/ui/stat-card'
import { PageHeader } from '@/components/PageHeader'
import { usePoll } from '@/hooks/use-poll'
import { api } from '@/lib/api'
import { formatCount, timeAgo } from '@/lib/format'
import type { Route } from '@/hooks/use-hash-route'
import type { MinuteBucket } from '@/lib/types'
import { GettingStarted } from './GettingStarted'
import { UsageChart } from './UsageChart'
import { TrafficShare } from './TrafficShare'
import { ProviderHealth } from './ProviderHealth'
import { Loader } from '@/components/Loader'

/** Hourly sums for the stat-card sparklines. */
function hourly(series: MinuteBucket[], stepMinutes: number, pick: (b: MinuteBucket) => number): number[] {
  const per = Math.max(1, Math.round(60 / stepMinutes))
  const out: number[] = []
  for (let i = 0; i < series.length; i += per) out.push(series.slice(i, i + per).reduce((s, b) => s + pick(b), 0))
  return out
}

export function OverviewPage({ onNavigate }: { onNavigate: (r: Route) => void }) {
  const { data, error } = usePoll(api.overview, 5000)
  if (!data) return error ? <p className="text-sm text-rose-500">{error}</p> : <Loader />

  const { totals, series, stepMinutes, counts } = data
  const tokens = totals.inputTokens + totals.outputTokens
  const errorRate = totals.requests ? (totals.errors / totals.requests) * 100 : 0
  const labels = hourly(series, stepMinutes, () => 0).map((_, i, a) => `${a.length - i}h ago`)

  return (
    <>
      <PageHeader
        title="Overview"
        description={`Traffic in the last 24 hours. Counted since this pod started ${timeAgo(data.startedAt)}; history resets on restart.`}
      />

      {(counts.channels === 0 || counts.groups === 0 || counts.keys === 0) && (
        <GettingStarted counts={counts} onNavigate={onNavigate} />
      )}

      <div className="mb-6 grid gap-4 grid-cols-1 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard label="Requests" value={totals.requests} trend={hourly(series, stepMinutes, (b) => b.requests)}
          trendLabels={labels} period="last 24 h" format={formatCount} showDelta={false} />
        <StatCard label="Tokens" value={tokens} trend={hourly(series, stepMinutes, (b) => b.inputTokens + b.outputTokens)}
          trendLabels={labels} period={`${formatCount(totals.inputTokens)} in · ${formatCount(totals.outputTokens)} out`} format={formatCount} showDelta={false} />
        <StatCard label="Error rate" value={errorRate} goodWhen="down"
          trend={hourly(series, stepMinutes, (b) => b.errors)} trendLabels={labels}
          period={`${formatCount(totals.errors)} failed requests`} format={(n) => `${n.toFixed(1)}%`} showDelta={false} />
        <StatCard label="Active keys" value={counts.activeKeys} trend={[counts.activeKeys]}
          period={`of ${counts.keys} keys`} format={formatCount} showDelta={false} />
      </div>

      <div className="grid gap-6 lg:grid-cols-3">
        <UsageChart series={series} stepMinutes={stepMinutes} className="lg:col-span-2" />
        <TrafficShare share={data.channelShare} />
      </div>

      <ProviderHealth channels={data.channels} onNavigate={onNavigate} />
    </>
  )
}
