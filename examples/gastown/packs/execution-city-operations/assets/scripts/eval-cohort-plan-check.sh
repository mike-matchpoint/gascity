#!/usr/bin/env bash
# Reject empty or structurally incomplete cohort plans before fan-out.
set -euo pipefail

die() { printf 'eval-cohort-plan-check: %s\n' "$*" >&2; exit 1; }
command -v jq >/dev/null 2>&1 || die "jq is required"
command -v gc >/dev/null 2>&1 || die "gc is required"
: "${GC_BEAD_ID:?GC_BEAD_ID is required}"

BEAD_JSON=$(gc bd show "$GC_BEAD_ID" --json)
BEAD=$(printf '%s' "$BEAD_JSON" | jq -c 'if type == "array" then .[0] else . end')
OUTPUT=$(printf '%s' "$BEAD" | jq -r '.metadata["gc.output_json"] // empty')
[ -n "$OUTPUT" ] || die "plan is missing gc.output_json"
SURFACE_KIND=$(printf '%s' "$BEAD" | jq -r '.metadata["eval.surface_kind"] // empty')
GRADER_CMD=$(printf '%s' "$BEAD" | jq -r '.metadata["eval.grader_cmd"] // empty')
RUN_ID=$(printf '%s' "$BEAD" | jq -r '.metadata["eval.run_id"] // empty')
EVAL_SUITE=$(printf '%s' "$BEAD" | jq -r '.metadata["eval.eval_suite"] // empty')
BINDING_PREFIX=$(printf '%s' "$BEAD" | jq -r '.metadata["eval.binding_prefix"] // empty')
printf '%s' "$OUTPUT" | jq -e \
  --arg surface "$SURFACE_KIND" --arg grader "$GRADER_CMD" --arg run "$RUN_ID" \
  --arg suite "$EVAL_SUITE" --arg prefix "$BINDING_PREFIX" '
  (.cases | type == "array" and length > 0) and
  all(.cases[];
    (.case_id | type == "string" and length > 0) and
    (.fixture_id | type == "string" and test("^STEP#[^#]+#[^#]+$")) and
    (.fixture_ref | type == "string" and length > 0) and
    (.fixture_payload_b64 | type == "string" and length > 0) and
    .surface_kind == $surface and .grader_cmd == $grader and
    .run_id == $run and .eval_suite == $suite and
    .binding_prefix == $prefix and (.binding_prefix | endswith(".")))
' >/dev/null || die "plan cases are empty or incomplete"

COUNT=$(printf '%s' "$OUTPUT" | jq '.cases | length')
printf 'validated %s cohort cases\n' "$COUNT"
