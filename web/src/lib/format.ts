const compact = new Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 1 })

/** Whole numbers, compact above 1000. Rounds, because stat cards animate through fractional values. */
export function formatCount(n: number): string {
  return n < 1000 ? String(Math.round(n)) : compact.format(n)
}

export function formatPercent(part: number, total: number): string {
  if (!total) return '0%'
  return `${((part / total) * 100).toFixed(part / total < 0.1 ? 1 : 0)}%`
}

export function timeAgo(ms: number | undefined): string {
  if (!ms) return 'never'
  const s = Math.round((Date.now() - ms) / 1000)
  if (s < 60) return `${s}s ago`
  if (s < 3600) return `${Math.floor(s / 60)}m ago`
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`
  return `${Math.floor(s / 86400)}d ago`
}

export function clockTime(ms: number): string {
  return new Date(ms).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

/** Splits a comma/newline separated text field into trimmed, non-empty values. */
export function splitList(text: string): string[] {
  return text.split(/[\n,]/).map((s) => s.trim()).filter(Boolean)
}
