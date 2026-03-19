# secure-claude-tester: Development Plan

## Current Phase: Initial Setup

### Completed
- [x] Go REST server (main.go) with /run, /results, /health
- [x] TLS + Bearer token auth matching fileserver pattern
- [x] entrypoint.sh with isolation checks
- [x] Concurrent run rejection (409 Conflict)

### TODO
- [ ] Unit tests (main_test.go) — httptest with mock exec
- [ ] test.sh for this repo
- [ ] Integration with tester_mcp.py stdio wrapper in agent
- [ ] Integration with .mcp.json in claude-server
