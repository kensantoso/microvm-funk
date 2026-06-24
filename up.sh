#!/usr/bin/env bash
# Launch a k3s cluster across MicroVMs: 1 control plane + N workers. There are
# TWO images:
#   control plane: base-prep + k3s server (CMD = start-controlplane)
#   worker:        base-prep, then idles ready to join (CMD = start-node)
#
# up.sh only supplies the runtime coordinates a worker can't know until launch
# (the control plane's MicroVM id + IPv6), pushed once over /exec via node-join.
# (Cross-node pod<->pod networking was removed.)
#
#   ./up.sh
set -o pipefail
. "$(dirname "$0")/env.sh"
cd "$ROOT"

KC="k3s kubectl --kubeconfig=/tmp/kubeconfig"

launchvm() {  # image-arn -> prints microvm id (after it reaches RUNNING)
  local id
  id=$(aws lambda-microvms run-microvm --region "$REGION" \
    --image-identifier "$1" --execution-role-arn "$EXEC_ROLE" \
    --idle-policy '{"maxIdleDurationSeconds":28800,"suspendedDurationSeconds":300,"autoResumeEnabled":true}' \
    --maximum-duration-in-seconds 28800 \
    --query microvmId --output text)
  for _ in $(seq 1 60); do
    [ "$(aws lambda-microvms get-microvm --region "$REGION" \
        --microvm-identifier "$id" --query state --output text 2>/dev/null)" = RUNNING ] && break
    sleep 2
  done
  echo "$id"
}
wait_until() {  # desc want snippet(run on cp)
  for i in $(seq 1 30); do
    [ "$(sh_ "$cp_id" "$3" 2>/dev/null | tail -1)" = "$2" ] && { echo "   $1"; return; }
    sleep 5
  done
  echo "   TIMEOUT: $1"
}

echo "==> launch control plane (k3s server starts from the baked CMD)"
cp_id=$(launchvm "$CP_IMG"); echo "   cp=$cp_id"
wait_until "apiserver up" 401 "curl -sk --max-time 4 -o /dev/null -w '%{http_code}' https://[::1]:6443/healthz"
cp_ip6=$(sh_ "$cp_id" "ip -6 addr show scope global | grep -oE '2600:[0-9a-f:]+' | head -1" 2>/dev/null | tail -1)
echo "   cp_ip6=$cp_ip6"

echo "==> launch workers n1, n2"
n1_id=$(launchvm "$NODE_IMG"); echo "   n1=$n1_id"
n2_id=$(launchvm "$NODE_IMG"); echo "   n2=$n2_id"

echo "==> join workers to the control plane (over /exec)"
sh_ "$n1_id" "CP_ID=$cp_id CP_IP6=$cp_ip6 NAME=n1 node-join" >/dev/null
sh_ "$n2_id" "CP_ID=$cp_id CP_IP6=$cp_ip6 NAME=n2 node-join" >/dev/null

cat >"$ENVF" <<EOF
cp_id=$cp_id
n1_id=$n1_id
n2_id=$n2_id
cp_ip6=$cp_ip6
EOF

wait_until "2 nodes Ready" 2 "$KC get nodes --no-headers | grep -c ' Ready '"

echo
echo "==> cluster up. env in $ENVF"
echo "    next: ./kubeconfig.sh ; ./nginx.sh"
