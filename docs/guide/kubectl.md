# kubectl & GitOps

This is the main way to run Valetkey: providers, tenants and keys are plain Kubernetes objects, so they live in Git and go through `kubectl`, Helm, Kustomize, Argo CD or Flux like the rest of your cluster. The [dashboard](/guide/dashboard) is an optional view on top.

This guide sets up one provider, one tenant and one key from YAML, then covers everyday tasks.

## 1. Store the provider's API key

Provider keys live in Secrets in the operator namespace:

```bash
kubectl create secret generic openai-key -n valetkey-system \
  --from-literal=apiKey=sk-YOUR_REAL_KEY
```

## 2. Add the provider

```yaml
# provider.yaml
apiVersion: valetkey.dev/v1alpha1
kind: Channel
metadata:
  name: openai
  namespace: valetkey-system
spec:
  type: openai                        # or anthropic
  baseURL: https://api.openai.com
  keySecretRef: { name: openai-key }  # data key defaults to "apiKey"
  models: [gpt-4o, gpt-4o-mini]       # omit to allow any model
```

## 3. Create the tenant

A tenant is a `Group` in the operator namespace whose **name is the tenant's namespace**:

```yaml
# tenant.yaml
apiVersion: valetkey.dev/v1alpha1
kind: Group
metadata:
  name: billing                       # the tenant namespace
  namespace: valetkey-system
spec:
  channelRefs: [openai]
```

## 4. Create an API key

Keys live in the tenant's namespace:

```yaml
# key.yaml
apiVersion: valetkey.dev/v1alpha1
kind: ProxyKey
metadata:
  name: invoice-api
  namespace: billing
spec:
  models: [gpt-4o-mini]               # optional
  subnets: [10.0.0.0/8]               # optional
  rpm: 120                            # optional
```

```bash
kubectl create namespace billing
kubectl apply -f provider.yaml -f tenant.yaml -f key.yaml
```

## 5. Check that everything is ready

```bash
kubectl get chan,grp -n valetkey-system
kubectl get pkey -A
```

```
NAME                                  TYPE     BASEURL                  STATUS    PRIORITY   READY
channel.valetkey.dev/openai   openai   https://api.openai.com   Enabled   10         true

NAME                                 STATUS    CHANNELS     RPM   TPM   READY
group.valetkey.dev/billing   Enabled   ["openai"]   1     1     true

NAMESPACE   NAME          STATUS    READY   SECRET                 MESSAGE
billing     invoice-api   Enabled   true    invoice-api-valetkey   Active
```

If something isn't ready, the `MESSAGE` column (or `kubectl describe`) says why, for example `group "billing" not found` or `key "apiKey" missing in secret "openai-key"`.

## 6. Read the virtual key

Valetkey writes the key into a Secret named `<key name>-valetkey`, next to the `ProxyKey`:

::: code-group

```bash [Linux / macOS]
kubectl get secret invoice-api-valetkey -n billing -o jsonpath='{.data.VALETKEY_KEY}' | base64 -d
```

```powershell [Windows PowerShell]
$k = kubectl get secret invoice-api-valetkey -n billing -o jsonpath='{.data.VALETKEY_KEY}'
[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($k))
```

:::

Apps usually don't need to copy it at all: mount the Secret with `envFrom` (see [API keys](/guide/api-keys#use-the-secret-in-your-workloads)).

## Everyday tasks

### Disable or enable a key

```bash
kubectl patch pkey invoice-api -n billing --type merge -p '{"spec":{"status":"Disabled"}}'
kubectl patch pkey invoice-api -n billing --type merge -p '{"spec":{"status":"Enabled"}}'
```

### Change a key's models or limits

Edit the YAML and `kubectl apply` it again, or:

```bash
kubectl patch pkey invoice-api -n billing --type merge -p '{"spec":{"rpm":300}}'
```

### Rotate a key

Delete its Secret, then touch the `ProxyKey` so Valetkey notices right away (otherwise it notices at the next resync, within 10 minutes):

```bash
kubectl delete secret invoice-api-valetkey -n billing
kubectl annotate pkey invoice-api -n billing valetkey.dev/rotated-at="$(date +%s)" --overwrite
```

The old key stops working and a new one is written to the same Secret. [`valetkeyctl rotate`](/guide/cli#rotate-a-key) does this in one step.

### Revoke a key

```bash
kubectl delete pkey invoice-api -n billing    # its Secret is deleted with it
```

### Turn off a whole tenant

```bash
kubectl patch grp billing -n valetkey-system --type merge -p '{"spec":{"status":"Disabled"}}'
```

### Replace a provider's API key

Update the Secret; Valetkey picks up the change within seconds, with no restart:

```bash
kubectl create secret generic openai-key -n valetkey-system \
  --from-literal=apiKey=sk-NEW_KEY --dry-run=client -o yaml | kubectl replace -f -
```

## GitOps (Argo CD, Flux)

Commit the `Channel`, `Group` and `ProxyKey` objects like any other manifests. A few things to keep in mind:

- **Never commit provider keys in plain Secrets.** Use whatever you already use for secrets in Git: Sealed Secrets, External Secrets, or SOPS.
- **Don't manage the generated `<name>-valetkey` Secrets in Git.** Valetkey creates them, and they're owned by their `ProxyKey`, so Argo CD shows them as children of the key, and deleting the key deletes them.
- **Valetkey writes `status`** on all three kinds. Argo CD and Flux ignore status differences by default, so your apps stay in sync.
- **The dashboard and GitOps can coexist,** but changes made in the dashboard will be reverted by your GitOps tool on its next sync if the object is also in Git. Pick one owner per object.

See [Custom resources](/reference/crds) for every field.
