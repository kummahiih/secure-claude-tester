#!/bin/bash
set -euo pipefail

echo "[unit] Running Go tester-server tests..."
go test -v ./...
