#!/usr/bin/env bash
# Load persisted spec-cartographer run state for a fresh-wake step session.
# Fresh sessions start in the shared cartographer launcher worktree. The
# state.env locator points this script at the per-run worktree created by
# init-run, and this script cd's there before returning.
#
# Usage from formula step descriptions:
#   WO_PATH_INPUT="specs/agent-work-orders/NNN-name.md" . .gc/scripts/cartographer-load-state.sh
#
# This script is sourced, not executed, so exported variables remain
# available to the formula step shell.

# This file is frequently sourced from agent-generated shell snippets. Keep the
# sourced/executed check POSIX-compatible so `/bin/sh` can load the state too.
if ! (return 0 2>/dev/null); then
  echo "cartographer-load-state.sh must be sourced, not executed" >&2
  exit 2
fi

LAUNCHER_REPO_ROOT="${CARTOGRAPHER_HOME:-${REPO_ROOT:-$(git rev-parse --show-toplevel 2>/dev/null)}}"
if [ -z "$LAUNCHER_REPO_ROOT" ] || [ ! -d "$LAUNCHER_REPO_ROOT" ]; then
  echo "FATAL: cannot resolve repository root for cartographer state load" >&2
  return 2
fi
cd "$LAUNCHER_REPO_ROOT" || return 2

WO_PATH_INPUT="${WO_PATH_INPUT:-${1:-}}"
if [ -z "$WO_PATH_INPUT" ] && [ -n "${GC_BEAD_ID:-}" ] && command -v jq >/dev/null 2>&1; then
  STEP_JSON="$(gc bd show "$GC_BEAD_ID" --json 2>/dev/null || true)"
  PARENT_ID="$(printf '%s\n' "$STEP_JSON" | jq -r '.[0].parent // empty' 2>/dev/null || true)"
  if [ -n "$PARENT_ID" ]; then
    PARENT_JSON="$(gc bd show "$PARENT_ID" --json 2>/dev/null || true)"
    WO_PATH_INPUT="$(printf '%s\n' "$PARENT_JSON" | jq -r '.[0].metadata.work_order_path // empty' 2>/dev/null || true)"
  fi
fi
if [ -z "$WO_PATH_INPUT" ]; then
  echo "FATAL: cartographer-load-state requires work_order_path argument or parent metadata" >&2
  return 2
fi

CITY_ROOT="${CITY_ROOT:-${GC_CITY_ROOT:-}}"
if [ -z "$CITY_ROOT" ] || [ ! -f "$CITY_ROOT/city.toml" ]; then
  CITY_ROOT="$LAUNCHER_REPO_ROOT"
  while [ "$CITY_ROOT" != "/" ] && [ ! -f "$CITY_ROOT/city.toml" ]; do
    CITY_ROOT=$(dirname "$CITY_ROOT")
  done
fi
if [ ! -f "$CITY_ROOT/city.toml" ]; then
  echo "FATAL: cannot locate city.toml walking up from $LAUNCHER_REPO_ROOT" >&2
  return 2
fi

WO_PATH="$WO_PATH_INPUT"
WORK_ORDER_ID="${WORK_ORDER_ID:-$(basename "$WO_PATH" .md)}"

STATE_ENV=""
if [ -n "${RUN_DIR:-}" ] && [ -f "$RUN_DIR/state.env" ]; then
  STATE_ENV="$RUN_DIR/state.env"
elif [ -n "${RUN_DIR:-}" ] && [ -f "$REPO_ROOT/$RUN_DIR/state.env" ]; then
  STATE_ENV="$REPO_ROOT/$RUN_DIR/state.env"
else
  RUN_BASE="$LAUNCHER_REPO_ROOT/.cartographer-runs/$WORK_ORDER_ID"
  if [ -d "$RUN_BASE" ]; then
    STATE_ENV=$(find "$RUN_BASE" -mindepth 2 -maxdepth 2 -name state.env -type f 2>/dev/null | sort | tail -n 1)
  fi
fi

if [ -z "$STATE_ENV" ] || [ ! -f "$STATE_ENV" ]; then
  echo "FATAL: cannot locate state.env for $WORK_ORDER_ID under $LAUNCHER_REPO_ROOT/.cartographer-runs" >&2
  return 2
fi

# shellcheck disable=SC1090
. "$STATE_ENV"

: "${WORK_ORDER_ID:?state.env missing WORK_ORDER_ID}"
: "${RIG:?state.env missing RIG}"
: "${REPO_ROOT:?state.env missing REPO_ROOT}"
: "${RUN_DIR:?state.env missing RUN_DIR}"

CARTOGRAPHER_HOME="${CARTOGRAPHER_HOME:-$LAUNCHER_REPO_ROOT}"
RUN_WORKTREE="${RUN_WORKTREE:-$REPO_ROOT}"
RUN_INDEX_DIR="${RUN_INDEX_DIR:-}"

CITY_ROOT="${CITY_ROOT:-${GC_CITY_ROOT:-}}"
if [ -z "$CITY_ROOT" ] || [ ! -f "$CITY_ROOT/city.toml" ]; then
  CITY_ROOT="$REPO_ROOT"
  while [ "$CITY_ROOT" != "/" ] && [ ! -f "$CITY_ROOT/city.toml" ]; do
    CITY_ROOT=$(dirname "$CITY_ROOT")
  done
fi
if [ ! -f "$CITY_ROOT/city.toml" ]; then
  echo "FATAL: cannot locate city.toml walking up from $REPO_ROOT" >&2
  return 2
fi

case "$RUN_DIR" in
  /*) ;;
  *) RUN_DIR="$REPO_ROOT/$RUN_DIR" ;;
esac

RIG_ROOT="${RIG_ROOT:-${GC_RIG_ROOT:-$REPO_ROOT}}"
GC_RIG_ROOT="${GC_RIG_ROOT:-$RIG_ROOT}"

GC_CITY_ROOT="${GC_CITY_ROOT:-$CITY_ROOT}"

export WO_PATH WORK_ORDER_ID EPOCH RIG REPO_ROOT CARTOGRAPHER_HOME RUN_WORKTREE RUN_INDEX_DIR CITY_ROOT GC_CITY_ROOT RIG_ROOT GC_RIG_ROOT RUN_DIR RIG_BEADS_JSONL
cd "$REPO_ROOT" || return 2
