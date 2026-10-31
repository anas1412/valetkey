import { useState } from 'react'
import { Plus, Pencil, Trash2, RefreshCw, Power } from 'lucide-react'
import { AdvancedDataTable, type ColumnDef } from '@/components/ui/advanced-data-table'
import { SplitButton } from '@/components/ui/split-button'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { PageHeader } from '@/components/PageHeader'
import { ConfirmSheet } from '@/components/ConfirmSheet'
import { SpendBar } from '@/components/SpendBar'
import { usePoll } from '@/hooks/use-poll'
import { api } from '@/lib/api'
import { copyToClipboard } from '@/lib/utils'
import { useToast } from '@/lib/toast'
import { formatCount, timeAgo, timeUntil } from '@/lib/format'
import type { ProxyKey } from '@/lib/types'
import { KeySheet } from './KeySheet'
import { KeyDetails } from './KeyDetails'

type Row = ProxyKey & { id: string; state: string }

function stateOf(k: ProxyKey): string {
  if ((k.spec.status ?? 'Enabled') !== 'Enabled') return 'disabled'
  return k.ready ? 'active' : 'not-ready'
}

export function KeysPage() {
  const keys = usePoll(api.keys, 5000)
  const groups = usePoll(api.groups, 30_000)
  const overview = usePoll(api.overview, 60_000)
  const [editing, setEditing] = useState<ProxyKey | 'new'>()
  const [confirm, setConfirm] = useState<{ key: ProxyKey; action: 'delete' | 'rotate' }>()
  const notify = useToast()

  const copy = async (k: ProxyKey) => {
    try {
      const { key } = await api.revealKey(k.namespace, k.name)
      notify(await copyToClipboard(key) ? 'Key copied' : 'Copy failed', { description: `${k.namespace}/${k.name}` })
    } catch (err) {
      notify('Could not read the key', { description: (err as Error).message, variant: 'error' })
    }
  }

  const toggle = async (k: ProxyKey) => {
    const enable = (k.spec.status ?? 'Enabled') !== 'Enabled'
    try {
      await api.updateKey(k.namespace, k.name, { ...k.spec, status: enable ? 'Enabled' : 'Disabled' })
      notify(enable ? 'Key enabled' : 'Key disabled', { description: `${k.namespace}/${k.name}` })
      void keys.reload()
    } catch (err) {
      notify('Update failed', { description: (err as Error).message, variant: 'error' })
    }
  }

  const columns: ColumnDef<Row>[] = [
    {
      id: 'name', header: 'Key', accessorKey: 'name', sortable: true, priority: 'high',
      cell: ({ row }) => (
        <div>
          <div className="font-medium text-text-primary">{row.name}</div>
          <div className="text-xs text-text-muted">{row.namespace}</div>
        </div>
      ),
    },
    {
      id: 'namespace', header: 'Tenant', accessorKey: 'namespace', sortable: true, filterable: true, priority: 'low', defaultHidden: true,
      filterOptions: (groups.data ?? []).map((g) => ({ label: g.name, value: g.name })),
    },
    {
      id: 'state', header: 'Status', accessorKey: 'state', filterable: true, priority: 'high',
      filterOptions: [{ label: 'Active', value: 'active' }, { label: 'Not ready', value: 'not-ready' }, { label: 'Disabled', value: 'disabled' }],
      cell: ({ row }) => row.state === 'active' ? <Badge tone="success" dot>Active</Badge>
        : row.state === 'disabled' ? <Badge tone="neutral">Disabled</Badge>
        : <Badge tone="warning" dot title={row.message}>{row.message ?? 'Pending'}</Badge>,
    },
    {
      id: 'limits', header: 'Limits', priority: 'low',
      accessorFn: (k) => [k.spec.models?.length ? `${k.spec.models.length} model(s)` : '', k.spec.subnets?.length ? 'IP list' : '',
        k.spec.rpm ? `${k.spec.rpm} rpm` : '', k.spec.tpm ? `${formatCount(k.spec.tpm)} tpm` : '',
        k.spec.maxTokens ? `max ${formatCount(k.spec.maxTokens)} tokens` : '',
        k.spec.modelBudgets ? `${Object.keys(k.spec.modelBudgets).length} model budget(s)` : '',
        k.spec.expiresAt ? (Date.parse(k.spec.expiresAt) > Date.now() ? `expires ${timeUntil(k.spec.expiresAt)}` : 'expired') : ''].filter(Boolean).join(', ') || 'none',
      cell: ({ value }) => <span className="text-text-secondary">{value}</span>,
    },
    { id: 'requests', header: keys.data?.[0]?.requestsWindow === '30d' ? 'Requests (30d)' : 'Requests', accessorKey: 'requests', sortable: true, align: 'right', priority: 'medium',
      cell: ({ value }) => <span className="tabular-nums">{formatCount(value)}</span> },
    { id: 'spend', header: 'Spent', accessorFn: (k) => k.spend?.spent ?? 0, sortable: true, priority: 'medium',
      cell: ({ row }) => <SpendBar spend={row.spend} /> },
    { id: 'lastUsed', header: 'Last used', accessorFn: (k) => k.lastUsed ?? 0, sortable: true, priority: 'low',
      cell: ({ row }) => <span className="text-text-secondary">{timeAgo(row.lastUsed)}</span> },
    {
      id: 'actions', header: '', align: 'right', priority: 'high',
      cell: ({ row }) => (
        <SplitButton
          size="sm"
          variant="secondary"
          label="Copy key"
          disabled={!row.ready}
          onClick={() => void copy(row)}
          options={[
            { value: 'toggle', label: row.state === 'disabled' ? 'Enable' : 'Disable', icon: <Power className="size-3.5" />, onSelect: () => void toggle(row) },
            { value: 'rotate', label: 'Rotate key', icon: <RefreshCw className="size-3.5" />, onSelect: () => setConfirm({ key: row, action: 'rotate' }) },
            { value: 'edit', label: 'Edit', icon: <Pencil className="size-3.5" />, onSelect: () => setEditing(row) },
            { value: 'delete', label: 'Delete', icon: <Trash2 className="size-3.5" />, onSelect: () => setConfirm({ key: row, action: 'delete' }) },
          ]}
        />
      ),
    },
  ]

  const c = confirm?.key
  return (
    <>
      <PageHeader
        title="API keys"
        description="Virtual keys for your tenants' apps. Expand a key to copy it and get usage examples."
        actions={<Button variant="primary" leftIcon={<Plus className="size-4" />} onClick={() => setEditing('new')}>Create key</Button>}
      />
      <AdvancedDataTable
        data={(keys.data ?? []).map((k) => ({ ...k, id: `${k.namespace}/${k.name}`, state: stateOf(k) }))}
        columns={columns}
        getRowId={(r) => r.id}
        isLoading={keys.loading}
        error={keys.error}
        searchPlaceholder="Search keys…"
        renderSubComponent={(row) => <KeyDetails apiKey={row} proxyURL={overview.data?.proxyURL ?? ''} />}
      />
      {keys.data?.length === 0 && (
        <p className="mt-4 text-sm text-text-muted text-center">No keys yet. Create one per app that should reach your providers.</p>
      )}

      <KeySheet
        open={!!editing}
        apiKey={editing === 'new' ? undefined : editing}
        groups={groups.data ?? []}
        onClose={() => setEditing(undefined)}
        onSaved={keys.reload}
      />
      <ConfirmSheet
        open={!!confirm}
        action={confirm?.action}
        title={confirm?.action === 'rotate' ? `Rotate ${c?.name}?` : `Delete ${c?.name}?`}
        description={confirm?.action === 'rotate'
          ? 'A new key is generated right away and the current key stops working. Apps reading the Secret pick up the new key on restart.'
          : 'The key and its Secret are deleted. Apps using it get "invalid API key".'}
        onClose={() => setConfirm(undefined)}
        onConfirm={async () => {
          if (confirm!.action === 'rotate') await api.rotateKey(c!.namespace, c!.name)
          else await api.deleteKey(c!.namespace, c!.name)
          notify(confirm!.action === 'rotate' ? 'New key issued' : 'Key deleted', { description: `${c!.namespace}/${c!.name}` })
          void keys.reload()
        }}
      />
    </>
  )
}
