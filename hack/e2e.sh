#!/usr/bin/env bash
# End-to-end checks against the playground (hack/demo.sh up): requests through the
# proxy, embeddings and the Responses API, a budget that runs out, key expiry, spend
# that survives a restart, two replicas, and the Prometheus alert rules.
#
#   hack/demo.sh up && hack/e2e.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
export KUBECONFIG="$ROOT/hack/.demo/kubeconfig"
[[ -f $KUBECONFIG ]] || { echo "No playground: run hack/demo.sh up first" >&2; exit 1; }
NS=valetkey-system
PORT=18765

step() { printf '\n==> %s\n' "$*"; }
fail() { echo "FAIL: $*" >&2; exit 1; }
pass() { echo "ok: $*"; }

PF_PID=
forward() {
  [[ -n $PF_PID ]] && kill "$PF_PID" 2>/dev/null || true
  kubectl port-forward -n "$NS" svc/valetkey "$PORT:8000" >/dev/null 2>&1 &
  PF_PID=$!
  for _ in $(seq 1 50); do curl -s "localhost:$PORT/v1/models" >/dev/null 2>&1 && return; sleep 0.2; done
  fail "port-forward to the proxy did not start"
}
cleanup() {
  [[ -n $PF_PID ]] && kill "$PF_PID" 2>/dev/null || true
  kubectl delete pkey -n billing e2e e2e-expiring --ignore-not-found >/dev/null 2>&1 || true
  kubectl scale -n "$NS" deploy/valetkey --replicas=1 >/dev/null 2>&1 || true
}
trap cleanup EXIT

# post <key> <path> <json>: prints the HTTP status, body in $BODY_FILE
BODY_FILE=$(mktemp)
post() { curl -s -o "$BODY_FILE" -w '%{http_code}' -H "Authorization: Bearer $1" -H 'Content-Type: application/json' -d "$3" "localhost:$PORT$2"; }
key_of() { # namespace name
  for _ in $(seq 1 60); do
    k=$(kubectl get secret -n "$1" "$2-valetkey" -o jsonpath='{.data.VALETKEY_KEY}' 2>/dev/null | base64 -d 2>/dev/null || true)
    [[ -n $k ]] && { echo "$k"; return; }
    sleep 1
  done
  fail "key $1/$2 was never minted"
}
spend_of() { kubectl get pkey -n billing "$1" -o jsonpath='{.status.spend.usd}'; }

step "Keys are ready"
kubectl wait -n billing pkey/invoice-api --for=jsonpath='{.status.ready}'=true --timeout=2m >/dev/null
pass "invoice-api is ready"

step "Requests through the proxy"
kubectl apply -f - >/dev/null <<YAML
apiVersion: valetkey.dev/v1alpha1
kind: ProxyKey
metadata: { name: e2e, namespace: billing }
spec:
  maxTokens: 50
  budget: { usd: "0.02", period: daily }
YAML
forward
KEY=$(key_of billing e2e)
sleep 2 # the route is added right after the key is written
code=$(post "$KEY" /v1/chat/completions '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}')
[[ $code == 200 ]] || fail "chat completion: HTTP $code $(cat "$BODY_FILE")"
pass "chat completion"
code=$(post "$KEY" /v1/embeddings '{"model":"gpt-4o-mini","input":"hello"}')
[[ $code == 200 ]] && grep -q '"embedding"' "$BODY_FILE" || fail "embeddings: HTTP $code $(cat "$BODY_FILE")"
pass "embeddings"
code=$(post "$KEY" /v1/responses '{"model":"gpt-4o-mini","input":"hi"}')
[[ $code == 200 ]] && grep -q '"output"' "$BODY_FILE" || fail "responses: HTTP $code $(cat "$BODY_FILE")"
pass "responses API"
code=$(post "sk-proxy-not-a-key" /v1/chat/completions '{"model":"gpt-4o-mini","messages":[]}')
[[ $code == 401 ]] || fail "unknown key got HTTP $code"
pass "unknown key refused"

step "A \$0.02 daily budget runs out and is never exceeded"
refused=
for i in $(seq 1 400); do
  code=$(post "$KEY" /v1/chat/completions '{"model":"gpt-4o","messages":[{"role":"user","content":"spend"}]}')
  if [[ $code == 429 ]] && grep -q insufficient_quota "$BODY_FILE"; then refused=$i; break; fi
done
[[ -n $refused ]] || fail "budget never ran out"
pass "refused after $refused requests: $(sed 's/.*"message":"\([^"]*\)".*/\1/' "$BODY_FILE")"
sleep 7 # spend is saved every 5 s
spent=$(spend_of e2e)
awk -v s="$spent" 'BEGIN { exit !(s > 0.015 && s <= 0.02) }' || fail "spend $spent is not just under \$0.02"
pass "spend saved in status: \$$spent"
kubectl get events -n billing --field-selector reason=BudgetReached -o name | grep -q . || fail "no BudgetReached event"
pass "BudgetReached event recorded"

step "Spend survives a restart"
kubectl rollout restart -n "$NS" deploy/valetkey >/dev/null
kubectl rollout status -n "$NS" deploy/valetkey --timeout=2m >/dev/null
forward
sleep 3
code=$(post "$KEY" /v1/chat/completions '{"model":"gpt-4o","messages":[{"role":"user","content":"still?"}]}')
[[ $code == 429 ]] || fail "after a restart the budget is forgotten: HTTP $code"
pass "budget still used up after the restart"

step "Key expiry"
expires=$(date -u -d '+25 seconds' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -v+25S +%Y-%m-%dT%H:%M:%SZ)
kubectl apply -f - >/dev/null <<YAML
apiVersion: valetkey.dev/v1alpha1
kind: ProxyKey
metadata: { name: e2e-expiring, namespace: billing }
spec: { expiresAt: "$expires" }
YAML
EKEY=$(key_of billing e2e-expiring)
sleep 2
code=$(post "$EKEY" /v1/chat/completions '{"model":"gpt-4o-mini","messages":[]}')
[[ $code == 200 ]] || fail "key before expiry: HTTP $code $(cat "$BODY_FILE")"
until [[ $(date -u +%s) -gt $(( $(date -u -d "$expires" +%s 2>/dev/null || date -u -j -f %Y-%m-%dT%H:%M:%SZ "$expires" +%s) + 1 )) ]]; do sleep 1; done
code=$(post "$EKEY" /v1/chat/completions '{"model":"gpt-4o-mini","messages":[]}')
[[ $code == 401 ]] || fail "expired key got HTTP $code"
kubectl wait -n billing pkey/e2e-expiring --for=jsonpath='{.status.conditions[0].reason}'=Expired --timeout=30s >/dev/null
pass "expired key refused and marked Expired"

step "A provider that rejects its key gets no traffic"
kubectl apply -f - >/dev/null <<YAML
apiVersion: v1
kind: Secret
metadata: { name: e2e-revoked-key, namespace: $NS }
stringData: { apiKey: rejected }
---
apiVersion: valetkey.dev/v1alpha1
kind: Channel
metadata: { name: e2e-revoked, namespace: $NS }
spec: { type: openai, baseURL: "http://fake-openai.fake-llm.svc", keySecretRef: { name: e2e-revoked-key } }
YAML
kubectl wait -n "$NS" channel/e2e-revoked --for=jsonpath='{.status.conditions[0].reason}'=KeyRejected --timeout=30s >/dev/null ||
  fail "rejected key not detected: $(kubectl get channel -n "$NS" e2e-revoked -o jsonpath='{.status.conditions[0].message}')"
pass "rejected key detected: $(kubectl get channel -n "$NS" e2e-revoked -o jsonpath='{.status.conditions[0].message}')"
kubectl patch secret -n "$NS" e2e-revoked-key --type merge -p '{"stringData":{"apiKey":"sk-fixed"}}' >/dev/null
kubectl wait -n "$NS" channel/e2e-revoked --for=jsonpath='{.status.conditions[0].reason}'=KeyResolved --timeout=30s >/dev/null ||
  fail "fixed key still marked: $(kubectl get channel -n "$NS" e2e-revoked -o jsonpath='{.status.conditions[0].reason}')"
pass "a new key clears it"
kubectl delete channel -n "$NS" e2e-revoked >/dev/null
kubectl delete secret -n "$NS" e2e-revoked-key >/dev/null

step "Two replicas"
kubectl scale -n "$NS" deploy/valetkey --replicas=2 >/dev/null
kubectl rollout status -n "$NS" deploy/valetkey --timeout=2m >/dev/null
holder=$(kubectl get lease -n "$NS" valetkey -o jsonpath='{.spec.holderIdentity}')
kubectl get pods -n "$NS" -o name | grep -q "$holder" || fail "lease holder $holder is not a running pod"
pass "one leader: $holder"
kubectl delete pkey -n billing e2e-expiring >/dev/null
kubectl apply -f - >/dev/null <<YAML
apiVersion: valetkey.dev/v1alpha1
kind: ProxyKey
metadata: { name: e2e-expiring, namespace: billing }
spec: {}
YAML
forward
RKEY=$(key_of billing e2e-expiring)
sleep 3
for i in 1 2 3 4 5 6; do
  code=$(post "$RKEY" /v1/chat/completions '{"model":"gpt-4o-mini","messages":[]}')
  [[ $code == 200 ]] || fail "request $i with two replicas: HTTP $code"
done
pass "a new key works on both replicas"

step "Prometheus alert rules load"
helm template valetkey "$ROOT/charts/valetkey" -n "$NS" --set prometheusRule.enabled=true -s templates/prometheusrule.yaml |
  sed -n '/^  groups:/,$p' | sed 's/^  //' |
  kubectl exec -i -n monitoring deploy/prometheus -- sh -c 'cat > /tmp/rules.yml && promtool check rules /tmp/rules.yml' >/dev/null ||
  fail "promtool rejected the rules"
pass "promtool accepts the rules"

echo
echo "All end-to-end checks passed."
