# valetkeyctl CLI

`valetkeyctl` is a small command-line tool for the [admin API](/reference/admin-api): status at a glance, provider checks, and showing or rotating keys. To create or change providers, tenants and keys from the terminal, use [kubectl](/guide/kubectl).

## Install

It's a single bash script that needs `bash`, `curl`, `jq` and `kubectl`. It runs on Linux and macOS, and on Windows through WSL or Git Bash.

```bash
curl -fsSLo valetkeyctl https://raw.githubusercontent.com/anas1412/valetkey/main/scripts/valetkeyctl
chmod +x valetkeyctl
sudo mv valetkeyctl /usr/local/bin/     # or anywhere on your PATH
```

## How it connects

There's nothing to configure. Each command opens a `kubectl port-forward` to the Valetkey pod using your current kubeconfig, makes its call, and closes it again. If your Kubernetes user may port-forward to the pod, `valetkeyctl` works; if not, it doesn't. There is no separate password.

| Variable | Default | |
|---|---|---|
| `VALETKEY_NAMESPACE` | `valetkey-system` | Namespace Valetkey runs in |
| `VALETKEY_TARGET` | `deploy/valetkey` | What to port-forward to |
| `VALETKEY_SERVER` | none | Use this admin URL instead of starting a port-forward (e.g. `http://localhost:8082`) |

## Status

```bash
valetkeyctl status
```

```
channels: 1  groups: 1  keys: 1

CHANNEL  TYPE    READY
openai   openai  yes

GROUP    STATUS   CHANNELS
billing  Enabled  openai

KEY                  READY
billing/invoice-api  yes
```

Narrow it down with `status channels`, `status groups` or `status keys`:

```bash
valetkeyctl status keys
```

```
NAMESPACE  NAME         READY
billing    invoice-api  yes
```

## Check a provider

Asks the provider for its model list with the real key, without sending a prompt:

```bash
valetkeyctl test channel openai
```

```json
{
  "latencyMs": 5,
  "message": "reachable, key accepted, 3 models",
  "status": 200,
  "success": true
}
```

## Models a tenant can use

```bash
valetkeyctl models billing
```

```
gpt-4o
gpt-4o-mini
```

## Show a key

```bash
valetkeyctl key billing/invoice-api
```

```
sk-proxy-…
```

Every reveal is recorded in the [audit trail](/guide/security#audit-trail).

## Rotate a key

```bash
valetkeyctl rotate billing/invoice-api
```

```
rotated billing/invoice-api; the old key no longer works
```

A new key is written to the same Secret right away, and the old one stops working.

## Open the dashboard

```bash
valetkeyctl dashboard          # http://localhost:8082, Ctrl+C to stop
```

## Details as JSON

```bash
valetkeyctl channel openai
valetkeyctl group billing
```

These print the full object as JSON, handy for piping into `jq`.

## Scripting the admin API

Anything the dashboard does, you can do with `curl` through a port-forward. Every request needs the `X-Valetkey-Request` header. For example, adding a provider and storing its key in one call:

```bash
kubectl port-forward -n valetkey-system deploy/valetkey 8082:8082 &
curl -X POST http://localhost:8082/api/v1/channels \
  -H "X-Valetkey-Request: 1" \
  -H "Content-Type: application/json" \
  -d '{"name":"groq","apiKey":"gsk-…","spec":{"type":"openai","baseURL":"https://api.groq.com/openai"}}'
```

See the [admin API reference](/reference/admin-api) for every endpoint.
