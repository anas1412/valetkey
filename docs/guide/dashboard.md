# Dashboard tour

The dashboard is built into LLMP and served on the internal admin port. Open it with:

```bash
kubectl port-forward -n llmproxy-system svc/llmproxy-admin 8081:8081
```

It follows your system's light or dark setting and works on phones.

<Walkthrough />

## Overview

Requests, tokens, error rate and active keys for the last 24 hours, a requests chart (1 h / 6 h / 24 h), the share of traffic per provider, and whether every provider's key could be read. On a fresh install it shows the three setup steps instead.

<Shot name="overview" alt="Overview with traffic chart and provider share" />

::: info
History is kept in memory and resets when the pod restarts. For long-term data, scrape the [Prometheus metrics](/guide/monitoring).
:::

## Providers

Every upstream API, its models, priority and status. **Test** checks that the provider accepts the key (it lists the provider's models; no prompt is sent).

<Shot name="providers" alt="Providers list" />

## Tenants

One row per tenant namespace: its providers, number of keys, limit multipliers and status.

<Shot name="tenants" alt="Tenants list" />

## API keys

All keys across tenants, with their rules, request count and when they were last used. Expand a key to show or copy it and get ready-to-paste examples for cURL, Python, TypeScript and Kubernetes.

<Shot name="key-details" alt="An expanded API key with usage snippets" />

## Activity

The last 500 requests (metadata only, never prompts or responses), admin changes, key events and operator events, filterable and searchable.

<Shot name="activity" alt="Activity log" />
