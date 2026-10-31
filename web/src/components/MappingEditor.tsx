import { ArrowRight, Plus, X } from 'lucide-react'
import { Input } from './ui/form'
import { Button } from './ui/button'

export type MappingRow = { from: string; to: string }

/** Rows of "requested model → model sent to the provider". */
export function MappingEditor({ rows, onChange, suggestions }: {
  rows: MappingRow[]
  onChange: (rows: MappingRow[]) => void
  /** Upstream model names offered for the right-hand field. */
  suggestions?: string[]
}) {
  const set = (i: number, patch: Partial<MappingRow>) => onChange(rows.map((r, j) => (j === i ? { ...r, ...patch } : r)))
  const listId = 'mapping-suggestions'

  return (
    <div className="space-y-2">
      {rows.map((r, i) => (
        <div key={i} className="flex items-center gap-2">
          <Input value={r.from} placeholder="gpt-4o" className="flex-1" onChange={(e) => set(i, { from: e.target.value })} aria-label="Requested model" />
          <ArrowRight className="size-4 shrink-0 text-text-muted" />
          <Input value={r.to} placeholder="my-deployment" className="flex-1" list={listId} onChange={(e) => set(i, { to: e.target.value })} aria-label="Model sent to the provider" />
          <Button type="button" variant="ghost" size="icon" aria-label="Remove mapping" onClick={() => onChange(rows.filter((_, j) => j !== i))}>
            <X className="size-4" />
          </Button>
        </div>
      ))}
      {suggestions && suggestions.length > 0 && (
        <datalist id={listId}>{suggestions.map((m) => <option key={m} value={m} />)}</datalist>
      )}
      <Button type="button" variant="secondary" size="sm" leftIcon={<Plus className="size-3.5" />} onClick={() => onChange([...rows, { from: '', to: '' }])}>
        Add mapping
      </Button>
    </div>
  )
}

export function toRows(m?: Record<string, string>): MappingRow[] {
  return Object.entries(m ?? {}).map(([from, to]) => ({ from, to }))
}

export function fromRows(rows: MappingRow[]): Record<string, string> | undefined {
  const entries = rows.map((r) => [r.from.trim(), r.to.trim()]).filter(([a, b]) => a && b)
  return entries.length ? Object.fromEntries(entries) : undefined
}
