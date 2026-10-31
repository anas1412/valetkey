# llmproxy-k8s

A lightweight Kubernetes-native LLM proxy — give your tenants API keys that forward to your LLM providers. No database, no Redis. Just CRDs, Secrets, and a built-in dashboard.

## How it works

1. **You** add providers as `Channel`s (OpenAI-compatible or Anthropic) with their real API keys
2. **You** create a `Group` per tenant, listing which channels it may use — the group name is the tenant's namespace
3. **Tenants** (or you) create `ProxyKey`s in the tenant namespace
4. **llmproxy** mints a virtual key (`sk-proxy-…`) into a Secret next to each ProxyKey and relays requests, swapping in the real provider key

```
Tenant app  ──POST /v1/chat/completions──▶  llmproxy  ──▶  OpenAI / Anthropic / any OpenAI-compatible API
               (virtual key)                    │
         Channel ──▶ Group ──▶ ProxyKey ──▶ Secret
         (provider)  (tenant)  (key def)    (virtual key)
```

Tenants never see your provider keys, and you can disable, rotate, rate-limit or restrict any key without touching the provider.

## Quick start

### 1. Install

```bash
kubectl create namespace llmproxy-system
kubectl apply -f deploy/crds.yaml
kubectl apply -f deploy/operator.yaml
```

### 2. Set an admin key

The dashboard and admin API stay disabled until you set one. Use any long random string:

```bash
kubectl create secret generic llmproxy-admin -n llmproxy-system --from-literal=adminKey=CHANGE-ME-TO-A-LONG-RANDOM-STRING
kubectl rollout restart deploy/llmproxy -n llmproxy-system
```

### 3. Open the dashboard

```bash
kubectl port-forward -n llmproxy-system svc/llmproxy-admin 8081:8081
```

Open <http://localhost:8081>, sign in with the admin key, and follow the three steps on the Overview page: add a provider, create a tenant, create a key. Each key comes with ready-to-paste curl / Python / TypeScript snippets.

### Or do it with kubectl

```bash
# Provider key + channel
kubectl create secret generic openai-real-key -n llmproxy-system --from-literal=apiKey=sk-YOUR_REAL_KEY
cat <<EOF | kubectl apply -f -
apiVersion: llmproxy.llmproxy.io/v1alpha1
kind: Channel
metadata: { name: openai, namespace: llmproxy-system }
spec:
  type: openai
  baseURL: https://api.openai.com
  keySecretRef: { name: openai-real-key }
  models: [gpt-4o, gpt-4o-mini]
---
apiVersion: llmproxy.llmproxy.io/v1alpha1
kind: Group
metadata: { name: my-team, namespace: llmproxy-system }   # name = tenant namespace
spec:
  channelRefs: [openai]
EOF

# Tenant key
kubectl create namespace my-team
cat <<EOF | kubectl apply -f -
apiVersion: llmproxy.llmproxy.io/v1alpha1
kind: ProxyKey
metadata: { name: my-app, namespace: my-team }
spec:
  models: [gpt-4o-mini]
EOF

# Read the virtual key
kubectl get secret my-app-llmproxy -n my-team -o jsonpath='{.data.LLMPROXY_KEY}' | base64 -d
```

See [deploy/examples.yaml](deploy/examples.yaml) for every field.

### Use it

```bash
curl http://llmproxy.llmproxy-system:8000/v1/chat/completions \
  -H "Authorization: Bearer sk-proxy-YOUR_VIRTUAL_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hello"}]}'
```

Any OpenAI SDK works by setting its base URL to `http://llmproxy.llmproxy-system:8000/v1`. Anthropic SDKs work against `/v1/messages`.

## Features

- **No database, no Redis** — state lives in CRDs and Secrets
- **Dashboard** — providers, tenants, keys, live usage and activity, on an internal port
- **Multi-tenancy** — a `Group` per tenant namespace with its own channel pool
- **Virtual API keys** — minted into the tenant's namespace, rotatable at any time
- **Model allowlists** per key; **IP allow lists** (IPv4 and IPv6 CIDR) per key
- **Rate limits** — per-key requests/tokens per minute, scaled per tenant with `rpmRatio` / `tpmRatio`
- **Priority-weighted routing** with **automatic failover** on connection errors, 429 and 5xx
- **OpenAI ↔ Anthropic** — OpenAI clients can use Anthropic channels (requests, responses, streaming, tools and images are translated); Anthropic clients get native passthrough on `/v1/messages`
- **Model mapping** — rename models per channel (e.g. to Azure deployment names)
- **Per-channel outbound proxy** and optional TLS verification skip for self-hosted endpoints
- **SSE streaming** passes through unbuffered
- **Prometheus metrics** — requests, tokens, latency, errors, retries, rate limiting
- **Kubernetes-style status** — `Ready` conditions on every resource (`kubectl get pkey,chan,grp -A`)

## Resources

| Kind | Lives in | Purpose |
|------|----------|---------|
| `Channel` | operator namespace | An upstream provider: `type` (`openai` / `anthropic`), `baseURL`, `keySecretRef`, `models`, `priority`, `modelMapping`, `proxyURL`, `skipTLSVerify` |
| `Group` | operator namespace | A tenant. Its name is the tenant namespace. `channelRefs`, `status`, `rpmRatio`, `tpmRatio` |
| `ProxyKey` | tenant namespace | A virtual key: `status`, `models`, `subnets`, `rpm`, `tpm` |

### ProxyKey Secret

For each ProxyKey, llmproxy creates `<name>-llmproxy` in the same namespace:

| Key | Value |
|-----|-------|
| `LLMPROXY_KEY` | The virtual API key (`sk-proxy-...`) |
| `LLMPROXY_ENDPOINT` | URL to reach the proxy |
| `LLMPROXY_CHANNEL` | First channel of the tenant's group at mint time |
| `LLMPROXY_GROUP` | The tenant (group) name |

The Secret is owned by the ProxyKey, so deleting the ProxyKey deletes it. Deleting just the Secret (or **Rotate** in the dashboard) issues a new key; the old one stops working.

### IP allow lists

```yaml
spec:
  subnets: [10.0.0.0/8, 2001:db8::/32, 192.168.1.5]
```

The proxy checks the address the connection comes from. Behind an ingress or load balancer, set `TRUST_PROXY` (see below) so the real client address from `X-Forwarded-For` is used; for a `LoadBalancer` Service, `externalTrafficPolicy: Local` keeps the client address.

### Rate limits

```yaml
spec:
  rpm: 60        # requests per minute
  tpm: 100000    # tokens per minute
```

The tenant's `rpmRatio` / `tpmRatio` multiply these. Limits are counted in memory per pod and reset on restart. Token counts come from the provider's usage data; for OpenAI streaming responses that means the client must send `stream_options: {"include_usage": true}`.

## Ports and security

| Port | Service | Serves |
|------|---------|--------|
| 8000 | `llmproxy` | `/v1/*` only — tenant traffic |
| 8081 | `llmproxy-admin` | dashboard, `/api/v1/*` admin API, `/healthz`, `/readyz`, `/metrics` |

Keep 8081 private: reach it with `kubectl port-forward`, and optionally apply [deploy/networkpolicy.yaml](deploy/networkpolicy.yaml) so only namespaces you label (e.g. monitoring) can connect. If you expose the dashboard through an Ingress, use TLS and a strong admin key.

The admin API accepts the dashboard's session cookie or `Authorization: Bearer <ADMIN_KEY>`.

- **Sign-in protection:** after 5 wrong keys from one IP, sign-in is locked for 15 minutes. Sessions last 8 hours, signing out revokes the session, and changing `ADMIN_KEY` ends every session.
- **Audit trail:** every admin change (create, update, delete, key reveal, key rotation) and every sign-in is shown under Activity → Admin and written to the pod log as JSON (`"msg":"admin action"`), so your log pipeline keeps it after restarts.
- **Browser hardening:** the admin port sends a strict Content-Security-Policy, `X-Frame-Options: DENY`, `nosniff` and `no-referrer`; the session cookie is `HttpOnly` and `SameSite=Strict`.
- **Privacy:** request logs keep metadata only (time, key, model, status, tokens, latency), never prompts or responses.

## API reference

### Relay (port 8000, virtual key)

| Endpoint | Description |
|----------|-------------|
| `POST /v1/chat/completions` | OpenAI-compatible chat (any channel type) |
| `POST /v1/messages` | Anthropic Messages API (Anthropic channels) |
| `GET /v1/models` | Models the calling key can use |

Keys are accepted as `Authorization: Bearer …` or `x-api-key: …`.

### Admin (port 8081, admin key)

| Endpoint | Description |
|----------|-------------|
| `POST /api/v1/session` | Dashboard sign-in `{ "key": "…" }` |
| `GET /api/v1/overview` | Totals, 24 h series, channel health |
| `GET /api/v1/activity` | Recent requests and operator events |
| `GET /api/v1/status` | Compact summary (used by `llmproxyctl`) |
| `GET/POST /api/v1/channels`, `GET/PUT/DELETE /api/v1/channels/:name` | Channels; `POST` / `PUT` accept `apiKey` to store the provider key |
| `POST /api/v1/channels/:name/test` | Check that the provider accepts the key |
| `GET/POST /api/v1/groups`, `GET/PUT/DELETE /api/v1/groups/:name` | Groups |
| `GET/POST /api/v1/proxykeys`, `GET/PUT/DELETE /api/v1/proxykeys/:ns/:name` | ProxyKeys |
| `GET /api/v1/proxykeys/:ns/:name/key` | Reveal a virtual key |
| `POST /api/v1/proxykeys/:ns/:name/rotate` | Issue a new virtual key |
| `GET /api/v1/namespaces` | Namespaces, for the tenant picker |

## CLI (llmproxyctl)

An optional script in [scripts/llmproxyctl](scripts/llmproxyctl) for terminals (needs bash, curl, jq). It is not in the image, which ships only Node and the app. Run it against a port-forward to 8081:

```bash
kubectl port-forward -n llmproxy-system svc/llmproxy-admin 8081:8081 &
export LLMPROXY_ADMIN_KEY=...
./scripts/llmproxyctl status
./scripts/llmproxyctl test channel openai
```

## Prometheus metrics

Scrape `http://llmproxy-admin.llmproxy-system:8081/metrics`:

| Metric | Type | Labels |
|--------|------|--------|
| `llmproxy_requests_total` | Counter | channel, model, group, proxykey_ns, proxykey_name, status_code |
| `llmproxy_tokens_total` | Counter | channel, model, group, type |
| `llmproxy_errors_total` | Counter | channel, model, group, error_type |
| `llmproxy_rate_limited_total` | Counter | group, tier |
| `llmproxy_request_duration_seconds` | Histogram | channel, model |
| `llmproxy_ttfb_seconds` | Histogram | channel, model |
| `llmproxy_upstream_duration_seconds` | Histogram | channel, model |
| `llmproxy_retries_total` | Counter | channel |
| `llmproxy_channel_errors_total` | Counter | channel |
| `llmproxy_keys_minted_total` | Counter | group |
| `llmproxy_channels_active` | Gauge | type |
| `llmproxy_proxykeys_active` | Gauge | group |

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `OPERATOR_NAMESPACE` | `llmproxy-system` | Namespace for channels, groups and provider secrets |
| `PROXY_URL` | `http://llmproxy.llmproxy-system.svc.cluster.local:8000` | URL written into tenant secrets and shown in snippets |
| `PROXY_PORT` | `8000` | Tenant-facing port |
| `ADMIN_PORT` | `8081` | Dashboard, admin API, health and metrics |
| `ADMIN_KEY` | — | Admin key; unset disables the dashboard and admin API |
| `TRUST_PROXY` | — | Number of reverse proxies in front of llmproxy (e.g. `1` behind one ingress); the client IP is then read from `X-Forwarded-For` |

## Limits

- Run **one replica**: the reconciler has no leader election, and rate limits and dashboard history are in memory.
- Dashboard history (last 24 h, last 500 requests) resets when the pod restarts. Use Prometheus for long-term data.
- `/v1/messages` only routes to Anthropic channels.

## Development

llmproxy is a single Go binary with the dashboard embedded. You need Go 1.26+ and Node.js 24+ (only to build the dashboard).

```bash
cd web && npm ci && npm run build && cd ..   # dashboard → web/dist (embedded into the binary)
go test ./internal/...                        # unit, relay and admin API tests, no cluster needed
go run ./cmd/llmproxy                         # uses your current kubeconfig
```

Dashboard development with hot reload: run the operator as above, then `cd web && npm run dev` and open <http://localhost:5181> (API calls are proxied to `:8081`).

### Local cluster test

Works with [kind](https://kind.sigs.k8s.io) or [k3d](https://k3d.io) on Windows, macOS and Linux:

```bash
kind create cluster --name llmproxy
docker build -t llmproxy-k8s:dev .
kind load docker-image llmproxy-k8s:dev --name llmproxy
kubectl create namespace llmproxy-system
kubectl apply -f deploy/crds.yaml -f deploy/operator.yaml
kubectl set image -n llmproxy-system deploy/llmproxy llmproxy=llmproxy-k8s:dev
```

Then follow the quick start from step 2.

## Footprint

The image is distroless: one static binary (~28 MB) plus CA certificates, running as a non-root user with no shell. Memory use at idle is a few tens of MB.
