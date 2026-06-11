#!/usr/bin/env bash
# handoff-terminal-sweep — publish GasCityExecutionTerminal.v1 for completed
# handoff molecules and close their handoff beads.
#
# Triggered by orders/handoff-terminal-sweep.toml on a cooldown.
#
# Completion contract: handoff-work-dispatch cooked the requested formula as a
# molecule attached to the handoff bead, so the handoff bead is blocked on the
# molecule root. When the molecule terminates, this sweep derives the terminal
# status, assembles the FLAT terminal detail (echoing correlation,
# formula_bundle_hash, and job_id from the original request verbatim — the
# domain ledger validates them), publishes via publish-cross-city-event.sh,
# stamps handoff.terminal_published_at, and closes the handoff bead.
#
# Status derivation (first match wins):
#   1. handoff.terminal_status metadata on the handoff bead — set by the
#      city's finalizer agent for semantic outcomes (REJECTED, etc.).
#   2. molecule_failed=true on the molecule root        -> FAILED
#   3. any step bead with handoff.step_status=failed    -> FAILED
#   4. otherwise                                        -> SUCCEEDED
# A SUCCEEDED execution whose request demanded an artifact manifest
# (payload.artifact_manifest_required=true) but has no
# handoff.artifact_manifest_uri stamp is downgraded to FAILED — the domain
# resumer would reject it anyway, and a local failure is more actionable.
#
# Molecule-root housekeeping: step beads are children of the molecule root
# (ids <root>.<n>). If every step is closed but the root is still open, the
# sweep closes the root mechanically so the handoff bead unblocks; this keeps
# the sweep correct even when no convoy/deacon automation closed the root.
#
# Everything here is mechanical — no LLM judgment — so the controller runs it
# directly via exec. Idempotent: published beads carry
# handoff.terminal_published_at and are filtered out; the publish itself uses
# a deterministic per-execution dedupe key and the domain ledger dedupes by
# terminal payload hash.
set -euo pipefail

HANDOFF_SCOPE="terminal-sweep"
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=./handoff-state.sh
. "$SCRIPT_DIR/handoff-state.sh"

if ! handoff_acquire_lock; then
  echo "[handoff-terminal-sweep] another run holds the lock — skipping tick"
  exit 0
fi
trap handoff_cleanup EXIT

PUBLISH="$(handoff_publish_script)"

ALL_JSON=$(gc bd list --all --json --limit=0 2>/dev/null || echo '[]')

CANDIDATES=$(printf '%s' "$ALL_JSON" | jq -r '
  .[]
  | select(.status != "closed")
  | select((.metadata."gc.kind" // "") == "event_handoff")
  | select((.metadata."handoff.molecule_root" // "") != "")
  | select((.metadata."handoff.terminal_published_at" // "") == "")
  | .id
' 2>/dev/null || true)

bead_by_id() {
  printf '%s' "$ALL_JSON" | jq -c --arg id "$1" '.[] | select(.id == $id)'
}

steps_of_root() {
  printf '%s' "$ALL_JSON" | jq -c --arg root "$1" '[.[] | select(.id | startswith($root + "."))]'
}

PUBLISHED=0
WAITING=0

while IFS= read -r BEAD_ID; do
  [ -n "$BEAD_ID" ] || continue

  BEAD_JSON=$(bead_by_id "$BEAD_ID")
  ROOT_ID=$(printf '%s' "$BEAD_JSON" | jq -r '.metadata."handoff.molecule_root"')
  ROOT_JSON=$(bead_by_id "$ROOT_ID")
  if [ -z "$ROOT_JSON" ]; then
    echo "[handoff-terminal-sweep] WARN $BEAD_ID references missing molecule root $ROOT_ID" >&2
    WAITING=$((WAITING + 1))
    continue
  fi

  STEPS_JSON=$(steps_of_root "$ROOT_ID")
  ROOT_STATUS=$(printf '%s' "$ROOT_JSON" | jq -r '.status // "open"')
  STEP_COUNT=$(printf '%s' "$STEPS_JSON" | jq 'length')
  OPEN_STEPS=$(printf '%s' "$STEPS_JSON" | jq '[.[] | select(.status != "closed")] | length')

  if [ "$ROOT_STATUS" != "closed" ]; then
    if [ "$STEP_COUNT" -gt 0 ] && [ "$OPEN_STEPS" -eq 0 ]; then
      gc bd close "$ROOT_ID" --reason "all molecule steps closed (handoff-terminal-sweep)" >/dev/null 2>&1 || true
      ROOT_STATUS="closed"
    else
      WAITING=$((WAITING + 1))
      continue
    fi
  fi

  EXPLICIT_STATUS=$(printf '%s' "$BEAD_JSON" | jq -r '.metadata."handoff.terminal_status" // empty')
  MOLECULE_FAILED=$(printf '%s' "$ROOT_JSON" | jq -r '.metadata.molecule_failed // empty')
  FAILED_STEPS=$(printf '%s' "$STEPS_JSON" | jq -r '
    [.[] | select((.metadata."handoff.step_status" // "") == "failed") | .id] | join(",")
  ')

  if [ -n "$EXPLICIT_STATUS" ]; then
    STATUS="$EXPLICIT_STATUS"
    REASON=$(printf '%s' "$BEAD_JSON" | jq -r '.metadata."handoff.terminal_reason" // empty')
    [ -n "$REASON" ] || REASON="terminal status recorded by finalizer"
  elif [ "$MOLECULE_FAILED" = "true" ]; then
    STATUS="FAILED"
    REASON="molecule $ROOT_ID is marked molecule_failed"
  elif [ -n "$FAILED_STEPS" ]; then
    STATUS="FAILED"
    REASON="molecule steps failed: $FAILED_STEPS"
  else
    STATUS="SUCCEEDED"
    REASON="all_required_beads_closed"
  fi

  MANIFEST_URI=$(printf '%s' "$BEAD_JSON" | jq -r '.metadata."handoff.artifact_manifest_uri" // empty')
  MANIFEST_REQUIRED=$(printf '%s' "$BEAD_JSON" | jq -r '.metadata."payload.artifact_manifest_required" // "false"' | tr '[:upper:]' '[:lower:]')
  if [ "$STATUS" = "SUCCEEDED" ] && [ "$MANIFEST_REQUIRED" = "true" ] && [ -z "$MANIFEST_URI" ]; then
    STATUS="FAILED"
    REASON="artifact manifest required but handoff.artifact_manifest_uri is not stamped"
  fi

  NOW=$(handoff_now)
  PAYLOAD_FILE="$HANDOFF_RUN_DIR/terminal-$BEAD_ID.json"
  printf '%s' "$BEAD_JSON" | jq \
    --arg city "${GASCITY_SOURCE_CITY:-unknown-city}" \
    --arg molecule "$ROOT_ID" \
    --arg status "$STATUS" \
    --arg reason "$REASON" \
    --arg manifest "$MANIFEST_URI" \
    --arg now "$NOW" '
    (.metadata // {}) as $m
    | {
        execution_id: ($m."payload.execution_id" // $m.correlation_id),
        gascity_city: $city,
        gascity_molecule_id: $molecule,
        status: $status,
        terminal_reason: $reason,
        formula_name: ($m."payload.formula_name" // "unknown"),
        target_env: ($m."payload.target_env" // "unknown"),
        started_at: ($m."handoff.dispatched_at" // $m.occurred_at // $now),
        completed_at: $now,
        correlation: (try ($m."payload.correlation" | fromjson) catch {})
      }
    + (if ($m."payload.job_id" // "") != "" then {job_id: $m."payload.job_id"} else {} end)
    + (if ($m."payload.vehicle_type" // "") != "" then {vehicle_type: $m."payload.vehicle_type"} else {} end)
    + (if ($m."payload.formula_bundle_hash" // "") != "" then {formula_bundle_hash: $m."payload.formula_bundle_hash"} else {} end)
    + (if $manifest != "" then {artifact_manifest_s3_uri: $manifest} else {} end)
  ' > "$PAYLOAD_FILE"

  EXECUTION_ID=$(jq -r '.execution_id // empty' "$PAYLOAD_FILE")
  if "$PUBLISH" --event-type GasCityExecutionTerminal.v1 \
    --schema-file "$PACK_SCHEMA_DIR/gascity-execution-terminal.v1.schema.json" \
    --payload-file "$PAYLOAD_FILE" \
    --dedupe-key "${EXECUTION_ID:-$BEAD_ID}:terminal"; then
    gc bd update "$BEAD_ID" \
      --set-metadata "handoff.terminal_published_at=$NOW" \
      --set-metadata "handoff.terminal_status=$STATUS" >/dev/null
    gc bd close "$BEAD_ID" --reason "execution terminal published: $STATUS ($REASON)" >/dev/null 2>&1 || true
    echo "[handoff-terminal-sweep] published $STATUS terminal for $BEAD_ID (execution=$EXECUTION_ID molecule=$ROOT_ID)"
    PUBLISHED=$((PUBLISHED + 1))
  else
    echo "[handoff-terminal-sweep] WARN publish failed for $BEAD_ID; will retry next tick" >&2
    WAITING=$((WAITING + 1))
  fi
done <<EOF
$CANDIDATES
EOF

echo "[handoff-terminal-sweep] done: published=$PUBLISHED waiting=$WAITING"
