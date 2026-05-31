#!/usr/bin/env bash
# Restore a blocked spec-cartographer step to the cartographer dispatch route.
#
# Usage:
#   RIG=<rig-name> .gc/scripts/cartographer-human-review-resume.sh <step-bead-id>
set -euo pipefail

: "${RIG:?RIG is required}"
STEP_ID="${1:?step bead id is required}"

NOW=$(date -u +%FT%TZ)
BEAD_JSON=$(gc --rig "$RIG" bd show "$STEP_ID" --json)
ROUTE=$(printf '%s' "$BEAD_JSON" | jq -r '.[0].metadata."cartographer.resume_routed_to" // empty')
if [ -z "$ROUTE" ]; then
  ROUTE="$RIG/codegen-support.cartographer"
fi

gc --rig "$RIG" bd update "$STEP_ID" \
  --status open \
  --assignee "" \
  --set-metadata "gc.routed_to=$ROUTE" \
  --set-metadata "cartographer.review_state=resumed" \
  --set-metadata "cartographer.review_resumed_at=$NOW" \
  --set-metadata "cartographer.review_resumed_by=${GC_AGENT:-operator}" \
  --unset-metadata decision_state \
  --notes "Resumed cartographer step after human review; restored route $ROUTE"

gc --rig "$RIG" bd show "$STEP_ID" --json \
  | jq -e --arg route "$ROUTE" '
      .[0].status == "open" and
      ((.[0].assignee // "") == "") and
      .[0].metadata."gc.routed_to" == $route and
      .[0].metadata."cartographer.review_state" == "resumed"
    ' >/dev/null

echo "cartographer-human-review-resume: $RIG/$STEP_ID restored to $ROUTE"
