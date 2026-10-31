import { FormItem, FormLabel, FormControl, FormDescription, Input } from '@/components/ui/form'
import { SpringSelect } from '@/components/ui/spring-select'
import type { Budget, BudgetPeriod } from '@/lib/types'

export interface BudgetDraft {
  usd: string
  period: BudgetPeriod
  timezone: string
}

export const toBudgetDraft = (b?: Budget): BudgetDraft => ({ usd: b?.usd ?? '', period: b?.period ?? 'monthly', timezone: b?.timezone ?? '' })

/** The CRD budget, or undefined when no amount is set. */
export function fromBudgetDraft(d: BudgetDraft): Budget | undefined {
  const usd = d.usd.trim().replace(/^\$/, '')
  if (!usd) return undefined
  return { usd, period: d.period, ...(d.timezone.trim() ? { timezone: d.timezone.trim() } : {}) }
}

export const validBudgetAmount = (usd: string) => !usd.trim() || /^\$?\d+(\.\d+)?$/.test(usd.trim())

const PERIODS = [
  { value: 'daily', label: 'Per day' },
  { value: 'weekly', label: 'Per week', description: 'Resets on Monday' },
  { value: 'monthly', label: 'Per month', description: 'Resets on the 1st' },
]

/** Dollar budget inputs shared by the key and tenant sheets. */
export function BudgetFields({ value, onChange, hint }: { value: BudgetDraft; onChange: (v: BudgetDraft) => void; hint: string }) {
  return (
    <div className="space-y-3">
      <div className="grid grid-cols-2 gap-4">
        <FormItem>
          <FormLabel>Budget (USD)</FormLabel>
          <FormControl>
            <Input inputMode="decimal" value={value.usd} placeholder="no budget" onChange={(e) => onChange({ ...value, usd: e.target.value })} />
          </FormControl>
        </FormItem>
        <SpringSelect label="Period" options={PERIODS} value={value.period} onChange={(p) => onChange({ ...value, period: p as BudgetPeriod })} />
      </div>
      {value.usd.trim() && (
        <FormItem>
          <FormLabel>Time zone</FormLabel>
          <FormControl>
            <Input value={value.timezone} placeholder="UTC" onChange={(e) => onChange({ ...value, timezone: e.target.value })} />
          </FormControl>
          <FormDescription>Where the period starts, e.g. Europe/Paris. Default UTC.</FormDescription>
        </FormItem>
      )}
      <FormDescription>{hint}</FormDescription>
    </div>
  )
}
