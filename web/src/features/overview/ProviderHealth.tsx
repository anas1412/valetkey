import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Panel } from '@/components/PageHeader'
import type { Route } from '@/hooks/use-hash-route'
import type { Overview } from '@/lib/types'

export function ProviderHealth({ channels, onNavigate }: { channels: Overview['channels']; onNavigate: (r: Route) => void }) {
  if (!channels.length) return null

  return (
    <Panel title="Providers" description="Whether each provider's API key could be read" className="mt-6">
      <ul className="divide-y divide-border">
        {channels.map((c) => (
          <li key={c.name} className="flex items-center justify-between gap-3 py-2.5">
            <div className="min-w-0">
              <div className="text-sm font-medium text-text-primary">{c.name}</div>
              <div className="text-xs text-text-muted truncate">
                {c.type === 'anthropic' ? 'Anthropic' : 'OpenAI-compatible'}
                {c.message && !c.ready ? ` · ${c.message}` : ''}
              </div>
            </div>
            {!c.enabled ? <Badge tone="neutral">Disabled</Badge>
              : c.ready ? <Badge tone="success" dot>Ready</Badge>
              : <Badge tone="danger" dot pulse>Key missing</Badge>}
          </li>
        ))}
      </ul>
      <Button variant="link" size="sm" className="mt-2 px-0" onClick={() => onNavigate('providers')}>Manage providers</Button>
    </Panel>
  )
}
