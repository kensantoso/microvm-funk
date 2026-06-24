# Shared config for every kube-minimal script. Override any value via the
# environment. ROOT is the repo root (the Go module lives there; tundial is the
# only in-repo binary still invoked via `go run`).
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

export BUCKET=${BUCKET:-microvm-pty-probe-${ACCOUNT}}
export BUILD_ROLE=${BUILD_ROLE:-arn:aws:iam::${ACCOUNT}:role/microvm-pty-probe-build}
export EXEC_ROLE=${EXEC_ROLE:-arn:aws:iam::${ACCOUNT}:role/microvm-pty-probe-exec}

# Two images: one per role (the role is baked in; no run hook at boot).
export CP_IMAGE_NAME=${CP_IMAGE_NAME:-k3s-cp}
export NODE_IMAGE_NAME=${NODE_IMAGE_NAME:-k3s-node}
export CP_IMG=${CP_IMG:-arn:aws:lambda:${REGION}:${ACCOUNT}:microvm-image:${CP_IMAGE_NAME}}
export NODE_IMG=${NODE_IMG:-arn:aws:lambda:${REGION}:${ACCOUNT}:microvm-image:${NODE_IMAGE_NAME}}

export ENVF=${ENVF:-/tmp/kube-minimal.env}

# Resolve a MicroVM's native ingress endpoint hostname.
mv_endpoint() {  # id
  aws lambda-microvms get-microvm --region "$REGION" \
    --microvm-identifier "$1" --query endpoint --output text
}

# Mint a fresh port-scoped ingress auth token for a MicroVM.
mv_token() {  # id port
  aws lambda-microvms create-microvm-auth-token --region "$REGION" \
    --microvm-identifier "$1" --expiration-in-minutes 30 \
    --allowed-ports "[{\"port\":$2}]" \
    --query 'authToken."X-aws-proxy-auth"' --output text
}

# Run a one-shot command in a MicroVM via the in-image executor /exec endpoint
# (POST the command as the body so no URL-encoding is needed).
sh_() {  # id cmd
  local ep tok
  ep=$(mv_endpoint "$1") || return 1
  tok=$(mv_token "$1" 8080) || return 1
  printf '%s' "$2" | curl -sk --max-time 290 -X POST "https://$ep/exec" \
    -H "X-aws-proxy-auth: $tok" -H "X-aws-proxy-port: 8080" --data-binary @-
}
