#!/usr/bin/env bash
# Build the TWO MicroVM images — control plane and worker node — from ONE
# Dockerfile and ONE build context. The images are identical except for the
# image-level env var $ROLE, baked at build time, which the single `start`
# entrypoint dispatches on. No run hook at boot.
# A Go builder stage compiles executor/microtunnel/tundial from source and bakes
# them in, so the only thing uploaded is the source build context (one zip).
#
# Every image is created with ALL OS capabilities and an 8 GiB memory floor.
#
#   ./build.sh
set -euo pipefail
. "$(dirname "$0")/env.sh"
cd "$ROOT"

# Lambda-managed AL2023 base image (from ListManagedMicrovmImages); hard-coded.
BASE_IMAGE="arn:aws:lambda:${REGION}:aws:microvm-image:al2023-1"

OUT=$(mktemp -d)
trap 'rm -rf "$OUT"' EXIT

echo "==> zip build context (shared by both images)"
zip -qr "$OUT/context.zip" go.mod go.sum executor microtunnel tundial image Dockerfile

CODE_URI="s3://$BUCKET/microvm-images/k3s/code-artifact.zip"
echo "==> upload build context to $CODE_URI"
aws s3 cp "$OUT/context.zip" "$CODE_URI" --region "$REGION"

# Create the image (all caps, 8 GiB). If the name already exists, create a new
# version via update instead. Then wait for the async build to finish.
create_image() {  # image-name role
  local name="$1" role="$2"
  local arn="arn:aws:lambda:${REGION}:${ACCOUNT}:microvm-image:${name}"
  echo "==> image '$name' (ROLE=$role, all caps, 8 GiB)"

  local args=(
    --region "$REGION"
    --base-image-arn "$BASE_IMAGE"
    --build-role-arn "$BUILD_ROLE"
    --code-artifact "uri=$CODE_URI"
    --additional-os-capabilities ALL
    --resources minimumMemoryInMiB=8192
    --environment-variables "ROLE=$role"
  )

  local err
  if err=$(aws lambda-microvms create-microvm-image "${args[@]}" --name "$name" 2>&1 >/dev/null); then
    :
  elif grep -qi 'already exists' <<<"$err"; then
    echo "   exists; creating a new version"
    aws lambda-microvms update-microvm-image "${args[@]}" --image-identifier "$arn" >/dev/null
  else
    echo "$err" >&2
    return 1
  fi

  echo -n "   building"
  for _ in $(seq 1 120); do  # ~20 min
    local state
    state=$(aws lambda-microvms get-microvm-image --region "$REGION" \
      --image-identifier "$arn" --query state --output text)
    case "$state" in
      CREATED|UPDATED) echo " ready ($state)"; return 0 ;;
      *FAILED)         echo " FAILED ($state)"; return 1 ;;
    esac
    echo -n "."
    sleep 10
  done
  echo " TIMEOUT"; return 1
}

create_image "$CP_IMAGE_NAME"   controlplane
create_image "$NODE_IMAGE_NAME" node

echo "==> done."
echo "    control plane = $CP_IMG"
echo "    worker node   = $NODE_IMG"
