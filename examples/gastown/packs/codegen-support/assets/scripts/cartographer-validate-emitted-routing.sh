#!/usr/bin/env bash
# Validate cartographer-emitted task routing without racing normal downstream
# handoff from polecat to refinery.
set -euo pipefail

if [ -n "${RUN_DIR:-}" ]; then
  DEFAULT_EMITTED="$RUN_DIR/emitted.json"
  DEFAULT_QUERY="$RUN_DIR/validation_query.json"
  DEFAULT_OUTPUT="$RUN_DIR/validation_store_routing.json"
else
  DEFAULT_EMITTED=""
  DEFAULT_QUERY=""
  DEFAULT_OUTPUT=""
fi

MODE="${CARTOGRAPHER_ROUTING_MODE:-lifecycle}"
EMITTED="${CARTOGRAPHER_ROUTING_EMITTED:-${1:-$DEFAULT_EMITTED}}"
QUERY="${CARTOGRAPHER_ROUTING_QUERY:-${2:-$DEFAULT_QUERY}}"
OUTPUT="${CARTOGRAPHER_ROUTING_OUTPUT:-${3:-$DEFAULT_OUTPUT}}"

if [ "$MODE" != "emission" ] && [ "$MODE" != "lifecycle" ]; then
  echo "FAIL: CARTOGRAPHER_ROUTING_MODE must be emission or lifecycle, got '$MODE'" >&2
  exit 2
fi
if [ -z "$EMITTED" ] || [ ! -f "$EMITTED" ]; then
  echo "FAIL: emitted.json missing: ${EMITTED:-<unset>}" >&2
  exit 2
fi
if [ -z "$QUERY" ] || [ ! -f "$QUERY" ]; then
  echo "FAIL: routing query JSON missing: ${QUERY:-<unset>}" >&2
  exit 2
fi
if [ -z "$OUTPUT" ]; then
  echo "FAIL: routing validation output path is unset" >&2
  exit 2
fi

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

TARGET=$(jq -r '.routing_target // empty' "$EMITTED")
if [ -z "$TARGET" ]; then
  echo "FAIL: emitted.json missing routing_target" >&2
  exit 2
fi

REFINERY_TARGET="${RIG:+$RIG/}gastown.refinery"
FAILED=0
> "$TMPDIR/records.jsonl"
> "$TMPDIR/failures.jsonl"

record_result() {
  local id="$1"
  local status="$2"
  local route="$3"
  local assignee="$4"
  local branch="$5"
  local target_meta="$6"
  local routing_status="$7"
  local reason="$8"

  jq -nc \
    --arg id "$id" \
    --arg status "$status" \
    --arg route "$route" \
    --arg assignee "$assignee" \
    --arg branch "$branch" \
    --arg target_meta "$target_meta" \
    --arg routing_status "$routing_status" \
    --arg reason "$reason" \
    '{
      id: $id,
      status: $status,
      gc_routed_to: $route,
      assignee: $assignee,
      branch: $branch,
      target: $target_meta,
      routing_status: $routing_status,
      reason: $reason
    }' >> "$TMPDIR/records.jsonl"
}

record_failure() {
  local id="$1"
  local detail="$2"

  jq -nc \
    --arg check_id "n" \
    --arg id "$id" \
    --arg detail "$detail" \
    '{check_id: $check_id, bead_id: $id, detail: $detail}' >> "$TMPDIR/failures.jsonl"
  echo "FAIL: $id $detail" >&2
  FAILED=1
}

jq -r '.beads_emitted[]?.id // empty' "$EMITTED" > "$TMPDIR/bead_ids.txt"

while IFS= read -r BEAD; do
  [ -n "$BEAD" ] || continue

  RECORD=$(jq -c --arg id "$BEAD" '.[]? | select(.id == $id)' "$QUERY" | head -n 1)
  if [ -z "$RECORD" ]; then
    record_result "$BEAD" "" "" "" "" "" "missing" "emitted bead absent from validation query"
    record_failure "$BEAD" "missing from validation_query.json"
    continue
  fi

  STATUS=$(jq -r '.status // empty' <<<"$RECORD")
  ROUTED=$(jq -r '(.metadata // {})["gc.routed_to"] // empty' <<<"$RECORD")
  ASSIGNEE=$(jq -r '.assignee // empty' <<<"$RECORD")
  BRANCH=$(jq -r '(.metadata // {}).branch // empty' <<<"$RECORD")
  TARGET_META=$(jq -r '(.metadata // {}).target // empty' <<<"$RECORD")

  if [ "$MODE" = "emission" ]; then
    if [ "$ROUTED" = "$TARGET" ]; then
      record_result "$BEAD" "$STATUS" "$ROUTED" "$ASSIGNEE" "$BRANCH" "$TARGET_META" \
        "emission_routed" "freshly emitted bead carries the initial routing target"
    else
      record_result "$BEAD" "$STATUS" "$ROUTED" "$ASSIGNEE" "$BRANCH" "$TARGET_META" \
        "emission_route_mismatch" "freshly emitted bead did not carry the initial routing target"
      record_failure "$BEAD" "emission gc.routed_to='$ROUTED' expected '$TARGET'"
    fi
    continue
  fi

  if [ "$ROUTED" = "$TARGET" ]; then
    record_result "$BEAD" "$STATUS" "$ROUTED" "$ASSIGNEE" "$BRANCH" "$TARGET_META" \
      "initial_route_present" "bead is still visible to the initial dispatch pool"
  elif [ "$STATUS" = "closed" ]; then
    record_result "$BEAD" "$STATUS" "$ROUTED" "$ASSIGNEE" "$BRANCH" "$TARGET_META" \
      "completed" "bead already completed after initial dispatch"
  elif [ "$ROUTED" = "$REFINERY_TARGET" ] || [ "$ASSIGNEE" = "$REFINERY_TARGET" ]; then
    record_result "$BEAD" "$STATUS" "$ROUTED" "$ASSIGNEE" "$BRANCH" "$TARGET_META" \
      "downstream_refinery" "bead progressed from polecat to refinery"
  elif [[ "$ASSIGNEE" == */gastown.polecat ]]; then
    record_result "$BEAD" "$STATUS" "$ROUTED" "$ASSIGNEE" "$BRANCH" "$TARGET_META" \
      "downstream_polecat_claim" "bead was claimed by a polecat session"
  elif [[ "$BRANCH" == polecat/* ]] && [ -n "$TARGET_META" ]; then
    record_result "$BEAD" "$STATUS" "$ROUTED" "$ASSIGNEE" "$BRANCH" "$TARGET_META" \
      "downstream_branch_handoff" "bead carries polecat handoff metadata"
  else
    record_result "$BEAD" "$STATUS" "$ROUTED" "$ASSIGNEE" "$BRANCH" "$TARGET_META" \
      "not_dispatchable" "bead is neither initially routed nor in an expected downstream state"
    record_failure "$BEAD" "gc.routed_to='$ROUTED' is not '$TARGET' and no downstream handoff evidence is present"
  fi
done < "$TMPDIR/bead_ids.txt"

SUMMARY_STATUS="ok"
if [ "$FAILED" != "0" ]; then
  SUMMARY_STATUS="failed"
fi

mkdir -p "$(dirname "$OUTPUT")"
jq -s \
  --arg status "$SUMMARY_STATUS" \
  --arg mode "$MODE" \
  --arg expected_initial_route "$TARGET" \
  --arg expected_refinery_route "$REFINERY_TARGET" \
  --slurpfile failures "$TMPDIR/failures.jsonl" \
  '{
    status: $status,
    mode: $mode,
    expected_initial_route: $expected_initial_route,
    expected_refinery_route: $expected_refinery_route,
    records: .,
    failures: $failures
  }' "$TMPDIR/records.jsonl" > "$OUTPUT"

if [ "$FAILED" != "0" ]; then
  exit 1
fi
