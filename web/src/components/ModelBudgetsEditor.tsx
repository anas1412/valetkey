import { Plus, X } from 'lucide-react'
import { Input } from './ui/form'
import { Button } from './ui/button'
import type { Budget, BudgetPeriod } from '@/lib/types'

export type ModelBudgetRow = { model: string; usd: string; period: BudgetPeriod }

const PERIODS: Array<[BudgetPeriod, string]> = [['daily', 'per day'], ['weekly', 'per week'], ['monthly', 'per month']]

/** Rows of "model: $ per period", for budgets on single models of a key. */
export function ModelBudgetsEditor({ rows, onChange, models }: {
  rows: ModelBudgetRow[]
  onChange: (rows: ModelBudgetRow[]) => void
  models: string[]
}) {
  const set = (i: number, patch: Partial<ModelBudgetRow>) => onChange(rows.map((r, j) => (j === i ? { ...r, ...patch } : r)))
  const listId = 'model-budget-models'

  return (
    <div className="space-y-2">
      {rows.map((r, i) => (
        <div key={i} className="flex items-center gap-2">
          <Input value={r.model} placeholder="model" className="flex-1" list={listId} onChange={(e) => set(i, { model: e.target.value })} aria-label="Model" />
          <Input value={r.usd} placeholder="USD" inputMode="decimal" className="w-20" onChange={(e) => set(i, { usd: e.target.value })} aria-label="Budget in US dollars" />
          <select value={r.period} onChange={(e) => set(i, { period: e.target.value as BudgetPeriod })} aria-label="Period"
            className="h-9 rounded-lg border border-border bg-surface px-2 text-sm text-text-primary">
            {PERIODS.map(([v, label]) => <option key={v} value={v}>{label}</option>)}
          </select>
          <Button type="button" variant="ghost" size="icon" aria-label="Remove model budget" onClick={() => onChange(rows.filter((_, j) => j !== i))}>
            <X className="size-4" />
          </Button>
        </div>
      ))}
      {models.length > 0 && <datalist id={listId}>{models.map((m) => <option key={m} value={m} />)}</datalist>}
      <Button type="button" variant="secondary" size="sm" leftIcon={<Plus className="size-3.5" />}
        onClick={() => onChange([...rows, { model: '', usd: '', period: 'daily' }])}>
        Add model budget
      </Button>
    </div>
  )
}

export const toModelBudgetRows = (m?: Record<string, Budget>): ModelBudgetRow[] =>
  Object.entries(m ?? {}).map(([model, b]) => ({ model, usd: b.usd, period: b.period ?? 'monthly' }))

export function fromModelBudgetRows(rows: ModelBudgetRow[]): Record<string, Budget> | undefined {
  const out: Record<string, Budget> = {}
  for (const r of rows) {
    if (r.model.trim() && r.usd.trim()) out[r.model.trim()] = { usd: r.usd.trim().replace(/^\$/, ''), period: r.period }
  }
  return Object.keys(out).length ? out : undefined
}
