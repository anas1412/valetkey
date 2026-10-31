# Helm values

```bash
helm install valetkey oci://ghcr.io/anas1412/charts/valetkey \
  -n valetkey-system --create-namespace -f values.yaml
```

The chart installs the custom resource definitions (from its `crds/` folder), RBAC, the Deployment and the `valetkey` Service. Helm installs CRDs on first install but doesn't upgrade them; after an upgrade that changes them, apply them again:

```bash
helm show crds oci://ghcr.io/anas1412/charts/valetkey | kubectl apply --server-side -f -
```

## Values

| Key | Default | Description |
|---|---|---|
| `image.repository` | `ghcr.io/anas1412/valetkey` | Container image |
| `image.tag` | chart `appVersion` | Image tag |
| `image.pullPolicy` | `IfNotPresent` | |
| `proxyURL` | `http://<release>.<namespace>.svc.cluster.local:8000` | URL written into key Secrets and shown in examples |
| `trustProxy` | `0` | Number of reverse proxies in front of Valetkey; with N > 0 the client IP comes from `X-Forwarded-For` |
| `service.type` | `ClusterIP` | |
| `service.proxyPort` | `8000` | |
| `service.metricsPort` | `8081` | |
| `resources` | 20m / 32Mi requests, 256Mi limit | |
| `replicas` | `1` | Pods serving requests; see [Scaling](/guide/limits#scaling) |
| `prometheusRule.enabled` | `false` | Create a Prometheus Operator `PrometheusRule` with Valetkey's alerts |
| `prometheusRule.labels` | `{}` | Extra labels, e.g. for your Prometheus rule selector |
| `prometheusRule.providerErrors` | `20` | Failed upstream attempts per provider in 10 minutes before alerting |
| `pricing.refreshInterval` | `6h` | How often live prices are fetched; `"off"` uses the built-in prices only. See [Budgets & costs](/guide/budgets) |
| `prometheus.url` | `""` | Prometheus that scrapes Valetkey. The dashboard then reads usage (24 h, 7 d, 30 d) from it, see [Usage history](/guide/usage-history) |
| `grafanaDashboard.enabled` | `false` | Create a ConfigMap with the Grafana dashboard for Grafana's sidecar |
| `grafanaDashboard.namespace` | release namespace | Namespace your Grafana watches |
| `grafanaDashboard.labels` | `grafana_dashboard: "1"` | Labels the sidecar selects on |
| `grafanaDashboard.annotations` | `{}` | e.g. `grafana_folder: Valetkey` |
| `networkPolicy.enabled` | `false` | Allow the proxy port from anywhere in the cluster, metrics only from labelled namespaces |
| `networkPolicy.metricsNamespaceLabel` | `valetkey.dev/metrics: "true"` | Label your monitoring namespace with this |
| `serviceMonitor.enabled` | `false` | Create a Prometheus Operator `ServiceMonitor` |
| `serviceMonitor.interval` | `30s` | |
| `serviceMonitor.labels` | `{}` | Extra labels, e.g. for your Prometheus selector |
| `podAnnotations`, `nodeSelector`, `tolerations`, `affinity` | empty | Standard pod settings |

See [Scaling](/guide/limits#scaling) for running more than one replica.

## Upgrading

Helm installs the CRDs on the first install and never updates them. Apply the new version's CRDs before upgrading, so new fields (like budgets) are kept:

```bash
helm show crds oci://ghcr.io/anas1412/charts/valetkey | kubectl apply --server-side -f -
helm upgrade valetkey oci://ghcr.io/anas1412/charts/valetkey -n valetkey-system
```

## Without Helm

Each release also ships plain manifests rendered from the chart:

```bash
kubectl create namespace valetkey-system
kubectl apply -n valetkey-system -f https://github.com/anas1412/valetkey/releases/latest/download/valetkey.yaml
```

## GitOps

Argo CD and Flux can install the chart straight from the OCI registry. For example with Flux:

```yaml
apiVersion: source.toolkit.fluxcd.io/v1
kind: OCIRepository
metadata: { name: valetkey, namespace: flux-system }
spec:
  url: oci://ghcr.io/anas1412/charts/valetkey
  ref: { semver: "0.1.x" }
  interval: 1h
---
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata: { name: valetkey, namespace: valetkey-system }
spec:
  chartRef: { kind: OCIRepository, name: valetkey, namespace: flux-system }
  interval: 1h
  install: { crds: Create }
  upgrade: { crds: CreateReplace }
```
