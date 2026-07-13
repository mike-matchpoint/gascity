#!/usr/bin/env bash
# Controller check wrapper for a city-provided deterministic grader command.
set -euo pipefail

die() { printf 'eval-grader-check: %s\n' "$*" >&2; exit 1; }

command -v jq >/dev/null 2>&1 || die "jq is required"
command -v gc >/dev/null 2>&1 || die "gc is required"
: "${GC_BEAD_ID:?GC_BEAD_ID is required}"
: "${GC_ARTIFACT_DIR:?GC_ARTIFACT_DIR is required}"

BEAD_JSON=$(gc bd show "$GC_BEAD_ID" --json)
BEAD=$(printf '%s' "$BEAD_JSON" | jq -c 'if type == "array" then .[0] else . end')
GRADER_CMD=$(printf '%s' "$BEAD" | jq -r '.metadata["eval.grader_cmd"] // empty')
[ -n "$GRADER_CMD" ] || die "bead is missing eval.grader_cmd"

export EVAL_CASE_ID
export EVAL_FIXTURE_ID
export EVAL_FIXTURE_REF
EVAL_CASE_ID=$(printf '%s' "$BEAD" | jq -r '.metadata["eval.case_id"] // empty')
EVAL_FIXTURE_ID=$(printf '%s' "$BEAD" | jq -r '.metadata["eval.fixture_id"] // empty')
EVAL_FIXTURE_REF=$(printf '%s' "$BEAD" | jq -r '.metadata["eval.fixture_ref"] // empty')
[ -n "$EVAL_CASE_ID" ] && [ -n "$EVAL_FIXTURE_ID" ] && [ -n "$EVAL_FIXTURE_REF" ] || die "bead is missing fixture identity"

mkdir -p "$GC_ARTIFACT_DIR"
export EVAL_ACTUAL_TRACE_REF="$GC_ARTIFACT_DIR/actual-trace.json"
TRACE_JSON=$(printf '%s' "$BEAD" | jq -er '.metadata["gc.output_json"] | fromjson') \
  || die "execution trace is missing or is not JSON"
printf '%s' "$TRACE_JSON" | jq -e '
  type == "object" and
  (.terminal_status | type == "string" and length > 0) and
  (.domain_commands | type == "array" and all(.[]; type == "string" and length > 0)) and
  (.artifact_refs | type == "array" and length == 2) and
  ([.artifact_refs[].artifact_name] | sort == ["manifest.json", "output.json"]) and
  (all(.artifact_refs[];
    (.uri | type == "string" and length > 0) and
    (.content_sha256 | type == "string" and test("^[a-f0-9]{64}$"))))
' >/dev/null || die "execution trace does not satisfy StepExecutionTrace"
printf '%s' "$TRACE_JSON" | jq -cS . >"$EVAL_ACTUAL_TRACE_REF"

RESULT_TMP="$GC_ARTIFACT_DIR/grading-result.json.tmp"
RESULT_FILE="$GC_ARTIFACT_DIR/grading-result.json"
set +e
bash -o pipefail -c "$GRADER_CMD" >"$RESULT_TMP"
GRADER_STATUS=$?
set -e

jq -e '
  type == "object" and
  (.case_id | type == "string" and length > 0) and
  (.status | IN("completed", "not_implemented", "failed")) and
  (.terminal_status_match | type == "boolean") and
  (.expected_terminal_status | type == "string" and length > 0) and
  (.actual_terminal_status | type == "string" and length > 0) and
  (.command_set_diff | type == "object") and
  (.command_set_diff.matched | type == "array") and
  (.command_set_diff.missing | type == "array") and
  (.command_set_diff.unexpected | type == "array") and
  (.rubric_scores | type == "array" and length > 0) and
  (.citation_findings | type == "array")
' "$RESULT_TMP" >/dev/null || die "grader output does not satisfy the grading-result contract"

[ "$(jq -r '.case_id' "$RESULT_TMP")" = "$EVAL_CASE_ID" ] || die "grader case_id does not match the execution bead"
mv -f "$RESULT_TMP" "$RESULT_FILE"
gc bd update "$GC_BEAD_ID" --set-metadata "eval.grading_result_ref=$RESULT_FILE" >/dev/null

if [ "$GRADER_STATUS" -ne 0 ]; then
  printf 'eval-grader-check: grader exited %d; result preserved at %s\n' "$GRADER_STATUS" "$RESULT_FILE" >&2
  exit "$GRADER_STATUS"
fi
[ "$(jq -r '.status' "$RESULT_FILE")" = "completed" ] || die "grader did not complete"
printf '%s\n' "$RESULT_FILE"
