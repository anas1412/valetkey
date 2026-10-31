import { useSidebar } from '@/components/ui/sidebar'
import { Logo } from '@/components/Logo'

/** Full logo, or just the mark when the sidebar is collapsed to icons. */
export function SidebarLogo() {
  const { collapsed } = useSidebar()
  return <Logo compact={collapsed} />
}
