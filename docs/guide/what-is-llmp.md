# What is LLMP?

LLMP (short for LLM Proxy) sits between your apps and your LLM providers, inside your Kubernetes cluster.

You add your **real** provider keys once. LLMP then hands out **virtual keys** to tenants (teams, apps, customers or environments), each in its own namespace. Apps call LLMP with their virtual key; LLMP checks the key's rules, swaps in the real key and forwards the request.

```
Tenant app ──POST /v1/chat/completions──▶ LLMP ──▶ OpenAI / Anthropic / vLLM / Ollama / …
              (sk-proxy-… virtual key)      │
                                            │  checks: key enabled? model allowed?
                                            │          IP allowed? under its limits?
```

## Why use it

- **Provider keys stay secret.** Tenants only ever see their own `sk-proxy-…` key. Rotating or revoking one tenant's access never touches the provider.
- **One place for rules.** Which models a key may use, from which IP ranges, and how many requests or tokens per minute.
- **No new infrastructure.** No database, no Redis, no SaaS. Everything is a Kubernetes object, so GitOps, RBAC and backups work as they already do.
- **Works with every SDK.** It speaks the OpenAI Chat Completions API and the Anthropic Messages API, so existing clients only change their base URL and key.

## The three building blocks

| Resource | Who creates it | What it is |
|---|---|---|
| **Provider** (`Channel`) | You (the admin) | An upstream API and its real key: OpenAI, Anthropic, or any OpenAI-compatible endpoint. |
| **Tenant** (`Group`) | You | A namespace allowed to use some providers, with optional limit multipliers. |
| **API key** (`ProxyKey`) | You or the tenant | A virtual key in the tenant's namespace, with its own rules. LLMP writes the key into a Secret next to it. |

You can manage all three from the [dashboard](/guide/dashboard) or with `kubectl`. They are the same objects.

## What it is not

- Not a billing system: it counts requests and tokens, but doesn't price them.
- Not a long-term analytics store: the dashboard keeps the last 24 hours in memory. Use the [Prometheus metrics](/guide/monitoring) for history.
- Not multi-replica yet: run one pod (see [limits](/guide/limits#scaling)).
