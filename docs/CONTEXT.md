# secure-claude-tester: Project Context

## What This Is

A Go REST server that runs tests against whatever repository is mounted at `/workspace`.
Part of the secure-claude cluster — sits alongside the fileserver (mcp-server) and
plan-server as shared infrastructure.

The tester receives `/workspace:ro` and executes the workspace's `test.sh` on demand.
No Docker socket, no privilege escalation — tests run as direct subprocesses with
Go, Python, and pytest pre-installed in the container image.

## Architecture

```
claude-server (or external caller)
  └─> POST /run (Bearer MCP_API_TOKEN)
        └─> tester-server:8443
              ├─ executes: bash /workspace/test.sh
              ├─ captures stdout+stderr
              └─ stores result in memory

  └─> GET /results (Bearer MCP_API_TOKEN)
        └─> returns {status, exit_code, timestamp, output}

  └─> GET /health (no auth)
        └─> 200 OK
```

## Endpoints

| Method | Path | Auth | Description |
| :--- | :--- | :--- | :--- |
| POST | /run | Bearer MCP_API_TOKEN | Start async test run. Returns 409 if already running. |
| GET | /results | Bearer MCP_API_TOKEN | Last test result as JSON |
| GET | /health | None | Healthcheck |

## Response Format

```json
{
  "status": "pass|fail|running|pending",
  "exit_code": 0,
  "timestamp": "2026-03-19T20:00:00Z",
  "output": "...stdout+stderr..."
}
```

## Security

Follows the same isolation pattern as mcp-server (fileserver):

- Runs as UID 1000 (appuser), non-root
- TLS with internal CA-signed certificate
- Bearer token auth (MCP_API_TOKEN) on all endpoints except /health
- entrypoint.sh rejects startup if forbidden secrets are present
- Workspace mounted read-only — tests cannot modify source
- No Docker socket access
- No network access to external services (int_net only)

## Token Isolation

| Token | tester-server |
| :--- | :--- |
| ANTHROPIC_API_KEY | forbidden |
| DYNAMIC_AGENT_KEY | forbidden |
| CLAUDE_API_TOKEN | forbidden |
| MCP_API_TOKEN | required |

## Configuration

| Env var | Default | Description |
| :--- | :--- | :--- |
| MCP_API_TOKEN | (required) | Bearer token for auth |
| TEST_SCRIPT | /workspace/test.sh | Path to test script |
| SSL_CERT_FILE | /app/certs/ca.crt | CA bundle |

## Decisions

| Decision | Chosen | Rejected | Reason |
| :--- | :--- | :--- | :--- |
| Test execution | Direct subprocess | Docker-in-Docker | No socket access needed, simpler, no privilege escalation |
| Language | Go | Python | Matches fileserver pattern, single static binary |
| Concurrency | Reject concurrent runs (409) | Queue | Simple, prevents resource contention |
| Workspace access | Read-only mount | Read-write | Tests should never modify source |
