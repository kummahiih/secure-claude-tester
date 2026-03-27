# secure-claude-tester: Development Plan

## Current Phase: Initial Setup

### Completed
- [x] Go REST server (main.go) with /run, /results, /health
- [x] TLS + Bearer token auth matching fileserver pattern
- [x] entrypoint.sh with isolation checks
- [x] Concurrent run rejection (409 Conflict)

### TODO
- [x] Unit tests (main_test.go) — httptest with mock exec
- [x] test.sh for this repo
- [x] Subprocess timeout (context.WithTimeout, 300s default, configurable via TEST_TIMEOUT env, exit code 124 — RR-2)
- [ ] Integration with tester_mcp.py stdio wrapper in agent
- [ ] Integration with .mcp.json in claude-server
