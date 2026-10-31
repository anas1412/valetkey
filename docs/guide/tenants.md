# Tenants

A tenant (Kubernetes kind: `Group`) is a namespace that is allowed to use Valetkey. Its name **is** the namespace name, and every API key in that namespace belongs to it. A tenant can be a team, an app, a customer or an environment: whatever you split by namespace.

<Shot name="tenants" alt="Tenants list" />

## With kubectl (recommended)

```yaml
apiVersion: valetkey.dev/v1alpha1
kind: Group
metadata:
  name: billing               # = the tenant namespace
  namespace: valetkey-system
spec:
  channelRefs: [openai, anthropic]
  rpmRatio: 1.0
  tpmRatio: 1.0
```

## In the dashboard: create a tenant

Open **Tenants → Create tenant**:

1. **Namespace**: pick one from the list (only namespaces without a tenant are shown). Create a new namespace first with `kubectl create namespace <name>` if needed.
2. **Providers**: tick the providers its keys may use.
3. **Request limit × / Token limit ×**: multipliers for every key's own per-minute limits. `1` keeps them, `2` doubles them, `0.5` halves them.
4. **Tenant enabled**: turn off to stop every key of the tenant at once, without deleting anything.

The tenant is **Ready** when it's enabled and at least one of its providers is usable.

## What tenants can and can't do

- A tenant can only reach the providers you assigned to it.
- Keys can't pick another tenant: the tenant is always the key's own namespace.
- Tenants don't create or change keys. You create them (in YAML or in the dashboard) and hand each key to the person or app that uses it. All a tenant ever holds is the key itself.
