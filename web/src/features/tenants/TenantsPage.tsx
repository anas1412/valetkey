import { useState } from 'react'
import { Plus, Trash2, FileCode } from 'lucide-react'
import { AdvancedDataTable, type ColumnDef } from '@/components/ui/advanced-data-table'
import { SplitButton } from '@/components/ui/split-button'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { PageHeader } from '@/components/PageHeader'
import { ConfirmSheet } from '@/components/ConfirmSheet'
import { YamlSheet, type YamlTarget } from '@/components/YamlSheet'
import { SpendBar } from '@/components/SpendBar'
import { usePoll } from '@/hooks/use-poll'
import { api } from '@/lib/api'
import { useToast } from '@/lib/toast'
import type { Group } from '@/lib/types'
import { TenantSheet } from './TenantSheet'

export function TenantsPage() {
  const groups = usePoll(api.groups, 10_000)
  const channels = usePoll(api.channels, 30_000)
  const [editing, setEditing] = useState<Group | 'new'>()
  const [deleting, setDeleting] = useState<Group>()
  const [yaml, setYaml] = useState<YamlTarget>()
  const notify = useToast()

  const columns: ColumnDef<Group>[] = [
    { id: 'name', header: 'Tenant', accessorKey: 'name', sortable: true, priority: 'high',
      cell: ({ value }) => <span className="font-medium text-text-primary">{value}</span> },
    {
      id: 'channels', header: 'Providers', accessorFn: (g) => g.spec.channelRefs.join(', '), priority: 'medium',
      cell: ({ row }) => (
        <div className="flex flex-wrap gap-1">
          {row.spec.channelRefs.map((c) => <Badge key={c} tone="neutral" variant="outline">{c}</Badge>)}
        </div>
      ),
    },
    { id: 'keys', header: 'Keys', accessorKey: 'keys', sortable: true, align: 'right', priority: 'medium' },
    { id: 'spend', header: 'Spent', accessorFn: (g) => g.spend?.spent ?? 0, sortable: true, priority: 'medium',
      cell: ({ row }) => <SpendBar spend={row.spend} /> },
    {
      id: 'limits', header: 'Limit ×', priority: 'low',
      accessorFn: (g) => `${g.spec.rpmRatio ?? 1} / ${g.spec.tpmRatio ?? 1}`,
      cell: ({ value }) => <span className="text-text-secondary tabular-nums" title="requests × / tokens ×">{value}</span>,
    },
    {
      id: 'status', header: 'Status', priority: 'high', filterable: true,
      accessorFn: (g) => ((g.spec.status ?? 'Enabled') !== 'Enabled' ? 'disabled' : g.ready ? 'ready' : 'not-ready'),
      filterOptions: [{ label: 'Ready', value: 'ready' }, { label: 'Not ready', value: 'not-ready' }, { label: 'Disabled', value: 'disabled' }],
      cell: ({ value, row }) => value === 'ready' ? <Badge tone="success" dot>Ready</Badge>
        : value === 'disabled' ? <Badge tone="neutral">Disabled</Badge>
        : <Badge tone="warning" dot title={row.message}>{row.message ?? 'Not ready'}</Badge>,
    },
    {
      id: 'actions', header: '', align: 'right', priority: 'high',
      cell: ({ row }) => (
        <SplitButton size="sm" variant="secondary" label="Edit" onClick={() => setEditing(row)}
          options={[
            { value: 'yaml', label: 'View YAML', icon: <FileCode className="size-3.5" />, onSelect: () => setYaml({ resource: 'groups', path: [row.name], title: `Tenant ${row.name}` }) },
            { value: 'delete', label: 'Delete', icon: <Trash2 className="size-3.5" />, onSelect: () => setDeleting(row) },
          ]} />
      ),
    },
  ]

  return (
    <>
      <PageHeader
        title="Tenants"
        description="One tenant per namespace. Choose which providers its keys may use, scale its rate limits and set a budget."
        actions={<Button variant="primary" leftIcon={<Plus className="size-4" />} onClick={() => setEditing('new')}>Create tenant</Button>}
      />
      <AdvancedDataTable
        data={groups.data ?? []}
        columns={columns}
        getRowId={(g) => g.name}
        isLoading={groups.loading}
        error={groups.error}
        searchPlaceholder="Search tenants…"
      />
      {groups.data?.length === 0 && (
        <p className="mt-4 text-sm text-text-muted text-center">No tenants yet. Create one for each team or app namespace.</p>
      )}

      <YamlSheet target={yaml} onClose={() => setYaml(undefined)} />
      <TenantSheet
        open={!!editing}
        group={editing === 'new' ? undefined : editing}
        channels={channels.data ?? []}
        onClose={() => setEditing(undefined)}
        onSaved={groups.reload}
      />
      <ConfirmSheet
        open={!!deleting}
        title={`Delete tenant ${deleting?.name}?`}
        description="Every API key in this namespace stops working until a tenant with the same name exists again. The keys themselves are kept."
        onClose={() => setDeleting(undefined)}
        onConfirm={async () => {
          await api.deleteGroup(deleting!.name)
          notify('Tenant deleted', { description: deleting!.name })
          void groups.reload()
        }}
      />
    </>
  )
}
