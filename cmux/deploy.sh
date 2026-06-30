#!/usr/bin/env bash
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
: "${AWS_REGION:=us-east-1}"
: "${STACK_NAME:=microvm-shell}"
: "${IMAGE_NAME:=microvm-shell}"

ACCOUNT=$(aws sts get-caller-identity --query Account --output text)
BUCKET="${IMAGE_NAME}-artifacts-${ACCOUNT}"
KEY="code/shell-$(date +%Y%m%d-%H%M%S).zip"

aws cloudformation deploy --region "$AWS_REGION" \
  --stack-name "$STACK_NAME" --template-file "$HERE/template.yml" \
  --capabilities CAPABILITY_NAMED_IAM \
  --parameter-overrides "ImageName=$IMAGE_NAME"

zip -j /tmp/code.zip "$HERE/Dockerfile"
aws s3 cp --region "$AWS_REGION" /tmp/code.zip "s3://${BUCKET}/${KEY}"

aws cloudformation deploy --region "$AWS_REGION" \
  --stack-name "$STACK_NAME" --template-file "$HERE/template.yml" \
  --capabilities CAPABILITY_NAMED_IAM \
  --parameter-overrides "ImageName=$IMAGE_NAME" "CodeArtifactKey=$KEY"
