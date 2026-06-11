#!/usr/bin/env bash
# domain-command-reconcile — apply inbound GasCityDomainCommandTerminal.v1
# beads to their waiting domain_command_request beads.
#
# Triggered by orders/domain-command-reconcile.toml on a cooldown.
#
# The domain bridge answers a command request by publishing a cross-city
# ENVELOPE event addressed to this city with payload.route =
# <binding>.command-terminal-reconciler; the ingress adapter turns it into
# a routed event_handoff bead whose payload.* metadata carries the flat
# terminal fields (command_id, status, result_s3_uri, ...).
#
# For each such bead this order:
#   1. finds the open domain_command_request waiter stamped with the same
#      handoff.command_id,
#   2. stamps the terminal result onto the waiter
#      (handoff.command_status, handoff.command_terminal) and notes the
#      origin step bead,
#   3. closes the waiter — unblocking the molecule step that depends on it —
#   4. closes the inbound terminal bead.
#
# The consuming step agent reads handoff.command_status /
# handoff.command_terminal from the closed waiter and decides how a FAILED or
# REJECTED command affects its step. Duplicate terminals (waiter already
# closed) are closed as duplicates. Terminals with no matching waiter stay
# open and are re-checked next tick; a persistent orphan surfaces in city
# health patrols.
set -euo pipefail

HANDOFF_SCOPE="command-reconcile"
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=./handoff-state.sh
. "$SCRIPT_DIR/handoff-state.sh"

if ! handoff_acquire_lock; then
  echo "[domain-command-reconcile] another run holds the lock — skipping tick"
  exit 0
fi
trap handoff_cleanup EXIT

RECONCILE_ROUTE="${GC_COMMAND_REPLY_ROUTE:-domain-handoff.command-terminal-reconciler}"

ALL_JSON=$(gc bd list --all --json --limit=0 2>/dev/null || echo '[]')

CANDIDATES=$(printf '%s' "$ALL_JSON" | jq -r --arg route "$RECONCILE_ROUTE" '
  .[]
  | select(.status != "closed")
  | select((.metadata."gc.kind" // "") == "event_handoff")
  | select((.metadata.event_type // "") == "GasCityDomainCommandTerminal.v1")
  | select((.metadata."gc.routed_to" // "") == $route)
  | .id
' 2>/dev/null || true)

RECONCILED=0
DUPLICATES=0
ORPHANED=0

while IFS= read -r TERMINAL_ID; do
  [ -n "$TERMINAL_ID" ] || continue

  TERMINAL_JSON=$(printf '%s' "$ALL_JSON" | jq -c --arg id "$TERMINAL_ID" '.[] | select(.id == $id)')
  COMMAND_ID=$(printf '%s' "$TERMINAL_JSON" | jq -r '.metadata."payload.command_id" // empty')
  STATUS=$(printf '%s' "$TERMINAL_JSON" | jq -r '.metadata."payload.status" // empty')

  if [ -z "$COMMAND_ID" ] || [ -z "$STATUS" ]; then
    echo "[domain-command-reconcile] $TERMINAL_ID lacks payload.command_id/payload.status — closing as invalid" >&2
    gc bd close "$TERMINAL_ID" --reason "invalid command terminal: missing command_id or status" >/dev/null 2>&1 || true
    continue
  fi

  WAITER_JSON=$(printf '%s' "$ALL_JSON" | jq -c --arg cid "$COMMAND_ID" '
    [.[]
     | select((.metadata."gc.kind" // "") == "domain_command_request")
     | select((.metadata."handoff.command_id" // "") == $cid)]
    | sort_by(.status == "closed")
    | .[0] // empty
  ')

  if [ -z "$WAITER_JSON" ]; then
    echo "[domain-command-reconcile] no waiter found for command $COMMAND_ID (terminal $TERMINAL_ID); leaving for next tick" >&2
    ORPHANED=$((ORPHANED + 1))
    continue
  fi

  WAITER_ID=$(printf '%s' "$WAITER_JSON" | jq -r '.id')
  WAITER_STATUS=$(printf '%s' "$WAITER_JSON" | jq -r '.status')

  if [ "$WAITER_STATUS" = "closed" ]; then
    gc bd close "$TERMINAL_ID" --reason "duplicate command terminal for $COMMAND_ID (waiter $WAITER_ID already closed)" >/dev/null 2>&1 || true
    DUPLICATES=$((DUPLICATES + 1))
    continue
  fi

  # Compact flat terminal reconstructed from the adapter's payload.* metadata
  # projection, preserved on the waiter for the consuming step agent.
  TERMINAL_COMPACT=$(printf '%s' "$TERMINAL_JSON" | jq -c '
    (.metadata // {})
    | with_entries(select(.key | startswith("payload.")))
    | with_entries(.key |= sub("^payload\\."; ""))
  ')

  NOW=$(handoff_now)
  gc bd update "$WAITER_ID" \
    --set-metadata "handoff.command_status=$STATUS" \
    --set-metadata "handoff.command_terminal=$TERMINAL_COMPACT" \
    --set-metadata "handoff.reconciled_at=$NOW" >/dev/null

  ORIGIN_BEAD=$(printf '%s' "$WAITER_JSON" | jq -r '.metadata.origin_bead // empty')
  if [ -n "$ORIGIN_BEAD" ]; then
    gc bd update "$ORIGIN_BEAD" \
      --notes "domain command $COMMAND_ID terminal: $STATUS (details on waiter $WAITER_ID)" >/dev/null 2>&1 || true
  fi

  gc bd close "$WAITER_ID" --reason "domain command terminal: $STATUS" >/dev/null
  gc bd close "$TERMINAL_ID" --reason "reconciled to waiter $WAITER_ID" >/dev/null 2>&1 || true
  echo "[domain-command-reconcile] reconciled $COMMAND_ID -> waiter $WAITER_ID status=$STATUS"
  RECONCILED=$((RECONCILED + 1))
done <<EOF
$CANDIDATES
EOF

echo "[domain-command-reconcile] done: reconciled=$RECONCILED duplicates=$DUPLICATES orphaned=$ORPHANED"
