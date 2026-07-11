#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
KIND_CLUSTER_NAME=${KIND_CLUSTER_NAME:-kubesqueeze-e2e}
KIND_NODE_IMAGE=${KIND_NODE_IMAGE:?set KIND_NODE_IMAGE to an immutable kindest/node reference}
KUBESQUEEZE_BIN=${KUBESQUEEZE_BIN:-"$ROOT/bin/kubesqueeze"}
KUBECONFIG=${KUBECONFIG:-"$ROOT/.e2e-kubeconfig"}
export KUBECONFIG

for command in kind kubectl jq; do
  command -v "$command" >/dev/null || { echo "missing required command: $command" >&2; exit 2; }
done
test -x "$KUBESQUEEZE_BIN" || { echo "binary is not executable: $KUBESQUEEZE_BIN" >&2; exit 2; }

cleanup() {
  if [[ "${PRESERVE_CLUSTER:-0}" != "1" ]]; then
    kind delete cluster --name "$KIND_CLUSTER_NAME" >/dev/null
    rm -f "$KUBECONFIG"
  fi
}
trap cleanup EXIT

rm -f "$KUBECONFIG"
kind create cluster --name "$KIND_CLUSTER_NAME" --image "$KIND_NODE_IMAGE" --kubeconfig "$KUBECONFIG" --wait 120s
CONTEXT="kind-$KIND_CLUSTER_NAME"

kubectl apply -f "$ROOT/test/e2e/fixtures/workloads.yaml"
if kubectl api-resources --api-group=batch -o name | grep -qx 'cronjobs.batch'; then
  if kubectl get --raw /apis/batch/v1 2>/dev/null | jq -e '.resources[] | select(.name == "cronjobs")' >/dev/null; then
    kubectl apply -f "$ROOT/test/e2e/fixtures/cronjob-v1.yaml"
  else
    kubectl apply -f "$ROOT/test/e2e/fixtures/cronjob-v1beta1.yaml"
  fi
else
  echo "cluster does not serve CronJobs" >&2
  exit 1
fi

common_args=(
  --kubeconfig "$KUBECONFIG"
  --context "$CONTEXT"
  --namespace squeeze-e2e
  --include-name-regex '(api|db|report)-.*'
  --include-label-selector 'app.kubernetes.io/part-of=store'
  --ignore-label-selector 'squeeze.example.com/tier=critical'
)

assert_value() {
  local expected=$1 resource=$2 jsonpath=$3
  local actual
  actual=$(kubectl -n squeeze-e2e get "$resource" -o "jsonpath=$jsonpath")
  if [[ "$actual" != "$expected" ]]; then
    echo "$resource: expected '$expected', got '$actual' for $jsonpath" >&2
    exit 1
  fi
}

assert_json_contract() {
  local operation=$1 file=$2
  jq -e --arg operation "$operation" --arg context "$CONTEXT" '
    .operation == $operation and
    .cluster.context == $context and
    (.cluster.server | type == "string" and length > 0) and
    (.discovered | type == "number") and
    (.included | type == "number") and
    (.ignored | type == "array") and
    (.mutated | type == "array")
  ' "$file" >/dev/null

  # Volatile API-server addresses are normalized; all remaining JSON must
  # exactly match the checked-in operation golden.
  jq -S '.cluster.server = "<server>"' "$file" > "$file.normalized"
  diff -u "$ROOT/test/e2e/golden/$operation.json" "$file.normalized"
}

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"; cleanup' EXIT

"$KUBESQUEEZE_BIN" squeeze "${common_args[@]}" > "$tmpdir/squeeze.json"
assert_json_contract squeeze "$tmpdir/squeeze.json"
assert_value 0 deployment/api-main '{.spec.replicas}'
assert_value 3 deployment/api-main '{.metadata.annotations.kubesqueeze\.io/original-replicas}'
assert_value 0 statefulset/db-main '{.spec.replicas}'
assert_value 2 statefulset/db-main '{.metadata.annotations.kubesqueeze\.io/original-replicas}'
assert_value true cronjob/report-main '{.spec.suspend}'
assert_value false cronjob/report-main '{.metadata.annotations.kubesqueeze\.io/original-suspend}'
assert_value 4 deployment/api-critical '{.spec.replicas}'
assert_value 5 deployment/website-control '{.spec.replicas}'

# A second squeeze must preserve the original snapshots.
"$KUBESQUEEZE_BIN" squeeze "${common_args[@]}" >/dev/null
assert_value 3 deployment/api-main '{.metadata.annotations.kubesqueeze\.io/original-replicas}'

"$KUBESQUEEZE_BIN" restore "${common_args[@]}" > "$tmpdir/restore.json"
assert_json_contract restore "$tmpdir/restore.json"
assert_value 3 deployment/api-main '{.spec.replicas}'
assert_value 2 statefulset/db-main '{.spec.replicas}'
assert_value false cronjob/report-main '{.spec.suspend}'
assert_value 4 deployment/api-critical '{.spec.replicas}'
assert_value 5 deployment/website-control '{.spec.replicas}'

# Restore is repeatable and the namespace boundary protects the control.
"$KUBESQUEEZE_BIN" restore "${common_args[@]}" >/dev/null
assert_value 3 deployment/api-main '{.spec.replicas}'
control=$(kubectl -n squeeze-control get deployment/api-other-namespace -o jsonpath='{.spec.replicas}')
[[ "$control" == 6 ]] || { echo "out-of-scope deployment was mutated" >&2; exit 1; }

echo "E2E passed for $KIND_NODE_IMAGE"
