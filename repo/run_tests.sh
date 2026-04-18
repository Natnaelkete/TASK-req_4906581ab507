#!/usr/bin/env bash
set -euo pipefail

# HarborClass test runner.
#
# Runs the full test suite (unit + HTTP integration + Templ renderers)
# entirely inside Docker against the committed go.mod + go.sum. No host
# Go toolchain is required and no dependency installation happens at
# test time - modules resolve from the locked go.sum during the
# `docker build` layer that produces the tests image, then tests run
# offline against that image.
#
# Usage: ./run_tests.sh

cd "$(dirname "$0")"

IMAGE_TAG="harborclass-tests:local"

docker build --target tests -t "${IMAGE_TAG}" .

# `--network none` proves the tests run without reaching the module
# proxy or any other external endpoint: deps are already baked into the
# image by the deps stage and the HTTP integration tests use
# httptest.NewRecorder against the in-process router, never a socket.
docker run --rm --network none "${IMAGE_TAG}"
