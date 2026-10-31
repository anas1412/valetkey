import { useEffect, useState, type FormEvent } from 'react'
import { KeyRound, ListChecks } from 'lucide-react'
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetDescription, SheetBody, SheetFooter } from '@/components/ui/sheet'
import { Form, FormItem, FormLabel, FormControl, FormDescription, FormMessage, Input, Checkbox } from '@/components/ui/form'
import { SpringSelect } from '@/components/ui/spring-select'
import { LockInput } from '@/components/ui/lock-input'
import { LiquidToggle } from '@/components/ui/liquid-toggle'
import { Button } from '@/components/ui/button'
import { ModelPicker } from '@/components/ModelPicker'
import { MappingEditor, toRows, fromRows, type MappingRow } from '@/components/MappingEditor'
import { api } from '@/lib/api'
import { useToast } from '@/lib/toast'
import type { Provider, ProviderSpec } from '@/lib/types'
import { PROVIDER_PRESETS, presetFor } from './presets'

interface Draft {
  name: string
  preset: string
  baseURL: string
  apiKey: string
  anyModel: boolean
  models: string[]
  priority: string
  enabled: boolean
  mapping: MappingRow[]
  proxyURL: string
  skipTLSVerify: boolean
}

function toDraft(c?: Provider): Draft {
  if (!c) {
    return { name: '', preset: 'openai', baseURL: PROVIDER_PRESETS[0].baseURL, apiKey: '', anyModel: true, models: [], priority: '10',
      enabled: true, mapping: [], proxyURL: '', skipTLSVerify: false }
  }
  const s = c.spec
  return {
    name: c.name, preset: presetFor(s.type, s.baseURL), baseURL: s.baseURL, apiKey: '',
    anyModel: !s.models?.length, models: s.models ?? [],
    priority: String(s.priority ?? 10), enabled: (s.status ?? 'Enabled') === 'Enabled',
    mapping: toRows(s.modelMapping), proxyURL: s.proxyURL ?? '', skipTLSVerify: !!s.skipTLSVerify,
  }
}

/** Add or edit a provider (Provider). A pasted API key is stored as a Kubernetes Secret and never shown again. */
export function ProviderSheet({ open, provider, onClose, onSaved }: {
  open: boolean
  provider?: Provider
  onClose: () => void
  onSaved: () => void
}) {
  const editing = !!provider
  const [d, setD] = useState<Draft>(() => toDraft(provider))
  const [available, setAvailable] = useState<string[]>([])
  const [loadingModels, setLoadingModels] = useState(false)
  const [modelsError, setModelsError] = useState<string>()
  const [error, setError] = useState<string>()
  const [busy, setBusy] = useState(false)
  const notify = useToast()
  const set = <K extends keyof Draft>(k: K, v: Draft[K]) => setD((prev) => ({ ...prev, [k]: v }))

  useEffect(() => {
    if (!open) return
    const draft = toDraft(provider)
    setD(draft)
    setAvailable(draft.models)
    setModelsError(undefined)
    setError(undefined)
  }, [open, provider])

  const preset = PROVIDER_PRESETS.find((p) => p.id === d.preset)!
  const canLoad = !!d.baseURL && (!!d.apiKey || editing)

  const loadModels = async () => {
    setLoadingModels(true)
    setModelsError(undefined)
    try {
      const spec = { type: preset.type, baseURL: d.baseURL.trim(), proxyURL: d.proxyURL || undefined, skipTLSVerify: d.skipTLSVerify || undefined }
      const { models } = await api.discoverModels(spec, d.apiKey || undefined, editing ? provider.name : undefined)
      setAvailable([...new Set([...(models ?? []), ...d.models])].sort())
      set('anyModel', false)
    } catch (err) {
      setModelsError((err as Error).message)
      set('anyModel', false)
    } finally {
      setLoadingModels(false)
    }
  }

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (!editing && !d.name) return setError('Name the provider')
    if (!editing && !d.apiKey) return setError('Paste the provider API key')
    if (!d.baseURL) return setError('Enter the base URL')
    if (!d.anyModel && !d.models.length) return setError('Select at least one model, or allow any model')
    const spec: ProviderSpec = {
      type: preset.type,
      baseURL: d.baseURL.trim(),
      models: d.anyModel ? [] : d.models,
      priority: Number(d.priority) || 10,
      status: d.enabled ? 'Enabled' : 'Disabled',
      modelMapping: fromRows(d.mapping),
      proxyURL: d.proxyURL.trim() || undefined,
      skipTLSVerify: d.skipTLSVerify || undefined,
      keySecretRef: provider?.spec.keySecretRef,
    }
    setBusy(true)
    setError(undefined)
    try {
      if (editing) await api.updateProvider(provider.name, spec, d.apiKey || undefined)
      else await api.createProvider(d.name.trim(), spec, d.apiKey)
      notify(editing ? 'Provider updated' : 'Provider added', { description: d.name })
      onSaved()
      onClose()
    } catch (err) {
      setError((err as Error).message)
      notify('Could not save', { description: (err as Error).message, variant: 'error' })
    } finally {
      setBusy(false)
    }
  }

  return (
    <Sheet open={open} onOpenChange={(o) => !o && onClose()}>
      <SheetContent side="right" className="sm:max-w-lg">
        <Form onSubmit={submit} className="flex flex-col h-full">
          <SheetHeader>
            <SheetTitle>{editing ? `Edit ${provider.name}` : 'Add a provider'}</SheetTitle>
            <SheetDescription>Requests are forwarded here with the provider's real key. Tenants never see it.</SheetDescription>
          </SheetHeader>

          <SheetBody className="space-y-5 overflow-y-auto">
            <SpringSelect
              label="Provider"
              options={PROVIDER_PRESETS.map((p) => ({ value: p.id, label: p.label, description: p.description }))}
              value={d.preset}
              disabled={editing}
              onChange={(id) => {
                const p = PROVIDER_PRESETS.find((x) => x.id === id)!
                setD((prev) => ({ ...prev, preset: id, baseURL: p.baseURL, name: prev.name || (id === 'custom' ? '' : id) }))
                setAvailable([])
              }}
            />

            <FormItem>
              <FormLabel required>Name</FormLabel>
              <FormControl>
                <Input value={d.name} disabled={editing} placeholder="openai" onChange={(e) => set('name', e.target.value.toLowerCase())} />
              </FormControl>
              <FormDescription>Lowercase letters, digits and "-". Used when picking providers for a tenant.</FormDescription>
            </FormItem>

            <FormItem>
              <FormLabel required>Base URL</FormLabel>
              <FormControl>
                <Input value={d.baseURL} placeholder="https://my-endpoint.example.com" onChange={(e) => set('baseURL', e.target.value)} />
              </FormControl>
            </FormItem>

            <LockInput
              label={editing ? 'New API key' : 'API key'}
              type="password"
              autoComplete="off"
              value={d.apiKey}
              onChange={(e) => set('apiKey', e.target.value)}
              placeholder={editing ? 'Leave empty to keep the current key' : 'sk-…'}
              description="Saved as a Kubernetes Secret in the operator namespace. It can't be shown again."
              leftIcon={<KeyRound className="size-3.5" />}
            />

            <FormItem>
              <div className="flex items-center justify-between gap-3">
                <div>
                  <FormLabel>Models</FormLabel>
                  <FormDescription>{d.anyModel ? 'Any model the provider offers is allowed.' : 'Only the selected models are allowed.'}</FormDescription>
                </div>
                <div className="flex items-center gap-2 shrink-0">
                  <span className="text-xs text-text-muted">Allow any</span>
                  <LiquidToggle value={d.anyModel} onChange={(v) => set('anyModel', v)} />
                </div>
              </div>
              {!d.anyModel && (
                available.length || loadingModels || modelsError ? (
                  <ModelPicker
                    available={available}
                    selected={d.models}
                    onChange={(m) => set('models', m)}
                    loading={loadingModels}
                    error={modelsError}
                    onReload={canLoad ? () => void loadModels() : undefined}
                  />
                ) : null
              )}
              {!d.anyModel && !available.length && !loadingModels && !modelsError && (
                <Button type="button" variant="secondary" leftIcon={<ListChecks className="size-4" />} disabled={!canLoad} onClick={() => void loadModels()}>
                  {canLoad ? 'Load models from the provider' : 'Paste the API key to load models'}
                </Button>
              )}
              {d.anyModel && (
                <Button type="button" variant="link" size="sm" className="px-0 self-start" disabled={!canLoad} onClick={() => void loadModels()}>
                  Choose specific models instead
                </Button>
              )}
            </FormItem>

            <div className="flex items-end justify-between gap-4">
              <FormItem className="flex-1">
                <FormLabel>Priority</FormLabel>
                <FormControl>
                  <Input type="number" min={1} value={d.priority} onChange={(e) => set('priority', e.target.value)} />
                </FormControl>
                <FormDescription>Higher gets more traffic when a tenant has several providers.</FormDescription>
              </FormItem>
              <div className="pb-6 flex flex-col items-center gap-1">
                <LiquidToggle value={d.enabled} onChange={(v) => set('enabled', v)} />
                <span className="text-xs text-text-muted">{d.enabled ? 'Enabled' : 'Disabled'}</span>
              </div>
            </div>

            <details className="group rounded-lg border border-border p-3">
              <summary className="cursor-pointer text-sm font-medium text-text-primary">Advanced</summary>
              <div className="mt-4 space-y-4">
                <FormItem>
                  <FormLabel>Model mapping</FormLabel>
                  <FormDescription>Rename a requested model before it is sent, e.g. to an Azure deployment name.</FormDescription>
                  <MappingEditor rows={d.mapping} onChange={(rows) => set('mapping', rows)} suggestions={available} />
                </FormItem>
                <FormItem>
                  <FormLabel>Outbound proxy</FormLabel>
                  <FormControl>
                    <Input value={d.proxyURL} placeholder="http://proxy.internal:3128" onChange={(e) => set('proxyURL', e.target.value)} />
                  </FormControl>
                </FormItem>
                <Checkbox
                  label="Skip TLS certificate check"
                  description="Only for self-hosted endpoints with self-signed certificates."
                  checked={d.skipTLSVerify}
                  onChange={(e) => set('skipTLSVerify', e.target.checked)}
                />
              </div>
            </details>
          </SheetBody>

          <SheetFooter className="flex-col items-stretch gap-2">
            <FormMessage error={error} />
            <div className="flex justify-end gap-2">
              <Button type="button" variant="ghost" onClick={onClose}>Cancel</Button>
              <Button type="submit" variant="primary" isLoading={busy} loadingText="Saving…">
                {editing ? 'Save changes' : 'Add provider'}
              </Button>
            </div>
          </SheetFooter>
        </Form>
      </SheetContent>
    </Sheet>
  )
}
