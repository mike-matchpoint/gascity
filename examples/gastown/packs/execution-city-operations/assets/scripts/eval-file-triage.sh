#!/usr/bin/env bash
# Validate and file manifest triage packets through the existing classifier.
set -euo pipefail

die() { printf 'eval-file-triage: %s\n' "$*" >&2; exit 1; }

MANIFEST=""
ROUTE=""
while [ $# -gt 0 ]; do
  case "$1" in
    --manifest) MANIFEST="$2"; shift 2 ;;
    --route) ROUTE="$2"; shift 2 ;;
    *) die "unknown argument: $1" ;;
  esac
done

command -v jq >/dev/null 2>&1 || die "jq is required"
command -v gc >/dev/null 2>&1 || die "gc is required"
[ -f "$MANIFEST" ] || die "--manifest must name an existing file"
[ -n "$ROUTE" ] || die "--route is required"

jq -e '
  .gate_outcome == "failed" and
  (.triage_packets | type == "array" and length > 0) and
  all(.triage_packets[];
    (.eval_suite | type == "string" and length > 0) and
    (.case_id | type == "string" and length > 0) and
    (.prompt_name | type == "string" and length > 0) and
    (.prompt_version | type == "string" and length > 0) and
    (.model | type == "string" and length > 0) and
    (.provider | type == "string" and length > 0) and
    (.run_id | type == "string" and length > 0) and
    has("expected_outcome") and has("actual_outcome") and
    (.score | type == "number") and (.threshold | type == "number") and
    (.failure_label | type == "string" and length > 0) and
    has("prompt_input") and has("redacted_output_excerpt") and
    has("trace_excerpt") and (.scorer_rationale | type == "string" and length > 0) and
    (.eval_artifact_refs | type == "array" and length > 0) and
    has("related_execution") and has("prior_passing_run") and
    has("regression_window") and has("changed_dependency") and
    (.evaluator_limitations | type == "array") and
    has("nondeterministic_variance") and (.missing_artifacts | type == "array"))
' "$MANIFEST" >/dev/null || die "manifest triage packets are incomplete"

LABELS=$(jq -r '[.triage_packets[].failure_label] | unique[]' "$MANIFEST")
[ -n "$LABELS" ] || die "manifest has no failure classes"
while IFS= read -r LABEL; do
  [ -n "$LABEL" ] || continue
  PACKET=$(jq -c --arg label "$LABEL" '{
    manifest: {contract, run_id, eval_suite, gate_outcome, threshold, aggregate_scores, grader, timestamps},
    cases: [.triage_packets[] | select(.failure_label == $label)]
  }' "$MANIFEST")
  COUNT=$(printf '%s' "$PACKET" | jq '.cases | length')
  [ "$COUNT" -gt 0 ] || die "failure class $LABEL has no cases"
  META=$(jq -n --arg route "$ROUTE" --arg label "$LABEL" --arg packet "$PACKET" '{
    "gc.kind": "prompt_eval_triage", "gc.routed_to": $route,
    "eval.failure_label": $label, "eval.evidence_packet": $packet
  }')
  CREATED=$(gc bd create "Eval triage: $LABEL" --type task --metadata "$META" --description "$PACKET" --json)
  ID=$(printf '%s' "$CREATED" | jq -r '.id // empty')
  [ -n "$ID" ] || die "classifier bead creation returned no id"
  printf '%s\n' "$ID"
done <<< "$LABELS"
