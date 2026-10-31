import { useMemo, useState } from 'react'
import { Search, RefreshCw } from 'lucide-react'
import { Input, Checkbox } from './ui/form'
import { Badge } from './ui/badge'
import { Button } from './ui/button'
import { Skeleton } from './ui/skeleton'

/**
 * Pick models from a list fetched from the provider. Selected models that are no
 * longer offered stay visible (as removable badges) so nothing is dropped silently.
 */
export function ModelPicker({ available, selected, onChange, loading, error, onReload, emptyHint }: {
  available: string[]
  selected: string[]
  onChange: (models: string[]) => void
  loading?: boolean
  error?: string
  onReload?: () => void
  emptyHint?: string
}) {
  const [query, setQuery] = useState('')
  const shown = useMemo(
    () => available.filter((m) => m.toLowerCase().includes(query.trim().toLowerCase())),
    [available, query],
  )
  const toggle = (m: string, on: boolean) => onChange(on ? [...selected, m] : selected.filter((x) => x !== m))

  return (
    <div className="rounded-lg border border-border">
      <div className="flex items-center gap-2 p-2 border-b border-border">
        <Input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder={`Search ${available.length} models…`}
          leftIcon={<Search className="size-3.5" />}
          className="flex-1"
        />
        {onReload && (
          <Button type="button" variant="ghost" size="icon" aria-label="Reload models" onClick={onReload} disabled={loading}>
            <RefreshCw className={`size-4 ${loading ? 'animate-spin' : ''}`} />
          </Button>
        )}
      </div>

      <div className="max-h-56 overflow-y-auto p-2 space-y-1.5">
        {loading ? (
          [0, 1, 2, 3].map((i) => <Skeleton key={i} className="h-6" />)
        ) : error ? (
          <p className="p-2 text-xs text-rose-500">{error}</p>
        ) : shown.length ? (
          shown.map((m) => (
            <Checkbox key={m} label={m} checked={selected.includes(m)} onChange={(e) => toggle(m, e.target.checked)} />
          ))
        ) : (
          <p className="p-2 text-xs text-text-muted">{available.length ? 'No model matches.' : emptyHint ?? 'No models.'}</p>
        )}
      </div>

      <div className="flex flex-wrap items-center gap-1.5 p-2 border-t border-border min-h-11">
        {selected.length ? (
          <>
            {selected.map((m) => <Badge key={m} tone="neutral" variant="outline" onRemove={() => toggle(m, false)}>{m}</Badge>)}
            <Button type="button" variant="link" size="sm" className="ml-auto px-1" onClick={() => onChange([])}>Clear</Button>
          </>
        ) : (
          <span className="text-xs text-text-muted px-1">No model selected</span>
        )}
      </div>
    </div>
  )
}
