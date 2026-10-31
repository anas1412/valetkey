# Dashboard (optional)

Valetkey is configured with [custom resources](/guide/kubectl); the dashboard is an optional web view on top of them. It shows usage and activity, and can create and edit the same objects you'd write in YAML.

## Open it

The dashboard only listens inside the pod (on `127.0.0.1:8082`), so there's no Service, no Ingress and no password. You reach it with a port-forward, which your Kubernetes RBAC controls:

```bash
kubectl port-forward -n valetkey-system deploy/valetkey 8082:8082
```

Then open <http://localhost:8082>. Or let the CLI do it: `valetkeyctl dashboard`.

It follows your system's light or dark setting and works on phones.

<Walkthrough />

::: tip Using GitOps?
Changes made in the dashboard are real objects in the cluster, so your GitOps tool will revert them on its next sync if the same objects are in Git. Use the dashboard to look and Git to change, or pick one owner per object.
:::

## Overview

Requests, tokens, error rate and active keys for the last 24 hours, a requests chart (1 h / 6 h / 24 h), the share of traffic per provider, and whether every provider's key could be read.

<Shot name="overview" alt="Overview with traffic chart and provider share" />

::: info
Without Prometheus, the Overview shows the last 24 hours from the pod's memory, which resets when the pod restarts. Set `prometheus.url` to get 7 and 30 days that survive restarts: see [Usage history with Prometheus](/guide/usage-history).
:::

## Providers

Every upstream API, its models, priority and status. **Test** checks that the provider accepts the key (it lists the provider's models; no prompt is sent). **Add provider** lets you paste a key and tick models from the provider's own list.

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
