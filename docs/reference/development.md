# Development

LLMP is one Go binary with the dashboard embedded. You need **Go 1.26+** and **Node.js 24+** (Node only builds the dashboard and these docs). Everything works the same on Windows, macOS and Linux.

## Layout

```
cmd/llmproxy/        entry point
internal/
  admin/             admin API handlers (dashboard backend)
  auth/              virtual-key auth, admin sessions
  reconciler/        informers, key minting, CRD status
  relay/             proxying, failover, rate limits, OpenAI↔Anthropic translation
  kube/ registry/ stats/ metrics/ server/ …
web/                 dashboard (React + EasyUI), built into web/dist and embedded
docs/                this site (VitePress)
deploy/              CRDs and manifests
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

Dashboard with hot reload: run the operator, then `cd web && npm run dev` and open <http://localhost:5181>.

## Local cluster

```bash
kind create cluster --name llmp --kubeconfig ./kind.kubeconfig
export KUBECONFIG=./kind.kubeconfig
docker build -t llmp:dev .
kind load docker-image llmp:dev --name llmp
kubectl create namespace llmproxy-system
kubectl apply -f deploy/crds.yaml -f deploy/operator.yaml
kubectl set image -n llmproxy-system deploy/llmproxy llmproxy=llmp:dev
```

## Docs, screenshots and the video

```bash
cd docs && npm ci && npm run dev     # http://localhost:5173/llmp/
```

Screenshots and the walkthrough video come from a running dashboard, captured with a local Chromium:

```bash
ADMIN_KEY=... node docs/scripts/screenshots.mjs http://localhost:8081
ADMIN_KEY=... node docs/scripts/walkthrough.mjs http://localhost:8081   # needs ffmpeg
```

## Releases

Pushing a `v*` tag builds and pushes `ghcr.io/anas1412/llmp:<version>` for amd64 and arm64.
