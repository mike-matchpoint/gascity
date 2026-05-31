#!/usr/bin/env bash
# blocker-unblock-sweep — find BLOCKED beads whose `blocked_by_bug`
# references are all closed and flip them back to `open`.
#
# Triggered by `orders/blocker-unblock-sweep.toml` on a 60s cooldown.
#
# Scope: every non-HQ, non-suspended rig. Per-rig, list BLOCKED beads
# with `metadata.blocked_by_bug` set. For each, look up every comma-
# separated blocker ID; if all are status=closed, flip the bead to
# `open` with assignee preserved.
#
# Cheap when idle: one `bd list` per rig returns empty in the common
# case. Silent when no action is taken.
set -euo pipefail

UNBLOCKED_BY="codegen-support.blocker-unblock-sweep"

# DRY_RUN=1 logs the unblock decisions without mutating bead state.
# Used to verify the sweep against live data before enabling the order.
DRY_RUN="${DRY_RUN:-0}"

RIGS_JSON=$(gc rig list --json)

while read -r RIG_JSON; do
  RIG=$(echo "$RIG_JSON" | jq -r '.name')

  # Server-side filter: only BLOCKED beads that carry blocked_by_bug.
  CANDIDATES=$(gc --rig "$RIG" bd list \
    --status=blocked \
    --has-metadata-key blocked_by_bug \
    --json --limit=0 | jq -r '.[] | .id')

  if [ -z "$CANDIDATES" ]; then
    continue
  fi

  while read -r BEAD; do
    [ -z "$BEAD" ] && continue

    BEAD_JSON=$(gc --rig "$RIG" bd show "$BEAD" --json 2>/dev/null || echo '[]')
    BLOCKERS=$(echo "$BEAD_JSON" | jq -r '.[0].metadata.blocked_by_bug // ""')

    if [ -z "$BLOCKERS" ]; then
      # Edge case: metadata key present but value emptied between list
      # and show. Skip — treat as no-op.
      continue
    fi

    ALL_CLOSED=true
    STILL_OPEN=()
    IFS=',' read -ra BLOCKER_IDS <<<"$BLOCKERS"
    for B in "${BLOCKER_IDS[@]}"; do
      B_TRIM=$(echo "$B" | xargs)
      [ -z "$B_TRIM" ] && continue
      # Blockers live in the same rig as the blocked bead in the polecat
      # scope-out flow. If cross-rig blockers appear later, the show
      # falls back to the global ID lookup.
      STATUS=$(gc --rig "$RIG" bd show "$B_TRIM" --json 2>/dev/null \
        | jq -r '.[0].status // "missing"')
      if [ "$STATUS" != "closed" ]; then
        ALL_CLOSED=false
        STILL_OPEN+=("$B_TRIM[$STATUS]")
      fi
    done

    if [ "$ALL_CLOSED" = "true" ]; then
      NOW=$(date -u +%FT%TZ)
      if [ "$DRY_RUN" = "1" ]; then
        echo "blocker-unblock-sweep[DRY_RUN]: $RIG/$BEAD WOULD unblock (blockers all closed: $BLOCKERS)"
      else
        gc --rig "$RIG" bd update "$BEAD" \
          --status open \
          --set-metadata "unblocked_at=$NOW" \
          --set-metadata "unblocked_by=$UNBLOCKED_BY" >/dev/null
        echo "blocker-unblock-sweep: $RIG/$BEAD unblocked (blockers all closed: $BLOCKERS)"
      fi
    elif [ "$DRY_RUN" = "1" ]; then
      echo "blocker-unblock-sweep[DRY_RUN]: $RIG/$BEAD still blocked — open: ${STILL_OPEN[*]:-}"
    fi
    # Silent when still blocked — that's the normal idle state. Operators
    # can inspect via `gc bd show $BEAD` if they want the STILL_OPEN list.
  done <<<"$CANDIDATES"
done < <(echo "$RIGS_JSON" | jq -c '.rigs[] | select(.hq == false) | select(.suspended == false)')
