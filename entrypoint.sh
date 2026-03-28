#!/bin/sh
set -e
# tester-server isolation checks
for var in ANTHROPIC_API_KEY CLAUDE_API_TOKEN DYNAMIC_AGENT_KEY MCP_API_TOKEN; do
  eval val=\$$var 2>/dev/null || val=""
  if [ -n "$val" ]; then
    echo "FATAL: $var present in tester-server" >&2; exit 1
  fi
done
if [ -z "$TESTER_API_TOKEN" ]; then
  echo "FATAL: TESTER_API_TOKEN missing" >&2; exit 1
fi
# Verify workspace is mounted
if [ ! -d "/workspace" ]; then
  echo "FATAL: /workspace not mounted" >&2; exit 1
fi
exec /app/tester-server "$@"
