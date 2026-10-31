# Monitoring

## Dashboard

The [Overview](/guide/dashboard#overview) and [Activity](/guide/dashboard#activity) pages show the last 24 hours on the running pod. To keep history across restarts and see 7 and 30 days, see [Usage history with Prometheus](/guide/usage-history).

## Prometheus metrics

Scrape `http://valetkey.valetkey-system:8081/metrics`, or enable the chart's `serviceMonitor` if you use the Prometheus Operator. A ready-made Grafana dashboard ships with the chart (`grafanaDashboard.enabled`), see [Usage history](/guide/usage-history#_3-add-the-grafana-dashboard).

| Metric | Type | Labels |
|---|---|---|
| `valetkey_requests_total` | Counter | channel, model, group, proxykey_ns, proxykey_name, status_code |
| `valetkey_tokens_total` | Counter | channel, model, group, type |
| `valetkey_errors_total` | Counter | channel, model, group, error_type |
| `valetkey_rate_limited_total` | Counter | group, tier |
| `valetkey_request_duration_seconds` | Histogram | channel, model |
| `valetkey_ttfb_seconds` | Histogram | channel, model |
| `valetkey_upstream_duration_seconds` | Histogram | channel, model |
| `valetkey_retries_total` | Counter | channel |
| `valetkey_channel_errors_total` | Counter | channel |
| `valetkey_keys_minted_total` | Counter | group |
| `valetkey_cost_usd_total` | Counter | channel, model, group, proxykey_ns, proxykey_name |
| `valetkey_budget_spent_usd` | Gauge | scope (key, tenant), name |
| `valetkey_budget_usd` | Gauge | scope, name |
| `valetkey_budget_rejections_total` | Counter | group, scope |
| `valetkey_pricing_fetched_timestamp_seconds` | Gauge | |
| `valetkey_pricing_source_up` | Gauge | source |
| `valetkey_channels_active` | Gauge | type |
| `valetkey_proxykeys_active` | Gauge | group |

Plus the standard Go and process metrics. `channel` is the provider and `group` is the tenant.

### Example queries

```promql
# Tokens per tenant over the last day
sum by (group) (increase(valetkey_tokens_total[1d]))

# Error rate per provider
sum by (channel) (rate(valetkey_requests_total{status_code=~"5.."}[5m]))
  / sum by (channel) (rate(valetkey_requests_total[5m]))
```

## Alerts

With the Prometheus Operator, `prometheusRule.enabled: true` adds these alerts:

| Alert | When |
|---|---|
| `ValetkeyBudgetAlmostUsed` | A key or tenant has used more than 80% of its budget |
| `ValetkeyBudgetReached` | Requests were refused because a budget is used up |
| `ValetkeyHighErrorRate` | More than 10% of requests fail with a 5xx for 10 minutes |
| `ValetkeyProviderFailing` | A provider failed more than 20 times in 10 minutes |
| `ValetkeyPricesStale` | Prices haven't been refreshed for a day |
| `ValetkeyDown` | Prometheus can't scrape Valetkey |

Without Prometheus, budgets still raise Kubernetes Events on the key or tenant (`BudgetAlmostUsed` at 80%, `BudgetReached` when used up):

```bash
kubectl get events -A --field-selector reason=BudgetReached
```

## Health

- `GET /healthz`: the process is up.
- `GET /readyz`: returns 503 until Valetkey has loaded every provider, tenant and key, so no traffic arrives before it can route.

## Kubernetes status

Every resource reports a `Ready` condition:

```bash
kubectl get chan,grp -n valetkey-system
kubectl get pkey -A
```
