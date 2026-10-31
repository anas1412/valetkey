# Quick start

You need a Kubernetes cluster (any distribution, 1.26+) and `kubectl`. Everything below works the same on Windows, macOS and Linux.

## 1. Install

```bash
kubectl create namespace llmproxy-system
kubectl apply -f https://raw.githubusercontent.com/anas1412/llmp/main/deploy/crds.yaml
kubectl apply -f https://raw.githubusercontent.com/anas1412/llmp/main/deploy/operator.yaml
```

This creates the three custom resources, a service account with the permissions LLMP needs, the deployment, and two services:

- `llmproxy` on port **8000**: what your apps call.
- `llmproxy-admin` on port **8081**: the dashboard, admin API, health and metrics. Keep it private.

## 2. Set an admin key

The dashboard stays off until you set one. Use any long random string:

```bash
kubectl create secret generic llmproxy-admin -n llmproxy-system \
  --from-literal=adminKey=CHANGE-ME-TO-A-LONG-RANDOM-STRING
kubectl rollout restart deploy/llmproxy -n llmproxy-system
```

## 3. Open the dashboard

```bash
kubectl port-forward -n llmproxy-system svc/llmproxy-admin 8081:8081
```

Open <http://localhost:8081> and sign in with the admin key.

<Shot name="overview" alt="Overview page" />

## 4. Follow the three steps

The Overview page walks you through it:

1. **[Add a provider](/guide/providers)**: pick OpenAI, Anthropic or another endpoint, paste its API key, tick the models to allow.
2. **[Create a tenant](/guide/tenants)**: pick a namespace and the providers it may use.
3. **[Create an API key](/guide/api-keys)**: pick the tenant and, if you like, the models, IP ranges and limits.

## 5. Make a request

Open the key in the **API keys** page to copy it, together with ready-made examples. From inside the cluster:

```bash
curl http://llmproxy.llmproxy-system:8000/v1/chat/completions \
  -H "Authorization: Bearer sk-proxy-YOUR_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Hello"}]}'
```

The request shows up on the **Activity** page right away. See [Using a key in your app](/guide/using-keys) for SDK examples.

::: tip Prefer YAML?
Every step above can be done with `kubectl apply` instead. See [Custom resources](/reference/crds) and [`deploy/examples.yaml`](https://github.com/anas1412/llmp/blob/main/deploy/examples.yaml).
:::
