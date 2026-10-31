# Development

Valetkey is one Go binary with the dashboard embedded. You need **Go 1.26+** and **Node.js 24+** (Node only builds the dashboard and these docs). Everything works the same on Windows, macOS and Linux.

## Layout

```
cmd/valetkey/        entry point
internal/
  admin/             admin API handlers (dashboard backend)
  auth/              virtual-key auth
  reconciler/        informers, key minting, CRD status
  relay/             proxying, failover, rate limits, OpenAI↔Anthropic translation
  kube/ registry/ stats/ metrics/ server/ …
web/                 dashboard (React + EasyUI), built into web/dist and embedded
docs/                this site (VitePress)
charts/valetkey/     Helm chart (CRDs in crds/, Grafana dashboard in dashboards/)
hack/                playground (demo.sh) and its fake provider
examples/            sample Channel, Group and ProxyKey objects
```

## Build and test

```bash
cd web && npm ci && npm run build && cd ..   # dashboard → web/dist
go test ./internal/...                        # unit, relay and admin API tests, no cluster needed
go run ./cmd/valetkey                         # uses your current kubeconfig
```

::: danger Check your kube context
`go run` talks to whatever cluster your kubeconfig points at. Use a local test cluster and pass `KUBECONFIG` explicitly.
:::

Run locally, the dashboard is at <http://localhost:8082>. With hot reload: run the operator, then `cd web && npm run dev` and open <http://localhost:5181>.

## Playground

One command gives you a throwaway cluster with everything wired up: Valetkey built from your checkout, Prometheus, Grafana, two fake providers (OpenAI-style and Anthropic-style), three tenants with keys, and steady traffic, including some refused models and rate-limited requests. Nothing calls a real model. You need kind, kubectl, helm and Docker or Podman.

```bash
hack/demo.sh up        # create or update it (a few minutes the first time)
hack/demo.sh forward   # dashboard on :8082, Grafana on :3000, Prometheus on :9090
hack/demo.sh down      # delete it
```

The cluster has its own kubeconfig in `hack/.demo/kubeconfig`; your current kubectl context is never touched. Run `up` again after changing code to rebuild and redeploy.

To see history survive a restart, restart Valetkey and check that the 7 and 30 day numbers on the Overview stay put:

```bash
KUBECONFIG=hack/.demo/kubeconfig kubectl rollout restart -n valetkey-system deploy/valetkey
```

## Local cluster by hand

```bash
kind create cluster --name valetkey --kubeconfig ./kind.kubeconfig
export KUBECONFIG=./kind.kubeconfig
docker build -t valetkey:dev .
kind load docker-image valetkey:dev --name valetkey
helm install valetkey charts/valetkey -n valetkey-system --create-namespace \
  --set image.repository=valetkey --set image.tag=dev
```

## Docs, screenshots and the video

```bash
cd docs && npm ci && npm run dev     # http://localhost:5173/valetkey/
```

Screenshots and the walkthrough video come from a running dashboard, captured with a local Chromium:

```bash
kubectl port-forward -n valetkey-system deploy/valetkey 8082:8082 &
node docs/scripts/screenshots.mjs http://localhost:8082
node docs/scripts/walkthrough.mjs http://localhost:8082   # needs ffmpeg
```

## Releases

Pushing a `v*` tag publishes the image `ghcr.io/anas1412/valetkey:<version>` (amd64 and arm64), the chart `oci://ghcr.io/anas1412/charts/valetkey`, and a GitHub release with the chart and rendered `valetkey.yaml`.
