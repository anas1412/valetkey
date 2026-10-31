import type {
  Channel, ChannelSpec, ChannelTestResult, Group, GroupSpec, HistoryRange, OperatorEvent, Overview, Pricing, ProxyKey, ProxyKeySpec, RequestEntry,
} from './types'

/** Thrown for any non-2xx answer; `message` is the server's explanation. */
export class ApiError extends Error {
  readonly status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
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
  return res.status === 204 ? (undefined as T) : res.json()
}

const enc = encodeURIComponent

export const api = {
  overview: (range: HistoryRange = '24h') => request<Overview>('GET', `/overview?range=${range}`),
  activity: () => request<{ requests: RequestEntry[]; events: OperatorEvent[] }>('GET', '/activity'),
  namespaces: () => request<Array<{ name: string; hasGroup: boolean }>>('GET', '/namespaces'),

  channels: () => request<Channel[]>('GET', '/channels'),
  createChannel: (name: string, spec: ChannelSpec, apiKey?: string) => request<Channel>('POST', '/channels', { name, spec, apiKey }),
  updateChannel: (name: string, spec: ChannelSpec, apiKey?: string) => request<Channel>('PUT', `/channels/${enc(name)}`, { spec, apiKey }),
  deleteChannel: (name: string) => request<void>('DELETE', `/channels/${enc(name)}`),
  testChannel: (name: string) => request<ChannelTestResult>('POST', `/channels/${enc(name)}/test`),
  /** Asks the provider for its model list. Pass `channel` to use a saved provider's key. */
  discoverModels: (spec: ChannelSpec, apiKey?: string, channel?: string) =>
    request<{ models: string[] | null }>('POST', '/models/discover', { spec, apiKey, channel }),

  groups: () => request<Group[]>('GET', '/groups'),
  createGroup: (name: string, spec: GroupSpec) => request<Group>('POST', '/groups', { name, spec }),
  updateGroup: (name: string, spec: GroupSpec) => request<Group>('PUT', `/groups/${enc(name)}`, { spec }),
  deleteGroup: (name: string) => request<void>('DELETE', `/groups/${enc(name)}`),
  /** Every model the tenant's keys can use. */
  groupModels: (name: string) => request<{ models: string[] | null }>('GET', `/groups/${enc(name)}/models`),

  keys: () => request<ProxyKey[]>('GET', '/proxykeys'),
  pricing: () => request<Pricing>('GET', '/pricing'),
  createKey: (namespace: string, name: string, spec: ProxyKeySpec) => request<ProxyKey>('POST', '/proxykeys', { namespace, name, spec }),
  updateKey: (ns: string, name: string, spec: ProxyKeySpec) => request<ProxyKey>('PUT', `/proxykeys/${enc(ns)}/${enc(name)}`, { spec }),
  deleteKey: (ns: string, name: string) => request<void>('DELETE', `/proxykeys/${enc(ns)}/${enc(name)}`),
  revealKey: (ns: string, name: string) => request<{ key: string }>('GET', `/proxykeys/${enc(ns)}/${enc(name)}/key`),
  rotateKey: (ns: string, name: string) => request<void>('POST', `/proxykeys/${enc(ns)}/${enc(name)}/rotate`),
}
