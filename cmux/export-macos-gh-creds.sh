#!/usr/bin/env bash
set -euo pipefail

ID="${1%.microvm}"
HOST="root@${ID}.microvm"

TOKEN=$(gh auth token)

ssh "$HOST" "umask 077 && cat > /root/.git-credentials && \
             git config --global credential.helper store" \
  <<< "https://x-access-token:${TOKEN}@github.com"

scp -q ~/.gitconfig "${HOST}:/root/.gitconfig"
