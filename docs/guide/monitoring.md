# Monitoring

## Dashboard

The [Overview](/guide/dashboard#overview) and [Activity](/guide/dashboard#activity) pages show the last 24 hours on the running pod.

## Prometheus metrics

Scrape `http://llmproxy.llmproxy-system:8081/metrics`, or enable the chart's `serviceMonitor` if you use the Prometheus Operator:

| Metric | Type | Labels |
|---|---|---|
| `llmproxy_requests_total` | Counter | channel, model, group, proxykey_ns, proxykey_name, status_code |
| `llmproxy_tokens_total` | Counter | channel, model, group, type |
| `llmproxy_errors_total` | Counter | channel, model, group, error_type |
| `llmproxy_rate_limited_total` | Counter | group, tier |
| `llmproxy_request_duration_seconds` | Histogram | channel, model |
| `llmproxy_ttfb_seconds` | Histogram | channel, model |
| `llmproxy_upstream_duration_seconds` | Histogram | channel, model |
| `llmproxy_retries_total` | Counter | channel |
| `llmproxy_channel_errors_total` | Counter | channel |
| `llmproxy_keys_minted_total` | Counter | group |
| `llmproxy_channels_active` | Gauge | type |
| `llmproxy_proxykeys_active` | Gauge | group |

Plus the standard Go and process metrics. `channel` is the provider and `group` is the tenant.

### Example queries

```promql
# Tokens per tenant over the last day
sum by (group) (increase(llmproxy_tokens_total[1d]))

# Error rate per provider
sum by (channel) (rate(llmproxy_requests_total{status_code=~"5.."}[5m]))
  / sum by (channel) (rate(llmproxy_requests_total[5m]))
```

## Health

- `GET /healthz`: the process is up.
- `GET /readyz`: returns 503 until LLMP has loaded every provider, tenant and key, so no traffic arrives before it can route.

## Kubernetes status

Every resource reports a `Ready` condition:

```bash
kubectl get chan,grp -n llmproxy-system
kubectl get pkey -A
```
