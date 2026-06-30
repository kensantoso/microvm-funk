#!/usr/bin/env bash
set -euo pipefail

ID="${1%.microvm}"
HOST="root@${ID}.microvm"

CREDS=$(security find-generic-password -s "Claude Code-credentials" -w)

ssh "$HOST" "mkdir -p /root/.claude && umask 077 && cat > /root/.claude/.credentials.json" <<< "$CREDS"
ssh "$HOST" "echo '{\"hasCompletedOnboarding\": true}' > /root/.claude.json"
