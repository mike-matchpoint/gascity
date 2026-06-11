#!/usr/bin/env bash
# handoff-work-dispatch — turn inbound GasCityWorkRequested.v1 handoff beads
# into running molecules.
#
# Triggered by orders/handoff-work-dispatch.toml on a cooldown.
#
# The cross-city ingress adapter creates one durable bead per accepted
# GasCityWorkRequested.v1 event with gc.kind=event_handoff and
# gc.routed_to=payload.route. When the route is this dispatcher
# (default domain-handoff.work-dispatcher), this script cooks the
# requested city-resolved formula (payload.formula_name) as a molecule
# ATTACHED to the handoff bead (`gc formula cook --attach`): the bead gains a
# blocking dependency on the sub-DAG root and cannot close until the molecule
# completes. The molecule's step beads carry gc.kind metadata, so the city's
# own routing scan and agent pools execute them — fan-out and fan-in stay
# entirely inside GasCity primitives (beads, dependencies, pools, molecules).
#
# Enumeration and cooking are 100% mechanical (one `bd list`, one
# `formula cook` per new bead, metadata stamps). No LLM judgment, so the
# controller runs it directly via exec. Frequent firing is safe: dispatched
# beads carry handoff.molecule_root and are filtered out of the candidate set.
#
# Failure contract: when the requested formula cannot be compiled (unknown
# name, bad bundle), the script publishes a FAILED GasCityExecutionTerminal.v1
# back to the requesting domain runtime via publish-cross-city-event.sh and
# closes the handoff bead with the actionable reason, so the domain ledger
# never waits forever on an undispatchable request.
set -euo pipefail

HANDOFF_SCOPE="work-dispatch"
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=./handoff-state.sh
. "$SCRIPT_DIR/handoff-state.sh"

if ! handoff_acquire_lock; then
  echo "[handoff-work-dispatch] another run holds the lock — skipping tick"
  exit 0
fi
trap handoff_cleanup EXIT

DISPATCH_ROUTE="${GC_HANDOFF_DISPATCH_ROUTE:-domain-handoff.work-dispatcher}"
PUBLISH="$(handoff_publish_script)"

OPEN_JSON=$(gc bd list --status=open --json --limit=0 2>/dev/null || echo '[]')

CANDIDATES=$(printf '%s' "$OPEN_JSON" | jq -r --arg route "$DISPATCH_ROUTE" '
  .[]
  | select((.metadata."gc.kind" // "") == "event_handoff")
  | select((.metadata.event_type // "") == "GasCityWorkRequested.v1")
  | select((.metadata."gc.routed_to" // "") == $route)
  | select((.metadata."handoff.molecule_root" // "") == "")
  | select((.metadata."handoff.terminal_published_at" // "") == "")
  | .id
' 2>/dev/null || true)

# Build --var arguments for `gc formula cook` from the handoff bead metadata:
# the standard execution fields plus every scalar in payload.variables.
# Variable keys are restricted to [A-Za-z0-9_] to stay shell- and
# formula-substitution-safe; non-scalar values are skipped (agents read them
# from the workflow root bead's payload.* metadata instead).
build_var_args() {
  local bead_json="$1"
  printf '%s' "$bead_json" | jq -r '
    (.metadata // {}) as $m
    | (
        [
          ["execution_id", ($m."payload.execution_id" // "")],
          ["job_id", ($m."payload.job_id" // "")],
          ["target_env", ($m."payload.target_env" // "")],
          ["vehicle_type", ($m."payload.vehicle_type" // "")],
          ["formula_name", ($m."payload.formula_name" // "")],
          ["formula_bundle_hash", ($m."payload.formula_bundle_hash" // "")],
          ["artifact_prefix", ($m."payload.artifact_prefix" // "")],
          ["correlation_json", ($m."payload.correlation" // "")]
        ]
        + (
            (try ($m."payload.variables" | fromjson) catch {})
            | to_entries
            | map(select(.value | type == "string" or type == "number" or type == "boolean"))
            | map([.key, (.value | tostring)])
          )
      )
    | .[]
    | select(.[0] | test("^[A-Za-z0-9_]+$"))
    | select(.[1] != "")
    | "--var\n\(.[0])=\(.[1])"
  '
}

publish_failed_terminal() {
  local bead_json="$1"
  local bead_id="$2"
  local reason="$3"
  local now payload_file
  now=$(handoff_now)
  payload_file="$HANDOFF_RUN_DIR/terminal-$bead_id.json"
  printf '%s' "$bead_json" | jq \
    --arg city "${GASCITY_SOURCE_CITY:-unknown-city}" \
    --arg molecule "$bead_id" \
    --arg reason "$reason" \
    --arg now "$now" '
    (.metadata // {}) as $m
    | {
        execution_id: ($m."payload.execution_id" // $m.correlation_id),
        gascity_city: $city,
        gascity_molecule_id: $molecule,
        status: "FAILED",
        terminal_reason: $reason,
        formula_name: ($m."payload.formula_name" // "unknown"),
        target_env: ($m."payload.target_env" // "unknown"),
        started_at: ($m.occurred_at // $now),
        completed_at: $now,
        correlation: (try ($m."payload.correlation" | fromjson) catch {})
      }
    + (if ($m."payload.job_id" // "") != "" then {job_id: $m."payload.job_id"} else {} end)
    + (if ($m."payload.vehicle_type" // "") != "" then {vehicle_type: $m."payload.vehicle_type"} else {} end)
    + (if ($m."payload.formula_bundle_hash" // "") != "" then {formula_bundle_hash: $m."payload.formula_bundle_hash"} else {} end)
  ' > "$payload_file"

  local execution_id
  execution_id=$(jq -r '.execution_id // empty' "$payload_file")
  if "$PUBLISH" --event-type GasCityExecutionTerminal.v1 \
    --schema-file "$PACK_SCHEMA_DIR/gascity-execution-terminal.v1.schema.json" \
    --payload-file "$payload_file" \
    --dedupe-key "${execution_id:-$bead_id}:terminal"; then
    gc bd update "$bead_id" \
      --set-metadata "handoff.terminal_published_at=$now" \
      --set-metadata "handoff.terminal_status=FAILED" >/dev/null
    gc bd close "$bead_id" --reason "dispatch failed: $reason" >/dev/null
    return 0
  fi
  echo "[handoff-work-dispatch] WARN failed to publish FAILED terminal for $bead_id; will retry next tick" >&2
  return 1
}

DISPATCHED=0
FAILED=0

while IFS= read -r BEAD_ID; do
  [ -n "$BEAD_ID" ] || continue

  BEAD_JSON=$(printf '%s' "$OPEN_JSON" | jq -c --arg id "$BEAD_ID" '.[] | select(.id == $id)')
  FORMULA=$(printf '%s' "$BEAD_JSON" | jq -r '.metadata."payload.formula_name" // empty')
  TITLE=$(printf '%s' "$BEAD_JSON" | jq -r '.title // empty')

  if [ -z "$FORMULA" ]; then
    echo "[handoff-work-dispatch] $BEAD_ID has no payload.formula_name — publishing FAILED terminal" >&2
    publish_failed_terminal "$BEAD_JSON" "$BEAD_ID" "handoff request has no payload.metadata.formula_name to dispatch" || true
    FAILED=$((FAILED + 1))
    continue
  fi

  VAR_ARGS=()
  while IFS= read -r line; do
    [ -n "$line" ] && VAR_ARGS+=("$line")
  done < <(build_var_args "$BEAD_JSON")

  COOK_ERR="$HANDOFF_RUN_DIR/cook-$BEAD_ID.err"
  if COOK_JSON=$(gc formula cook "$FORMULA" --attach "$BEAD_ID" ${VAR_ARGS[@]+"${VAR_ARGS[@]}"} --json 2>"$COOK_ERR"); then
    ROOT_ID=$(printf '%s' "$COOK_JSON" | jq -r '.root_id // empty')
    if [ -z "$ROOT_ID" ]; then
      echo "[handoff-work-dispatch] WARN cook returned no root_id for $BEAD_ID" >&2
      FAILED=$((FAILED + 1))
      continue
    fi
    NOW=$(handoff_now)
    gc bd update "$BEAD_ID" \
      --set-metadata "handoff.molecule_root=$ROOT_ID" \
      --set-metadata "handoff.dispatched_at=$NOW" >/dev/null
    echo "[handoff-work-dispatch] dispatched $BEAD_ID -> molecule $ROOT_ID ($FORMULA)"
    DISPATCHED=$((DISPATCHED + 1))
  else
    REASON="formula dispatch failed for $FORMULA: $(head -c 400 "$COOK_ERR" | tr '\n' ' ')"
    echo "[handoff-work-dispatch] $BEAD_ID cook failed: $REASON" >&2
    publish_failed_terminal "$BEAD_JSON" "$BEAD_ID" "$REASON" || true
    FAILED=$((FAILED + 1))
  fi
done <<EOF
$CANDIDATES
EOF

echo "[handoff-work-dispatch] done: dispatched=$DISPATCHED failed=$FAILED"
