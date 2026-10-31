import { useMemo } from 'react'
import { ActivityFeed, type ActivityEvent } from '@/components/ui/activity-feed'
import { PageHeader } from '@/components/PageHeader'
import { Loader } from '@/components/Loader'
import { usePoll } from '@/hooks/use-poll'
import { api } from '@/lib/api'
import { clockTime } from '@/lib/format'
import type { OperatorEvent, RequestEntry } from '@/lib/types'

function fromRequest(r: RequestEntry, i: number): ActivityEvent {
  const failed = r.status >= 400
  return {
    id: `r-${r.time}-${i}`,
    type: failed ? 'error' : 'api',
    status: failed ? (r.status === 429 || r.status === 403 ? 'warning' : 'error') : 'success',
    title: `${r.status} · ${r.model} → ${r.channel}`,
    description: `${r.namespace}/${r.key}${r.error ? ` · ${r.error}` : ''}`,
    timestamp: clockTime(r.time),
    isoTimestamp: new Date(r.time).toISOString(),
    duration: `${r.latencyMs} ms`,
    payload: { tenant: r.group, key: r.key, model: r.model, provider: r.channel, status: r.status, inputTokens: r.inputTokens, outputTokens: r.outputTokens, latencyMs: r.latencyMs },
  }
}

function fromEvent(e: OperatorEvent, i: number): ActivityEvent {
  return {
    id: `e-${e.time}-${i}`,
    type: e.kind === 'admin' ? 'deploy' : e.level === 'error' ? 'error' : e.kind === 'key-minted' ? 'security' : 'system',
    status: e.level === 'warn' ? 'warning' : e.level,
    title: e.message,
    description: e.kind,
    timestamp: clockTime(e.time),
    isoTimestamp: new Date(e.time).toISOString(),
  }
}

/** Last 500 requests (metadata only, never prompts) plus operator events, newest first. */
export function ActivityPage() {
  const { data, error } = usePoll(api.activity, 3000)
  const events = useMemo(() => {
    if (!data) return []
    const merged = [
      ...data.requests.map((r, i) => ({ t: r.time, e: fromRequest(r, i) })),
      ...data.events.map((ev, i) => ({ t: ev.time, e: fromEvent(ev, i) })),
    ]
    return merged.sort((a, b) => b.t - a.t).map((m) => m.e)
  }, [data])

  return (
    <>
      <PageHeader
        title="Activity"
        description="Requests, admin changes and operator events on this pod. Only metadata is kept, never prompts or responses. Admin changes are also written to the pod log."
      />
      {!data ? (error ? <p className="text-sm text-rose-500">{error}</p> : <Loader />) : events.length ? (
        <ActivityFeed
          events={events}
          enableLiveSimulation={false}
          enableFilters
          enableSearch
          maxEntries={700}
          filters={[
            { id: 'api', label: 'Requests' },
            { id: 'error', label: 'Errors' },
            { id: 'deploy', label: 'Admin' },
            { id: 'security', label: 'Keys' },
            { id: 'system', label: 'Operator' },
          ]}
        />
      ) : (
        <p className="py-16 text-center text-sm text-text-muted">Nothing yet. Requests show up here as soon as a key is used.</p>
      )}
    </>
  )
}
