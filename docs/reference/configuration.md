# Configuration

With Helm, configure LLMP through [Helm values](/reference/helm). Underneath, the binary reads these environment variables:

| Variable | Default | Description |
|---|---|---|
| `OPERATOR_NAMESPACE` | `llmproxy-system` | Namespace for providers, tenants and provider-key Secrets (set from the pod's namespace) |
| `PROXY_URL` | `http://llmproxy.llmproxy-system.svc.cluster.local:8000` | URL written into key Secrets and shown in the dashboard's examples |
| `PROXY_PORT` | `8000` | Tenant-facing port |
| `METRICS_PORT` | `8081` | Health checks and Prometheus metrics |
| `ADMIN_ADDR` | `127.0.0.1:8082` | Dashboard and admin API. Loopback by default, so only `kubectl port-forward` reaches it |
| `TRUST_PROXY` | none | Number of reverse proxies in front of LLMP (e.g. `1` behind one ingress). The client IP for allow lists is then read from `X-Forwarded-For` |

## Ports

| Port | Exposed by | Paths |
|---|---|---|
| 8000 | Service `llmproxy` | `/v1/chat/completions`, `/v1/messages`, `/v1/models` |
| 8081 | Service `llmproxy` | `/healthz`, `/readyz`, `/metrics` |
| 8082 | nothing (pod loopback) | `/` (dashboard), `/api/v1/*` |

## Resources

The chart requests 20m CPU and 32 Mi memory with a 256 Mi limit. An idle pod uses about 15 MB.

## RBAC

The service account can:

- read and write `proxykeys` in every namespace, and their status;
- create, update and delete Secrets in every namespace (key Secrets have generated names, so they can't be narrowed with `resourceNames`);
- list namespaces (for the dashboard's tenant picker);
- read and write `channels` and `groups` and their status, and Secrets, in its own namespace.

See [`charts/llmp/templates/rbac.yaml`](https://github.com/anas1412/llmp/blob/main/charts/llmp/templates/rbac.yaml).
