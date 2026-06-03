#!/usr/bin/env bash
# Validate the concrete bd create --graph plan before cartographer mutates beads.
set -euo pipefail

if [ -n "${RUN_DIR:-}" ]; then
  DEFAULT_EMIT_PLAN="$RUN_DIR/emit_plan.json"
  DEFAULT_GRAPH="$RUN_DIR/graph.json"
  DEFAULT_TASKS="$RUN_DIR/tasks.json"
else
  DEFAULT_EMIT_PLAN=""
  DEFAULT_GRAPH=""
  DEFAULT_TASKS=""
fi

EMIT_PLAN="${1:-$DEFAULT_EMIT_PLAN}"
GRAPH="${2:-$DEFAULT_GRAPH}"
TASKS="${3:-$DEFAULT_TASKS}"

: "${WORK_ORDER_ID:?WORK_ORDER_ID must be set}"

if [ -z "$EMIT_PLAN" ] || [ ! -f "$EMIT_PLAN" ]; then
  echo "FAIL: emit plan file missing: ${EMIT_PLAN:-<unset>}" >&2
  exit 2
fi
if [ -z "$GRAPH" ] || [ ! -f "$GRAPH" ]; then
  echo "FAIL: graph file missing: ${GRAPH:-<unset>}" >&2
  exit 2
fi

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

TARGET="integration/${WORK_ORDER_ID}"
FAILED=0

record_fail() {
  echo "FAIL: $*" >&2
  FAILED=1
}

jq -e '.nodes | type == "array"' "$EMIT_PLAN" >/dev/null || {
  echo "FAIL: emit plan must contain nodes array" >&2
  exit 2
}
jq -e '.edges | type == "array"' "$EMIT_PLAN" >/dev/null || {
  echo "FAIL: emit plan must contain edges array" >&2
  exit 2
}

jq -r --arg target "$TARGET" '
  .nodes[]?
  | select((.type // .issue_type) == "convoy")
  | select((((.labels // []) | index("owned")) | not)
           or (((.metadata // {}).owned // "") != "true")
           or (((.metadata // {}).target // "") != $target))
  | .key
' "$EMIT_PLAN" > "$TMPDIR/bad_convoys.txt"
if [ -s "$TMPDIR/bad_convoys.txt" ]; then
  record_fail "convoy nodes missing owned label, metadata.owned=true, or metadata.target=$TARGET: $(tr '\n' ' ' < "$TMPDIR/bad_convoys.txt")"
fi

jq -r --arg target "$TARGET" '
  .nodes[]?
  | select((.type // .issue_type) == "task")
  | select(((.labels // []) | index("placeholder:cross-wo-blocker")) | not)
  | select((((.metadata // {}).target // "") != $target)
           or (((.metadata // {})["gc.routed_to"] // "") == ""))
  | .key
' "$EMIT_PLAN" > "$TMPDIR/bad_task_metadata.txt"
if [ -s "$TMPDIR/bad_task_metadata.txt" ]; then
  record_fail "task nodes missing metadata.target=$TARGET or gc.routed_to: $(tr '\n' ' ' < "$TMPDIR/bad_task_metadata.txt")"
fi

jq -r '
  .nodes[]?
  | select((.type // .issue_type) == "task")
  | select(((.labels // []) | index("placeholder:cross-wo-blocker")) | not)
  | select(.parent_key? or .parent_id?)
  | select((.parent_key? and .parent_id?) or ((.parent_key? // .parent_id?) == ""))
  | .key
' "$EMIT_PLAN" > "$TMPDIR/bad_parent_refs.txt"
if [ -s "$TMPDIR/bad_parent_refs.txt" ]; then
  record_fail "task nodes have invalid convoy parent references: $(tr '\n' ' ' < "$TMPDIR/bad_parent_refs.txt")"
fi

jq -c '
  .convoys[]? as $convoy
  | ($convoy.members // $convoy.member_draft_ids // [])[]?
  | {
      task_key: .,
      parent_key: (if (($convoy.use_existing_convoy_id // null) == null)
                   then $convoy.draft_convoy_id else null end),
      parent_id: ($convoy.use_existing_convoy_id // null)
    }
' "$GRAPH" > "$TMPDIR/expected_parent_refs.jsonl"

while IFS= read -r expected; do
  [ -z "$expected" ] && continue
  task_key=$(jq -r '.task_key' <<<"$expected")
  expected_parent_key=$(jq -r '.parent_key // empty' <<<"$expected")
  expected_parent_id=$(jq -r '.parent_id // empty' <<<"$expected")
  if [ -n "$expected_parent_key" ]; then
    jq -e --arg task "$task_key" --arg parent "$expected_parent_key" '
      .nodes[]? | select(.key == $task) | .parent_key == $parent
    ' "$EMIT_PLAN" >/dev/null || record_fail "convoy member $task_key missing parent_key=$expected_parent_key in emit_plan"
  elif [ -n "$expected_parent_id" ]; then
    jq -e --arg task "$task_key" --arg parent "$expected_parent_id" '
      .nodes[]? | select(.key == $task) | .parent_id == $parent
    ' "$EMIT_PLAN" >/dev/null || record_fail "convoy member $task_key missing parent_id=$expected_parent_id in emit_plan"
  fi
done < "$TMPDIR/expected_parent_refs.jsonl"

if [ -n "$TASKS" ] && [ -f "$TASKS" ]; then
  jq -r 'if type == "array" then .[] else .tasks[] end | .draft_id // .id // empty' "$TASKS" \
    | sort -u > "$TMPDIR/task_ids.txt"
  jq -r '.convoys[]? | (.members // .member_draft_ids // [])[]?' "$GRAPH" \
    | sort -u > "$TMPDIR/member_ids.txt"
  missing_members=$(comm -23 "$TMPDIR/member_ids.txt" "$TMPDIR/task_ids.txt" || true)
  if [ -n "$missing_members" ]; then
    record_fail "convoy member ids missing from tasks.json: $(printf '%s\n' "$missing_members" | tr '\n' ' ')"
  fi
fi

jq -c '
  .holding_stubs_to_create[]? as $holding
  | $holding.dependent_draft_ids[]?
  | {
      task_key: .,
      holding_key: $holding.draft_holding_id,
      hold_label: ("hold:" + $holding.blocker_wo_id)
    }
' "$GRAPH" > "$TMPDIR/expected_holding_links.jsonl"

while IFS= read -r link; do
  [ -z "$link" ] && continue
  task_key=$(jq -r '.task_key' <<<"$link")
  holding_key=$(jq -r '.holding_key' <<<"$link")
  hold_label=$(jq -r '.hold_label' <<<"$link")
  jq -e --arg task "$task_key" --arg hold_label "$hold_label" '
    .nodes[]? | select(.key == $task) | ((.labels // []) | index($hold_label))
  ' "$EMIT_PLAN" >/dev/null || record_fail "HOLDING dependent $task_key missing label $hold_label in emit_plan"
  jq -e --arg task "$task_key" --arg holding "$holding_key" '
    .edges[]? | select(.from_key == $task and .to_key == $holding)
  ' "$EMIT_PLAN" >/dev/null || record_fail "HOLDING dependent $task_key missing edge to $holding_key in emit_plan"
done < "$TMPDIR/expected_holding_links.jsonl"

jq -r '
  def labels: (.labels // []);
  .nodes[]?
  | select((.type // .issue_type) == "task")
  | select((labels | index("placeholder:cross-wo-blocker")) | not)
  | select([labels[]? | select(startswith("hold:"))] | length > 0)
  | .key
' "$EMIT_PLAN" > "$TMPDIR/held_task_keys.txt"

while IFS= read -r task_key; do
  [ -z "$task_key" ] && continue
  has_hold_edge=$(jq --arg task "$task_key" '
    . as $root
    | [$root.edges[]?
     | select(.from_key == $task)
     | select(.to_key as $to
       | [$root.nodes[]?
          | select(.key == $to)
          | select((.labels // []) | index("placeholder:cross-wo-blocker"))]
       | length > 0)]
    | length
  ' "$EMIT_PLAN")
  if [ "$has_hold_edge" = "0" ]; then
    record_fail "held task $task_key has hold:* label but no edge to a HOLDING node"
  fi
done < "$TMPDIR/held_task_keys.txt"

if [ "$FAILED" -ne 0 ]; then
  exit 1
fi

echo "cartographer emit plan preflight passed: $EMIT_PLAN"
