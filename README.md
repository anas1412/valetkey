<p align="center">
  <img src="docs/public/logo.svg" width="72" alt="LLMP logo">
</p>

<h1 align="center">LLMP</h1>

<p align="center">
  A Kubernetes-native LLM proxy. Give each tenant its own API key for OpenAI, Anthropic or any OpenAI-compatible model,<br>
  with limits, a dashboard and an audit trail. Your real provider keys never leave the cluster.
</p>

<p align="center">
  <a href="https://anas.cyberia.tn/llmp/"><b>Documentation</b></a> ·
  <a href="https://anas.cyberia.tn/llmp/guide/quick-start">Quick start</a> ·
  <a href="https://anas.cyberia.tn/llmp/guide/dashboard">Dashboard tour (video)</a>
</p>

<p align="center">
  <a href="https://anas.cyberia.tn/llmp/guide/dashboard">
    <img src="docs/public/screenshots/overview-dark.png" alt="LLMP dashboard" width="900">
  </a>
</p>

## Why

- **Provider keys stay secret.** Tenants get their own `sk-proxy-…` keys; disable, rotate or delete one without touching the provider.
- **Any provider.** OpenAI, Anthropic, and anything OpenAI-compatible (vLLM, Ollama, OpenRouter, Groq, DeepSeek, Azure). OpenAI clients can use Anthropic models too.
- **Rules per key.** Allowed models, IP ranges, requests and tokens per minute, scaled per tenant. Automatic failover between providers.
- **Built-in dashboard.** Add a provider by pasting its key and ticking models from its own list; create keys with copy-paste examples; watch usage and activity.
- **Audited admin access.** Sign-in lockout, revocable sessions, strict browser headers, and every admin change in the activity log and pod log.
- **Small and boring to run.** One 32 MB distroless image, about 15 MB of memory, no database, no Redis. State lives in CRDs and Secrets.

## How it works

```
Tenant app ──POST /v1/chat/completions──▶ LLMP ──▶ OpenAI / Anthropic / vLLM / Ollama / …
              (sk-proxy-… virtual key)      checks the key, model, IP and limits,
                                            then swaps in the real provider key
```

| You create | Kind | What it is |
|---|---|---|
| Provider | `Channel` | An upstream API and its real key |
| Tenant | `Group` | A namespace allowed to use some providers |
| API key | `ProxyKey` | A virtual key in the tenant's namespace; LLMP writes it into a Secret |

All three can be managed from the dashboard or with `kubectl`.

## Quick start

```bash
kubectl create namespace llmproxy-system
kubectl apply -f https://raw.githubusercontent.com/anas1412/llmp/main/deploy/crds.yaml
kubectl apply -f https://raw.githubusercontent.com/anas1412/llmp/main/deploy/operator.yaml

# the dashboard stays off until you set an admin key
kubectl create secret generic llmproxy-admin -n llmproxy-system --from-literal=adminKey=CHANGE-ME
kubectl rollout restart deploy/llmproxy -n llmproxy-system

kubectl port-forward -n llmproxy-system svc/llmproxy-admin 8081:8081
```

Open <http://localhost:8081>, sign in, and follow the three steps: add a provider, create a tenant, create a key. Then call it like any OpenAI endpoint:

```bash
curl http://llmproxy.llmproxy-system:8000/v1/chat/completions \
  -H "Authorization: Bearer sk-proxy-YOUR_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Hello"}]}'
```

The [documentation](https://anas.cyberia.tn/llmp/) covers providers, tenants, keys, SDK usage, limits, security, monitoring, the custom resources and the admin API.

## Development

One Go binary with the dashboard embedded. Needs Go 1.26+ and Node.js 24+.

```bash
cd web && npm ci && npm run build && cd ..
go test ./internal/...
go run ./cmd/llmproxy        # uses your current kubeconfig
```

See [Development](https://anas.cyberia.tn/llmp/reference/development) for local clusters, the docs site and regenerating screenshots and the video.

## License

[ISC](LICENSE)
