import { DonutChart } from '@/components/ui/donut-chart'
import { Panel } from '@/components/PageHeader'
import { formatCount } from '@/lib/format'

export function TrafficShare({ share }: { share: Record<string, number> }) {
  const data = Object.entries(share).map(([label, value]) => ({ label, value }))

  return (
    <Panel title="Traffic by provider" description="Last 500 requests">
      {data.length ? (
        <DonutChart data={data} centerLabel="requests" formatValue={formatCount} ariaLabel="Requests per provider" />
      ) : (
        <p className="py-10 text-center text-sm text-text-muted">No requests yet.</p>
      )}
    </Panel>
  )
}
