#!/usr/bin/env bash
# Move the current spec-cartographer step into passive human review.
#
# Required env:
#   RIG         rig name that owns the step bead
#   GC_BEAD_ID  current formula step bead
#
# Optional env:
#   WORK_ORDER_ID, EPOCH, RUN_DIR, REVIEW_REASON
set -euo pipefail

: "${RIG:?RIG is required}"
: "${GC_BEAD_ID:?GC_BEAD_ID is required}"

REASON="${1:-${REVIEW_REASON:-cartographer step requires human review}}"
NOW=$(date -u +%FT%TZ)

BEAD_JSON=$(gc --rig "$RIG" bd show "$GC_BEAD_ID" --json)
CURRENT_ROUTE=$(printf '%s' "$BEAD_JSON" | jq -r '.[0].metadata."gc.routed_to" // empty')
CURRENT_ASSIGNEE=$(printf '%s' "$BEAD_JSON" | jq -r '.[0].assignee // empty')
if [ -z "$CURRENT_ROUTE" ]; then
  CURRENT_ROUTE="$RIG/codegen-support.cartographer"
fi

args=(
  --status blocked
  --assignee ""
  --set-metadata decision_state=human_review
  --set-metadata "gc.routed_to=human-escalation"
  --set-metadata "cartographer.review_state=human_review"
  --set-metadata "cartographer.review_reason=$REASON"
  --set-metadata "cartographer.review_requested_at=$NOW"
  --set-metadata "cartographer.resume_routed_to=$CURRENT_ROUTE"
  --set-metadata "cartographer.previous_assignee=$CURRENT_ASSIGNEE"
)

if [ -n "${WORK_ORDER_ID:-}" ]; then
  args+=(--set-metadata "work_order_id=$WORK_ORDER_ID")
fi
if [ -n "${EPOCH:-}" ]; then
  args+=(--set-metadata "cartographer.epoch=$EPOCH")
fi
if [ -n "${RUN_DIR:-}" ]; then
  args+=(--set-metadata "cartographer.failure_run_dir=$RUN_DIR")
fi

gc --rig "$RIG" bd update "$GC_BEAD_ID" "${args[@]}" \
  --notes "Blocked for human review: $REASON"

gc --rig "$RIG" bd show "$GC_BEAD_ID" --json \
  | jq -e --arg reason "$REASON" --arg route "$CURRENT_ROUTE" '
      .[0].status == "blocked" and
      ((.[0].assignee // "") == "") and
      .[0].metadata."gc.routed_to" == "human-escalation" and
      .[0].metadata."cartographer.review_state" == "human_review" and
      .[0].metadata."cartographer.review_reason" == $reason and
      .[0].metadata."cartographer.resume_routed_to" == $route
    ' >/dev/null

echo "cartographer-human-review-block: $RIG/$GC_BEAD_ID blocked for human review"
