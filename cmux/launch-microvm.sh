#!/usr/bin/env bash
set -euo pipefail

: "${AWS_REGION:=us-east-1}"
: "${STACK_NAME:=microvm-shell}"

IMAGE_ARN=$(aws cloudformation describe-stacks --region "$AWS_REGION" --stack-name "$STACK_NAME" \
  --query "Stacks[0].Outputs[?OutputKey=='ImageArn'].OutputValue" --output text)
EXEC_ROLE=$(aws cloudformation describe-stacks --region "$AWS_REGION" --stack-name "$STACK_NAME" \
  --query "Stacks[0].Outputs[?OutputKey=='ExecRoleArn'].OutputValue" --output text)

ID=$(aws lambda-microvms run-microvm --region "$AWS_REGION" \
  --image-identifier "$IMAGE_ARN" \
  --execution-role-arn "$EXEC_ROLE" \
  --ingress-network-connectors "arn:aws:lambda:${AWS_REGION}:aws:network-connector:aws-network-connector:SHELL_INGRESS" \
  --idle-policy '{"maxIdleDurationSeconds":600,"suspendedDurationSeconds":120,"autoResumeEnabled":true}' \
  --maximum-duration-in-seconds 28800 \
  --query microvmId --output text)

sleep 30
mkdir -p ~/.config/microvm
echo "$ID" > ~/.config/microvm/last

exec ssh "root@${ID}.microvm"
