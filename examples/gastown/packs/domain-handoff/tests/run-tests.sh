#!/usr/bin/env bash
# Unit tests for the domain-handoff pack primitives.
#
# Exercises the REAL pack scripts (publish-cross-city-event.sh and the four
# handoff order scripts) against a fake `gc` (JSON bead-state file) and a fake
# `aws` (put-events capture file). No network, no live city, no LLM.
#
# Run directly:  bash tests/run-tests.sh   (or via `make test-packs`)
set -euo pipefail

TESTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PACK_DIR="$(cd "$TESTS_DIR/.." && pwd)"
SCRIPTS="$PACK_DIR/assets/scripts"
SCHEMAS="$PACK_DIR/schemas/events"
ECO_PACK="$(cd "$PACK_DIR/../execution-city-operations" && pwd)"
PUBLISH="$ECO_PACK/assets/scripts/publish-cross-city-event.sh"
EXAMPLES="$SCHEMAS/examples"

PASS=0
FAIL=0
CURRENT_TEST=""

note() { printf '  %s\n' "$*"; }
ok() { PASS=$((PASS + 1)); printf 'ok   %s\n' "$CURRENT_TEST"; }
bad() {
  FAIL=$((FAIL + 1))
  printf 'FAIL %s: %s\n' "$CURRENT_TEST" "$*" >&2
}

assert_eq() {
  local expected="$1" actual="$2" what="$3"
  if [ "$expected" != "$actual" ]; then
    bad "$what: expected [$expected], got [$actual]"
    return 1
  fi
}

assert_contains() {
  local haystack="$1" needle="$2" what="$3"
  case "$haystack" in
    *"$needle"*) return 0 ;;
    *) bad "$what: [$needle] not found"; return 1 ;;
  esac
}

# Fresh isolated environment per test: bead state, capture file, lock base,
# and a fake city root so handoff-state.sh resolves CITY_ROOT.
setup() {
  WORK_DIR=$(mktemp -d "${TMPDIR:-/tmp}/eco-handoff-test.XXXXXX")
  touch "$WORK_DIR/city.toml"
  export CITY_ROOT="$WORK_DIR"
  export GC_FAKE_STATE="$WORK_DIR/beads.json"
  export AWS_FAKE_CAPTURE="$WORK_DIR/put-events.jsonl"
  export HANDOFF_STATE_DIR="$WORK_DIR/handoff-state"
  export GASCITY_EVENT_BUS="gascity-handoff-test"
  export AWS_REGION="us-west-2"
  export GASCITY_SOURCE_CITY="test-execution-city-dev"
  export GASCITY_SOURCE_CITY_ROLE="execution-monitoring-city"
  export GASCITY_PROCESS_SLUG="test-process"
  export GASCITY_CITY_PAIR_SLUG="test-process"
  unset GASCITY_CODEGEN_OWNERSHIP_JSON || true
  echo '[]' > "$GC_FAKE_STATE"
  : > "$AWS_FAKE_CAPTURE"
  export PATH="$TESTS_DIR/fakes:$PATH"
}

teardown() {
  rm -rf "$WORK_DIR"
}

beads() { cat "$GC_FAKE_STATE"; }
bead() { beads | jq -c --arg id "$1" '.[] | select(.id == $id)'; }
captured_details() { jq -c '.[0].Detail | fromjson' "$AWS_FAKE_CAPTURE" 2>/dev/null || true; }
capture_count() { wc -l < "$AWS_FAKE_CAPTURE" | tr -d ' '; }

seed_handoff_bead() {
  local id="$1" formula="$2" extra="${3:-}"
  [ -n "$extra" ] || extra="{}"
  beads | jq --arg id "$id" --arg formula "$formula" --argjson extra "$extra" '
    . + [{
      id: $id, title: ("handoff " + $id), status: "open", type: "task",
      metadata: ({
        "gc.kind": "event_handoff",
        "gc.routed_to": "domain-handoff.work-dispatcher",
        event_type: "GasCityWorkRequested.v1",
        correlation_id: "gce-test-001",
        idempotency_key: "test-idem-001",
        occurred_at: "2026-06-10T12:00:00Z",
        "payload.execution_id": "gce-test-001",
        "payload.job_id": "job-001",
        "payload.target_env": "dev",
        "payload.vehicle_type": "tractor",
        "payload.formula_name": $formula,
        "payload.formula_bundle_hash": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
        "payload.artifact_manifest_required": "true",
        "payload.correlation": ({job_id: "job-001", vehicle_type: "tractor", category: "drawbar"} | tojson),
        "payload.variables": ({category: "drawbar", sample_size: 25} | tojson)
      } + $extra)
    }]
  ' > "$GC_FAKE_STATE.tmp" && mv "$GC_FAKE_STATE.tmp" "$GC_FAKE_STATE"
}

# ---------------------------------------------------------------------------
CURRENT_TEST="schemas: every schema and example parses and validates"
setup
{
  RESULT=0
  for schema in "$SCHEMAS"/*.schema.json; do
    jq empty "$schema" || { bad "schema does not parse: $schema"; RESULT=1; }
  done
  for example in "$EXAMPLES"/*.example.json; do
    jq empty "$example" || { bad "example does not parse: $example"; RESULT=1; }
  done
  if command -v python3 >/dev/null 2>&1 && python3 -c "import jsonschema" >/dev/null 2>&1; then
    for example in "$EXAMPLES"/*.example.json; do
      name=$(basename "$example" .example.json)
      schema="$SCHEMAS/$name.schema.json"
      [ -f "$schema" ] || { bad "no schema for example $name"; RESULT=1; continue; }
      python3 - "$schema" "$example" <<'PY' || { bad "example fails schema: $name"; RESULT=1; }
import json, sys
import jsonschema
jsonschema.validate(instance=json.load(open(sys.argv[2])), schema=json.load(open(sys.argv[1])))
PY
    done
  else
    note "python3+jsonschema unavailable; skipped full example validation"
  fi
  [ "$RESULT" -eq 0 ] && ok
}
teardown

# ---------------------------------------------------------------------------
CURRENT_TEST="publish: flat event publishes raw detail with domain source"
setup
{
  if "$PUBLISH" \
    --event-type GasCityExecutionTerminal.v1 --schema-file "$SCHEMAS/gascity-execution-terminal.v1.schema.json" \
    --payload-file "$EXAMPLES/gascity-execution-terminal.v1.example.json" \
    --dedupe-key "gce-test:terminal" > "$WORK_DIR/out.log" 2>&1; then
    ENTRY=$(jq -c '.[0]' "$AWS_FAKE_CAPTURE")
    assert_eq "GasCity" "$(printf '%s' "$ENTRY" | jq -r '.Source')" "flat event Source" \
      && assert_eq "GasCityExecutionTerminal.v1" "$(printf '%s' "$ENTRY" | jq -r '.DetailType')" "flat DetailType" \
      && assert_eq "gce-20260610-tractor-drawbar-001" \
        "$(printf '%s' "$ENTRY" | jq -r '.Detail | fromjson | .execution_id')" "flat detail is the raw payload" \
      && ok
  else
    bad "publish exited nonzero: $(cat "$WORK_DIR/out.log")"
  fi
}
teardown

# ---------------------------------------------------------------------------
CURRENT_TEST="publish: envelope event wraps payload and requires target city"
setup
{
  jq '.payload' "$EXAMPLES/gascity-work-requested.v1.example.json" > "$WORK_DIR/payload.json"
  if "$PUBLISH" \
    --event-type GasCityWorkRequested.v1 --schema-file "$SCHEMAS/gascity-work-requested.v1.schema.json" \
    --payload-file "$WORK_DIR/payload.json" \
    --target-city test-execution-city-dev \
    --target-city-role execution-monitoring-city \
    --dry-run > "$WORK_DIR/out.log" 2>&1; then
    assert_contains "$(cat "$WORK_DIR/out.log")" "event_class=envelope" "dry-run reports envelope class" \
      && ok
  else
    bad "dry-run exited nonzero: $(cat "$WORK_DIR/out.log")"
  fi
  if "$PUBLISH" \
    --event-type GasCityWorkRequested.v1 --schema-file "$SCHEMAS/gascity-work-requested.v1.schema.json" \
    --payload-file "$WORK_DIR/payload.json" \
    --dry-run > "$WORK_DIR/missing.log" 2>&1; then
    bad "envelope publish without --target-city should fail"
  fi
}
teardown

# ---------------------------------------------------------------------------
CURRENT_TEST="publish: repo events still require the codegen ownership index"
setup
{
  jq '.payload' "$ECO_PACK/schemas/events/examples/repo-bug-reported.v1.example.json" > "$WORK_DIR/payload.json"
  if "$PUBLISH" \
    --event-type RepoBugReported.v1 \
    --payload-file "$WORK_DIR/payload.json" \
    --dry-run > "$WORK_DIR/out.log" 2>&1; then
    bad "repo event publish without ownership index should fail"
  else
    assert_contains "$(cat "$WORK_DIR/out.log")" "GASCITY_CODEGEN_OWNERSHIP_JSON" "names the missing index" \
      && ok
  fi
}
teardown

# ---------------------------------------------------------------------------
CURRENT_TEST="dispatch: cooks formula onto handoff bead and is idempotent"
setup
{
  seed_handoff_bead "hb-1" "vehicle.schema_exploration.v1"
  bash "$SCRIPTS/handoff-work-dispatch.sh" > "$WORK_DIR/out.log" 2>&1 || bad "dispatch run 1 failed: $(cat "$WORK_DIR/out.log")"
  ROOT=$(bead "hb-1" | jq -r '.metadata."handoff.molecule_root" // empty')
  assert_eq "hb-1-mol" "$ROOT" "molecule root stamped" || true
  MOL_VARS=$(bead "hb-1-mol" | jq -r '.metadata."cook.vars" // "{}" | fromjson | .execution_id // empty')
  assert_eq "gce-test-001" "$MOL_VARS" "execution_id passed as cook var" || true
  COUNT_BEFORE=$(beads | jq 'length')
  bash "$SCRIPTS/handoff-work-dispatch.sh" > "$WORK_DIR/out2.log" 2>&1 || bad "dispatch run 2 failed"
  COUNT_AFTER=$(beads | jq 'length')
  assert_eq "$COUNT_BEFORE" "$COUNT_AFTER" "second run creates no new beads" \
    && assert_eq "0" "$(capture_count)" "no terminal published on success path" \
    && ok
}
teardown

# ---------------------------------------------------------------------------
CURRENT_TEST="dispatch: unknown formula publishes FAILED terminal and closes bead"
setup
{
  seed_handoff_bead "hb-2" "vehicle.unknown_formula.v1"
  bash "$SCRIPTS/handoff-work-dispatch.sh" > "$WORK_DIR/out.log" 2>&1 || true
  DETAIL=$(captured_details)
  assert_eq "FAILED" "$(printf '%s' "$DETAIL" | jq -r '.status // empty')" "terminal status" || true
  assert_eq "gce-test-001" "$(printf '%s' "$DETAIL" | jq -r '.execution_id // empty')" "terminal execution_id" || true
  assert_eq "job-001" "$(printf '%s' "$DETAIL" | jq -r '.correlation.job_id // empty')" "correlation echoed" || true
  assert_eq "closed" "$(bead "hb-2" | jq -r '.status')" "handoff bead closed" \
    && assert_eq "FAILED" "$(bead "hb-2" | jq -r '.metadata."handoff.terminal_status" // empty')" "terminal status stamped" \
    && ok
}
teardown

# ---------------------------------------------------------------------------
CURRENT_TEST="sweep: open molecule means no terminal yet"
setup
{
  seed_handoff_bead "hb-3" "vehicle.schema_exploration.v1" \
    '{"handoff.molecule_root": "hb-3-mol", "handoff.dispatched_at": "2026-06-10T12:01:00Z"}'
  beads | jq '. + [
    {id: "hb-3-mol", title: "molecule", status: "open", type: "molecule", metadata: {}},
    {id: "hb-3-mol.1", title: "step 1", status: "open", type: "task", metadata: {}}
  ]' > "$GC_FAKE_STATE.tmp" && mv "$GC_FAKE_STATE.tmp" "$GC_FAKE_STATE"
  bash "$SCRIPTS/handoff-terminal-sweep.sh" > "$WORK_DIR/out.log" 2>&1 || bad "sweep failed: $(cat "$WORK_DIR/out.log")"
  assert_eq "0" "$(capture_count)" "no terminal published while steps open" \
    && assert_eq "open" "$(bead "hb-3" | jq -r '.status')" "handoff bead still open" \
    && ok
}
teardown

# ---------------------------------------------------------------------------
CURRENT_TEST="sweep: completed molecule publishes SUCCEEDED with manifest and closes root"
setup
{
  seed_handoff_bead "hb-4" "vehicle.schema_exploration.v1" \
    '{"handoff.molecule_root": "hb-4-mol", "handoff.dispatched_at": "2026-06-10T12:01:00Z",
      "handoff.artifact_manifest_uri": "s3://test-bucket/gascity/executions/gce-test-001/manifest.json"}'
  beads | jq '. + [
    {id: "hb-4-mol", title: "molecule", status: "open", type: "molecule", metadata: {}},
    {id: "hb-4-mol.1", title: "step 1", status: "closed", type: "task", metadata: {}},
    {id: "hb-4-mol.2", title: "step 2", status: "closed", type: "task", metadata: {}}
  ]' > "$GC_FAKE_STATE.tmp" && mv "$GC_FAKE_STATE.tmp" "$GC_FAKE_STATE"
  bash "$SCRIPTS/handoff-terminal-sweep.sh" > "$WORK_DIR/out.log" 2>&1 || bad "sweep failed: $(cat "$WORK_DIR/out.log")"
  DETAIL=$(captured_details)
  assert_eq "SUCCEEDED" "$(printf '%s' "$DETAIL" | jq -r '.status // empty')" "terminal status" || true
  assert_eq "hb-4-mol" "$(printf '%s' "$DETAIL" | jq -r '.gascity_molecule_id // empty')" "molecule id" || true
  assert_eq "s3://test-bucket/gascity/executions/gce-test-001/manifest.json" \
    "$(printf '%s' "$DETAIL" | jq -r '.artifact_manifest_s3_uri // empty')" "manifest uri" || true
  assert_eq "2026-06-10T12:01:00Z" "$(printf '%s' "$DETAIL" | jq -r '.started_at // empty')" "started_at from dispatch stamp" || true
  assert_eq "closed" "$(bead "hb-4-mol" | jq -r '.status')" "molecule root housekept closed" || true
  assert_eq "closed" "$(bead "hb-4" | jq -r '.status')" "handoff bead closed" \
    && ok
}
teardown

# ---------------------------------------------------------------------------
CURRENT_TEST="sweep: required manifest missing downgrades to FAILED"
setup
{
  seed_handoff_bead "hb-5" "vehicle.schema_exploration.v1" \
    '{"handoff.molecule_root": "hb-5-mol", "handoff.dispatched_at": "2026-06-10T12:01:00Z"}'
  beads | jq '. + [
    {id: "hb-5-mol", title: "molecule", status: "closed", type: "molecule", metadata: {}},
    {id: "hb-5-mol.1", title: "step 1", status: "closed", type: "task", metadata: {}}
  ]' > "$GC_FAKE_STATE.tmp" && mv "$GC_FAKE_STATE.tmp" "$GC_FAKE_STATE"
  bash "$SCRIPTS/handoff-terminal-sweep.sh" > "$WORK_DIR/out.log" 2>&1 || bad "sweep failed"
  DETAIL=$(captured_details)
  assert_eq "FAILED" "$(printf '%s' "$DETAIL" | jq -r '.status // empty')" "terminal status" || true
  assert_contains "$(printf '%s' "$DETAIL" | jq -r '.terminal_reason // empty')" "artifact manifest" "reason names the manifest" \
    && ok
}
teardown

# ---------------------------------------------------------------------------
CURRENT_TEST="sweep: failed step and explicit terminal status are honored"
setup
{
  seed_handoff_bead "hb-6" "vehicle.schema_exploration.v1" \
    '{"handoff.molecule_root": "hb-6-mol", "handoff.dispatched_at": "2026-06-10T12:01:00Z"}'
  beads | jq '. + [
    {id: "hb-6-mol", title: "molecule", status: "closed", type: "molecule", metadata: {}},
    {id: "hb-6-mol.1", title: "step 1", status: "closed", type: "task",
     metadata: {"handoff.step_status": "failed"}}
  ]' > "$GC_FAKE_STATE.tmp" && mv "$GC_FAKE_STATE.tmp" "$GC_FAKE_STATE"
  bash "$SCRIPTS/handoff-terminal-sweep.sh" > "$WORK_DIR/out.log" 2>&1 || bad "sweep run 1 failed"
  DETAIL=$(captured_details)
  assert_eq "FAILED" "$(printf '%s' "$DETAIL" | jq -r '.status // empty')" "failed step yields FAILED" || true
  assert_contains "$(printf '%s' "$DETAIL" | jq -r '.terminal_reason // empty')" "hb-6-mol.1" "reason names failed step" || true

  echo '[]' > "$GC_FAKE_STATE"; : > "$AWS_FAKE_CAPTURE"
  seed_handoff_bead "hb-7" "vehicle.schema_exploration.v1" \
    '{"handoff.molecule_root": "hb-7-mol", "handoff.dispatched_at": "2026-06-10T12:01:00Z",
      "handoff.terminal_status": "REJECTED", "handoff.terminal_reason": "approval rejected: coverage incomplete"}'
  beads | jq '. + [
    {id: "hb-7-mol", title: "molecule", status: "closed", type: "molecule", metadata: {}}
  ]' > "$GC_FAKE_STATE.tmp" && mv "$GC_FAKE_STATE.tmp" "$GC_FAKE_STATE"
  rm -rf "$HANDOFF_STATE_DIR"
  bash "$SCRIPTS/handoff-terminal-sweep.sh" > "$WORK_DIR/out2.log" 2>&1 || bad "sweep run 2 failed"
  DETAIL=$(captured_details)
  assert_eq "REJECTED" "$(printf '%s' "$DETAIL" | jq -r '.status // empty')" "explicit terminal status wins" \
    && assert_eq "approval rejected: coverage incomplete" \
      "$(printf '%s' "$DETAIL" | jq -r '.terminal_reason // empty')" "explicit reason wins" \
    && ok
}
teardown

# ---------------------------------------------------------------------------
CURRENT_TEST="command publish: publishes waiter payload with injected reply"
setup
{
  CMD_PAYLOAD=$(jq -c 'del(.reply)' "$EXAMPLES/gascity-domain-command-requested.v1.example.json")
  beads | jq --arg payload "$CMD_PAYLOAD" '. + [
    {id: "wait-1", title: "domain command waiter", status: "open", type: "task",
     metadata: {"gc.kind": "domain_command_request", origin_bead: "step-1", payload: $payload}}
  ]' > "$GC_FAKE_STATE.tmp" && mv "$GC_FAKE_STATE.tmp" "$GC_FAKE_STATE"
  bash "$SCRIPTS/domain-command-publish.sh" > "$WORK_DIR/out.log" 2>&1 || bad "publish failed: $(cat "$WORK_DIR/out.log")"
  DETAIL=$(captured_details)
  assert_eq "gcdc-20260610-survey-tractor-001" "$(printf '%s' "$DETAIL" | jq -r '.command_id // empty')" "command id" || true
  assert_eq "test-execution-city-dev" "$(printf '%s' "$DETAIL" | jq -r '.reply.target_city // empty')" "reply city injected" || true
  assert_eq "domain-handoff.command-terminal-reconciler" \
    "$(printf '%s' "$DETAIL" | jq -r '.reply.route // empty')" "reply route injected" || true
  assert_eq "open" "$(bead "wait-1" | jq -r '.status')" "waiter stays open" || true
  assert_eq "gcdc-20260610-survey-tractor-001" \
    "$(bead "wait-1" | jq -r '.metadata."handoff.command_id" // empty')" "command id stamped" || true
  COUNT=$(capture_count)
  bash "$SCRIPTS/domain-command-publish.sh" > "$WORK_DIR/out2.log" 2>&1 || bad "publish run 2 failed"
  assert_eq "$COUNT" "$(capture_count)" "second run publishes nothing" \
    && ok
}
teardown

# ---------------------------------------------------------------------------
CURRENT_TEST="command reconcile: applies terminal to waiter and unblocks origin"
setup
{
  beads | jq '. + [
    {id: "step-1", title: "origin step", status: "open", type: "task", metadata: {}},
    {id: "wait-2", title: "waiter", status: "open", type: "task",
     metadata: {"gc.kind": "domain_command_request", origin_bead: "step-1",
                "handoff.command_id": "gcdc-test-9", "handoff.command_published_at": "2026-06-10T12:05:00Z"}},
    {id: "term-1", title: "inbound terminal", status: "open", type: "task",
     metadata: {"gc.kind": "event_handoff", event_type: "GasCityDomainCommandTerminal.v1",
                "gc.routed_to": "domain-handoff.command-terminal-reconciler",
                "payload.command_id": "gcdc-test-9", "payload.status": "SUCCEEDED",
                "payload.result_s3_uri": "s3://test-bucket/gascity/executions/gce-test-001/beads/step-1/output.json"}}
  ]' > "$GC_FAKE_STATE.tmp" && mv "$GC_FAKE_STATE.tmp" "$GC_FAKE_STATE"
  bash "$SCRIPTS/domain-command-reconcile.sh" > "$WORK_DIR/out.log" 2>&1 || bad "reconcile failed: $(cat "$WORK_DIR/out.log")"
  assert_eq "closed" "$(bead "wait-2" | jq -r '.status')" "waiter closed" || true
  assert_eq "SUCCEEDED" "$(bead "wait-2" | jq -r '.metadata."handoff.command_status" // empty')" "status stamped" || true
  assert_eq "gcdc-test-9" "$(bead "wait-2" | jq -r '.metadata."handoff.command_terminal" // "{}" | fromjson | .command_id // empty')" "terminal preserved" || true
  assert_eq "closed" "$(bead "term-1" | jq -r '.status')" "terminal bead closed" || true
  assert_contains "$(bead "step-1" | jq -r '(.notes // []) | join(" ")')" "gcdc-test-9" "origin step noted" \
    && ok
}
teardown

# ---------------------------------------------------------------------------
CURRENT_TEST="command reconcile: duplicates close, orphans stay open"
setup
{
  beads | jq '. + [
    {id: "wait-3", title: "waiter", status: "closed", type: "task",
     metadata: {"gc.kind": "domain_command_request", "handoff.command_id": "gcdc-dup-1"}},
    {id: "term-2", title: "duplicate terminal", status: "open", type: "task",
     metadata: {"gc.kind": "event_handoff", event_type: "GasCityDomainCommandTerminal.v1",
                "gc.routed_to": "domain-handoff.command-terminal-reconciler",
                "payload.command_id": "gcdc-dup-1", "payload.status": "SUCCEEDED"}},
    {id: "term-3", title: "orphan terminal", status: "open", type: "task",
     metadata: {"gc.kind": "event_handoff", event_type: "GasCityDomainCommandTerminal.v1",
                "gc.routed_to": "domain-handoff.command-terminal-reconciler",
                "payload.command_id": "gcdc-none-1", "payload.status": "FAILED"}}
  ]' > "$GC_FAKE_STATE.tmp" && mv "$GC_FAKE_STATE.tmp" "$GC_FAKE_STATE"
  bash "$SCRIPTS/domain-command-reconcile.sh" > "$WORK_DIR/out.log" 2>&1 || bad "reconcile failed"
  assert_eq "closed" "$(bead "term-2" | jq -r '.status')" "duplicate terminal closed" || true
  assert_eq "open" "$(bead "term-3" | jq -r '.status')" "orphan terminal stays open" \
    && ok
}
teardown

# ---------------------------------------------------------------------------
printf '\n%d passed, %d failed\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ]
