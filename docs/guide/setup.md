# Setup from A to Z

This guide takes you from an empty Git repository to a working Valetkey: installed from Git with Flux or Argo CD, with your first provider, tenant and key, linked to Prometheus for usage history and to Grafana for dashboards. Every file is shown in full, and each step ends with a way to check that it worked.

Just want to try Valetkey for five minutes? Use the [Quick start](/guide/quick-start) instead.

## What you'll build

<SetupOverview />

- **In Git (you write it):** the Valetkey install, your providers (with their API keys encrypted), and your tenants with their keys.
- **Created by Valetkey (never in Git):** the generated keys, which it writes into a Secret next to each `ProxyKey`, and the `status` of every object (ready or not, spend so far).

The three kinds of objects you'll write:

| Kind | What it is |
|---|---|
| `Provider` | An upstream API such as OpenAI, and which Secret holds its real API key |
| `Tenant` | A namespace that may use some providers, with an optional budget |
| `ProxyKey` | A key for one app, in its tenant's namespace, with its own rules |

## 1. Before you start

You need:

- A Kubernetes cluster (1.26 or newer) and `kubectl` access to it.
- **Flux** or **Argo CD** installed in the cluster and connected to a Git repository you can push to. See [Flux: get started](https://fluxcd.io/flux/get-started/) or [Argo CD: getting started](https://argo-cd.readthedocs.io/en/stable/getting_started/).
- A way to keep secrets in Git: [External Secrets](https://external-secrets.io), [Sealed Secrets](https://github.com/bitnami-labs/sealed-secrets) or [SOPS](https://fluxcd.io/flux/guides/mozilla-sops/). Whatever your cluster already uses is fine.
- An API key for at least one provider, for example OpenAI.

## 2. Create the repository layout

Create a `valetkey/` folder in your repository. The fastest way is to copy [`examples/gitops/`](https://github.com/anas1412/valetkey/tree/main/examples/gitops) from the Valetkey repository; it holds exactly the files of this guide.

```
valetkey/
  monitoring/
    kube-prometheus-stack.yaml   # optional: Prometheus + Grafana (step 3)
  install/
    valetkey.yaml                # Valetkey, linked to Prometheus (step 4)
  config/
    providers/
      openai.yaml                # the API key (encrypted) + Provider (step 5)
    tenants/
      billing.yaml               # Namespace + Tenant + its ProxyKeys (step 6)
```

One file per provider and one file per tenant. There are no `kustomization.yaml` files to keep up to date: everything under `config/` is applied, so a new tenant is just a new file. See [GitOps](/guide/gitops) for more on this layout.

With Argo CD, `monitoring/` and `install/` aren't needed: the same settings go into Argo CD Applications (shown in steps 3 and 4).

## 3. Prometheus and Grafana (optional)

Valetkey works without Prometheus, but then the dashboard only shows the last 24 hours from the pod's memory, which resets on restart. With Prometheus you get 7 and 30 days of history that survives restarts, alerts, and a Grafana dashboard.

- **No Prometheus yet:** follow [Option A](#option-a-install-prometheus-and-grafana) to install it with the same repository.
- **Prometheus already running**, in any namespace: follow [Option B](#option-b-use-the-prometheus-you-already-have).

### Option A: install Prometheus and Grafana

This installs [kube-prometheus-stack](https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack): Prometheus, its operator and Grafana, in the `monitoring` namespace.

::: code-group

```yaml [Flux]
# valetkey/monitoring/kube-prometheus-stack.yaml
apiVersion: v1
kind: Namespace
metadata: { name: monitoring }
---
apiVersion: source.toolkit.fluxcd.io/v1
kind: HelmRepository
metadata: { name: prometheus-community, namespace: monitoring }
spec:
  type: oci
  interval: 1h
  url: oci://ghcr.io/prometheus-community/charts
---
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata: { name: kube-prometheus-stack, namespace: monitoring }
spec:
  interval: 1h
  timeout: 10m
  chart:
    spec:
      chart: kube-prometheus-stack
      version: 92.17.0
      sourceRef: { kind: HelmRepository, name: prometheus-community }
  install: { crds: CreateReplace }
  upgrade: { crds: CreateReplace }
  values:
    prometheus:
      prometheusSpec:
        retention: 35d                                # Valetkey's dashboard shows up to 30 days
        serviceMonitorSelectorNilUsesHelmValues: false # scrape every ServiceMonitor, Valetkey's included
        ruleSelectorNilUsesHelmValues: false           # load every PrometheusRule, Valetkey's alerts included
    grafana:
      sidecar:
        dashboards:
          searchNamespace: ALL                        # pick up Valetkey's dashboard from its namespace
```

```yaml [Argo CD]
# an Application in the argocd namespace
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: kube-prometheus-stack
  namespace: argocd
  annotations: { argocd.argoproj.io/sync-wave: "0" }
spec:
  project: default
  source:
    repoURL: https://prometheus-community.github.io/helm-charts
    chart: kube-prometheus-stack
    targetRevision: 92.17.0
    helm:
      valuesObject:
        prometheus:
          prometheusSpec:
            retention: 35d                                # Valetkey's dashboard shows up to 30 days
            serviceMonitorSelectorNilUsesHelmValues: false # scrape every ServiceMonitor
            ruleSelectorNilUsesHelmValues: false           # load every PrometheusRule
        grafana:
          sidecar:
            dashboards:
              searchNamespace: ALL                        # pick up Valetkey's dashboard
  destination:
    server: https://kubernetes.default.svc
    namespace: monitoring
  syncPolicy:
    automated: { prune: true, selfHeal: true }
    syncOptions: [CreateNamespace=true, ServerSideApply=true]   # its CRDs are too big for client-side apply
```

:::

The settings under `prometheusSpec` and `grafana` are what make the link with Valetkey work: keep 30 days of data, pick up Valetkey's ServiceMonitor and alert rules, and load its Grafana dashboard. Continue with [step 4](#_4-install-valetkey) as written.

### Option B: use the Prometheus you already have

Valetkey needs three things from your Prometheus: it scrapes Valetkey's pods, it keeps at least 30 days of data, and Valetkey can reach it to read history back. Which settings you change depends on how your Prometheus is installed.

#### With the Prometheus operator (kube-prometheus-stack and similar)

Find out four things about your Prometheus, whatever namespace it's in:

```bash
# Where it runs, and its service address
kubectl get prometheus -A
kubectl get svc -A -l operated-prometheus=true

# Which labels it selects ServiceMonitors and PrometheusRules by, and in which namespaces
kubectl get prometheus -A -o jsonpath='{range .items[*]}{.metadata.namespace}/{.metadata.name}{"\n  serviceMonitorSelector: "}{.spec.serviceMonitorSelector}{"\n  serviceMonitorNamespaceSelector: "}{.spec.serviceMonitorNamespaceSelector}{"\n  ruleSelector: "}{.spec.ruleSelector}{"\n  ruleNamespaceSelector: "}{.spec.ruleNamespaceSelector}{"\n  retention: "}{.spec.retention}{"\n"}{end}'
```

Say it prints namespace `observability`, a selector `{"matchLabels":{"release":"prom"}}`, and retention `15d`. Then in step 4:

- Set `serviceMonitor.labels` and `prometheusRule.labels` to the selector's labels, here `{ release: prom }`.
- Set `prometheus.url` to `http://prometheus-operated.<its namespace>.svc:9090`, here `http://prometheus-operated.observability.svc:9090`.
- If a **namespace selector** is set (not empty `{}`), Prometheus only looks in matching namespaces: give `valetkey-system` that label, for example `kubectl label namespace valetkey-system <key>=<value>` (or add it to the `Namespace` in `install/valetkey.yaml`).
- If **retention** is under 30 days, raise it to at least `35d`, or the 30 day view shows less.

#### Without the operator (plain Prometheus)

Turn off the operator objects in step 4 (`serviceMonitor.enabled: false`, `prometheusRule.enabled: false`) and add a scrape job that finds each Valetkey pod. Scrape the pods, not the Service: with two replicas, scraping the Service would reach a different pod each time and mix their counters.

```yaml
# prometheus.yml (or your Prometheus Helm values under extraScrapeConfigs)
scrape_configs:
  - job_name: valetkey
    kubernetes_sd_configs:
      - role: endpoints
        namespaces: { names: [valetkey-system] }
    relabel_configs:
      - source_labels: [__meta_kubernetes_service_name, __meta_kubernetes_endpoint_port_name]
        regex: valetkey;metrics
        action: keep
      - source_labels: [__meta_kubernetes_pod_name]
        target_label: pod
```

Prometheus needs permission to list endpoints in `valetkey-system`; the community Prometheus chart has it cluster-wide by default. For the alerts, add Valetkey's rules to your rule files: `helm template valetkey oci://ghcr.io/anas1412/charts/valetkey --set prometheusRule.enabled=true -s templates/prometheusrule.yaml` prints them (the `groups:` part is a standard rule file).

Set `prometheus.url` to your Prometheus service, for example `http://prometheus-server.monitoring.svc:80`.

#### Grafana in another namespace

The chart's dashboard is a ConfigMap labelled `grafana_dashboard: "1"`, which Grafana's sidecar picks up. If your sidecar only watches its own namespace, set `grafanaDashboard.namespace` to Grafana's namespace in step 4. Without a sidecar, import [`valetkey.json`](https://github.com/anas1412/valetkey/blob/main/charts/valetkey/dashboards/valetkey.json) under **Dashboards > New > Import**.

#### Network policies and other query APIs

- Valetkey must reach Prometheus on its port. If Prometheus' namespace has a NetworkPolicy, allow traffic from `valetkey-system`.
- Prometheus must reach Valetkey on port 8081. With the chart's `networkPolicy.enabled`, label Prometheus' namespace `valetkey.dev/metrics=true`.
- `prometheus.url` also works with Prometheus-compatible query APIs that need no login, such as Thanos Query or VictoriaMetrics. A path in the URL is kept, for example `http://vmselect.monitoring.svc:8481/select/0/prometheus`.

## 4. Install Valetkey

Valetkey is a Helm chart. These values link it to Prometheus and Grafana:

| Value | What it does |
|---|---|
| `serviceMonitor.enabled` | Creates a ServiceMonitor, so Prometheus scrapes Valetkey's metrics |
| `prometheusRule.enabled` | Creates Valetkey's alert rules (budgets, provider errors, prices) |
| `prometheus.url` | Where Valetkey reads history back from, for the dashboard's 7 and 30 days |
| `grafanaDashboard.enabled` | Creates the Grafana dashboard for Grafana's sidecar to load |
| `release` labels | Match what your Prometheus selects on; harmless with step 3's settings, needed with most existing setups |
| `replicas: 2` | One pod can restart or move without an outage |

::: code-group

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
  values:
    replicas: 2
    serviceMonitor:
      enabled: true
      labels: { release: kube-prometheus-stack }
    prometheusRule:
      enabled: true
      labels: { release: kube-prometheus-stack }
    prometheus:
      url: http://prometheus-operated.monitoring.svc:9090
    grafanaDashboard:
      enabled: true
```

```yaml [Argo CD]
# Valetkey's chart registry is an OCI registry: register it once.
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
  annotations: { argocd.argoproj.io/sync-wave: "1" }
spec:
  project: default
  source:
    repoURL: ghcr.io/anas1412/charts
    chart: valetkey
    targetRevision: 0.1.0
    helm:
      valuesObject:
        replicas: 2
        serviceMonitor:
          enabled: true
          labels: { release: kube-prometheus-stack }
        prometheusRule:
          enabled: true
          labels: { release: kube-prometheus-stack }
        prometheus:
          url: http://prometheus-operated.monitoring.svc:9090
        grafanaDashboard:
          enabled: true
  destination:
    server: https://kubernetes.default.svc
    namespace: valetkey-system
  syncPolicy:
    automated: { prune: true, selfHeal: true }
    syncOptions: [CreateNamespace=true, ServerSideApply=true]
    retry: { limit: 10, backoff: { duration: 30s } }   # waits for Prometheus' CRDs on a new cluster
```

:::

`prometheus-operated` is the service the Prometheus operator always creates. With your own Prometheus, use its address instead. Upgrading Valetkey later is a one-line change: the chart version. New CRD fields come with it, since both examples apply the chart's CRDs on every upgrade.

## 5. Add your first provider

A provider is one file: the Secret holding its real API key, encrypted, then the `Provider` pointing at that Secret. Pick the tab for the secrets tool you use:

::: code-group

```yaml [External Secrets]
# valetkey/config/providers/openai.yaml
# 1. The real API key, fetched from your secret store into a Secret named openai-key
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
  models: [gpt-4o, gpt-4o-mini]       # optional, default: any model
  priority: 10
```

```bash [Sealed Secrets]
# 1. Encrypt the key into the file
kubectl create secret generic openai-key -n valetkey-system \
  --from-literal=apiKey=sk-YOUR_REAL_KEY --dry-run=client -o yaml \
  | kubeseal -o yaml > valetkey/config/providers/openai.yaml
# 2. Add a line with --- and the Provider (from the External Secrets tab) below it
```

```bash [SOPS (Flux)]
# 1. Write the Secret, a line with ---, and the Provider into the file
kubectl create secret generic openai-key -n valetkey-system \
  --from-literal=apiKey=sk-YOUR_REAL_KEY --dry-run=client -o yaml > valetkey/config/providers/openai.yaml
# (then add --- and the Provider from the External Secrets tab)
# 2. Encrypt only the key values, so the Provider stays readable
sops --encrypt --encrypted-regex '^(data|stringData)$' --in-place valetkey/config/providers/openai.yaml
# 3. Add `decryption: { provider: sops, secretRef: { name: sops-age } }` to the valetkey-config Kustomization (step 7)
```

:::

Other providers work the same way, one file each: `type: anthropic` for Anthropic, `type: openai` with another `baseURL` for DeepSeek, Groq, OpenRouter, Azure and other OpenAI-compatible APIs. See [Providers](/guide/providers).

## 6. Add your first tenant and key

A tenant is one file: its namespace, the `Tenant`, and one `ProxyKey` per app.

```yaml
# valetkey/config/tenants/billing.yaml
apiVersion: v1
kind: Namespace
metadata: { name: billing }
---
apiVersion: valetkey.dev/v1alpha1
kind: Tenant
metadata:
  name: billing                       # must be the tenant's namespace
  namespace: valetkey-system          # Tenants live in Valetkey's namespace
spec:
  providers: [openai]                 # providers this tenant may use
  budget: { usd: "200", period: monthly }   # optional, for all the tenant's keys
---
apiVersion: valetkey.dev/v1alpha1
kind: ProxyKey
metadata:
  name: invoice-api                   # one key per app
  namespace: billing
spec:
  models: [gpt-4o-mini]               # optional, default: every model of the tenant's providers
  rpm: 120                            # optional, requests per minute
  budget: { usd: "20", period: daily }
```

Budgets are a hard stop: a request that could go over is refused before it reaches the provider. See [Budgets & costs](/guide/budgets) and [Limits & access rules](/guide/limits) for every option.

## 7. Commit and sync

Tell your GitOps tool to apply the folders in order: monitoring, then Valetkey, then your config.

::: code-group

```yaml [Flux]
# clusters/<cluster>/valetkey.yaml, in the folder Flux already applies
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata: { name: valetkey-monitoring, namespace: flux-system }
spec:
  interval: 10m
  timeout: 10m
  path: ./valetkey/monitoring          # remove this Kustomization if you skipped step 3
  prune: true
  wait: true
  sourceRef: { kind: GitRepository, name: flux-system }
---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata: { name: valetkey-install, namespace: flux-system }
spec:
  dependsOn: [{ name: valetkey-monitoring }]   # remove too if you skipped step 3
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
  path: ./valetkey/config              # every YAML file below is applied
  prune: true                          # deleting a file deletes its objects
  sourceRef: { kind: GitRepository, name: flux-system }
```

```yaml [Argo CD]
# Your providers, tenants and keys: change repoURL to your repository.
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: valetkey-config
  namespace: argocd
  annotations: { argocd.argoproj.io/sync-wave: "2" }
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
    syncOptions: [SkipDryRunOnMissingResource=true]   # Valetkey's CRDs may not exist yet on the first sync
    retry: { limit: 10, backoff: { duration: 30s } }
```

:::

Commit and push. With Argo CD, also apply the Applications from steps 3, 4 and 7 once (`kubectl apply -f`), or add them to your app of apps.

**Check it worked** (allow a few minutes for Prometheus on a new cluster):

::: code-group

```bash [Flux]
flux get kustomizations
flux get helmreleases -A
```

```bash [Argo CD]
kubectl get applications -n argocd
```

:::

Everything should say `Ready` / `Synced` and `Healthy`. Then look at Valetkey's own objects:

```bash
kubectl get prov,tn -n valetkey-system
kubectl get pkey -A
```

The `READY` column should be `true` for all of them. If not, the `MESSAGE` column (or `kubectl describe`) says why; see [Troubleshooting](#troubleshooting).

## 8. Check the link with Prometheus and Grafana

**Prometheus scrapes Valetkey:**

```bash
kubectl port-forward -n monitoring svc/prometheus-operated 9090
```

Open <http://localhost:9090/targets>: the `valetkey` target should be `UP`, with one entry per Valetkey pod.

**Valetkey reads history from Prometheus:** open the dashboard

```bash
kubectl port-forward -n valetkey-system deploy/valetkey 8082:8082
```

and go to <http://localhost:8082>. The Overview shows a **Prometheus** badge and a 24 hours / 7 days / 30 days switch. If it shows a warning instead, `prometheus.url` is wrong or unreachable.

**Grafana has the dashboard:**

```bash
kubectl port-forward -n monitoring svc/kube-prometheus-stack-grafana 3000:80
kubectl get secret -n monitoring kube-prometheus-stack-grafana -o jsonpath='{.data.admin-password}' | base64 -d
```

Log in at <http://localhost:3000> as `admin` with that password, and open **Dashboards > Valetkey**.

**Alerts are loaded:** in Prometheus, **Alerts** lists the `valetkey` rules (budget almost used, budget reached, provider errors, and more). See [Monitoring](/guide/monitoring#alerts).

## 9. Give the key to the app

Valetkey generated the key when it saw the `ProxyKey`, and wrote it into a Secret named `<key name>-valetkey` in the tenant's namespace. It holds `VALETKEY_KEY` (the key) and `VALETKEY_ENDPOINT` (the proxy URL).

- **The app runs in the same cluster:** load the Secret in its Deployment. The key never leaves the cluster.

  ```yaml
  # in the app's Deployment, in namespace billing
  envFrom:
    - secretRef: { name: invoice-api-valetkey }
  ```

- **The app runs elsewhere:** copy the key from the dashboard (**Copy key** on its row) or with kubectl, and hand it to the tenant over a secure channel. Tenants never need access to the cluster.

  ::: code-group

  ```bash [Linux / macOS]
  kubectl get secret invoice-api-valetkey -n billing -o jsonpath='{.data.VALETKEY_KEY}' | base64 -d
  ```

  ```powershell [Windows PowerShell]
  $k = kubectl get secret invoice-api-valetkey -n billing -o jsonpath='{.data.VALETKEY_KEY}'
  [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($k))
  ```

  :::

## 10. Send a first request

Apps use Valetkey like the OpenAI API, with their key and Valetkey's address. To try it from your machine:

```bash
kubectl port-forward -n valetkey-system svc/valetkey 8000
```

```bash
curl http://localhost:8000/v1/chat/completions \
  -H "Authorization: Bearer sk-proxy-YOUR_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Hello"}]}'
```

Then look at it:

- **Dashboard:** the request in **Activity** with its cost, and the key's spend on **API keys**. Each pod shows its own recent requests; the totals come from Prometheus.
- **Grafana:** requests, tokens and cost on the Valetkey dashboard, within a minute.
- **kubectl:** `kubectl get pkey -n billing` shows the spend in the `SPENT` column.

Inside the cluster, apps use `http://valetkey.valetkey-system.svc:8000/v1` (the `VALETKEY_ENDPOINT` from the Secret). See [Using a key in your app](/guide/using-keys) for the OpenAI and Anthropic SDKs.

## 11. Day 2

Everything from here is a pull request:

| Task | Change in Git |
|---|---|
| Add a tenant | Add a file under `config/tenants/` |
| Give an app a key | Add a `ProxyKey` to its tenant's file |
| Change limits, models or budget | Edit the `spec` |
| Rotate a key | Set the `valetkey.dev/rotate` annotation to a new value |
| Revoke a key | Delete the `ProxyKey` |
| Add a provider | Add a file under `config/providers/` |
| Upgrade Valetkey | Change the chart version in step 4 |

More in [GitOps: everyday changes](/guide/gitops#everyday-changes). The dashboard stays useful for looking around; its **View YAML** gives you a tenant's whole file, ready to commit.

## Troubleshooting

| What you see | Why, and the fix |
|---|---|
| The `valetkey` target is missing in Prometheus | Prometheus doesn't select Valetkey's ServiceMonitor. Use step 3's `serviceMonitorSelectorNilUsesHelmValues: false`, or set `serviceMonitor.labels` to the label your Prometheus selects on. With the chart's `networkPolicy` on, label Prometheus' namespace `valetkey.dev/metrics=true` |
| The dashboard shows a Prometheus warning | `prometheus.url` is wrong or unreachable from Valetkey's namespace. Test it with `kubectl run -it --rm curl --image=curlimages/curl -n valetkey-system -- curl -s <url>/-/ready` |
| The dashboard only offers 24 hours | `prometheus.url` isn't set |
| 30 days shows less than 30 days | Prometheus' `retention` is shorter than 30 days |
| No Valetkey dashboard in Grafana | Grafana's sidecar doesn't watch Valetkey's namespace: set `searchNamespace: ALL` (step 3) or `grafanaDashboard.namespace` to Grafana's namespace |
| First sync fails with "no matches for kind Provider" | The config was applied before Valetkey's CRDs existed. Flux: keep the `dependsOn`. Argo CD: keep `SkipDryRunOnMissingResource=true` and `retry` |
| Valetkey's install fails with "no matches for kind ServiceMonitor" | The Prometheus operator isn't installed yet (or at all). Install it first (step 3), or set `serviceMonitor.enabled` and `prometheusRule.enabled` to `false` |
| A `Provider` shows `SecretUnavailable` | The key Secret doesn't exist or has no `apiKey` entry. Check `kubectl get secret -n valetkey-system <name>` and your secrets tool |
| A `Provider` shows `KeyRejected` | The provider refused the key (401/403). Fix the key in your secret store; Valetkey picks up the change within seconds |
| A `ProxyKey` shows `TenantNotFound` | There's no `Tenant` named after the key's namespace, or it isn't in `valetkey-system` |
| Requests get `429 insufficient_quota` | A budget is used up. The message says which one and when it resets |
| Requests get `403 model_not_priced` | The key or tenant has a budget, and Valetkey has no price for that model. Add a price on the provider, see [Budgets & costs](/guide/budgets#your-own-prices) |
