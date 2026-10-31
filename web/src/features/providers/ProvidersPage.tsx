import { useState } from 'react'
import { Plus, Pencil, Trash2, FileCode } from 'lucide-react'
import { AdvancedDataTable, type ColumnDef } from '@/components/ui/advanced-data-table'
import { SplitButton } from '@/components/ui/split-button'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { PageHeader } from '@/components/PageHeader'
import { ConfirmSheet } from '@/components/ConfirmSheet'
import { YamlSheet, type YamlTarget } from '@/components/YamlSheet'
import { usePoll } from '@/hooks/use-poll'
import { api } from '@/lib/api'
import { useToast } from '@/lib/toast'
import type { Channel } from '@/lib/types'
import { ProviderSheet } from './ProviderSheet'

type Row = Channel & { state: string }

function stateOf(c: Channel): string {
  if ((c.spec.status ?? 'Enabled') !== 'Enabled') return 'disabled'
  if (c.keyRejected) return 'key-rejected'
  return c.ready ? 'ready' : 'key-missing'
}

export function ProvidersPage() {
  const { data, error, reload } = usePoll(api.channels, 10_000)
  const [editing, setEditing] = useState<Channel | 'new'>()
  const [deleting, setDeleting] = useState<Channel>()
  const [yaml, setYaml] = useState<YamlTarget>()
  const notify = useToast()

  const test = async (c: Channel) => {
    try {
      const r = await api.testChannel(c.name)
      notify(r.success ? `${c.name} works` : `${c.name} failed`, {
        description: `${r.message}${r.latencyMs ? ` · ${r.latencyMs} ms` : ''}`,
        variant: r.success ? 'success' : 'error',
      })
      void reload() // a test updates the Key rejected badge
    } catch (err) {
      notify('Test failed', { description: (err as Error).message, variant: 'error' })
    }
  }

  const columns: ColumnDef<Row>[] = [
    {
      id: 'name', header: 'Name', accessorKey: 'name', sortable: true, priority: 'high',
      cell: ({ row }) => (
        <div>
          <div className="font-medium text-text-primary">{row.name}</div>
          <div className="text-xs text-text-muted truncate max-w-56">{row.spec.baseURL}</div>
        </div>
      ),
    },
    {
      id: 'type', header: 'Type', accessorFn: (r) => r.spec.type, filterable: true, priority: 'medium',
      filterOptions: [{ label: 'OpenAI-compatible', value: 'openai' }, { label: 'Anthropic', value: 'anthropic' }],
      cell: ({ value }) => <Badge tone="neutral" variant="outline">{value === 'anthropic' ? 'Anthropic' : 'OpenAI'}</Badge>,
    },
    {
      id: 'models', header: 'Models', accessorFn: (r) => (r.spec.models ?? []).join(', '), priority: 'low',
      cell: ({ row }) => {
        const m = row.spec.models ?? []
        return <span className="text-text-secondary">{m.length ? (m.length > 3 ? `${m.slice(0, 3).join(', ')} +${m.length - 3}` : m.join(', ')) : 'any'}</span>
      },
    },
    { id: 'priority', header: 'Priority', accessorFn: (r) => r.spec.priority ?? 10, sortable: true, align: 'right', priority: 'low' },
    {
      id: 'state', header: 'Status', accessorKey: 'state', filterable: true, priority: 'high',
      filterOptions: [{ label: 'Ready', value: 'ready' }, { label: 'Key rejected', value: 'key-rejected' }, { label: 'Key missing', value: 'key-missing' }, { label: 'Disabled', value: 'disabled' }],
      cell: ({ row }) => row.state === 'ready' ? <Badge tone="success" dot>Ready</Badge>
        : row.state === 'disabled' ? <Badge tone="neutral">Disabled</Badge>
        : <Badge tone="danger" dot title={row.message}>{row.state === 'key-rejected' ? 'Key rejected' : 'Key missing'}</Badge>,
    },
    {
      id: 'actions', header: '', align: 'right', priority: 'high',
      cell: ({ row }) => (
        <SplitButton
          size="sm"
          variant="secondary"
          label="Test"
          onClick={() => void test(row)}
          options={[
            { value: 'edit', label: 'Edit', icon: <Pencil className="size-3.5" />, onSelect: () => setEditing(row) },
            { value: 'yaml', label: 'View YAML', icon: <FileCode className="size-3.5" />, onSelect: () => setYaml({ resource: 'channels', path: [row.name], title: `Provider ${row.name}` }) },
            { value: 'delete', label: 'Delete', icon: <Trash2 className="size-3.5" />, onSelect: () => setDeleting(row) },
          ]}
        />
      ),
    },
  ]

  return (
    <>
      <PageHeader
        title="Providers"
        description="Upstream LLM APIs. Each provider holds one real API key; tenants are given access to providers, never to the key."
        actions={<Button variant="primary" leftIcon={<Plus className="size-4" />} onClick={() => setEditing('new')}>Add provider</Button>}
      />

      <AdvancedDataTable
        data={(data ?? []).map((c) => ({ ...c, state: stateOf(c) }))}
        columns={columns}
        getRowId={(r) => r.name}
        isLoading={!data && !error}
        error={error}
        searchPlaceholder="Search providers…"
      />
      {data?.length === 0 && (
        <p className="mt-4 text-sm text-text-muted text-center">No providers yet. Add one with your OpenAI or Anthropic API key to start.</p>
      )}

      <YamlSheet target={yaml} onClose={() => setYaml(undefined)} />
      <ProviderSheet
        open={!!editing}
        channel={editing === 'new' ? undefined : editing}
        onClose={() => setEditing(undefined)}
        onSaved={reload}
      />
      <ConfirmSheet
        open={!!deleting}
        title={`Delete ${deleting?.name}?`}
        description="Tenants using only this provider will stop working. A key you pasted in the dashboard is deleted with it."
        onClose={() => setDeleting(undefined)}
        onConfirm={async () => {
          await api.deleteChannel(deleting!.name)
          notify('Provider deleted', { description: deleting!.name })
          void reload()
        }}
      />
    </>
  )
}
