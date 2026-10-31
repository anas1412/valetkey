/** Shapes returned by the operator's admin API (src/management/*). */

export type EnabledStatus = 'Enabled' | 'Disabled'
export type ChannelType = 'openai' | 'anthropic'

export interface ChannelSpec {
  baseURL: string
  type: ChannelType
  keySecretRef?: { name: string; key?: string }
  priority?: number
  status?: EnabledStatus
  models?: string[]
  proxyURL?: string
  skipTLSVerify?: boolean
  modelMapping?: Record<string, string>
}

export interface Channel {
  name: string
  spec: ChannelSpec
  ready: boolean
  message?: string
}

export interface GroupSpec {
  status?: EnabledStatus
  channelRefs: string[]
  rpmRatio?: number
  tpmRatio?: number
}

export interface Group {
  name: string
  spec: GroupSpec
  ready: boolean
  message?: string
  keys: number
}

export interface ProxyKeySpec {
  status?: EnabledStatus
  models?: string[]
  subnets?: string[]
  rpm?: number
  tpm?: number
}

export interface ProxyKey {
  namespace: string
  name: string
  spec: ProxyKeySpec
  ready: boolean
  message?: string
  secretName?: string
  requests: number
  lastUsed?: number
}

export interface MinuteBucket {
  minute: number
  requests: number
  errors: number
  inputTokens: number
  outputTokens: number
}

export interface Overview {
  proxyURL: string
  startedAt: number
  stepMinutes: number
  totals: Omit<MinuteBucket, 'minute'>
  series: MinuteBucket[]
  channelShare: Record<string, number>
  counts: { channels: number; groups: number; keys: number; activeKeys: number }
  channels: Array<{ name: string; type: ChannelType; enabled: boolean; ready: boolean; message?: string }>
}

export interface RequestEntry {
  time: number
  namespace: string
  key: string
  group: string
  channel: string
  model: string
  status: number
  inputTokens: number
  outputTokens: number
  latencyMs: number
  error?: string
}

export interface OperatorEvent {
  time: number
  level: 'info' | 'warn' | 'error'
  kind: string
  message: string
}

export interface ChannelTestResult {
  success: boolean
  status?: number
  latencyMs: number
  message: string
}
