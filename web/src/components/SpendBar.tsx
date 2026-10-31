import { formatUSD, timeUntil } from '@/lib/format'
import type { Spend } from '@/lib/types'

/** Spend against a budget: "$1.20 / $5" with a bar, or just the month's spend without one. */
export function SpendBar({ spend }: { spend?: Spend }) {
  if (!spend) return <span className="text-text-muted">-</span>
  if (spend.budget === undefined) {
    return <span className="tabular-nums text-text-secondary" title="This month, no budget">{formatUSD(spend.spent)}</span>
  }
  const used = spend.budget > 0 ? Math.min(1, spend.spent / spend.budget) : 1
  const tone = used >= 1 ? 'bg-red-500' : used >= 0.8 ? 'bg-amber-500' : 'bg-emerald-500'
  return (
    <div className="min-w-32" title={`${spend.period} budget, resets ${timeUntil(spend.resets)}. Exact: $${spend.exact}`}>
      <div className="flex items-baseline justify-between gap-2 text-xs tabular-nums">
        <span className="text-text-primary">{formatUSD(spend.spent)}</span>
        <span className="text-text-muted">/ {formatUSD(spend.budget)}</span>
      </div>
      <div className="mt-1 h-1.5 rounded-full bg-surface-hover overflow-hidden">
        <div className={`h-full rounded-full ${tone}`} style={{ width: `${Math.max(used * 100, used > 0 ? 2 : 0)}%` }} />
      </div>
    </div>
  )
}
