# GitOps

Valetkey is built to be run from Git. Providers, tenants and keys are Kubernetes objects (`Channel`, `Group`, `ProxyKey`), and Valetkey has no database: what's in the cluster is the whole configuration. Argo CD or Flux applies your repository, and Valetkey does the rest.

This guide sets up a repository from scratch: installing Valetkey, storing provider keys safely, adding tenants and keys, and the everyday changes as pull requests.

## What goes in Git

| In Git (you write it) | Created by Valetkey (never in Git) |
|---|---|
| The Valetkey release (Argo CD Application or Flux HelmRelease) | Each key's Secret, `<key name>-valetkey`, holding the generated key |
| Provider API keys, encrypted (Sealed Secrets, External Secrets or SOPS) | `status` on every object: readiness, spend, errors |
| `Channel` per provider | The price cache (ConfigMap `valetkey-pricing`) |
| Tenant namespaces and a `Group` per tenant | The leader Lease when running more than one replica |
| `ProxyKey` per app | |

Generated keys never appear in Git. You commit "billing gets a key called invoice-api", and Valetkey creates the key itself in the tenant's namespace.

## Repository layout

```
valetkey/
  install/                    # Valetkey itself (Flux only, see below)
  config/
    kustomization.yaml
    providers/
      kustomization.yaml
      openai-key.yaml         # encrypted Secret
      openai.yaml             # Channel
    tenants/
      billing/
        kustomization.yaml
        namespace.yaml
        group.yaml
        keys.yaml
      support/
        ...
```

```yaml
# config/kustomization.yaml
resources:
  - providers
  - tenants/billing
  - tenants/support
```

One folder per tenant keeps each tenant's changes in their own pull requests. Don't set `namespace:` in these kustomizations: the `Group` lives in the operator namespace while its keys live in the tenant namespace, and a kustomization-wide namespace would move them all.

## Install Valetkey

Pin the chart version in Git and upgrade by changing it in a pull request.

::: code-group

```yaml [Argo CD]
# Register the chart registry once (it's an OCI registry).
apiVersion: v1
kind: Secret
metadata:
  name: valetkey-charts
  namespace: argocd
  labels: { argocd.argoproj.io/secret-type: repository }
stringData:
  type: helm
  name: valetkey
  url: ghcr.io/anas1412/charts
  enableOCI: "true"
---
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: valetkey
  namespace: argocd
spec:
  project: default
  source:
    repoURL: ghcr.io/anas1412/charts
    chart: valetkey
    targetRevision: 0.2.1
    helm:
      valuesObject: {}            # your Helm values, see Reference > Helm values
  destination:
    server: https://kubernetes.default.svc
    namespace: valetkey-system
  syncPolicy:
    automated: { prune: true, selfHeal: true }
    syncOptions: [CreateNamespace=true, ServerSideApply=true]
---
# Your providers, tenants and keys.
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: valetkey-config
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/your-org/platform.git
    path: valetkey/config
    targetRevision: main
  destination:
    server: https://kubernetes.default.svc
  syncPolicy:
    automated: { prune: true, selfHeal: true }
    # On a brand-new cluster the CRDs arrive with the first Application.
    syncOptions: [SkipDryRunOnMissingResource=true]
```

```yaml [Flux]
# valetkey/install/valetkey.yaml
apiVersion: v1
kind: Namespace
metadata: { name: valetkey-system }
---
apiVersion: source.toolkit.fluxcd.io/v1
kind: HelmRepository
metadata: { name: valetkey, namespace: valetkey-system }
spec:
  type: oci
  interval: 1h
  url: oci://ghcr.io/anas1412/charts
---
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata: { name: valetkey, namespace: valetkey-system }
spec:
  interval: 1h
  chart:
    spec:
      chart: valetkey
      version: 0.2.1
      sourceRef: { kind: HelmRepository, name: valetkey }
  install: { crds: CreateReplace }
  upgrade: { crds: CreateReplace }   # Helm alone never updates CRDs
  values: {}                         # your Helm values
---
# clusters/<cluster>/valetkey.yaml: install first, then the config.
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata: { name: valetkey-install, namespace: flux-system }
spec:
  interval: 10m
  path: ./valetkey/install
  prune: true
  wait: true
  sourceRef: { kind: GitRepository, name: flux-system }
---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata: { name: valetkey-config, namespace: flux-system }
spec:
  dependsOn: [{ name: valetkey-install }]
  interval: 10m
  path: ./valetkey/config
  prune: true
  sourceRef: { kind: GitRepository, name: flux-system }
```

:::

Argo CD applies the chart's CRDs on every sync, and the Flux example sets `crds: CreateReplace`, so new CRD fields arrive with each upgrade. Plain `helm upgrade` doesn't do this, see [Upgrading](/reference/helm#upgrading).

## Provider keys

A `Channel` points at a Secret in the operator namespace holding the provider's real API key. Never commit that Secret in plain text. Use what your cluster already has:

::: code-group

```bash [Sealed Secrets]
kubectl create secret generic openai-key -n valetkey-system \
  --from-literal=apiKey=sk-YOUR_REAL_KEY --dry-run=client -o yaml \
  | kubeseal -o yaml > config/providers/openai-key.yaml
```

```yaml [External Secrets]
# config/providers/openai-key.yaml
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata: { name: openai-key, namespace: valetkey-system }
spec:
  refreshInterval: 1h
  secretStoreRef: { kind: ClusterSecretStore, name: vault }
  target: { name: openai-key }
  data:
    - secretKey: apiKey
      remoteRef: { key: llm/openai, property: apiKey }
```

```bash [SOPS (Flux)]
kubectl create secret generic openai-key -n valetkey-system \
  --from-literal=apiKey=sk-YOUR_REAL_KEY --dry-run=client -o yaml > config/providers/openai-key.yaml
sops --encrypt --in-place config/providers/openai-key.yaml
# and add `decryption: { provider: sops, secretRef: { name: sops-age } }` to the valetkey-config Kustomization
```

:::

When the Secret changes, Valetkey picks up the new key within seconds, with no restart, and asks the provider whether it accepts it. A refused key shows as `KeyRejected` on the `Channel`, and that provider gets no traffic until the key is fixed (see [Providers](/guide/providers)).

## Providers

```yaml
# config/providers/openai.yaml
apiVersion: valetkey.dev/v1alpha1
kind: Channel
metadata:
  name: openai
  namespace: valetkey-system
spec:
  type: openai                        # also for any OpenAI-compatible API
  baseURL: https://api.openai.com
  keySecretRef: { name: openai-key }
  models: [gpt-4o, gpt-4o-mini]       # optional, default: any
  priority: 10
```

## Tenants

A tenant is a namespace plus a `Group` named after it. The `Group` lives in the operator namespace and says which providers the tenant may use, and optionally its budget.

```yaml
# config/tenants/billing/namespace.yaml
apiVersion: v1
kind: Namespace
metadata: { name: billing }
```

```yaml
# config/tenants/billing/group.yaml
apiVersion: valetkey.dev/v1alpha1
kind: Group
metadata:
  name: billing                       # the tenant namespace
  namespace: valetkey-system
spec:
  channelRefs: [openai]
  budget: { usd: "200", period: monthly }   # optional, for all the tenant's keys
```

```yaml
# config/tenants/billing/kustomization.yaml
resources: [namespace.yaml, group.yaml, keys.yaml]
```

## Keys

One `ProxyKey` per app, in the tenant's namespace:

```yaml
# config/tenants/billing/keys.yaml
apiVersion: valetkey.dev/v1alpha1
kind: ProxyKey
metadata:
  name: invoice-api
  namespace: billing
spec:
  models: [gpt-4o-mini]
  rpm: 120
  budget: { usd: "20", period: daily, timezone: Europe/Berlin }
  maxTokens: 1024                     # output cap for requests that don't set one
---
apiVersion: valetkey.dev/v1alpha1
kind: ProxyKey
metadata:
  name: contractor-trial
  namespace: billing
spec:
  expiresAt: "2026-12-31T23:59:00Z"   # stops working after this
```

See [Custom resources](/reference/crds) for every field.

## Getting the key to the app

After the sync, Valetkey writes each key into a Secret named `<key name>-valetkey` next to the `ProxyKey`. It holds `VALETKEY_KEY` (the key) and `VALETKEY_ENDPOINT` (the proxy URL).

- **The app runs in the same cluster:** reference the Secret from its Deployment, which can live in Git too. The key never leaves the cluster.

  ```yaml
  # in the app's Deployment, in namespace billing
  envFrom:
    - secretRef: { name: invoice-api-valetkey }
  ```

- **The app runs somewhere else:** the admin reads the key and hands it to the tenant over a secure channel. Tenants never need access to the cluster.

  ```bash
  valetkeyctl key billing/invoice-api
  ```

Don't put these Secrets in Git. They belong to their `ProxyKey` (Argo CD shows them under it), and deleting the key deletes its Secret.

## Everyday changes

Each of these is a pull request:

| Task | Change in Git |
|---|---|
| Add a tenant | Add a folder under `tenants/` and list it in `config/kustomization.yaml` |
| Give an app a key | Add a `ProxyKey` |
| Change a key's models, limits or budget | Edit its `spec` |
| Give temporary access | Set `expiresAt` |
| Pause a key | `status: Disabled` |
| Rotate a key | Set the `valetkey.dev/rotate` annotation to a new value (see below) |
| Revoke a key | Delete the `ProxyKey`; with `prune` on, its Secret goes with it and the key stops working |
| Pause a whole tenant | `status: Disabled` on its `Group` |
| Add a provider or move traffic | Add a `Channel`, or change `priority` |
| Replace a provider key | Update the encrypted Secret, or the value in your secret store |
| Upgrade Valetkey | Change the chart version |

### Rotating a key

The key value isn't in Git, so you rotate by changing an annotation. Any new value issues a new key, written to the same Secret, and the old key stops working:

```yaml
apiVersion: valetkey.dev/v1alpha1
kind: ProxyKey
metadata:
  name: invoice-api
  namespace: billing
  annotations:
    valetkey.dev/rotate: "2026-11-01"   # change to rotate again
spec:
  models: [gpt-4o-mini]
```

Apps that read the Secret through `envFrom` get the new key when they restart. Using a date as the value also leaves a record in Git of when each key was last rotated. The dashboard's **Rotate key** button works too, but leaves no trace in Git.

## Health in Argo CD

Valetkey reports readiness in each object's `status`, which Argo CD and Flux ignore when comparing with Git, so there's no drift. To make Argo CD show a key with a problem as Degraded (for example a refused provider key or an unknown tenant), add this to the `argocd-cm` ConfigMap:

```yaml
data:
  resource.customizations.health.valetkey.dev_ProxyKey: &valetkey-health |
    hs = {}
    if obj.status == nil or obj.status.conditions == nil then
      hs.status = "Progressing"
      hs.message = "Waiting for Valetkey"
      return hs
    end
    local c = obj.status.conditions[1]
    hs.message = c.message
    if c.status == "True" then
      hs.status = "Healthy"
    elseif c.reason == "Disabled" or c.reason == "GroupDisabled" or c.reason == "Expired" then
      hs.status = "Suspended"
    else
      hs.status = "Degraded"
    end
    return hs
  resource.customizations.health.valetkey.dev_Channel: *valetkey-health
  resource.customizations.health.valetkey.dev_Group: *valetkey-health
```

Keys and tenants you disabled on purpose, and expired keys, show as Suspended rather than Degraded. Without Argo CD, `kubectl get chan,grp,pkey -A` shows the same in its `READY` and `MESSAGE` columns.

## The dashboard and Git

The [dashboard](/guide/dashboard) writes the same objects, so it works next to Git, but Git wins: with `selfHeal` (Argo CD) or the next reconcile (Flux), a dashboard change to an object that's in Git is undone. Use the dashboard to look around and try things, then open **View YAML** on the provider, tenant or key and copy the result into your repository.
