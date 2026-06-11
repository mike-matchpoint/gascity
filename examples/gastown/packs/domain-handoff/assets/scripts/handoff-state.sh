#!/usr/bin/env bash
# Shared state + single-flight lock for domain-handoff
# orders (dispatch, terminal sweep, command publish, command reconcile).
#
# Sourced (not executed). The caller must set HANDOFF_SCOPE to a unique name
# before sourcing so each order gets an independent lock. Provides:
#   - CITY_ROOT              resolved city root (dir containing city.toml)
#   - PACK_SCHEMA_DIR        this pack's schemas/events directory
#   - HANDOFF_RUN_DIR        per-run temp dir (auto-removed on exit)
#   - handoff_acquire_lock   non-blocking single-flight lock; returns 1 if
#                            another run holds it (caller should exit 0)
#   - handoff_cleanup        remove the run dir and release the lock (wire to trap)
#   - handoff_now            UTC RFC3339 timestamp
#   - handoff_publish_script resolved path of the execution-city-operations
#                            pack's publish-cross-city-event.sh (the single
#                            approved event-bus emitter)
#
# Durable idempotency is NOT kept here: the bead store is the source of truth
# (dispatch/publish/reconcile stamps on beads make every order idempotent).
# This helper only prevents two overlapping ticks from racing and gives each
# run a self-cleaning scratch dir.

if ! (return 0 2>/dev/null); then
  echo "handoff-state.sh must be sourced, not executed" >&2
  exit 2
fi

: "${HANDOFF_SCOPE:?HANDOFF_SCOPE must be set before sourcing handoff-state.sh}"

CITY_ROOT="${CITY_ROOT:-${GC_CITY_ROOT:-$(pwd)}}"
while [ "$CITY_ROOT" != "/" ] && [ ! -f "$CITY_ROOT/city.toml" ]; do
  CITY_ROOT=$(dirname "$CITY_ROOT")
done
if [ ! -f "$CITY_ROOT/city.toml" ]; then
  echo "FATAL: cannot locate city.toml walking up from $(pwd)" >&2
  return 2
fi
export CITY_ROOT GC_CITY_ROOT="$CITY_ROOT"

PACK_SCHEMA_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../schemas/events" && pwd)"

HANDOFF_STATE_BASE="${HANDOFF_STATE_DIR:-${TMPDIR:-/tmp}/domain-handoff}/$HANDOFF_SCOPE"
HANDOFF_LOCK_DIR="$HANDOFF_STATE_BASE/lock"
HANDOFF_RUN_DIR=""

handoff_cleanup() {
  [ -n "$HANDOFF_RUN_DIR" ] && rm -rf "$HANDOFF_RUN_DIR"
  [ -n "${HANDOFF_LOCK_HELD:-}" ] && rmdir "$HANDOFF_LOCK_DIR" 2>/dev/null || true
}

# Non-blocking single-flight lock via atomic mkdir. A stale lock older than
# HANDOFF_LOCK_TTL seconds (default 900) is reclaimed so a crashed run does
# not wedge the order forever.
handoff_acquire_lock() {
  mkdir -p "$HANDOFF_STATE_BASE"
  local ttl="${HANDOFF_LOCK_TTL:-900}"
  if mkdir "$HANDOFF_LOCK_DIR" 2>/dev/null; then
    HANDOFF_LOCK_HELD=1
  else
    local mtime now age
    mtime=$(stat -f %m "$HANDOFF_LOCK_DIR" 2>/dev/null || stat -c %Y "$HANDOFF_LOCK_DIR" 2>/dev/null || echo 0)
    now=$(date +%s)
    age=$((now - mtime))
    if [ "$age" -gt "$ttl" ]; then
      rmdir "$HANDOFF_LOCK_DIR" 2>/dev/null || true
      if mkdir "$HANDOFF_LOCK_DIR" 2>/dev/null; then HANDOFF_LOCK_HELD=1; else return 1; fi
    else
      return 1
    fi
  fi
  HANDOFF_RUN_DIR=$(mktemp -d "$HANDOFF_STATE_BASE/run.XXXXXX")
  export HANDOFF_RUN_DIR
  return 0
}

handoff_now() {
  date -u +%Y-%m-%dT%H:%M:%SZ
}

# The emitter lives in the execution-city-operations pack (shared event-bus
# infrastructure). Resolve it the way agent prompts do: explicit env override,
# the installed system pack under the city root, a source-owned pack, then a
# sibling pack checkout (source-tree tests).
handoff_publish_script() {
  local script_dir candidate
  script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  for candidate in \
    "${EXECUTION_CITY_OPS_PACK_DIR:-}" \
    "$CITY_ROOT/.gc/system/packs/execution-city-operations" \
    "$CITY_ROOT/packs/execution-city-operations" \
    "$script_dir/../../../execution-city-operations"; do
    [ -n "$candidate" ] || continue
    if [ -x "$candidate/assets/scripts/publish-cross-city-event.sh" ]; then
      printf '%s/assets/scripts/publish-cross-city-event.sh\n' "$candidate"
      return 0
    fi
  done
  echo "FATAL: cannot locate publish-cross-city-event.sh in the execution-city-operations pack" >&2
  return 1
}
