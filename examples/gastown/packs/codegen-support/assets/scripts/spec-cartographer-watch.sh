#!/usr/bin/env bash
# spec-cartographer-watch — scan every registered rig's
# `specs/agent-work-orders/` directory for work-order files the
# cartographer has not yet planned, and sling the `spec-cartographer`
# formula for each new file.
#
# Triggered by `orders/spec-cartographer-watch.toml` on a 60s cooldown.
#
# This is the sole automatic trigger for the cartographer. The
# `mayor-cartographer-protocol` fragment is a guardrail that forbids
# the mayor from planning — humans who want a re-plan run
# `gc sling ... spec-cartographer --formula` themselves.
#
# Idempotency:
#   1. Files already represented by any `source:work-order:<basename>`
#      label or `metadata.work_order_id=<basename>` bead in the rig are
#      skipped (any status, any epoch — re-firing the cartographer just
#      because earlier output got closed is wrong). The broad planned-WO
#      index is an optimization only; each would-be sling is guarded by
#      exact per-WO label and metadata lookups immediately before dispatch.
#   2. If the cartographer has actionable ready work, an in-progress step,
#      or an open spec-cartographer molecule in the rig (NOT just for THIS
#      WO), defer all WOs in that rig to the next tick. Cartographer creates
#      one run worktree per molecule, but it remains a singleton planner:
#      two active planning sessions can still race on bead-store
#      reconciliation and duplicate output. Stale open scaffolds are handled
#      by the mayor heartbeat rather than by launching another planner.
#
# Cheap when idle: a few `bd list` calls per rig and exit. Frequent
# firing is safe because check 2 short-circuits the inner loop when
# the cartographer is busy.
#
# Only NEW files are processed. Edits to existing work orders are not
# automatically replanned — operators re-sling manually. When multiple
# unplanned work orders exist, the most recently changed source file wins so
# live intake does not starve behind historical backlog.
set -euo pipefail

work_order_already_planned() {
  local rig_name="$1"
  local wo_id="$2"
  local count=""

  if ! count=$(gc --rig "$rig_name" bd list --all --json --limit=1 \
    --label "source:work-order:${wo_id}" \
    | jq -r 'length'); then
    echo "[spec-cartographer-watch] could not verify source label for $rig_name :: $wo_id — deferring"
    return 2
  fi
  if [ "$count" != "0" ]; then
    return 0
  fi

  if ! count=$(gc --rig "$rig_name" bd list --all --json --limit=1 \
    --metadata-field "work_order_id=${wo_id}" \
    | jq -r 'length'); then
    echo "[spec-cartographer-watch] could not verify work_order_id metadata for $rig_name :: $wo_id — deferring"
    return 2
  fi
  if [ "$count" != "0" ]; then
    return 0
  fi

  return 1
}

RIGS_JSON=$(gc rig list --json)

echo "$RIGS_JSON" \
  | jq -c '.rigs[] | select(.hq == false) | select(.suspended == false)' \
  | while read -r RIG_JSON; do
  RIG_NAME=$(echo "$RIG_JSON" | jq -r '.name')
  RIG_PATH=$(echo "$RIG_JSON" | jq -r '.path')
  WO_DIR="$RIG_PATH/specs/agent-work-orders"

  [ -d "$WO_DIR" ] || continue

  # Idempotency check 2 (cartographer busy in this rig) is evaluated
  # ONCE per rig before the WO loop. Use the same dependency-aware
  # ready-work surface as the cartographer agent selector, plus explicit
  # in-progress cartographer steps and open planner molecules.
  CARTOGRAPHER_AGENT="$RIG_NAME/codegen-support.cartographer"
  READY_JSON=$(gc work count --agent "$CARTOGRAPHER_AGENT" --json 2>/dev/null || true)
  READY_COUNT=$(printf '%s\n' "$READY_JSON" | jq -r 'if .ok == true then (.count // 0) else empty end' 2>/dev/null || true)
  if [ -z "$READY_COUNT" ]; then
    echo "[spec-cartographer-watch] could not assess cartographer demand for $RIG_NAME — deferring"
    continue
  fi
  IN_PROGRESS=$(
    gc --rig "$RIG_NAME" bd list --status=in_progress \
      --type=step \
      --metadata-field formula=spec-cartographer \
      --json --limit=0 \
    | jq 'length'
  )
  OPEN_MOLECULES=$(
    gc --rig "$RIG_NAME" bd list --status=open \
      --type=molecule \
      --metadata-field formula=spec-cartographer \
      --json --limit=0 \
    | jq 'length'
  )
  if [ "$READY_COUNT" != "0" ] || [ "$IN_PROGRESS" != "0" ] || [ "$OPEN_MOLECULES" != "0" ]; then
    echo "[spec-cartographer-watch] cartographer busy in $RIG_NAME — deferring (will retry next tick)"
    continue
  fi

  work_order_sort_key() {
    local wo_file="$1"
    local epoch=""

    # Recency is only a priority heuristic. Avoid per-file `git log`; on
    # hosted rigs that turns this watcher into one slow repository-history
    # query per work order and can overrun the order timeout.
    epoch=$(stat -c %Y "$wo_file" 2>/dev/null || stat -f %m "$wo_file" 2>/dev/null || echo 0)
    printf '%s\t%s\n' "$epoch" "$wo_file"
  }

  if ! PLANNED_WO_IDS=$(gc --rig "$RIG_NAME" bd list --all --json --limit=0 \
    | jq -r '
        .[] |
        ((.labels // [])[]? | select(startswith("source:work-order:")) | sub("^source:work-order:"; "")),
        (.metadata.work_order_id? // empty)
      ' \
    | sort -u); then
    echo "[spec-cartographer-watch] could not build planned work-order index for $RIG_NAME — deferring"
    continue
  fi

  find "$WO_DIR" -maxdepth 1 -name '*.md' -type f \
    ! -name 'README.md' ! -name 'work-order-template.md' \
  | while read -r WO_FILE; do
    work_order_sort_key "$WO_FILE"
  done \
  | sort -t "$(printf '\t')" -k1,1nr -k2,2 \
  | cut -f2- \
  | while read -r WO_FILE; do
    WO_ID=$(basename "$WO_FILE" .md)

    # Idempotency check 1: any prior bead with this WO label or metadata
    # means the cartographer already planned it (open molecule, closed task
    # bead, tombstone, etc.). Use the broad index as a fast path, then do an
    # exact per-WO guard before sling so a partial/lossy broad list cannot
    # create a duplicate.
    if grep -Fxq -- "$WO_ID" <<<"$PLANNED_WO_IDS"; then continue; fi
    if work_order_already_planned "$RIG_NAME" "$WO_ID"; then
      continue
    else
      PLAN_CHECK_STATUS=$?
      if [ "$PLAN_CHECK_STATUS" != "1" ]; then break; fi
    fi

    REL_PATH="specs/agent-work-orders/$(basename "$WO_FILE")"
    echo "[spec-cartographer-watch] slinging cartographer for $RIG_NAME :: $WO_ID"
    SLING_JSON=$(gc sling "$RIG_NAME/codegen-support.cartographer" spec-cartographer --formula \
      --json \
      --var work_order_path="$REL_PATH" \
      --var rig_name="$RIG_NAME")
    echo "$SLING_JSON"

    ROOT_ID=$(echo "$SLING_JSON" | jq -r '.root_bead_id // .bead_id // .id // empty')
    if [ -n "$ROOT_ID" ] && gc --rig "$RIG_NAME" bd show "$ROOT_ID" >/dev/null 2>&1; then
      gc --rig "$RIG_NAME" bd update "$ROOT_ID" \
        --add-label "source:work-order:${WO_ID}" \
        --set-metadata work_order_id="$WO_ID" \
        --set-metadata work_order_path="$REL_PATH" \
        --set-metadata source="spec-cartographer-watch"
    fi

    # One sling per tick per rig — the slung cartographer's open
    # molecule is what check 2 reads on the next tick to serialize
    # the next WO. Break inner loop so we don't sling a second WO
    # against a freshly-busy rig.
    break
  done
done
