import { useEffect, useState, type FormEvent } from 'react'
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetDescription, SheetBody, SheetFooter } from '@/components/ui/sheet'
import { Form, FormItem, FormLabel, FormControl, FormDescription, FormMessage, Input, Checkbox } from '@/components/ui/form'
import { Combobox } from '@/components/ui/combobox'
import { LiquidToggle } from '@/components/ui/liquid-toggle'
import { Button } from '@/components/ui/button'
import { BudgetFields, fromBudgetDraft, toBudgetDraft, validBudgetAmount, type BudgetDraft } from '@/components/BudgetFields'
import { api } from '@/lib/api'
import { useToast } from '@/lib/toast'
import type { Provider, Tenant } from '@/lib/types'

interface Draft {
  name: string
  providers: string[]
  rpmRatio: string
  tpmRatio: string
  budget: BudgetDraft
  enabled: boolean
}

function toDraft(g?: Tenant): Draft {
  return {
    name: g?.name ?? '',
    providers: g?.spec.providers ?? [],
    rpmRatio: String(g?.spec.rpmRatio ?? 1),
    tpmRatio: String(g?.spec.tpmRatio ?? 1),
    budget: toBudgetDraft(g?.spec.budget),
    enabled: (g?.spec.status ?? 'Enabled') === 'Enabled',
  }
}

/** Create or edit a tenant (Tenant). A tenant is a namespace; its keys live there. */
export function TenantSheet({ open, tenant, providers, onClose, onSaved }: {
  open: boolean
  tenant?: Tenant
  providers: Provider[]
  onClose: () => void
  onSaved: () => void
}) {
  const editing = !!tenant
  const [d, setD] = useState<Draft>(() => toDraft(tenant))
  const [namespaces, setNamespaces] = useState<Array<{ name: string; hasTenant: boolean }>>([])
  const [error, setError] = useState<string>()
  const [busy, setBusy] = useState(false)
  const notify = useToast()
  const set = <K extends keyof Draft>(k: K, v: Draft[K]) => setD((prev) => ({ ...prev, [k]: v }))

  useEffect(() => {
    if (!open) return
    setD(toDraft(tenant))
    setError(undefined)
    if (!tenant) api.namespaces().then(setNamespaces).catch(() => setNamespaces([]))
  }, [open, tenant])

  const toggleProvider = (name: string, on: boolean) =>
    set('providers', on ? [...d.providers, name] : d.providers.filter((c) => c !== name))

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (!d.name) return setError('Pick the namespace this tenant uses')
    if (!d.providers.length) return setError('Pick at least one provider')
    if (!validBudgetAmount(d.budget.usd)) return setError('Budget must be a dollar amount, e.g. 200 or 0.50')
    const spec = {
      providers: d.providers,
      status: d.enabled ? 'Enabled' as const : 'Disabled' as const,
      rpmRatio: Number(d.rpmRatio) || 1,
      tpmRatio: Number(d.tpmRatio) || 1,
      budget: fromBudgetDraft(d.budget),
    }
    setBusy(true)
    setError(undefined)
    try {
      if (editing) await api.updateTenant(tenant.name, spec)
      else await api.createTenant(d.name, spec)
      notify(editing ? 'Tenant updated' : 'Tenant created', { description: d.name })
      onSaved()
      onClose()
    } catch (err) {
      setError((err as Error).message)
      notify('Could not save', { description: (err as Error).message, variant: 'error' })
    } finally {
      setBusy(false)
    }
  }

  const available = namespaces.filter((n) => !n.hasTenant)

  return (
    <Sheet open={open} onOpenChange={(o) => !o && onClose()}>
      <SheetContent side="right" className="sm:max-w-lg">
        <Form onSubmit={submit} className="flex flex-col h-full">
          <SheetHeader>
            <SheetTitle>{editing ? `Edit ${tenant.name}` : 'Create a tenant'}</SheetTitle>
            <SheetDescription>A tenant is a Kubernetes namespace. Its API keys can only use the providers you pick here.</SheetDescription>
          </SheetHeader>

          <SheetBody className="space-y-5 overflow-y-auto">
            <FormItem>
              <FormLabel required>Namespace</FormLabel>
              {editing ? (
                <FormControl><Input value={d.name} disabled /></FormControl>
              ) : available.length ? (
                <Combobox
                  options={available.map((n) => ({ value: n.name, label: n.name }))}
                  value={d.name || null}
                  onValueChange={(v) => set('name', v ?? '')}
                  placeholder="Pick a namespace"
                  searchPlaceholder="Search namespaces…"
                  emptyText="No matching namespace"
                />
              ) : (
                <FormControl>
                  <Input value={d.name} placeholder="my-team" onChange={(e) => set('name', e.target.value.toLowerCase())} />
                </FormControl>
              )}
              <FormDescription>
                {editing ? 'The tenant name is its namespace and cannot change.'
                  : 'Only namespaces without a tenant are listed. Create a new one with kubectl create namespace.'}
              </FormDescription>
            </FormItem>

            <FormItem>
              <FormLabel required>Providers</FormLabel>
              {providers.length ? (
                <div className="space-y-2.5 rounded-lg border border-border p-3">
                  {providers.map((c) => (
                    <Checkbox
                      key={c.name}
                      label={c.name}
                      description={`${c.spec.type === 'anthropic' ? 'Anthropic' : 'OpenAI-compatible'}${c.spec.models?.length ? ` · ${c.spec.models.join(', ')}` : ' · any model'}`}
                      checked={d.providers.includes(c.name)}
                      onChange={(e) => toggleProvider(c.name, e.target.checked)}
                    />
                  ))}
                </div>
              ) : (
                <p className="text-sm text-text-muted">Add a provider first.</p>
              )}
            </FormItem>

            <div className="grid grid-cols-2 gap-4">
              <FormItem>
                <FormLabel>Request limit ×</FormLabel>
                <FormControl><Input type="number" min={0} step={0.1} value={d.rpmRatio} onChange={(e) => set('rpmRatio', e.target.value)} /></FormControl>
              </FormItem>
              <FormItem>
                <FormLabel>Token limit ×</FormLabel>
                <FormControl><Input type="number" min={0} step={0.1} value={d.tpmRatio} onChange={(e) => set('tpmRatio', e.target.value)} /></FormControl>
              </FormItem>
            </div>
            <p className="-mt-3 text-xs text-text-muted">Multiplies each key's own per-minute limits. 1 = unchanged, 2 = double.</p>

            <BudgetFields value={d.budget} onChange={(b) => set('budget', b)}
              hint="For all of the tenant's keys together. Requests that might go over it are refused (429)." />

            <div className="flex items-center justify-between rounded-lg border border-border p-3">
              <div>
                <div className="text-sm font-medium text-text-primary">Tenant enabled</div>
                <div className="text-xs text-text-muted">When off, every key of this tenant stops working.</div>
              </div>
              <LiquidToggle value={d.enabled} onChange={(v) => set('enabled', v)} />
            </div>

          </SheetBody>

          <SheetFooter className="flex-col items-stretch gap-2">
            <FormMessage error={error} />
            <div className="flex justify-end gap-2">
              <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
              <Button type="submit" variant="primary" isLoading={busy} loadingText="Saving…">
                {editing ? 'Save changes' : 'Create tenant'}
              </Button>
            </div>
          </SheetFooter>
        </Form>
      </SheetContent>
    </Sheet>
  )
}
