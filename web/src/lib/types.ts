/** Shapes returned by the operator's admin API (src/management/*). */

export type EnabledStatus = 'Enabled' | 'Disabled'
export type ProviderType = 'openai' | 'anthropic'

export interface ProviderSpec {
  baseURL: string
  type: ProviderType
  keySecretRef?: { name: string; key?: string }
  priority?: number
  status?: EnabledStatus
  models?: string[]
  proxyURL?: string
  skipTLSVerify?: boolean
  modelMapping?: Record<string, string>
  /** Set in YAML; the dashboard keeps it as is. */
  pricing?: { provider?: string; models?: Record<string, Record<string, string>> }
}

export type BudgetPeriod = 'daily' | 'weekly' | 'monthly'

export interface Budget {
  /** US dollars per period, as a decimal string ("20", "0.50"). */
  usd: string
  period?: BudgetPeriod
  timezone?: string
}

/** Spend in the current budget period (or the current month without a budget). */
export interface Spend {
  spent: number
  exact: string
  reserved: number
  budget?: number
  period: BudgetPeriod
  periodStart: string
  resets?: string
  requests: number
  unpriced?: number
}

export interface Provider {
  name: string
  spec: ProviderSpec
  ready: boolean
  message?: string
  /** The provider answered 401/403 to the key; the provider gets no traffic. */
  keyRejected?: boolean
}

export interface TenantSpec {
  status?: EnabledStatus
  providers: string[]
  rpmRatio?: number
  tpmRatio?: number
  budget?: Budget
}

export interface Tenant {
  name: string
  spec: TenantSpec
  ready: boolean
  message?: string
  keys: number
  spend?: Spend
}

export interface ProxyKeySpec {
  status?: EnabledStatus
  models?: string[]
  subnets?: string[]
  rpm?: number
  tpm?: number
  budget?: Budget
  /** Output cap added to requests that don't set max_tokens. */
  maxTokens?: number
  /** RFC 3339; the key stops working then. */
  expiresAt?: string
  /** Budgets for single models, by model name. */
  modelBudgets?: Record<string, Budget>
}

export interface ProxyKey {
  namespace: string
  name: string
  spec: ProxyKeySpec
  ready: boolean
  message?: string
  secretName?: string
  requests: number
  /** "30d" when requests come from Prometheus, "pod" when counted since the pod started. */
  requestsWindow: '30d' | 'pod'
  lastUsed?: number
  spend?: Spend
  modelSpend?: Record<string, Spend>
}

export interface MinuteBucket {
  minute: number
  requests: number
  errors: number
  inputTokens: number
  outputTokens: number
  cost: number
}

export type HistoryRange = '24h' | '7d' | '30d'

export interface Overview {
  proxyURL: string
  startedAt: number
  /** Where usage numbers come from: Prometheus (spans restarts) or this pod's memory. */
  source: 'prometheus' | 'memory'
  range: HistoryRange
  /** Set when Prometheus is configured but couldn't be read. */
  warning?: string
  prometheusConfig: boolean
  stepMinutes: number
  totals: Omit<MinuteBucket, 'minute' | 'cost'>
  series: MinuteBucket[]
  providerShare: Record<string, number>
  /** US dollars spent over the range. */
  cost: number
  counts: { providers: number; tenants: number; keys: number; activeKeys: number }
  providers: Array<{ name: string; type: ProviderType; enabled: boolean; ready: boolean; message?: string }>
}

export interface RequestEntry {
  time: number
  namespace: string
  key: string
  tenant: string
  provider: string
  model: string
  status: number
  inputTokens: number
  outputTokens: number
  latencyMs: number
  error?: string
  costUSD: number
  /** "estimated" (worst case charged, no usage reported), "unpriced" or "no usage". */
  costNote?: 'estimated' | 'unpriced' | 'no usage'
}

export interface PriceSource {
  name: string
  url: string
  ok: boolean
  note?: string
  error?: string
}

/** What one provider model costs right now, in dollars per million tokens. */
export interface ModelPrice {
  provider: string
  model: string
  /** Price catalogue entry the model is priced as, e.g. deepseek. */
  catalog?: string
  known: boolean
  source?: string
  warning?: string
  input: number
  output: number
  cacheRead: number
  cacheWrite5m: number
  cacheWrite1h: number
  period?: 'peak' | 'off-peak'
  nextChange?: string
  offPeakInput?: number
}

export interface Pricing {
  fetchedAt?: string
  refresh: string
  sources: PriceSource[]
  prices: ModelPrice[]
}

export interface OperatorEvent {
  time: number
  level: 'info' | 'warn' | 'error'
  kind: string
  message: string
}

export interface ProviderTestResult {
  success: boolean
  status?: number
  latencyMs: number
  message: string
}
