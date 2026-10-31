# What is Valetkey?

Valetkey, the Minimal LLM Proxy, sits between your apps and your LLM providers, inside your Kubernetes cluster.

The name comes from a car's valet key: it starts the car but can't open the trunk. You keep the real provider key; each tenant gets a limited one.

You add your **real** provider keys once. Valetkey then hands out **virtual keys** to tenants (teams, apps, customers or environments), each in its own namespace. Apps call Valetkey with their virtual key; Valetkey checks the key's rules, swaps in the real key and forwards the request.

```
Tenant app ──POST /v1/chat/completions──▶ Valetkey ──▶ OpenAI / Anthropic / vLLM / Ollama / …
              (sk-proxy-… virtual key)      │
                                            │  checks: key enabled? model allowed?
                                            │          IP allowed? under its limits?
```

## Why use it

- **Provider keys stay secret.** Tenants only ever see their own `sk-proxy-…` key. Rotating or revoking one tenant's access never touches the provider.
- **One place for rules.** Which models a key may use, from which IP ranges, and how many requests or tokens per minute.
- **No new infrastructure.** No database, no Redis, no SaaS, no passwords. Everything is a Kubernetes object, so GitOps, RBAC, audit logs and backups work as they already do.
- **Works with every SDK.** It speaks the OpenAI Chat Completions API and the Anthropic Messages API, so existing clients only change their base URL and key.

## The three building blocks

| Kind | Who creates it | What it is |
|---|---|---|
| `Provider` | You (the admin) | An upstream API and its real key: OpenAI, Anthropic, or any OpenAI-compatible endpoint. |
| `Tenant` | You | A namespace allowed to use some providers, with optional limit multipliers. |
| `ProxyKey` (an API key) | You | A virtual key in the tenant's namespace, with its own rules. Valetkey writes the key into a Secret next to it, and you hand it to the tenant. |

You manage all three as YAML, from Git with [Argo CD or Flux](/guide/gitops) or with [`kubectl`](/guide/kubectl), and optionally look at them in the [dashboard](/guide/dashboard).

## What it is not

- Not a billing system: it counts requests and tokens, but doesn't price them.
- Not a long-term analytics store: the dashboard keeps the last 24 hours in memory. Use the [Prometheus metrics](/guide/monitoring) for history.
- Not multi-replica yet: run one pod (see [limits](/guide/limits#scaling)).
