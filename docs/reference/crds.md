# Custom resources

All three live in the API group `valetkey.dev/v1alpha1` and report a standard `Ready` condition in their status. A Channel's condition reason is `KeyResolved`, `SecretUnavailable` or `KeyRejected` (the provider answered 401 or 403 to the key). Short names: `chan`, `grp`, `pkey`.

## Channel

A provider. Namespace: the operator namespace.

| Field | Type | Default | Description |
|---|---|---|---|
| `type` | `openai` \| `anthropic` | required | `openai` covers every OpenAI-compatible API |
| `baseURL` | string | required | e.g. `https://api.openai.com`. A path prefix (like `/api`) is kept; a trailing `/v1` isn't doubled |
| `keySecretRef.name` | string | required | Secret in the operator namespace holding the real key |
| `keySecretRef.key` | string | `apiKey` | Key inside that Secret |
| `models` | string[] | any | Models this provider may serve |
| `priority` | int | `10` | Share of traffic when several providers serve a model |
| `status` | `Enabled` \| `Disabled` | `Enabled` | |
| `modelMapping` | map | none | Requested model → model sent upstream |
| `proxyURL` | string | none | Outbound HTTP proxy for this provider |
| `skipTLSVerify` | bool | `false` | Skip TLS verification (self-signed endpoints only) |
| `pricing.provider` | string | guessed from `baseURL` | Price catalogue provider (models.dev id), e.g. `deepseek` |
| `pricing.models` | map | none | Per-model prices in dollars per million tokens: `input`, `output`, `cacheRead`, `cacheWrite`, `cacheWrite1h`. See [Budgets & costs](/guide/budgets#your-own-prices) |

## Group

A tenant. Namespace: the operator namespace. **Its name is the tenant namespace.**

| Field | Type | Default | Description |
|---|---|---|---|
| `channelRefs` | string[] | required | Providers this tenant may use |
| `status` | `Enabled` \| `Disabled` | `Enabled` | Disabled stops every key of the tenant |
| `rpmRatio` | number | `1.0` | Multiplies each key's `rpm` |
| `tpmRatio` | number | `1.0` | Multiplies each key's `tpm` |
| `budget.usd` | string | none | Dollar budget per period for all the tenant's keys, e.g. `"200"` |
| `budget.period` | `daily` \| `weekly` \| `monthly` | `monthly` | |
| `budget.timezone` | string | `UTC` | IANA time zone where periods start |
| `modelBudgets` | map | none | Budgets for single models, e.g. `claude-opus-4-5: {usd: "5", period: daily}` |
| `maxTokens` | int | none | Output cap added to requests that don't set one |
| `expiresAt` | RFC 3339 time | never | When the key stops working |

## ProxyKey

An API key. Namespace: the tenant's namespace.

| Field | Type | Default | Description |
|---|---|---|---|
| `status` | `Enabled` \| `Disabled` | `Enabled` | |
| `models` | string[] | any | Models this key may use |
| `subnets` | string[] | any | Allowed client IP ranges (IPv4/IPv6 CIDR or single address) |
| `rpm` | int | unlimited | Requests per minute (× tenant `rpmRatio`) |
| `tpm` | int | unlimited | Tokens per minute (× tenant `tpmRatio`) |
| `budget.usd` | string | none | Dollar budget per period for this key, e.g. `"20"` |
| `budget.period` | `daily` \| `weekly` \| `monthly` | `monthly` | |
| `budget.timezone` | string | `UTC` | IANA time zone where periods start |

Status fields: `ready`, `message`, `conditions`, `secretName`, `keyHash` (SHA-256 of the key; the key itself is only in the Secret), `spend` (current period: `usd` across all replicas, `periodStart`, `requests`, and each replica's share under `pods`) and `modelSpend` (the same per model in `modelBudgets`). Groups have `spend` too.

## Full example

See [`examples/quickstart.yaml`](https://github.com/anas1412/valetkey/blob/main/examples/quickstart.yaml): two providers, one tenant, two keys.
