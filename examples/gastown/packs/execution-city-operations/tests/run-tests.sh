#!/usr/bin/env bash
set -euo pipefail

PACK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO_ROOT="$(git -C "$PACK_DIR" rev-parse --show-toplevel)"
WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/execution-eval-tests.XXXXXX")"
trap 'rm -rf "$WORK_DIR"' EXIT
export GC_FAKE_STATE="$WORK_DIR/beads.json"
export GRADER_CAPTURE="$WORK_DIR/grader.log"
export PATH="$PACK_DIR/tests/fakes:$PATH"
touch "$GRADER_CAPTURE"

GC_FAST_UNIT=1 go test "$PACK_DIR" -run 'TestEval'

cat >"$GC_FAKE_STATE" <<JSON
[
  {"id":"workflow-1","status":"open","metadata":{"gc.kind":"workflow"}},
  {"id":"execute-1","status":"closed","metadata":{
    "gc.root_bead_id":"workflow-1","gc.step_ref":"eval.replay.execute-under-test","gc.attempt":"1",
    "eval.case_id":"case-001","eval.fixture_id":"STEP#surface.execute#case-001",
    "eval.fixture_ref":"fixtures/recorded-step.json","eval.grader_cmd":"grader --format json"
  }}
]
JSON
export GC_BEAD_ID="execute-1"
export GC_ARTIFACT_DIR="$WORK_DIR/execute-artifacts"
"$PACK_DIR/assets/scripts/eval-grader-check.sh" >/dev/null
test "$(wc -l <"$GRADER_CAPTURE" | tr -d ' ')" = "1"
test "$(cat "$GRADER_CAPTURE")" = "--format json"
jq -e '.status == "completed" and .rubric_scores[0].score == 0.72' "$GC_ARTIFACT_DIR/grading-result.json" >/dev/null

jq '. + [{"id":"plan-empty","status":"closed","metadata":{"gc.output_json":"{\"cases\":[]}"}}]' \
  "$GC_FAKE_STATE" >"$GC_FAKE_STATE.tmp"
mv -f "$GC_FAKE_STATE.tmp" "$GC_FAKE_STATE"
export GC_BEAD_ID="plan-empty"
if "$PACK_DIR/assets/scripts/eval-cohort-plan-check.sh" >"$WORK_DIR/empty.out" 2>"$WORK_DIR/empty.err"; then
  printf 'expected zero-case cohort plan to fail\n' >&2
  exit 1
fi

jq '. + [{"id":"aggregate-1","status":"closed","metadata":{
  "gc.root_bead_id":"workflow-1","gc.step_ref":"eval.aggregate","gc.output_json":"{\"aggregate_scores\":{\"axes\":{\"overall\":0.72},\"case_count\":1}}"}},
  {"id":"gate-1","status":"closed","metadata":{
  "gc.root_bead_id":"workflow-1","gc.step_ref":"eval.threshold-gate.iteration.1",
  "eval.aggregate_step_ref":"aggregate","eval.gate_metric":"overall","eval.threshold":"0.80"}}]' \
  "$GC_FAKE_STATE" >"$GC_FAKE_STATE.tmp"
mv -f "$GC_FAKE_STATE.tmp" "$GC_FAKE_STATE"
export GC_BEAD_ID="gate-1"
export GC_ARTIFACT_DIR="$WORK_DIR/gate-artifacts"
if "$PACK_DIR/assets/scripts/eval-threshold-check.sh" >"$WORK_DIR/gate.out" 2>"$WORK_DIR/gate.err"; then
  printf 'expected below-threshold check to fail\n' >&2
  exit 1
fi
jq -e '.outcome == "failed" and .score == 0.72' "$GC_ARTIFACT_DIR/threshold-result.json" >/dev/null

MANIFEST="$PACK_DIR/schemas/eval/examples/eval-run-manifest.v1.example.json"
BEFORE=$(jq 'length' "$GC_FAKE_STATE")
"$PACK_DIR/assets/scripts/eval-file-triage.sh" --manifest "$MANIFEST" \
  --route "execution-city-operations.prompt-eval-classifier" >"$WORK_DIR/triage.out"
AFTER=$(jq 'length' "$GC_FAKE_STATE")
test "$AFTER" -eq $((BEFORE + 1))
jq -e '.[-1].metadata["gc.routed_to"] == "execution-city-operations.prompt-eval-classifier" and
  (.[-1].metadata["eval.evidence_packet"] | fromjson | .cases | length == 1)' "$GC_FAKE_STATE" >/dev/null

jq 'del(.triage_packets[0].failure_label)' "$MANIFEST" >"$WORK_DIR/missing-failure-label.json"
BEFORE=$(jq 'length' "$GC_FAKE_STATE")
if "$PACK_DIR/assets/scripts/eval-file-triage.sh" --manifest "$WORK_DIR/missing-failure-label.json" \
  --route "execution-city-operations.prompt-eval-classifier" >"$WORK_DIR/bad.out" 2>"$WORK_DIR/bad.err"; then
  printf 'expected incomplete evidence packet to fail\n' >&2
  exit 1
fi
test "$(jq 'length' "$GC_FAKE_STATE")" -eq "$BEFORE"

git -C "$REPO_ROOT" diff --quiet origin/main -- \
  examples/gastown/packs/execution-city-operations/agents/prompt-eval-classifier \
  examples/gastown/packs/execution-city-operations/agents/prompt-eval-judge \
  examples/gastown/packs/execution-city-operations/agents/prompt-eval-evidence-gatherer

printf 'execution-city-operations eval tests: PASS\n'
