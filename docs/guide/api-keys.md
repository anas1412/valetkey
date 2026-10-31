# API keys

An API key (Kubernetes kind: `ProxyKey`) is a virtual key for one app in a tenant's namespace. Valetkey generates the key (`sk-proxy-…`) and writes it into a Secret next to it, called `<name>-valetkey`.

<Shot name="keys" alt="API keys list" />

## With kubectl (recommended)

```yaml
apiVersion: valetkey.dev/v1alpha1
kind: ProxyKey
metadata:
  name: invoice-api
  namespace: billing           # the tenant
spec:
  models: [gpt-4o-mini]        # omit for every model the tenant's providers offer
  subnets: [10.0.0.0/8]        # omit for any address
  rpm: 120                     # requests per minute, omit for unlimited
  tpm: 100000                  # tokens per minute, omit for unlimited
```

```bash
kubectl apply -f key.yaml
kubectl get pkey invoice-api -n billing     # READY turns true within a second or two
```

Disable, rotate and revoke from the terminal or from Git: see [kubectl](/guide/kubectl#everyday-tasks) and [GitOps](/guide/gitops#everyday-changes).

## In the dashboard: create a key

Open **API keys → Create key**.

<Shot name="create-key" alt="The Create key form" />

1. **Tenant**: the namespace the key belongs to.
2. **Name**: usually the app that will use it, e.g. `invoice-api`.
3. **Allowed models**: leave **All models** on, or turn it off and tick models from the list of what the tenant's providers offer.
4. **Allowed IP ranges**: add ranges one at a time (press Enter), like `10.0.0.0/8` or `2001:db8::/32`. None means any address.
5. **Requests / min** and **Tokens / min**: empty means unlimited. The tenant's multipliers apply on top.

The key turns **Active** within a second or two.

## Copy a key and get examples

Expand a key to see its status, Secret, request count and last use, then **Show** or **Copy** the key. Below it are ready-to-paste examples for cURL, Python, TypeScript and Kubernetes, filled in with your endpoint and the key's first allowed model.

<Shot name="key-details" alt="Key details with code snippets" />

## Disable, rotate, delete in the dashboard

From the **Copy key** menu on each row:

- **Disable / Enable**: blocks the key immediately without deleting it.
- **Rotate key**: issues a new key right away; the old one stops working. You drag a slider to confirm. Apps that read the Secret pick up the new key when they restart.
- **Edit**: change models, IP ranges or limits.
- **View YAML**: the key as a ProxyKey CRD, ready to commit to Git.
- **Delete**: removes the key and its Secret (also confirmed by dragging).

## Temporary keys

Give a key an end date, for a contractor or a demo. It stops working at that moment (401) and shows **Expired**:

```yaml
spec:
  expiresAt: "2026-12-31T23:59:59Z"
```

In the dashboard, set **Expires** on the key form (in your browser's time zone).

## Use the Secret in your workloads

The Secret holds everything an app needs:

| Key | Value |
|---|---|
| `VALETKEY_KEY` | the virtual key |
| `VALETKEY_ENDPOINT` | the proxy URL |
| `VALETKEY_GROUP` | the tenant |
| `VALETKEY_CHANNEL` | the tenant's first provider when the key was issued |

```yaml
# in a Deployment in the tenant's namespace
envFrom:
  - secretRef:
      name: invoice-api-valetkey
```

The Secret is owned by the `ProxyKey`, so deleting the key deletes the Secret.
