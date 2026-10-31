# Valetkey

The minimal LLM proxy for Kubernetes. Go operator in `cmd/` and `internal/`, dashboard in `web/`, Helm chart in `charts/valetkey/`, docs (VitePress) in `docs/`.

## Product rules

- **No tenant self-service.** Tenants are normal users with no cluster or kubectl access. Only the admin creates, rotates and deletes keys (CRDs, `valetkeyctl` or the dashboard) and hands each key out. Don't suggest, build or document tenant RBAC, tenant portals or tenant-created keys.
- **CRD-first.** Providers (`Channel`), tenants (`Group`) and keys (`ProxyKey`) are configured as CRDs for GitOps; `valetkeyctl` is the helper; the dashboard is optional.
- **No admin login.** The dashboard and admin API listen on 127.0.0.1:8082 in the pod and are reached only with `kubectl port-forward`. Kubernetes RBAC is the access control.
- **Minimal.** One small distroless image, no database, no Redis. Never quote exact image size or memory numbers in docs or descriptions; they drift. Prompts and responses are never stored. Usage history comes from the user's own Prometheus.
- **Docs and UI text:** no em dashes, no emojis. Say "tenant", not "team".

## Testing

- Never apply anything to the default kubectl context. Use a throwaway cluster with its own kubeconfig: `hack/demo.sh up` (kind, own kubeconfig in `hack/.demo/`), `hack/demo.sh down` to remove it.
- Go: `go test -race ./internal/...`. Web: `cd web && npx tsc -b && npx vite build`. Chart: `helm lint charts/valetkey`.
