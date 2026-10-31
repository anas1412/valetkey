import { ArrowRight } from 'lucide-react'
import { Stepper } from '@/components/ui/stepper'
import { Button } from '@/components/ui/button'
import { Panel } from '@/components/PageHeader'
import type { Route } from '@/hooks/use-hash-route'
import type { Overview } from '@/lib/types'

const STEPS: Array<{ title: string; description: string; route: Route; action: string }> = [
  { title: 'Add a provider', description: 'Paste an OpenAI or Anthropic API key', route: 'providers', action: 'Add a provider' },
  { title: 'Create a tenant', description: 'Pick which providers a team may use', route: 'tenants', action: 'Create a tenant' },
  { title: 'Create an API key', description: 'Give the team a key for their app', route: 'keys', action: 'Create a key' },
]

/** First-run guide, shown until there is at least one provider, tenant and key. */
export function GettingStarted({ counts, onNavigate }: { counts: Overview['counts']; onNavigate: (r: Route) => void }) {
  const current = counts.channels === 0 ? 0 : counts.groups === 0 ? 1 : 2
  const step = STEPS[current]

  return (
    <Panel title="Get started" description="Three steps to the first proxied request." className="mb-6">
      <Stepper steps={STEPS.map(({ title }) => ({ title }))} current={current} />
      <div className="mt-5 flex flex-col sm:flex-row sm:items-center justify-between gap-3">
        <p className="text-sm text-text-secondary">{step.description}.</p>
        <Button variant="primary" rightIcon={<ArrowRight className="size-4" />} onClick={() => onNavigate(step.route)}>
          {step.action}
        </Button>
      </div>
    </Panel>
  )
}
