import { useState, type KeyboardEvent } from 'react'
import { Plus } from 'lucide-react'
import { Input } from './ui/form'
import { Badge } from './ui/badge'
import { Button } from './ui/button'

/** One value at a time: type, press Enter (or +), and it becomes a removable chip. */
export function ChipInput({ values, onChange, placeholder, validate }: {
  values: string[]
  onChange: (values: string[]) => void
  placeholder?: string
  /** Returns an error message, or undefined when the value is fine. */
  validate?: (value: string) => string | undefined
}) {
  const [draft, setDraft] = useState('')
  const [error, setError] = useState<string>()

  const add = () => {
    const v = draft.trim()
    if (!v) return
    const problem = validate?.(v) ?? (values.includes(v) ? 'Already added' : undefined)
    if (problem) return setError(problem)
    onChange([...values, v])
    setDraft('')
    setError(undefined)
  }

  const onKey = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      e.preventDefault()
      add()
    }
  }

  return (
    <div className="space-y-2">
      <div className="flex gap-2">
        <Input value={draft} placeholder={placeholder} error={!!error} className="flex-1"
          onChange={(e) => { setDraft(e.target.value); setError(undefined) }} onKeyDown={onKey} />
        <Button type="button" variant="secondary" size="icon" aria-label="Add" onClick={add}><Plus className="size-4" /></Button>
      </div>
      {error && <p className="text-xs text-rose-500">{error}</p>}
      {values.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {values.map((v) => <Badge key={v} tone="neutral" variant="outline" onRemove={() => onChange(values.filter((x) => x !== v))}>{v}</Badge>)}
        </div>
      )}
    </div>
  )
}

/** Basic IPv4/IPv6 address or CIDR check; the operator validates again and reports in the key status. */
export function validateCIDR(v: string): string | undefined {
  const [addr, prefix, extra] = v.split('/')
  if (extra !== undefined) return 'Use one address or range, like 10.0.0.0/8'
  const v4 = /^(\d{1,3})(\.\d{1,3}){3}$/.test(addr) && addr.split('.').every((p) => Number(p) <= 255)
  const v6 = addr.includes(':') && /^[0-9a-fA-F:.]+$/.test(addr)
  if (!v4 && !v6) return 'Not an IP address or range'
  if (prefix !== undefined) {
    const n = Number(prefix)
    if (!/^\d+$/.test(prefix) || n > (v4 ? 32 : 128)) return `Range must end in /0 to /${v4 ? 32 : 128}`
  }
  return undefined
}
