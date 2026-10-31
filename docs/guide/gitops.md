# GitOps

Valetkey is built to be run from Git. Providers, tenants and keys are Kubernetes objects (`Provider`, `Tenant`, `ProxyKey`), and Valetkey has no database: what's in the cluster is the whole configuration. Argo CD or Flux applies your repository, and Valetkey does the rest.

This guide sets up a repository from scratch: installing Valetkey, storing provider keys safely, adding tenants and keys, and the everyday changes as pull requests.

## What goes in Git

| In Git (you write it) | Created by Valetkey (never in Git) |
|---|---|
| The Valetkey release (Argo CD Application or Flux HelmRelease) | Each key's Secret, `<key name>-valetkey`, holding the generated key |
| Provider API keys, encrypted (Sealed Secrets, External Secrets or SOPS) | `status` on every object: readiness, spend, errors |
| `Provider` per provider | The price cache (ConfigMap `valetkey-pricing`) |
| Tenant namespaces and a `Tenant` per tenant | The leader Lease when running more than one replica |
| `ProxyKey` per app | |

Generated keys never appear in Git. You commit "billing gets a key called invoice-api", and Valetkey creates the key itself in the tenant's namespace.

## Repository layout

One file per provider and one file per tenant. There are no `kustomization.yaml` files to keep up to date: Flux and Argo CD apply every YAML file under `config/`, so adding a tenant means adding a file.

```
valetkey/
  install/
    valetkey.yaml             # installs Valetkey itself (Flux, see below)
  config/
    providers/
      openai.yaml             # OpenAI's API key (encrypted) + the Provider
      anthropic.yaml
    tenants/
      billing.yaml            # Namespace + Tenant + the tenant's ProxyKeys
      support.yaml
      research.yaml
      ...                     # one file per tenant, as many as you have
```

Everything about one tenant is in its own file, so each tenant's changes are small pull requests that are easy to review.

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
    targetRevision: 0.1.0
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
    directory: { recurse: true }     # every YAML file in the folder tree
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
      version: 0.1.0
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
  path: ./valetkey/config          # no kustomization.yaml needed: every YAML file below is applied
  prune: true                       # deleting a file deletes its objects
  sourceRef: { kind: GitRepository, name: flux-system }
```

:::

Argo CD applies the chart's CRDs on every sync, and the Flux example sets `crds: CreateReplace`, so new CRD fields arrive with each upgrade. Plain `helm upgrade` doesn't do this, see [Upgrading](/reference/helm#upgrading).

## Providers

A provider is one file: its real API key, stored encrypted, then the `Provider` that points at it. This example keeps the key in a secret store such as Vault through External Secrets:

```yaml
# config/providers/openai.yaml
# 1. The real API key, as a Secret named openai-key (never in plain text)
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
---
# 2. The provider
apiVersion: valetkey.dev/v1alpha1
kind: Provider
metadata:
  name: openai
  namespace: valetkey-system
spec:
  type: openai                        # also for any OpenAI-compatible API
  baseURL: https://api.openai.com
  keySecretRef: { name: openai-key }  # the Secret above
  models: [gpt-4o, gpt-4o-mini]       # optional, default: any
  priority: 10
```

Without a secret store, encrypt the Secret itself and put it at the top of the same file, followed by `---` and the `Provider`:

::: code-group

```bash [Sealed Secrets]
kubectl create secret generic openai-key -n valetkey-system \
  --from-literal=apiKey=sk-YOUR_REAL_KEY --dry-run=client -o yaml \
  | kubeseal -o yaml > config/providers/openai.yaml
# then add a line with --- and the Provider below it
```

```bash [SOPS (Flux)]
# write the Secret (kubectl create secret ... --dry-run=client -o yaml), ---, and the Provider
# into config/providers/openai.yaml, then encrypt only the key values:
sops --encrypt --encrypted-regex '^(data|stringData)$' --in-place config/providers/openai.yaml
# and add `decryption: { provider: sops, secretRef: { name: sops-age } }` to the valetkey-config Kustomization
```

:::

When the key changes, Valetkey picks it up within seconds, with no restart, and asks the provider whether it accepts it. A refused key shows as `KeyRejected` on the `Provider`, and that provider gets no traffic until the key is fixed (see [Providers](/guide/providers)).

## Tenants and their keys

A tenant is a namespace plus a `Tenant` named after it, and each app of that tenant gets a `ProxyKey` in that namespace. All of it goes in one file, separated by `---`:

```yaml
# config/tenants/billing.yaml
apiVersion: v1
kind: Namespace
metadata: { name: billing }
---
apiVersion: valetkey.dev/v1alpha1
kind: Tenant
metadata:
  name: billing                       # the tenant namespace
  namespace: valetkey-system          # Tenants live in the operator namespace
spec:
  providers: [openai]               # providers this tenant may use
  budget: { usd: "200", period: monthly }   # optional, for all the tenant's keys
---
apiVersion: valetkey.dev/v1alpha1
kind: ProxyKey
metadata:
  name: invoice-api                   # one key per app
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

- **The app runs somewhere else:** the admin reads the key and hands it to the tenant over a secure channel. Tenants never need access to the cluster. Copy it from the dashboard (**Copy key** on the key's row), or read it with kubectl:

  ::: code-group

  ```bash [Linux / macOS]
  kubectl get secret invoice-api-valetkey -n billing -o jsonpath='{.data.VALETKEY_KEY}' | base64 -d
  ```

  ```powershell [Windows PowerShell]
  $k = kubectl get secret invoice-api-valetkey -n billing -o jsonpath='{.data.VALETKEY_KEY}'
  [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($k))
  ```

  :::

Don't put these Secrets in Git. They belong to their `ProxyKey` (Argo CD shows them under it), and deleting the key deletes its Secret.

## Everyday changes

Each of these is a pull request:

| Task | Change in Git |
|---|---|
| Add a tenant | Add a file under `tenants/` |
| Remove a tenant | Delete its file; with `prune` on, its keys stop working and their Secrets are deleted |
| Give an app a key | Add a `ProxyKey` to the tenant's file |
| Change a key's models, limits or budget | Edit its `spec` |
| Give temporary access | Set `expiresAt` |
| Pause a key | `status: Disabled` |
| Rotate a key | Set the `valetkey.dev/rotate` annotation to a new value (see below) |
| Revoke a key | Delete the `ProxyKey`; with `prune` on, its Secret goes with it and the key stops working |
| Pause a whole tenant | `status: Disabled` on its `Tenant` |
| Add a provider | Add a file under `providers/` |
| Move traffic between providers | Change `priority` |
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
    elseif c.reason == "Disabled" or c.reason == "TenantDisabled" or c.reason == "Expired" then
      hs.status = "Suspended"
    else
      hs.status = "Degraded"
    end
    return hs
  resource.customizations.health.valetkey.dev_Provider: *valetkey-health
  resource.customizations.health.valetkey.dev_Tenant: *valetkey-health
```

Keys and tenants you disabled on purpose, and expired keys, show as Suspended rather than Degraded. Without Argo CD, `kubectl get prov,tn,pkey -A` shows the same in its `READY` and `MESSAGE` columns.

## The dashboard and Git

The [dashboard](/guide/dashboard) writes the same objects, so it works next to Git, but Git wins: with `selfHeal` (Argo CD) or the next reconcile (Flux), a dashboard change to an object that's in Git is undone. Use the dashboard to look around and try things, then open **View YAML** and copy the result into your repository: on a tenant it gives the whole `tenants/<name>.yaml` file, on a provider its `Provider` for `providers/<name>.yaml`.
