#!/usr/bin/env bash
# Controller check for a cohort aggregate score axis.
set -euo pipefail

die() { printf 'eval-threshold-check: %s\n' "$*" >&2; exit 1; }

command -v jq >/dev/null 2>&1 || die "jq is required"
command -v gc >/dev/null 2>&1 || die "gc is required"
: "${GC_BEAD_ID:?GC_BEAD_ID is required}"
: "${GC_ARTIFACT_DIR:?GC_ARTIFACT_DIR is required}"

BEAD_JSON=$(gc bd show "$GC_BEAD_ID" --json)
BEAD=$(printf '%s' "$BEAD_JSON" | jq -c 'if type == "array" then .[0] else . end')
ROOT_ID=$(printf '%s' "$BEAD" | jq -r '.metadata["gc.root_bead_id"] // empty')
AGGREGATE_REF=$(printf '%s' "$BEAD" | jq -r '.metadata["eval.aggregate_step_ref"] // empty')
METRIC=$(printf '%s' "$BEAD" | jq -r '.metadata["eval.gate_metric"] // empty')
THRESHOLD=$(printf '%s' "$BEAD" | jq -r '.metadata["eval.threshold"] // empty')
[ -n "$ROOT_ID" ] && [ -n "$AGGREGATE_REF" ] && [ -n "$METRIC" ] && [ -n "$THRESHOLD" ] || die "gate metadata is incomplete"
printf '%s' "$THRESHOLD" | jq -e 'tonumber >= 0 and tonumber <= 1' >/dev/null || die "threshold must be numeric in [0,1]"

ALL=$(gc bd list --all --json --limit=0)
AGGREGATE=$(printf '%s' "$ALL" | jq -c --arg root "$ROOT_ID" --arg ref "$AGGREGATE_REF" '
  [.[] | select(.metadata["gc.root_bead_id"] == $root) |
    select((.metadata["gc.step_ref"] == $ref) or (.metadata["gc.step_ref"] | endswith("." + $ref)))] |
  if length == 1 then .[0] else empty end
')
[ -n "$AGGREGATE" ] || die "expected exactly one aggregate bead"
OUTPUT=$(printf '%s' "$AGGREGATE" | jq -r '.metadata["gc.output_json"] // empty')
[ -n "$OUTPUT" ] || die "aggregate bead is missing gc.output_json"

SCORE=$(printf '%s' "$OUTPUT" | jq -er --arg metric "$METRIC" '
  select(.aggregate_scores.case_count > 0) |
  .aggregate_scores.axes[$metric] | select(type == "number")
') || die "aggregate output has no non-vacuous numeric gate metric"

mkdir -p "$GC_ARTIFACT_DIR"
RESULT_FILE="$GC_ARTIFACT_DIR/threshold-result.json"
jq -n --arg metric "$METRIC" --argjson score "$SCORE" --argjson minimum "$THRESHOLD" \
  '{metric: $metric, score: $score, minimum: $minimum, outcome: (if $score >= $minimum then "passed" else "failed" end)}' >"$RESULT_FILE.tmp"
mv -f "$RESULT_FILE.tmp" "$RESULT_FILE"
gc bd update "$GC_BEAD_ID" --set-metadata "eval.threshold_result_ref=$RESULT_FILE" >/dev/null

if ! jq -e '.outcome == "passed"' "$RESULT_FILE" >/dev/null; then
  printf 'eval-threshold-check: %s=%s is below %s\n' "$METRIC" "$SCORE" "$THRESHOLD" >&2
  exit 1
fi
printf '%s\n' "$RESULT_FILE"
