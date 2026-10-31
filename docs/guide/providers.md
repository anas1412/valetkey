# Providers

A provider (Kubernetes kind: `Channel`) is one upstream LLM API plus the real API key Valetkey uses for it. Providers live in the operator namespace (`valetkey-system`), so tenants never see their keys.

## With kubectl (recommended)

```yaml
apiVersion: v1
kind: Secret
metadata: { name: openai-key, namespace: valetkey-system }
stringData: { apiKey: sk-YOUR_REAL_KEY }
---
apiVersion: valetkey.dev/v1alpha1
kind: Channel
metadata: { name: openai, namespace: valetkey-system }
spec:
  type: openai                       # or anthropic
  baseURL: https://api.openai.com
  keySecretRef: { name: openai-key } # key defaults to "apiKey"
  models: [gpt-4o, gpt-4o-mini]      # omit to allow any model
  priority: 10
```

See [Custom resources](/reference/crds#channel) for every field.

## In the dashboard: add a provider

Open **Providers → Add provider**.

<Shot name="add-provider" alt="The Add provider form" />

1. **Provider**: pick OpenAI, Anthropic, OpenRouter, Groq, DeepSeek, or *Other (OpenAI-compatible)* for vLLM, Ollama, Azure, LiteLLM and similar. The base URL is filled in for you.
2. **Name**: lowercase, e.g. `openai`. You'll pick providers by this name when creating tenants.
3. **API key**: paste it. It's saved as a Kubernetes Secret (`<name>-api-key`) and never shown again.
4. **Models**: leave **Allow any** on, or choose **Choose specific models instead**: Valetkey asks the provider for its model list and you tick the ones to allow. No typing model names by hand.
5. **Priority**: when a tenant has several providers for the same model, higher priority gets proportionally more traffic (20 gets twice the traffic of 10).

Under **Advanced**:

- **Model mapping**: rename a requested model before it's sent, e.g. `gpt-4o → my-azure-deployment`.
- **Outbound proxy**: send this provider's traffic through an HTTP proxy.
- **Skip TLS certificate check**: only for self-hosted endpoints with self-signed certificates.

After saving, Valetkey reads the key and asks the provider whether it accepts it, by listing the provider's models (no prompt is sent). The provider shows:

- **Ready**: the key was read and the provider didn't refuse it.
- **Key rejected**: the provider answered 401 or 403. The provider gets no traffic until its key changes. Valetkey asks again every 10 minutes, in case the refusal was temporary.
- **Key missing**: the Secret or the key in it can't be found.

The check runs again whenever the key or the base URL changes, and every time you press **Test**. If the provider has no models endpoint or doesn't answer, the provider stays **Ready**: only a clear refusal marks the key.

## Failover

If a provider answers 429 (rate limited), a 5xx error, or doesn't answer at all, Valetkey retries the request on the tenant's next provider that serves the same model, before anything is sent back to the app. Up to three providers are tried.

A 401 from a provider means its key is bad, not the app's: Valetkey marks the provider **Key rejected** and tries the next one. A 403 also moves on to the next provider, but doesn't mark it, since some providers answer 403 for one model only.

## OpenAI ↔ Anthropic

- Apps using the **OpenAI** API can be routed to an **Anthropic** provider: requests, responses and streaming are translated, including tools, images and system prompts.
- Apps using the **Anthropic** API (`/v1/messages`) are passed through unchanged to Anthropic providers.
