# Configuration

LLMP is configured with environment variables on the `llmproxy` Deployment.

| Variable | Default | Description |
|---|---|---|
| `OPERATOR_NAMESPACE` | `llmproxy-system` | Namespace for providers, tenants and provider-key Secrets (set from the pod's namespace in the manifest) |
| `PROXY_URL` | `http://llmproxy.llmproxy-system.svc.cluster.local:8000` | URL written into key Secrets and shown in the dashboard's examples |
| `PROXY_PORT` | `8000` | Tenant-facing port |
| `ADMIN_PORT` | `8081` | Dashboard, admin API, health and metrics |
| `ADMIN_KEY` | none | Admin key (from the `llmproxy-admin` Secret). Unset turns the dashboard and admin API off. Surrounding whitespace is ignored. |
| `TRUST_PROXY` | none | Number of reverse proxies in front of LLMP (e.g. `1` behind one ingress). The client IP for allow lists is then read from `X-Forwarded-For`. |

## Ports and services

| Port | Service | Paths |
|---|---|---|
| 8000 | `llmproxy` | `/v1/chat/completions`, `/v1/messages`, `/v1/models` |
| 8081 | `llmproxy-admin` | `/` (dashboard), `/api/v1/*`, `/healthz`, `/readyz`, `/metrics` |

## Resources

The default manifest requests 20m CPU and 32 Mi memory with a 256 Mi limit. An idle pod uses about 15 MB.

## RBAC

The service account can:

- read and write `proxykeys` in every namespace, and their status;
- create, update and delete Secrets in every namespace (key Secrets have generated names, so they can't be narrowed with `resourceNames`);
- list namespaces (for the dashboard's tenant picker);
- read and write `channels` and `groups` and their status, and Secrets, in the operator namespace.

See [`deploy/operator.yaml`](https://github.com/anas1412/llmp/blob/main/deploy/operator.yaml).
