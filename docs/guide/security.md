# Security & audit trail

## Two ports, two audiences

| Port | Service | Serves | Who should reach it |
|---|---|---|---|
| 8000 | `llmproxy` | `/v1/*` only | Your apps |
| 8081 | `llmproxy-admin` | dashboard, admin API, `/healthz`, `/readyz`, `/metrics` | Admins (port-forward) and Prometheus |

Nothing admin-related is reachable on 8000. Keep 8081 private: use `kubectl port-forward`, and optionally apply [`deploy/networkpolicy.yaml`](https://github.com/anas1412/llmp/blob/main/deploy/networkpolicy.yaml) so only namespaces you label (e.g. monitoring) can connect. If you expose the dashboard through an Ingress, use TLS and a strong admin key.

## Admin sign-in

- The admin key comes from the `llmproxy-admin` Secret. Without it the dashboard and admin API are off.
- After **5 wrong keys** from one IP, sign-in is locked for **15 minutes**.
- Sessions last **8 hours**. **Sign out** revokes the session on the server, and changing `ADMIN_KEY` ends every session.
- Scripts can call the admin API with `Authorization: Bearer <ADMIN_KEY>`.

## Audit trail

Every admin change is recorded: creating, editing or deleting a provider, tenant or key, revealing a key, rotating a key, and every sign-in (successful or failed). You can see them in the dashboard under **Activity → Admin**, and they're written to the pod log as JSON:

```json
{"level":"INFO","msg":"admin action","action":"rotated key research/notebook","via":"dashboard","ip":"10.244.0.1"}
```

The dashboard view resets when the pod restarts; your log pipeline keeps the pod log.

<Shot name="activity" alt="Activity with admin events" />

## What's stored where

- **Provider keys**: Kubernetes Secrets in the operator namespace.
- **Virtual keys**: a Secret per key in the tenant's namespace; only a SHA-256 hash is kept in the key's status.
- **Request log**: metadata only (time, key, model, status, tokens, latency). Prompts and responses are never stored or logged.

## Hardening built in

- Runs as a non-root user, read-only filesystem, all Linux capabilities dropped, on a distroless image with no shell.
- The admin port sends a strict Content-Security-Policy, `X-Frame-Options: DENY`, `nosniff` and `no-referrer`; the session cookie is `HttpOnly` and `SameSite=Strict`.
- Only a short list of client headers is forwarded to providers; cookies, auth headers and `X-Forwarded-*` are dropped.
