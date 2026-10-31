# Development

LLMP is one Go binary with the dashboard embedded. You need **Go 1.26+** and **Node.js 24+** (Node only builds the dashboard and these docs). Everything works the same on Windows, macOS and Linux.

## Layout

```
cmd/llmproxy/        entry point
internal/
  admin/             admin API handlers (dashboard backend)
  auth/              virtual-key auth
  reconciler/        informers, key minting, CRD status
  relay/             proxying, failover, rate limits, OpenAI↔Anthropic translation
  kube/ registry/ stats/ metrics/ server/ …
web/                 dashboard (React + EasyUI), built into web/dist and embedded
docs/                this site (VitePress)
charts/llmp/         Helm chart (CRDs in crds/)
examples/            sample Channel, Group and ProxyKey objects
scripts/llmproxyctl  CLI helper
```

## Build and test

```bash
cd web && npm ci && npm run build && cd ..   # dashboard → web/dist
go test ./internal/...                        # unit, relay and admin API tests, no cluster needed
go run ./cmd/llmproxy                         # uses your current kubeconfig
```

::: danger Check your kube context
`go run` talks to whatever cluster your kubeconfig points at. Use a local test cluster and pass `KUBECONFIG` explicitly.
:::

Run locally, the dashboard is at <http://localhost:8082>. With hot reload: run the operator, then `cd web && npm run dev` and open <http://localhost:5181>.

## Local cluster

```bash
kind create cluster --name llmp --kubeconfig ./kind.kubeconfig
export KUBECONFIG=./kind.kubeconfig
docker build -t llmp:dev .
kind load docker-image llmp:dev --name llmp
helm install llmproxy charts/llmp -n llmproxy-system --create-namespace \
  --set image.repository=llmp --set image.tag=dev
```

## Docs, screenshots and the video

```bash
cd docs && npm ci && npm run dev     # http://localhost:5173/llmp/
```

Screenshots and the walkthrough video come from a running dashboard, captured with a local Chromium:

```bash
kubectl port-forward -n llmproxy-system deploy/llmproxy 8082:8082 &
node docs/scripts/screenshots.mjs http://localhost:8082
node docs/scripts/walkthrough.mjs http://localhost:8082   # needs ffmpeg
```

## Releases

Pushing a `v*` tag publishes the image `ghcr.io/anas1412/llmp:<version>` (amd64 and arm64), the chart `oci://ghcr.io/anas1412/charts/llmp`, and a GitHub release with the chart and rendered `llmp.yaml`.
