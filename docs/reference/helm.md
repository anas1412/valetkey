# Helm values

```bash
helm install llmproxy oci://ghcr.io/anas1412/charts/llmp --version 0.1.0 \
  -n llmproxy-system --create-namespace -f values.yaml
```

The chart installs the custom resource definitions (from its `crds/` folder), RBAC, the Deployment and the `llmproxy` Service. Helm installs CRDs on first install but doesn't upgrade them; after an upgrade that changes them, apply them again:

```bash
helm show crds oci://ghcr.io/anas1412/charts/llmp --version <new> | kubectl apply -f -
```

## Values

| Key | Default | Description |
|---|---|---|
| `image.repository` | `ghcr.io/anas1412/llmp` | Container image |
| `image.tag` | chart `appVersion` | Image tag |
| `image.pullPolicy` | `IfNotPresent` | |
| `proxyURL` | `http://<release>.<namespace>.svc.cluster.local:8000` | URL written into key Secrets and shown in examples |
| `trustProxy` | `0` | Number of reverse proxies in front of LLMP; with N > 0 the client IP comes from `X-Forwarded-For` |
| `service.type` | `ClusterIP` | |
| `service.proxyPort` | `8000` | |
| `service.metricsPort` | `8081` | |
| `resources` | 20m / 32Mi requests, 256Mi limit | |
| `networkPolicy.enabled` | `false` | Allow the proxy port from anywhere in the cluster, metrics only from labelled namespaces |
| `networkPolicy.metricsNamespaceLabel` | `llmproxy.llmproxy.io/metrics: "true"` | Label your monitoring namespace with this |
| `serviceMonitor.enabled` | `false` | Create a Prometheus Operator `ServiceMonitor` |
| `serviceMonitor.interval` | `30s` | |
| `serviceMonitor.labels` | `{}` | Extra labels, e.g. for your Prometheus selector |
| `podAnnotations`, `nodeSelector`, `tolerations`, `affinity` | empty | Standard pod settings |

The release runs one replica on purpose: see [Limits](/guide/limits#scaling).

## Without Helm

Each release also ships plain manifests rendered from the chart:

```bash
kubectl create namespace llmproxy-system
kubectl apply -n llmproxy-system -f https://github.com/anas1412/llmp/releases/download/v0.1.0/llmp.yaml
```

## GitOps

Argo CD and Flux can install the chart straight from the OCI registry. For example with Flux:

```yaml
apiVersion: source.toolkit.fluxcd.io/v1
kind: OCIRepository
metadata: { name: llmp, namespace: flux-system }
spec:
  url: oci://ghcr.io/anas1412/charts/llmp
  ref: { semver: "0.1.x" }
  interval: 1h
---
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata: { name: llmproxy, namespace: llmproxy-system }
spec:
  chartRef: { kind: OCIRepository, name: llmp, namespace: flux-system }
  interval: 1h
  install: { crds: Create }
  upgrade: { crds: CreateReplace }
```
