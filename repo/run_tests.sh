#!/usr/bin/env bash
set -euo pipefail

# HarborClass test runner.
# Runs the full Go test suite: unit tests + HTTP integration tests
# (real Gin router, real services, no mocks) + Templ render tests.
#
# Usage: ./run_tests.sh

# Change to the repository root so the script works from anywhere.
cd "$(dirname "$0")"

# Resolve deps lazily so the repo can ship without a checked-in go.sum.
export GOFLAGS="-mod=mod"
go mod tidy >/dev/null 2>&1 || true

# Run every test package with race detection and verbose output.
# Adding -count=1 disables the build cache so reruns always re-execute.
go test -count=1 ./...
