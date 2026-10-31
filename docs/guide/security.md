# Security & audit trail

## Who can do what

Valetkey doesn't have its own users or passwords. Everything is decided by Kubernetes RBAC:

| To… | You need |
|---|---|
| Add or change providers and tenants | write access to `channels` / `groups` in the Valetkey namespace |
| Issue, change or revoke keys | write access to `proxykeys` in the tenant's namespace |
| Read a tenant's key | read access to Secrets in the tenant's namespace |
| Use the dashboard or admin API | `pods/portforward` on the Valetkey pod |
| Call an LLM | a virtual key |

So the usual tools apply: grant roles to groups from your cluster's sign-in, and the Kubernetes audit log records who changed what and who opened a port-forward.

## Two network ports, one local port

| Port | Reachable from | Serves |
|---|---|---|
| 8000 | the cluster (Service `valetkey`) | `/v1/*` only: your apps' LLM calls |
| 8081 | the cluster (Service `valetkey`) | `/healthz`, `/readyz`, `/metrics` |
| 8082 | **inside the pod only** (`127.0.0.1`) | dashboard and admin API, via `kubectl port-forward` |

Nothing admin-related is reachable over the network. To also restrict who can reach 8000 and 8081, enable the chart's [network policy](/reference/helm).

## The admin API without a password

Because the admin API trusts anyone who can port-forward, it also has to make sure a request really comes from you and not from a web page you happen to have open while the port-forward runs:

- it only answers when the address is `localhost`, `127.0.0.1` or `[::1]`, which blocks DNS-rebinding pages;
- every request must carry an `X-Valetkey-Request` header, which other websites can't add, which blocks cross-site requests.

The dashboard sends the header for you. Scripts using `curl` add `-H "X-Valetkey-Request: 1"`.

## Audit trail

Admin changes made through the dashboard or admin API are recorded: creating, editing or deleting a provider, tenant or key, revealing a key and rotating a key. They show under **Activity → Admin** in the dashboard and are written to the pod log as JSON:

```json
{"level":"INFO","msg":"admin action","action":"rotated key billing/invoice-api"}
```

Who did it is in the Kubernetes audit log (the user that opened the port-forward). Changes made with `kubectl` or GitOps are in the Kubernetes audit log directly.

## What's stored where

- **Provider keys**: Kubernetes Secrets in the Valetkey namespace.
- **Virtual keys**: a Secret per key in the tenant's namespace; only a SHA-256 hash is kept in the key's status.
- **Request log**: metadata only (time, key, model, status, tokens, latency). Prompts and responses are never stored or logged.

## Hardening built in

- Runs as a non-root user, read-only filesystem, all Linux capabilities dropped, on a distroless image with no shell.
- The dashboard sends a strict Content-Security-Policy, `X-Frame-Options: DENY`, `nosniff` and `no-referrer`.
- Only a short list of client headers is forwarded to providers; cookies, auth headers and `X-Forwarded-*` are dropped.
