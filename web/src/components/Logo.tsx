import { Waypoints } from 'lucide-react'

/** App mark + wordmark. The favicon (public/favicon.svg) uses the same shape. */
export function Logo({ compact = false }: { compact?: boolean }) {
  return (
    <div className="flex items-center gap-2.5">
      <div className="size-8 shrink-0 rounded-lg bg-text-primary text-background flex items-center justify-center">
        <Waypoints className="size-4.5" />
      </div>
      {!compact && (
        <div className="leading-tight">
          <div className="text-sm font-semibold text-text-primary tracking-snug-heading">llmproxy</div>
          <div className="text-[11px] text-text-muted">admin</div>
        </div>
      )}
    </div>
  )
}
