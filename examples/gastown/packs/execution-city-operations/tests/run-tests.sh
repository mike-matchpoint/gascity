#!/usr/bin/env bash
set -euo pipefail

PACK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO_ROOT="$(git -C "$PACK_DIR" rev-parse --show-toplevel)"

GC_FAST_UNIT=1 go test "$PACK_DIR" -run 'TestEval'

git -C "$REPO_ROOT" diff --quiet origin/main -- \
  examples/gastown/packs/execution-city-operations/agents/prompt-eval-classifier \
  examples/gastown/packs/execution-city-operations/agents/prompt-eval-judge \
  examples/gastown/packs/execution-city-operations/agents/prompt-eval-evidence-gatherer

printf 'execution-city-operations eval tests: PASS\n'
