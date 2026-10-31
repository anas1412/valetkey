# Admin API

The dashboard uses this API; you can script it too. It listens on `127.0.0.1:8082` inside the pod, so you reach it through a port-forward. There's no password: access is controlled by who may port-forward to the pod (Kubernetes RBAC).

Every request needs the header `X-Valetkey-Request: 1` and must target `localhost` (see [Security](/guide/security#the-admin-api-without-a-password)). Errors are JSON: `{"message": "…"}`.

```bash
kubectl port-forward -n valetkey-system deploy/valetkey 8082:8082 &
curl -H "X-Valetkey-Request: 1" http://localhost:8082/api/v1/proxykeys
```

## Overview

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/overview` | Totals, 24 h series, traffic share, provider health, counts |
| `GET` | `/api/v1/activity` | Last 500 requests and recent events (newest first) |
| `GET` | `/api/v1/namespaces` | Namespaces, flagging which already have a tenant |

## Providers

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/channels` | List |
| `POST` | `/api/v1/channels` | Create: `{"name", "spec", "apiKey"}`; `apiKey` is stored in a managed Secret |
| `GET` / `PUT` / `DELETE` | `/api/v1/channels/{name}` | Read, replace spec (optional new `apiKey`), delete |
| `POST` | `/api/v1/channels/{name}/test` | Check the key by listing the provider's models |
| `GET` | `/api/v1/channels/{name}/yaml` | The Channel as YAML, without status or server-managed fields (its key Secret is never included) |
| `POST` | `/api/v1/models/discover` | List a provider's models before saving it: `{"spec", "apiKey"}` or `{"spec", "channel"}` |

## Tenants

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/groups` | List |
| `POST` | `/api/v1/groups` | Create: `{"name", "spec"}` |
| `GET` / `PUT` / `DELETE` | `/api/v1/groups/{name}` | Read, replace spec, delete |
| `GET` | `/api/v1/groups/{name}/models` | Every model the tenant's keys can use |
| `GET` | `/api/v1/groups/{name}/yaml` | The tenant's whole file as YAML: its Namespace, Group and ProxyKeys, without status or server-managed fields |

## API keys

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/proxykeys` | List (all namespaces) |
| `POST` | `/api/v1/proxykeys` | Create: `{"namespace", "name", "spec"}` |
| `GET` / `PUT` / `DELETE` | `/api/v1/proxykeys/{ns}/{name}` | Read, replace spec, delete |
| `GET` | `/api/v1/proxykeys/{ns}/{name}/key` | Reveal the key (audited) |
| `POST` | `/api/v1/proxykeys/{ns}/{name}/rotate` | Issue a new key; the old one stops working (audited) |
| `GET` | `/api/v1/proxykeys/{ns}/{name}/yaml` | The ProxyKey as YAML, without status or server-managed fields |

Every write is recorded in the [audit trail](/guide/security#audit-trail).
