# Secure Claude Tester

Test runner server for the [secure-claude](https://github.com/kummahiih/secure-claude) cluster.

A Go REST server that executes `/workspace/test.sh` on demand and reports results as JSON. The workspace under test is mounted read-only — no Docker socket or privilege escalation required.

## Endpoints

| Method | Path | Auth | Description |
| :--- | :--- | :--- | :--- |
| POST | /run | Bearer | Start a test run (async, 409 if already running) |
| GET | /results | Bearer | Get last test result |
| GET | /health | None | Healthcheck |

## Local Development

Run the Go unit tests:

```bash
go test -v ./...
```

## Documentation

- [docs/CONTEXT.md](docs/CONTEXT.md) — Architecture, security model, design decisions
- [docs/PLAN.md](docs/PLAN.md) — Development roadmap and task backlog

## Part of Secure Claude

This repo is a git submodule of [secure-claude](https://github.com/kummahiih/secure-claude) at `cluster/tester/`. See the parent repo for Docker orchestration and cluster setup.
