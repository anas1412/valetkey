import { AdvancedDataTable, type ColumnDef } from '@/components/ui/advanced-data-table'
import { Badge } from '@/components/ui/badge'
import { PageHeader, Panel } from '@/components/PageHeader'
import { usePoll } from '@/hooks/use-poll'
import { api } from '@/lib/api'
import { timeAgo, timeUntil } from '@/lib/format'
import type { ModelPrice } from '@/lib/types'

const DOCS = 'https://anas.cyberia.tn/valetkey/guide/budgets'

const perM = (n: number) => {
  const v = Math.round(n * 1e6) / 1e6 // float noise from the API's per-million conversion
  return `$${v >= 0.1 || v === 0 ? v.toFixed(2) : String(v)}`
}

/** What each provider model costs right now, and where the prices come from. */
export function PricingPage() {
  const { data, loading, error } = usePoll(api.pricing, 60_000)

  const columns: ColumnDef<ModelPrice & { id: string }>[] = [
    { id: 'channel', header: 'Provider', accessorKey: 'channel', sortable: true, filterable: true, priority: 'high',
      filterOptions: [...new Set((data?.prices ?? []).map((p) => p.channel))].map((c) => ({ label: c, value: c })),
      cell: ({ row }) => (
        <div>
          <div className="font-medium text-text-primary">{row.channel}</div>
          <div className="text-xs text-text-muted">{row.provider ? `priced as ${row.provider}` : 'provider unknown'}</div>
        </div>
      ) },
    { id: 'model', header: 'Model', accessorKey: 'model', sortable: true, priority: 'high' },
    { id: 'input', header: 'Input', accessorKey: 'input', sortable: true, align: 'right', priority: 'high',
      cell: ({ row }) => row.known ? <span className="tabular-nums">{perM(row.input)}</span> : <span className="text-text-muted">-</span> },
    { id: 'output', header: 'Output', accessorKey: 'output', sortable: true, align: 'right', priority: 'high',
      cell: ({ row }) => row.known ? <span className="tabular-nums">{perM(row.output)}</span> : <span className="text-text-muted">-</span> },
    { id: 'cacheRead', header: 'Cache read', accessorKey: 'cacheRead', align: 'right', priority: 'low',
      cell: ({ row }) => row.known ? <span className="tabular-nums text-text-secondary">{perM(row.cacheRead)}</span> : null },
    { id: 'cacheWrite', header: 'Cache write', accessorKey: 'cacheWrite5m', align: 'right', priority: 'low',
      cell: ({ row }) => row.known ? (
        <span className="tabular-nums text-text-secondary" title="5 minute / 1 hour cache">
          {row.cacheWrite5m === row.cacheWrite1h ? perM(row.cacheWrite5m) : `${perM(row.cacheWrite5m)} / ${perM(row.cacheWrite1h)}`}
        </span>
      ) : null },
    { id: 'period', header: 'Now', accessorFn: (p) => p.period ?? '', priority: 'medium',
      cell: ({ row }) => !row.period ? <span className="text-text-muted">flat</span> : (
        <div>
          {row.period === 'peak' ? <Badge tone="warning" dot>Peak</Badge> : <Badge tone="success" dot>Off-peak</Badge>}
          {row.nextChange && (
            <div className="mt-1 text-xs text-text-muted">
              {row.period === 'peak' ? 'off-peak' : 'peak'} {timeUntil(row.nextChange)}
            </div>
          )}
        </div>
      ) },
    { id: 'source', header: 'Source', accessorFn: (p) => p.source ?? '', priority: 'low',
      cell: ({ row }) => !row.known
        ? <Badge tone="warning" dot title="Requests with a budget are refused until it has a price. Set spec.pricing on the channel.">No price</Badge>
        : row.warning ? <Badge tone="warning" title={row.warning}>{row.source}, check</Badge>
        : <span className="text-text-secondary">{row.source}</span> },
  ]

  return (
    <>
      <PageHeader
        title="Pricing"
        description={<>Prices per million tokens at this moment, used for costs and budgets.{' '}
          <a href={DOCS} target="_blank" rel="noreferrer" className="underline underline-offset-2 hover:text-text-primary">How costs are worked out</a></>}
      />
      {data && (
        <Panel className="mb-6" title="Price sources"
          description={data.fetchedAt
            ? `Fetched ${timeAgo(Date.parse(data.fetchedAt))}${data.refresh === 'off' ? ', live fetching is off' : `, refreshed every ${data.refresh}`}`
            : 'Built-in prices; live fetching has not run yet'}>
          <ul className="divide-y divide-border">
            {data.sources.map((s) => (
              <li key={s.name} className="flex items-start justify-between gap-4 py-2.5 text-sm">
                <div className="min-w-0">
                  <a href={s.url} target="_blank" rel="noreferrer" className="font-medium text-text-primary hover:underline">{s.name}</a>
                  <div className="text-xs text-text-muted break-words">{s.error ? `${s.error}. ${s.note ?? ''}` : s.note}</div>
                </div>
                {s.ok ? <Badge tone="success" dot>OK</Badge> : <Badge tone="warning" dot>Failed</Badge>}
              </li>
            ))}
          </ul>
        </Panel>
      )}
      <AdvancedDataTable
        data={(data?.prices ?? []).map((p) => ({ ...p, id: `${p.channel}/${p.model}` }))}
        columns={columns}
        getRowId={(r) => r.id}
        isLoading={loading}
        error={error}
        searchPlaceholder="Search models…"
      />
    </>
  )
}
