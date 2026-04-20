#!/usr/bin/env bash
set -euo pipefail

GOFLAGS="${GOFLAGS:-}"
go run ./cmd/releasegate \
  -config benchmarks/config.json \
  -report benchmarks/results/latest.json
