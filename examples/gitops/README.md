# Example GitOps repository folder

A ready-to-copy version of the [Setup from A to Z](https://anas.cyberia.tn/valetkey/guide/setup) guide. Copy this folder into your own Git repository as `valetkey/`, then:

1. In `config/providers/openai.yaml`, add your provider's API key Secret, encrypted (see step 5 of the guide).
2. Rename `config/tenants/billing.yaml` and its names to your first tenant, and add one `ProxyKey` per app.
3. **Flux:** copy `flux/valetkey.yaml` into the folder Flux applies (`clusters/<cluster>/`).
   **Argo CD:** change `repoURL` in `argocd/applications.yaml` to your repository and apply it.
4. Already run Prometheus? Delete `monitoring/` (and its Flux Kustomization, or its Argo CD Application), and check that the `release` labels in the Valetkey values match what your Prometheus selects.

```
valetkey/
  monitoring/kube-prometheus-stack.yaml   # optional: Prometheus + Grafana (Flux)
  install/valetkey.yaml                   # Valetkey with monitoring wired in (Flux)
  config/
    providers/openai.yaml                 # API key Secret (encrypted) + Provider
    tenants/billing.yaml                  # Namespace + Tenant + its ProxyKeys
  flux/valetkey.yaml                      # Flux: the three Kustomizations, in order
  argocd/applications.yaml                # Argo CD: the same as Applications
```

`install/` and `monitoring/` are only used with Flux; with Argo CD the same settings are in `argocd/applications.yaml`.
