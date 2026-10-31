# Budgets & costs

Valetkey works out what every request costs in US dollars, from live provider prices. You can give a key, a tenant, or both a dollar budget per day, week or month. Valetkey never lets spend go past it.

## Set a budget

On a key:

```yaml
apiVersion: valetkey.dev/v1alpha1
kind: ProxyKey
metadata: { name: invoice-api, namespace: billing }
spec:
  budget:
    usd: "20"          # dollars per period
    period: monthly    # daily, weekly (resets Monday) or monthly (resets on the 1st)
    timezone: Europe/Paris   # where the period starts; default UTC
```

On a tenant, for all of its keys together:

```yaml
apiVersion: valetkey.dev/v1alpha1
kind: Group
metadata: { name: billing, namespace: valetkey-system }
spec:
  channelRefs: [openai]
  budget: { usd: "200", period: monthly }
```

On single models of a key, on top of the key's own budget:

```yaml
kind: ProxyKey
spec:
  budget: { usd: "50", period: monthly }
  modelBudgets:
    claude-opus-4-5: { usd: "5", period: daily }
```

A request must fit every budget that applies. You can also set them in the dashboard, on the key and tenant forms.

## What happens at the limit

Before Valetkey forwards a request, it **reserves the most the request could cost**:
- Input: the request body's size in bytes, since a token is at least one byte, at the dearest input price.
- Output: `max_tokens` (or `max_completion_tokens`), or the model's maximum output if the request doesn't set one.

If that doesn't fit in what's left of the budget, the app gets **429** with error type `insufficient_quota` (what OpenAI SDKs expect when credit runs out). The message says which budget ran out and when it resets:

```json
{"error": {"type": "insufficient_quota",
  "message": "Daily budget of $0.50 for tenant research reached (spent $0.4988, this request may cost up to $0.0081); resets 2026-11-01 00:00 UTC"}}
```

When the answer arrives, the reservation is released and the **real cost** is charged. Spend never passes the budget, even with many requests at once. The last few cents of a budget can go unused, because the next request might cost more than what's left.

::: tip Set max_tokens
Without `max_tokens`, Valetkey reserves the model's full output length, which can be large (Claude Sonnet: 64,000 tokens, about $0.96). Apps with a budget should send `max_tokens`, or give the key a default that Valetkey adds to requests without one:

```yaml
spec:
  maxTokens: 1024
```
:::

Streamed OpenAI-style requests under a budget get `stream_options.include_usage` added, so the provider reports usage at the end of the stream. If a stream is cut off before that, Valetkey charges the reserved worst case and marks the request "estimated" in Activity, so spend is never under-counted.

## How a cost is worked out

For each request, Valetkey reads the provider's usage report and prices each kind of token separately:

| Token kind | Reported by | Priced at |
|---|---|---|
| Input | all providers | input price |
| Cached input | OpenAI `cached_tokens`, DeepSeek `prompt_cache_hit_tokens`, Anthropic `cache_read_input_tokens` | cache read price |
| Cache writes | Anthropic `cache_creation_input_tokens` (5 minute and 1 hour) | cache write price (1 hour writes cost 2× input on Anthropic) |
| Output | all providers | output price |
| Reasoning | OpenAI and DeepSeek `reasoning_tokens` | part of output; a separate price only if the provider lists one |

Long prompts use long-context prices when a model has them (for example above 200,000 or 272,000 input tokens). The price is taken at the **moment the request starts**. All maths is done in whole pico-dollars, with no rounding until a number is shown.

## Where prices come from

No provider offers a pricing API, so Valetkey combines public sources and refreshes them every 6 hours:

| Source | What it gives |
|---|---|
| [models.dev](https://models.dev) | Input, output, cache and reasoning prices, long-context tiers and maximum output for 200+ providers |
| [OpenRouter](https://openrouter.ai/api/v1/models) | Anthropic 1 hour cache-write prices and a cross-check for OpenAI, Anthropic, Google and xAI. Models where the two disagree by more than 1% are flagged on the Pricing page |
| [DeepSeek's pricing page](https://api-docs.deepseek.com/quick_start/pricing) | Peak hours, read from DeepSeek's own page |
| [holiday-cn](https://github.com/NateScarlet/holiday-cn) | China's official public holidays, which DeepSeek bills at off-peak prices |

The provider is guessed from the channel's `baseURL` (`api.openai.com`, `api.anthropic.com`, `api.deepseek.com`, `api.groq.com`, `openrouter.ai` and others). For a gateway, a proxy or any other address, name it:

```yaml
kind: Channel
spec:
  baseURL: https://llm-gateway.internal
  pricing:
    provider: deepseek    # a models.dev provider id
```

The last fetched price list is kept in the ConfigMap `valetkey-pricing`, so a restart doesn't need the internet. Each Valetkey image also carries the prices from the day it was built.

## DeepSeek peak and off-peak hours

DeepSeek charges twice as much at peak times: **01:00 to 04:00 and 06:00 to 10:00 UTC, Monday to Friday**. Weekends and Chinese public holidays are off-peak all day. Valetkey reads these hours from DeepSeek's pricing page on every refresh. Holidays are checked against the date in Beijing.

The Pricing page shows whether a model is at peak price right now and when that changes. If DeepSeek's page can't be read, Valetkey uses the schedule above and shows a warning on the Pricing page.

## Your own prices

Negotiated rates, self-hosted models and Azure deployment names go on the channel, in dollars per million tokens. They win over every other source:

```yaml
kind: Channel
spec:
  pricing:
    models:
      llama-3.1-70b: { input: "0", output: "0" }   # self-hosted: free
      gpt-4o: { input: "2.25", output: "9", cacheRead: "1.125" }   # 10% discount
```

`cacheRead` and `cacheWrite` default to the input price. The dashboard keeps these overrides when you edit a provider there.

## No price known

A key or tenant with a budget can only use models with a known price, so the budget can be enforced. Its requests for other models get **403** `model_not_priced`. Add a price on the channel to fix it. Without a budget, such requests go through and show as "unpriced" in Activity.

## Seeing spend

- `kubectl get pkey -A` and `kubectl get grp -n valetkey-system` show **Spent** and **Budget** columns. The exact amount is in `status.spend`.
- The dashboard shows a spend bar on keys and tenants, a Spend card on the Overview, the cost of each request in Activity, and a **Pricing** page.
- Prometheus: `valetkey_cost_usd_total`, `valetkey_budget_spent_usd`, `valetkey_budget_usd` and `valetkey_budget_rejections_total`. The [Grafana dashboard](/guide/usage-history#_3-add-the-grafana-dashboard) has spend per tenant and budget use.

Spend is saved to the key's and tenant's status every 5 seconds and when Valetkey stops, and is loaded back when it starts. A crash (not a normal restart) can lose up to 5 seconds of spend.

## Alerts

At 80% and when a budget is used up, Valetkey records a Kubernetes Event on the key or tenant (`BudgetAlmostUsed`, `BudgetReached`) and an entry in the dashboard's Activity log, once per period:

```bash
kubectl describe pkey -n billing invoice-api
kubectl get events -A --field-selector reason=BudgetReached
```

With the Prometheus Operator, `prometheusRule.enabled: true` in the Helm values adds alerts for the same, see [Monitoring](/guide/monitoring#alerts).

## Without internet access

Set `pricing.refreshInterval: "off"` in the Helm values. Valetkey then uses the prices built into its image, plus your channel overrides. Otherwise it needs to reach `models.dev`, `openrouter.ai`, `api-docs.deepseek.com` and `raw.githubusercontent.com`.

## Good to know

- The provider's invoice is what you actually pay. Valetkey's numbers follow the public list price. Credits, taxes, enterprise discounts and batch pricing are only known through your own channel prices.
- Price catalogues can lag a provider's price change by a few hours.
- With [two replicas](/guide/limits#scaling), budgets are shared through status. Together the pods can go over a budget by what they spend in 5 seconds; with one replica, never.
