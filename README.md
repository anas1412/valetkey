<p align="center">
  <img src="docs/public/logo.svg" width="72" alt="Valetkey logo">
</p>

<h1 align="center">Valetkey</h1>

<p align="center"><b>Minimal LLM Proxy for Kubernetes</b></p>

<p align="center">
  Per-tenant API keys for OpenAI, Anthropic and any OpenAI-compatible model, configured as CRDs.<br>
  Your real provider keys never leave the cluster.
</p>

<p align="center">
  <a href="https://anas.cyberia.tn/valetkey/"><b>Documentation</b></a> ·
  <a href="https://anas.cyberia.tn/valetkey/guide/quick-start">Quick start</a> ·
  <a href="https://anas.cyberia.tn/valetkey/guide/gitops">GitOps</a>
</p>

## Why

- **Provider keys stay secret.** Tenants get their own `sk-proxy-…` keys; disable, rotate or revoke one without touching the provider.
- **Configured as CRDs.** Providers, tenants and keys are Kubernetes objects: keep them in Git, deploy them with Argo CD or Flux, review them like any other manifest.
- **No passwords.** Kubernetes RBAC decides who can change what, and the Kubernetes audit log records it.
- **Admin-managed keys, no self-service.** Tenants are plain users with no cluster access. The admin creates every key and hands it out; a tenant only ever holds its key.
- **Any provider.** OpenAI, Anthropic, and anything OpenAI-compatible (vLLM, Ollama, OpenRouter, Groq, DeepSeek, Azure). OpenAI clients can use Anthropic models too.
- **Rules per key.** Allowed models, IP ranges, requests and tokens per minute, scaled per tenant. Automatic failover between providers.
- **Dollar budgets.** Per key, per tenant and per model, per day, week or month, from live provider prices, DeepSeek peak hours included. Spend never goes past a budget, and you get Kubernetes Events and Prometheus alerts before it runs out.
- **Chat, embeddings and the Responses API.** `/v1/chat/completions`, `/v1/messages`, `/v1/embeddings` and `/v1/responses`. Keys can expire and carry a default `max_tokens`.

## Minimal by design

- **One small image.** A single static Go binary on distroless, light on memory. One replica is plenty; run two for uptime.
- **Nothing extra to run.** No database, no Redis, no passwords. State is CRDs and Secrets; access is Kubernetes RBAC.
- **No prompts stored.** Valetkey only counts requests and tokens. It never keeps prompts or responses.
- **Uses what you already have.** Usage history comes from your Prometheus, charts from your Grafana.

## Install

```bash
helm install valetkey oci://ghcr.io/anas1412/charts/valetkey \
  -n valetkey-system --create-namespace
```

Without Helm: `kubectl apply -n valetkey-system -f https://github.com/anas1412/valetkey/releases/latest/download/valetkey.yaml`. The image is `ghcr.io/anas1412/valetkey` (tagged `latest` and with each version). See [Helm values](https://anas.cyberia.tn/valetkey/reference/helm).

## Configure with CRDs

```yaml
apiVersion: valetkey.dev/v1alpha1
kind: Channel                      # a provider and its real key
metadata: { name: openai, namespace: valetkey-system }
spec:
  type: openai
  baseURL: https://api.openai.com
  keySecretRef: { name: openai-key }
  models: [gpt-4o, gpt-4o-mini]
---
apiVersion: valetkey.dev/v1alpha1
kind: Group                        # a tenant: named after its namespace
metadata: { name: billing, namespace: valetkey-system }
spec:
  channelRefs: [openai]
---
apiVersion: valetkey.dev/v1alpha1
kind: ProxyKey                     # a virtual key in the tenant's namespace
metadata: { name: invoice-api, namespace: billing }
spec:
  models: [gpt-4o-mini]
  rpm: 120
```

Valetkey writes the virtual key into the Secret `invoice-api-valetkey` next to the `ProxyKey`. Apps mount it and call Valetkey like any OpenAI endpoint:

```bash
curl http://valetkey.valetkey-system:8000/v1/chat/completions \
  -H "Authorization: Bearer $VALETKEY_KEY" -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Hello"}]}'
```

A complete example is in [`examples/quickstart.yaml`](examples/quickstart.yaml).

## Dashboard (optional)

A web view of usage, activity, providers, tenants and keys. It listens only inside the pod, with no Service and no password, so you reach it with a port-forward:

```bash
kubectl port-forward -n valetkey-system deploy/valetkey 8082:8082
```

<p align="center">
  <a href="https://anas.cyberia.tn/valetkey/guide/dashboard">
    <img src="docs/public/screenshots/overview-dark.png" alt="Valetkey dashboard" width="900">
  </a>
</p>

## Development

One Go binary with the dashboard embedded. Needs Go 1.26+ and Node.js 24+.

```bash
cd web && npm ci && npm run build && cd ..
go test ./internal/...
helm lint charts/valetkey
```

See [Development](https://anas.cyberia.tn/valetkey/reference/development) for local clusters, the docs site, and regenerating screenshots and the video. Pushing a `v*` tag publishes the image, the Helm chart and a GitHub release.

## License

[ISC](LICENSE)
