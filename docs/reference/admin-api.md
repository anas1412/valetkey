# Admin API

The dashboard uses this API; you can script it too. It's served on the admin port (8081) only.

Authenticate with the dashboard's session cookie or `Authorization: Bearer <ADMIN_KEY>`. Errors are JSON: `{"message": "…"}`.

```bash
kubectl port-forward -n llmproxy-system svc/llmproxy-admin 8081:8081 &
curl -H "Authorization: Bearer $ADMIN_KEY" http://localhost:8081/api/v1/status
```

## Session

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/v1/session` | Sign in: `{"key": "…"}`. Sets the session cookie. Locked for 15 min after 5 failures per IP |
| `GET` | `/api/v1/session` | Check the session |
| `DELETE` | `/api/v1/session` | Sign out (revokes the session) |

## Overview

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/overview` | Totals, 24 h series, traffic share, provider health, counts |
| `GET` | `/api/v1/activity` | Last 500 requests and recent events (newest first) |
| `GET` | `/api/v1/status` | Compact summary (used by `llmproxyctl`) |
| `GET` | `/api/v1/namespaces` | Namespaces, flagging which already have a tenant |

## Providers

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/channels` | List |
| `POST` | `/api/v1/channels` | Create: `{"name", "spec", "apiKey"}`; `apiKey` is stored in a managed Secret |
| `GET` / `PUT` / `DELETE` | `/api/v1/channels/{name}` | Read, replace spec (optional new `apiKey`), delete |
| `POST` | `/api/v1/channels/{name}/test` | Check the key by listing the provider's models |
| `POST` | `/api/v1/models/discover` | List a provider's models before saving it: `{"spec", "apiKey"}` or `{"spec", "channel"}` |

## Tenants

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/groups` | List |
| `POST` | `/api/v1/groups` | Create: `{"name", "spec"}` |
| `GET` / `PUT` / `DELETE` | `/api/v1/groups/{name}` | Read, replace spec, delete |
| `GET` | `/api/v1/groups/{name}/models` | Every model the tenant's keys can use |

## API keys

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/proxykeys` | List (all namespaces) |
| `POST` | `/api/v1/proxykeys` | Create: `{"namespace", "name", "spec"}` |
| `GET` / `PUT` / `DELETE` | `/api/v1/proxykeys/{ns}/{name}` | Read, replace spec, delete |
| `GET` | `/api/v1/proxykeys/{ns}/{name}/key` | Reveal the key (audited) |
| `POST` | `/api/v1/proxykeys/{ns}/{name}/rotate` | Issue a new key; the old one stops working (audited) |

Every write is recorded in the [audit trail](/guide/security#audit-trail).

## CLI

[`scripts/llmproxyctl`](https://github.com/anas1412/llmp/blob/main/scripts/llmproxyctl) wraps a few read calls (needs bash, curl and jq):

```bash
export LLMPROXY_ADMIN_KEY=...
./scripts/llmproxyctl status
./scripts/llmproxyctl test channel openai
```
