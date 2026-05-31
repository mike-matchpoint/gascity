#!/usr/bin/env bash
# debugger-watch — create plan-mode beads for pending bugs, surface the
# investigation-replan signal, and close bugs whose repair beads have
# landed.
#
# No verify beads. Verify-as-rerun would be redundant: validation
# already ran twice against the same merged_sha — once by polecat
# (`polecat-validate-before-commit`) on the repair branch, and once by
# refinery (`refinery-merge-close-contract`) on the merged tree before
# pushing. A third run in a fresh debugger session against the same
# immutable SHA cannot return new information. If the repair was
# insufficient, polecat/refinery surfaces it pre-push and a NEW bug is
# filed via `refinery-bug-filing`.
#
# Closing the bug is metadata-only — no git, no LLM. This script does
# it directly, mirroring the `convoy-autoclose` order's "poll +
# deterministic close" pattern (`gc convoy check`).
#
# Producer-owned plan lifecycle: abandoned plan beads whose consumer
# crashed are reaped by `scripts/debugger-plan-reap.sh`; non-fatal on
# failure. The reaper also burns legacy debugger wisps during cutover.
#
# Per-bug idempotency: an in-flight plan bead for a given bug_id
# blocks further plan-mode creation for that bug. Atomic claim inside the
# formula is the belt-and-suspenders guard.
set -euo pipefail

DEBUG_LOG="/tmp/debugger-watch.log"
log() { printf '[%s pid=%d] %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$$" "$*" >> "$DEBUG_LOG"; }
log "=== watch START (cwd=$(pwd), uid=$(id -u))"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# 1. Reap orphans first. In rig-scoped invocations this is the cleanup path for
# old duplicate rig-local debugger plans; creation is skipped below.
"$SCRIPT_DIR/debugger-plan-reap.sh" || \
  echo "debugger-watch: reap step failed (non-fatal); continuing to create plans" >&2

# This watcher is city-scoped: one debugger pool scans all rigs and creates
# city-scoped plan beads that the city-scoped debugger selector can claim.
# The codegen-support pack is also imported into rigs for formula/script access; a
# rig-scoped invocation would create duplicate rig-local mol-debugger-plan beads
# that no city-scoped debugger can claim or close.
if [ -n "${GC_RIG:-}" ]; then
  log "skipping rig-scoped debugger-watch invocation for GC_RIG=$GC_RIG"
  exit 0
fi

# City-scoped agent: one debugger pool serves every rig in the city.
# Rig binding lives on each plan bead's `rig_name` metadata field.
AGENT="codegen-support.debugger"
ROLE_ROUTE="codegen-support.debugger"
BINDING_PREFIX="gastown."

# 2. Find in-flight plan-mode beads so we don't double-create. Filter by
# formula so a legacy/foreign-formula bead can't block a legitimate
# plan-mode bead for the same bug. Claimed plan beads transition to
# in_progress under the typed claim path, so include both states.
IN_FLIGHT=$(gc bd list \
  --type=molecule --status=open,in_progress --metadata-field "gc.routed_to=$AGENT" --json 2>/dev/null \
  | jq -r '.[] | select(.metadata.formula == "mol-debugger-plan") | (.metadata.bug_id // empty)' \
  | sort -u)

is_in_flight() {
  local id="$1"
  echo "$IN_FLIGHT" | grep -qFx "$id"
}

# 3. Enumerate registered rigs. Skip the city itself (hq=true) and
# any suspended rigs.
RIGS_JSON=$(gc rig list --json)
log "rigs JSON: $(echo "$RIGS_JSON" | jq -c '.rigs[] | {name, hq, suspended}' | tr '\n' ' ')"
log "IN_FLIGHT plan-mode bug_ids: $(echo "$IN_FLIGHT" | tr '\n' ',')"
created=0
closed=0

while read -r RIG_JSON; do
  RIG=$(echo "$RIG_JSON" | jq -r '.name')
  log "iterating RIG=$RIG"

  # 4. Investigation re-plan signal. For each bug with
  # `decision_state=decided_investigation`, check whether its
  # investigation child closed. If yes, flip the bug back to `pending`
  # with the investigation's findings injected — plan-mode re-runs on
  # the next tick with the new evidence. Idempotent.
  INVEST_BUGS=$(gc --rig "$RIG" bd list \
    --type=bug --status=open \
    --metadata-field gc.kind=bug \
    --metadata-field decision_state=decided_investigation \
    --json | jq -r '.[].id')
  for bug in $INVEST_BUGS; do
    invest_id=$(gc --rig "$RIG" bd show "$bug" --json 2>/dev/null \
      | jq -r '.[0].metadata.repair_beads // ""' | xargs)
    [ -z "$invest_id" ] && continue
    invest_status=$(gc --rig "$RIG" bd show "$invest_id" --json 2>/dev/null \
      | jq -r '.[0].status // empty')
    if [ "$invest_status" = "closed" ]; then
      findings=$(gc --rig "$RIG" bd show "$invest_id" --json \
        | jq -r '.[0].metadata.findings // empty')
      recommendation=$(gc --rig "$RIG" bd show "$invest_id" --json \
        | jq -r '.[0].metadata.recommended_decision_class // empty')
      gc --rig "$RIG" bd update "$bug" \
        --set-metadata decision_state=pending \
        --set-metadata investigation_findings="$findings" \
        --set-metadata investigation_recommendation="$recommendation" \
        --assignee="" >/dev/null
      log "  re-plan: flipped $bug → pending (invest $invest_id closed)"
    fi
  done

# 5. Find pending bugs for this rig and create plan-mode beads.
  PENDING=$(gc --rig "$RIG" bd list \
    --type=bug --status=open \
    --metadata-field gc.kind=bug \
    --metadata-field decision_state=pending \
    --metadata-field "gc.routed_to=$ROLE_ROUTE" \
    --json | jq -r '.[].id')
  log "  PENDING[$RIG]: $(echo "$PENDING" | tr '\n' ',' | sed 's/,$//')"

  for bug in $PENDING; do
    if is_in_flight "$bug"; then continue; fi
    metadata=$(jq -cn \
      --arg agent "$AGENT" \
      --arg bug "$bug" \
      --arg rig "$RIG" \
      --arg binding_prefix "$BINDING_PREFIX" \
      '{
        "gc.routed_to": $agent,
        "bug_id": $bug,
        "rig_name": $rig,
        "binding_prefix": $binding_prefix,
        "formula": "mol-debugger-plan"
      }')
    plan=$(gc bd create "mol-debugger-plan" \
      --type molecule \
      --metadata "$metadata" \
      --json | jq -r '.id // empty')
    if [ -n "$plan" ] && [ "$plan" != "null" ]; then
      echo "Created debugger plan bead $plan for bug $bug in $RIG"
      created=$((created + 1))
    else
      echo "FAILED to create debugger plan bead for bug $bug in $RIG" >&2
    fi
  done

  # 6. Close decided bugs whose repair beads have all landed.
  #
  # Investigation-decided bugs are NOT closed here — they get re-planned
  # by step 4. This block excludes them by filtering on
  # `decision_state=decided` (the code-fix decision class).
  DECIDED=$(gc --rig "$RIG" bd list \
    --type=bug --status=open \
    --metadata-field gc.kind=bug \
    --metadata-field decision_state=decided \
    --json | jq -r '.[].id')
  log "  DECIDED[$RIG]: $(echo "$DECIDED" | tr '\n' ',' | sed 's/,$//')"

  for bug in $DECIDED; do
    REPAIR_BEADS=$(gc --rig "$RIG" bd show "$bug" --json 2>/dev/null \
      | jq -r '.[0].metadata.repair_beads // ""')
    if [ -z "$REPAIR_BEADS" ]; then
      # scope_expanded_to_feature and similar paths leave no
      # repair_beads — a human (or cartographer) owns the next step.
      continue
    fi

    ALL_LANDED=true
    REASON=""
    SHAS=""
    IFS=',' read -ra REPAIR_IDS <<< "$REPAIR_BEADS"
    for id in "${REPAIR_IDS[@]}"; do
      id=$(echo "$id" | xargs)
      [ -z "$id" ] && continue
      info=$(gc --rig "$RIG" bd show "$id" --json 2>/dev/null \
        | jq -r '.[0] | "\(.status)|\(.metadata.repair_kind // "")|\(.metadata.merge_result // "")|\(.metadata.merged_sha // "")"')
      st="${info%%|*}"; rest="${info#*|}"
      kind="${rest%%|*}"; rest="${rest#*|}"
      mr="${rest%%|*}"; sha="${rest##*|}"
      log "    landing-check $bug → $id: status=$st kind=$kind merge_result=$mr sha=${sha:0:12}"

      if [ "$st" != "closed" ]; then
        ALL_LANDED=false
        REASON="$id status=$st"
        break
      fi
      # CANONICAL repair_kind vocabulary (must match decision_class
       # exactly; see mol-debugger-plan formula "INVARIANT — repair_kind
       # vocabulary"). Strict allowlist: any other value deadlocks the
       # bug at decided-but-not-closed.
      case "$kind" in
        direct_bugfix|convoy)
          if [ "$mr" != "merged" ] || ! echo "$sha" | grep -qE '^[0-9a-f]{40}$'; then
            ALL_LANDED=false
            REASON="$id merge_result=$mr sha=$sha"
            break
          fi
          SHAS="${SHAS}${id}=${sha:0:12};"
          ;;
        *)
          # Unexpected kind on a decision_state=decided bug; play safe
          # and refuse to close, leave for human triage.
          ALL_LANDED=false
          REASON="$id unexpected repair_kind=$kind on decided bug"
          break
          ;;
      esac
    done

    if ! $ALL_LANDED; then
      log "    → $bug NOT closeable: $REASON"
      continue
    fi

    if gc --rig "$RIG" bd update "$bug" \
         --set-metadata "verify_status=passed_by_refinery_chain" \
         --set-metadata "verify_method=polecat-validate+refinery-merge-check" \
         --set-metadata "verify_landed_at=$(date -u +%FT%TZ)" \
         --set-metadata "verify_repair_beads_landed=${SHAS%;}" >/dev/null 2>&1 && \
       gc --rig "$RIG" bd close "$bug" \
         --reason "Repair landed: ${REPAIR_BEADS} (validated by polecat pre-commit + refinery pre-push; ${SHAS%;})" >/dev/null 2>&1; then
      echo "Closed bug $bug (repairs landed: ${SHAS%;})"
      closed=$((closed + 1))
    else
      echo "FAILED to close bug $bug after landing detection" >&2
    fi
  done

  # 7. Close decided-duplicate bugs whose canonical bug has closed.
  #
  # Duplicates carry decision_state=duplicate, duplicate_of=<canonical>,
  # and a formal `bd dep add <duplicate> <canonical>` edge. Dependents
  # of the duplicate were transferred to the canonical at emit time, so
  # their auto-unblock is already gated on the canonical's close — this
  # step is bookkeeping: keeps the bug list clean and records the close
  # provenance.
  DUPLICATES=$(gc --rig "$RIG" bd list \
    --type=bug --status=open \
    --metadata-field gc.kind=bug \
    --metadata-field decision_state=duplicate \
    --json | jq -r '.[].id')
  log "  DUPLICATES[$RIG]: $(echo "$DUPLICATES" | tr '\n' ',' | sed 's/,$//')"

  for bug in $DUPLICATES; do
    canon=$(gc --rig "$RIG" bd show "$bug" --json 2>/dev/null \
      | jq -r '.[0].metadata.duplicate_of // empty')
    if [ -z "$canon" ]; then
      log "    duplicate $bug missing duplicate_of metadata; skipping"
      continue
    fi
    canon_status=$(gc --rig "$RIG" bd show "$canon" --json 2>/dev/null \
      | jq -r '.[0].status // "missing"')
    if [ "$canon_status" = "closed" ]; then
      if gc --rig "$RIG" bd close "$bug" \
           --reason "Duplicate of $canon (now closed); resolved via canonical's repair" >/dev/null 2>&1; then
        echo "Closed duplicate bug $bug (canonical $canon closed)"
        closed=$((closed + 1))
      else
        echo "FAILED to close duplicate $bug" >&2
      fi
    else
      log "    duplicate $bug awaits canonical $canon (status=$canon_status)"
    fi
  done

  # 8. Deadlock safety net. Any bug whose decision_state is set to a
  # value outside the recognized closed set has no script path to
  # closure. Mail mayor exactly once (gated on metadata.deadlock_alerted)
  # so the operator can intervene before dependent tasks stall.
  #
  # Recognized terminal states: duplicate, decided, decided_investigation,
  # human_review, rerouted, closed-no-repro. Transient states (pending,
  # in_progress) are ignored.
  KNOWN_RE='^(pending|in_progress|decided|decided_investigation|human_review|rerouted|duplicate|closed-no-repro)$'

  STUCK=$(gc --rig "$RIG" bd list \
    --type=bug --status=open \
    --metadata-field gc.kind=bug \
    --json | jq -r --arg re "$KNOWN_RE" '
      [.[] | select(
        (.metadata.decision_state // "" | test("^$") | not)
        and (.metadata.decision_state // "" | test($re) | not)
        and ((.metadata.deadlock_alerted // "") == "")
      ) | "\(.id)|\(.metadata.decision_state // "<empty>")"
      ] | .[]')

  for entry in $STUCK; do
    bug="${entry%%|*}"
    state="${entry#*|}"
    if gc mail send mayor/ \
         -s "DEADLOCK: $bug has unrecognized decision_state=$state" \
         -m "Bug $bug in rig $RIG carries decision_state=$state, which no watch script can drain. Dependent tasks blocked on this bug (via formal bd dep edges) will remain blocked indefinitely.

Options:
- Fix the decision_state to one of: duplicate, decided, decided_investigation, human_review, rerouted, closed-no-repro.
- Close the bug manually if it's truly resolved.
- Extend the debugger primitive to handle this state (preferred for recurring patterns).

Check 'gc bd show $bug --json' for full context and 'gc bd dep list $bug --dependents' for the blocked work." >/dev/null 2>&1; then
      gc --rig "$RIG" bd update "$bug" \
        --set-metadata deadlock_alerted=true \
        --set-metadata deadlock_alerted_at="$(date -u +%FT%TZ)" >/dev/null 2>&1
      echo "ALERTED mayor about deadlock: $bug (decision_state=$state)"
      log "    deadlock alert sent: $bug (decision_state=$state)"
    else
      echo "FAILED to send deadlock alert for $bug" >&2
    fi
  done
done < <(echo "$RIGS_JSON" | jq -c '.rigs[] | select(.hq == false) | select(.suspended == false)')

# Dolt visibility lag: a bead tagged via `gc bd create --metadata`
# is not immediately queryable by `bd list --metadata-field`. If we exit
# here, the next cooldown tick's IN_FLIGHT check can miss the plan bead we
# just created and create a duplicate. Sleep before END so the bead is
# fully indexed before the next tick runs is_in_flight.
if [ "$created" -gt 0 ]; then
  log "stabilizing: sleeping 30s for Dolt metadata visibility"
  sleep 30
fi

log "=== watch END (created=$created, closed=$closed)"
exit 0
