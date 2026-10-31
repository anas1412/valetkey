# Usage history with Prometheus

On its own, Valetkey keeps the last 24 hours of usage in the pod's memory. That's enough for a quick look, but it's gone when the pod restarts. For history that lasts, let the Prometheus you already run scrape Valetkey, and point Valetkey at it.

Valetkey stores no prompts or responses either way, only counts: requests, tokens, errors and latency per tenant, key, provider and model.

## What you get

- **Dashboard:** the Overview gets a 24 hours / 7 days / 30 days switch and a *Prometheus* badge, and the API keys table shows each key's requests over the last 30 days. The numbers survive pod restarts.
- **Grafana:** a ready-made dashboard with requests, tokens, error rate, latency, rate-limited requests and top keys, filterable by tenant and provider.

## 1. Scrape Valetkey

Metrics are served on port 8081 at `/metrics`.

With the Prometheus Operator (kube-prometheus-stack), turn on the ServiceMonitor. Add the label your Prometheus selects on, if it uses one:

```yaml
# values.yaml
serviceMonitor:
  enabled: true
  labels:
    release: kube-prometheus-stack
```

With a plain Prometheus, add a scrape job:

```yaml
scrape_configs:
  - job_name: valetkey
    static_configs:
      - targets: [valetkey.valetkey-system.svc:8081]
```

If you enabled the chart's `networkPolicy`, label the namespace Prometheus runs in, or its scrapes are blocked:

```bash
kubectl label namespace monitoring valetkey.dev/metrics=true
```

## 2. Point Valetkey at Prometheus

```yaml
# values.yaml
prometheus:
  url: http://prometheus-operated.monitoring.svc:9090
```

Use the address of your Prometheus service (for kube-prometheus-stack it's usually `prometheus-operated` in the `monitoring` namespace). Valetkey only runs read queries against it.

If Prometheus can't be reached, the dashboard falls back to the pod's own 24 hours and shows a warning saying why.

## 3. Add the Grafana dashboard

```yaml
# values.yaml
grafanaDashboard:
  enabled: true
```

This creates a ConfigMap with the label `grafana_dashboard: "1"`, which the Grafana sidecar in kube-prometheus-stack and the Grafana Helm chart pick up by default. If your Grafana only watches its own namespace, set `grafanaDashboard.namespace` to it. To put the dashboard in a folder, add the annotation your sidecar uses, for example `grafanaDashboard.annotations.grafana_folder: Valetkey`.

No sidecar? Import [`charts/valetkey/dashboards/valetkey.json`](https://github.com/anas1412/valetkey/blob/main/charts/valetkey/dashboards/valetkey.json) in Grafana under *Dashboards > New > Import*.

## How far back it goes

As far as your Prometheus keeps data. Prometheus keeps 15 days by default, so for the 30 day view set `--storage.tsdb.retention.time=30d` or more (`prometheus.prometheusSpec.retention` in kube-prometheus-stack).

## Useful queries

```promql
# Tokens per tenant over the last day
sum by (tenant) (increase(valetkey_tokens_total[1d]))

# Requests per key over the last 30 days
sum by (proxykey_ns, proxykey_name) (increase(valetkey_requests_total[30d]))

# Share of failed requests per provider, last 5 minutes
sum by (provider) (rate(valetkey_requests_total{status_code=~"[45].."}[5m]))
  / sum by (provider) (rate(valetkey_requests_total[5m]))

# 95th percentile latency per provider
histogram_quantile(0.95, sum by (le, provider) (rate(valetkey_request_duration_seconds_bucket[5m])))

# Tokens per model this week
sum by (model) (increase(valetkey_tokens_total[7d]))
```

All metrics are listed in [Monitoring](/guide/monitoring#prometheus-metrics).

## Try it locally

The repository has a playground with Prometheus, Grafana, fake providers and steady traffic. See [Development](/reference/development#playground).
