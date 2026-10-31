#!/usr/bin/env bash
# A throwaway Valetkey playground: kind cluster, Valetkey built from this checkout, Prometheus,
# Grafana, two fake providers, three tenants and steady traffic. Nothing calls a real model.
#
#   hack/demo.sh up        create or update the playground
#   hack/demo.sh forward   open the dashboard, Grafana and Prometheus on localhost
#   hack/demo.sh down      delete the cluster and everything in it
#
# The cluster gets its own kubeconfig (hack/.demo/kubeconfig); your current kubectl
# context is never used. Needs kind, kubectl, helm and Docker or Podman.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
STATE="$ROOT/hack/.demo"
CLUSTER=valetkey-demo
export KUBECONFIG="$STATE/kubeconfig"

say() { printf '\n==> %s\n' "$*"; }

engine() {
  if command -v docker >/dev/null && docker info >/dev/null 2>&1; then
    echo docker
  elif command -v podman >/dev/null; then
    export KIND_EXPERIMENTAL_PROVIDER=podman
    echo podman
  else
    echo "Docker or Podman is required" >&2
    exit 1
  fi
}

# Builds an image and loads it into the kind nodes.
build() {
  local name=$1 dockerfile=$2
  "$ENGINE" build -q -t "localhost/$name:dev" -f "$ROOT/$dockerfile" "$ROOT" >/dev/null
  "$ENGINE" save -o "$STATE/$name.tar" "localhost/$name:dev"
  kind load image-archive "$STATE/$name.tar" --name "$CLUSTER"
  rm -f "$STATE/$name.tar"
}

up() {
  for tool in kind kubectl helm; do
    command -v "$tool" >/dev/null || { echo "$tool is required" >&2; exit 1; }
  done
  ENGINE=$(engine)
  [[ $ENGINE == podman ]] && export KIND_EXPERIMENTAL_PROVIDER=podman
  mkdir -p "$STATE"

  if ! kind get clusters 2>/dev/null | grep -qx "$CLUSTER"; then
    say "Creating kind cluster $CLUSTER"
    kind create cluster --name "$CLUSTER" --kubeconfig "$KUBECONFIG" --wait 2m
  fi
  kind export kubeconfig --name "$CLUSTER" --kubeconfig "$KUBECONFIG" >/dev/null

  say "Building Valetkey and the fake provider ($ENGINE)"
  build valetkey Dockerfile
  build valetkey-fakeprovider hack/fakeprovider/Dockerfile

  say "Installing Valetkey, Prometheus and Grafana"
  kubectl create namespace monitoring --dry-run=client -o yaml | kubectl apply -f - >/dev/null
  # Helm installs CRDs once and never upgrades them; apply them so new fields work.
  kubectl apply --server-side --force-conflicts -f "$ROOT/charts/valetkey/crds/crds.yaml" >/dev/null
  helm upgrade --install valetkey "$ROOT/charts/valetkey" -n valetkey-system --create-namespace --wait \
    --set image.repository=localhost/valetkey --set image.tag=dev --set image.pullPolicy=Never \
    --set prometheus.url=http://prometheus.monitoring.svc:9090 \
    --set grafanaDashboard.enabled=true --set grafanaDashboard.namespace=monitoring
  kubectl rollout restart -n valetkey-system deploy/valetkey >/dev/null # pick up a rebuilt image
  kubectl apply -f "$ROOT/hack/demo/monitoring.yaml" >/dev/null

  say "Adding fake providers, tenants, keys and traffic"
  kubectl apply -f "$ROOT/hack/demo/providers.yaml" >/dev/null
  kubectl apply -f "$ROOT/hack/demo/tenants.yaml" >/dev/null
  kubectl rollout restart -n fake-llm deploy >/dev/null
  for ns in billing support research; do kubectl rollout restart -n "$ns" deploy/traffic >/dev/null; done

  say "Waiting for everything to start"
  kubectl rollout status -n valetkey-system deploy/valetkey --timeout 2m
  kubectl rollout status -n monitoring deploy/prometheus --timeout 3m
  kubectl rollout status -n monitoring deploy/grafana --timeout 3m
  kubectl rollout status -n fake-llm deploy/fake-openai --timeout 2m
  kubectl rollout status -n fake-llm deploy/fake-anthropic --timeout 2m
  kubectl rollout status -n fake-llm deploy/fake-deepseek --timeout 2m
  for ns in billing support research; do kubectl rollout status -n "$ns" deploy/traffic --timeout 2m; done

  cat <<MSG

Valetkey playground is running. Traffic flows from three tenants (billing, support, research).
The research tenant has a \$0.50 daily budget and runs out after a few minutes.

  Open the UIs:        hack/demo.sh forward
  Dashboard:           http://localhost:8082
  Grafana:             http://localhost:3000
  Prometheus:          http://localhost:9090

  Use kubectl:         export KUBECONFIG=$KUBECONFIG
  Restart Valetkey:    kubectl rollout restart -n valetkey-system deploy/valetkey
                       (the 7 and 30 day history keeps its numbers: they come from Prometheus)
  Remove everything:   hack/demo.sh down
MSG
}

forward() {
  [[ -f $KUBECONFIG ]] || { echo "No playground yet, run: hack/demo.sh up" >&2; exit 1; }
  trap 'kill 0' EXIT
  # kubectl port-forward exits when its pod goes away (restart, upgrade) or the
  # connection drops; reconnect instead of leaving the UI unreachable.
  keep() { while true; do kubectl port-forward "$@" >/dev/null 2>&1; sleep 1; done; }
  keep -n valetkey-system deploy/valetkey 8082:8082 &
  keep -n monitoring svc/grafana 3000:3000 &
  keep -n monitoring svc/prometheus 9090:9090 &
  echo "Dashboard   http://localhost:8082"
  echo "Grafana     http://localhost:3000"
  echo "Prometheus  http://localhost:9090"
  echo "Press Ctrl+C to stop."
  wait
}

down() {
  ENGINE=$(engine)
  [[ $ENGINE == podman ]] && export KIND_EXPERIMENTAL_PROVIDER=podman
  kind delete cluster --name "$CLUSTER" --kubeconfig "$KUBECONFIG"
  rm -rf "$STATE"
}

case "${1:-}" in
  up) up ;;
  forward) forward ;;
  down) down ;;
  *) sed -n '2,10p' "$0" | sed 's/^# \{0,1\}//'; exit 1 ;;
esac
