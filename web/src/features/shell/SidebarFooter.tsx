import { LogOut, ShieldCheck } from 'lucide-react'
import { useSidebar } from '@/components/ui/sidebar'

/** Account area at the bottom of the sidebar: who is signed in, and sign out. */
export function AccountRow({ onSignOut }: { onSignOut: () => void }) {
  const { collapsed } = useSidebar()
  const signOut = (
    <button
      type="button"
      onClick={onSignOut}
      aria-label="Sign out"
      title="Sign out"
      className="grid size-8 shrink-0 place-items-center rounded-lg text-neutral-500 outline-none transition-colors hover:bg-neutral-100 hover:text-neutral-900 focus-visible:ring-2 focus-visible:ring-neutral-400/50 dark:text-neutral-400 dark:hover:bg-neutral-900 dark:hover:text-white"
    >
      <LogOut className="size-4" />
    </button>
  )
  if (collapsed) return <div className="flex justify-center">{signOut}</div>

  return (
    <div className="flex items-center gap-2.5 px-1.5 py-1">
      <div className="grid size-8 shrink-0 place-items-center rounded-full bg-surface-raised text-text-secondary border border-border">
        <ShieldCheck className="size-4" />
      </div>
      <div className="min-w-0 flex-1 leading-tight">
        <div className="text-sm font-medium text-text-primary">Admin</div>
        <div className="truncate text-[11px] text-text-muted">Signed in with admin key</div>
      </div>
      {signOut}
    </div>
  )
}
