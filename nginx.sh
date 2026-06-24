#!/usr/bin/env bash
# Launch an nginx pod and curl it through the MicroVM's NATIVE AWS ingress
# (no tundial). The native ingress reaches the node's MAIN netns, so we run
# nginx as a hostNetwork pod -> it binds :80 on n1 -> the port header hits it.
#
#   ./nginx.sh        (run after up.sh + kubeconfig.sh)
set -o pipefail
. "$(dirname "$0")/env.sh"
cd "$ROOT"
. "$ENVF"
export KUBECONFIG=/tmp/kubeconfig.laptop

echo "==> launch hostNetwork nginx pod on n1"
kubectl run nginx-host --image=public.ecr.aws/nginx/nginx:latest \
  --overrides='{"spec":{"hostNetwork":true,"nodeSelector":{"kubernetes.io/hostname":"n1"}}}' \
  2>/dev/null || true
for i in $(seq 1 30); do
  [ "$(kubectl get pod nginx-host -o jsonpath='{.status.phase}' 2>/dev/null)" = "Running" ] && break
  sleep 4
done
kubectl get pod nginx-host -o wide

echo
echo "==> curl nginx via the native AWS ingress (token + X-aws-proxy-port: 80)"
ep=$(mv_endpoint "$n1_id")
tok=$(mv_token "$n1_id" 80)
curl -sk --max-time 60 "https://$ep/" \
  -H "X-aws-proxy-auth: $tok" -H "X-aws-proxy-port: 80" | head -6
