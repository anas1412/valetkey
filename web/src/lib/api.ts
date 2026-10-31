import type {
  Provider, ProviderSpec, ProviderTestResult, Tenant, TenantSpec, HistoryRange, OperatorEvent, Overview, Pricing, ProxyKey, ProxyKeySpec, RequestEntry,
} from './types'

/** Thrown for any non-2xx answer; `message` is the server's explanation. */
export class ApiError extends Error {
  readonly status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

async function send(method: string, path: string, body?: unknown): Promise<Response> {
  // The admin API requires this header: other websites can't add it, so they can't
  // use your open port-forward (see internal/admin guard).
  const headers: Record<string, string> = { 'x-valetkey-request': '1' }
  if (body !== undefined) headers['content-type'] = 'application/json'
  const res = await fetch(`/api/v1${path}`, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) {
    const data = await res.json().catch(() => ({}))
    throw new ApiError(res.status, data.message ?? data.error?.message ?? `request failed (HTTP ${res.status})`)
  }
  return res
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await send(method, path, body)
  return res.status === 204 ? (undefined as T) : res.json()
}

const enc = encodeURIComponent

export type YamlResource = 'providers' | 'tenants' | 'proxykeys'

export const api = {
  overview: (range: HistoryRange = '24h') => request<Overview>('GET', `/overview?range=${range}`),
  activity: () => request<{ requests: RequestEntry[]; events: OperatorEvent[] }>('GET', '/activity'),
  namespaces: () => request<Array<{ name: string; hasTenant: boolean }>>('GET', '/namespaces'),

  providers: () => request<Provider[]>('GET', '/providers'),
  createProvider: (name: string, spec: ProviderSpec, apiKey?: string) => request<Provider>('POST', '/providers', { name, spec, apiKey }),
  updateProvider: (name: string, spec: ProviderSpec, apiKey?: string) => request<Provider>('PUT', `/providers/${enc(name)}`, { spec, apiKey }),
  deleteProvider: (name: string) => request<void>('DELETE', `/providers/${enc(name)}`),
  testProvider: (name: string) => request<ProviderTestResult>('POST', `/providers/${enc(name)}/test`),
  /** Asks the provider for its model list. Pass `provider` to use a saved provider's key. */
  discoverModels: (spec: ProviderSpec, apiKey?: string, provider?: string) =>
    request<{ models: string[] | null }>('POST', '/models/discover', { spec, apiKey, provider }),

  tenants: () => request<Tenant[]>('GET', '/tenants'),
  createTenant: (name: string, spec: TenantSpec) => request<Tenant>('POST', '/tenants', { name, spec }),
  updateTenant: (name: string, spec: TenantSpec) => request<Tenant>('PUT', `/tenants/${enc(name)}`, { spec }),
  deleteTenant: (name: string) => request<void>('DELETE', `/tenants/${enc(name)}`),
  /** Every model the tenant's keys can use. */
  tenantModels: (name: string) => request<{ models: string[] | null }>('GET', `/tenants/${enc(name)}/models`),

  keys: () => request<ProxyKey[]>('GET', '/proxykeys'),
  pricing: () => request<Pricing>('GET', '/pricing'),
  createKey: (namespace: string, name: string, spec: ProxyKeySpec) => request<ProxyKey>('POST', '/proxykeys', { namespace, name, spec }),
  updateKey: (ns: string, name: string, spec: ProxyKeySpec) => request<ProxyKey>('PUT', `/proxykeys/${enc(ns)}/${enc(name)}`, { spec }),
  deleteKey: (ns: string, name: string) => request<void>('DELETE', `/proxykeys/${enc(ns)}/${enc(name)}`),
  /** The live object as CRD YAML, without status or server-managed fields. */
  yaml: (resource: YamlResource, ...path: string[]) => send('GET', `/${resource}/${path.map(enc).join('/')}/yaml`).then((r) => r.text()),
  revealKey: (ns: string, name: string) => request<{ key: string }>('GET', `/proxykeys/${enc(ns)}/${enc(name)}/key`),
  rotateKey: (ns: string, name: string) => request<void>('POST', `/proxykeys/${enc(ns)}/${enc(name)}/rotate`),
}
