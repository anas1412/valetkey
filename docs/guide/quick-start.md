# Quick start

Try Valetkey in five minutes with `kubectl`. To set it up for real, from Git with Prometheus and Grafana, follow [Setup from A to Z](/guide/setup).

You need a Kubernetes cluster (1.26+), `kubectl` and, for the recommended install, `helm`. Everything below works the same on Windows, macOS and Linux.

## 1. Install

::: code-group

```bash [Helm (recommended)]
helm install valetkey oci://ghcr.io/anas1412/charts/valetkey \
  -n valetkey-system --create-namespace
```

```bash [kubectl]
kubectl create namespace valetkey-system
kubectl apply -n valetkey-system -f https://github.com/anas1412/valetkey/releases/latest/download/valetkey.yaml
```

:::

This installs the three custom resources, a service account with the permissions Valetkey needs, the deployment, and a `valetkey` Service:

- port **8000**: what your apps call;
- port **8081**: health checks and Prometheus metrics.

There's no admin password to set. The optional dashboard and the admin API only listen inside the pod and are reached with `kubectl port-forward`, so your Kubernetes permissions decide who can use them. Helm settings are listed in [Helm values](/reference/helm).

## 2. Add a provider

Store the provider's real API key in a Secret, then point a `Provider` at it:

```bash
kubectl create secret generic openai-key -n valetkey-system --from-literal=apiKey=sk-YOUR_REAL_KEY
```

```yaml
apiVersion: valetkey.dev/v1alpha1
kind: Provider
metadata: { name: openai, namespace: valetkey-system }
spec:
  type: openai
  baseURL: https://api.openai.com
  keySecretRef: { name: openai-key }
  models: [gpt-4o, gpt-4o-mini]
```

## 3. Create a tenant and a key

A tenant is a `Tenant` named after the tenant's namespace; keys are `ProxyKey`s in that namespace:

```yaml
apiVersion: valetkey.dev/v1alpha1
kind: Tenant
metadata: { name: billing, namespace: valetkey-system }
spec:
  providers: [openai]
---
apiVersion: valetkey.dev/v1alpha1
kind: ProxyKey
metadata: { name: invoice-api, namespace: billing }
spec:
  models: [gpt-4o-mini]
```

```bash
kubectl create namespace billing
kubectl apply -f provider.yaml -f tenant-and-key.yaml
kubectl get pkey -n billing        # READY turns true within a second or two
```

## 4. Use the key

Valetkey writes the key into a Secret next to the `ProxyKey`. Mount it in your app with `envFrom`, or read it:

```bash
kubectl get secret invoice-api-valetkey -n billing -o jsonpath='{.data.VALETKEY_KEY}' | base64 -d
```

Then call Valetkey like any OpenAI endpoint:

```bash
curl http://valetkey.valetkey-system:8000/v1/chat/completions \
  -H "Authorization: Bearer sk-proxy-YOUR_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Hello"}]}'
```

## Next

- [GitOps](/guide/gitops): run Valetkey from Git with Argo CD or Flux.
- [kubectl](/guide/kubectl): the same objects from the terminal, and everyday tasks.
- [Dashboard](/guide/dashboard): an optional web view, through port-forward.
- [Using a key in your app](/guide/using-keys): SDK examples for OpenAI and Anthropic.
