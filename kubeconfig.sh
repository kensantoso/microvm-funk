#!/usr/bin/env bash
# Give this laptop a kubectl: start a local tundial to the apiserver MicroVM
# (127.0.0.1:6443 -> cp:6443 over the token-gated WebSocket) and write a
# kubeconfig pointing at it.
#
#   ./kubeconfig.sh
#   export KUBECONFIG=/tmp/kubeconfig.laptop && kubectl get nodes -o wide
set -o pipefail
. "$(dirname "$0")/env.sh"
cd "$ROOT"
. "$ENVF"

pkill -f 'tundial .*-listen 127.0.0.1:6443' 2>/dev/null || true
sleep 1
nohup go run ./tundial -microvm-id "$cp_id" -agent-port 8081 -target-port 6443 \
  -listen 127.0.0.1:6443 >/tmp/tundial-apiserver.log 2>&1 &
echo "tundial(apiserver) -> $cp_id :6443"
sleep 3

sh_ "$cp_id" 'cat /tmp/kubeconfig' 2>/dev/null \
  | sed -n '/^apiVersion:/,$p' \
  | sed -E 's#server: https://\[?[^]/]*\]?:6443#server: https://127.0.0.1:6443#' \
  > /tmp/kubeconfig.laptop

echo "wrote /tmp/kubeconfig.laptop"
echo "  export KUBECONFIG=/tmp/kubeconfig.laptop"
echo "  kubectl get nodes -o wide"
