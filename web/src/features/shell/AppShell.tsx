import { useState, type ReactNode } from 'react'
import { LayoutDashboard, Server, Users, KeyRound, Activity, Menu } from 'lucide-react'
import { Sidebar } from '@/components/ui/sidebar'
import { Button } from '@/components/ui/button'
import { Logo } from '@/components/Logo'
import { SidebarLogo } from './SidebarLogo'
import type { Route } from '@/hooks/use-hash-route'

const SECTIONS = [
  {
    items: [
      { id: 'overview', label: 'Overview', icon: <LayoutDashboard className="size-4" /> },
      { id: 'activity', label: 'Activity', icon: <Activity className="size-4" /> },
    ],
  },
  {
    title: 'Manage',
    items: [
      { id: 'providers', label: 'Providers', icon: <Server className="size-4" /> },
      { id: 'tenants', label: 'Tenants', icon: <Users className="size-4" /> },
      { id: 'keys', label: 'API keys', icon: <KeyRound className="size-4" /> },
    ],
  },
]

export function AppShell({ route, onNavigate, children }: {
  route: Route
  onNavigate: (r: Route) => void
  children: ReactNode
}) {
  const [mobileOpen, setMobileOpen] = useState(false)

  return (
    <div className="min-h-dvh bg-background flex">
      <div className="sticky top-0 h-dvh shrink-0 max-md:w-0">
        <Sidebar
          sections={SECTIONS}
          activeId={route}
          onActiveChange={(id) => onNavigate(id as Route)}
          mobileOpen={mobileOpen}
          onMobileOpenChange={setMobileOpen}
          header={<SidebarLogo />}
          togglePlacement="header"
        />
      </div>
      <div className="flex-1 min-w-0">
        <div className="md:hidden flex items-center gap-3 px-4 h-14 border-b border-border bg-surface">
          <Button variant="ghost" size="icon" aria-label="Open menu" onClick={() => setMobileOpen(true)}>
            <Menu className="size-5" />
          </Button>
          <Logo />
        </div>
        <main className="mx-auto w-full max-w-6xl px-4 sm:px-6 lg:px-8 py-6 sm:py-8">{children}</main>
      </div>
    </div>
  )
}
