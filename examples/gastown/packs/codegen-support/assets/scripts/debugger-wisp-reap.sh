#!/usr/bin/env bash
# Compatibility entrypoint for older docs or installed orders that still
# invoke debugger-wisp-reap.sh. The cutover implementation lives in
# debugger-plan-reap.sh and handles both durable plan beads and legacy wisps.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec "$SCRIPT_DIR/debugger-plan-reap.sh" "$@"
