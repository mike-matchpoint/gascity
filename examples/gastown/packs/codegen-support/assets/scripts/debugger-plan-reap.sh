#!/usr/bin/env bash
# debugger-plan-reap - recover abandoned debugger plan beads.
#
# Durable debugger plans are issue-tier molecule beads. Legacy debugger
# wisps can still exist during cutover; they are burned only after the
# same orphan checks pass. Liveness is matched by unique session identity
# (session id or session name), never by the reusable debugger namepool
# alias.

set -euo pipefail

AGENT="${DEBUGGER_AGENT:-codegen-support.debugger}"
PLAN_FORMULA="mol-debugger-plan"
VERIFY_FORMULA="mol-debugger-verify"
DURABLE_GRACE_SECONDS="${DEBUGGER_PLAN_REAP_GRACE_SECONDS:-300}"
LEGACY_GRACE_SECONDS="${DEBUGGER_LEGACY_WISP_REAP_GRACE_SECONDS:-900}"
MAX_SESSION_CACHE_AGE_SECONDS="${DEBUGGER_REAP_MAX_SESSION_CACHE_AGE_SECONDS:-90}"

now_iso=$(date -u +%Y-%m-%dT%H:%M:%SZ)
now_epoch=$(date -u +%s)

iso_to_epoch() {
  local value="$1"
  if date -j -u -f "%Y-%m-%dT%H:%M:%SZ" "$value" +%s 2>/dev/null; then
    return 0
  fi
  date -u -d "$value" +%s 2>/dev/null || echo 0
}

if ! sessions_json=$(gc session list --state all --json 2>/dev/null); then
  echo "debugger-plan-reap: session list unavailable; skipping reaping" >&2
  exit 0
fi

cache_age=$(printf '%s' "$sessions_json" | jq -r '._cache_age_s // 0')
if awk -v age="$cache_age" -v max="$MAX_SESSION_CACHE_AGE_SECONDS" 'BEGIN { exit !(age > max) }'; then
  echo "debugger-plan-reap: session cache age ${cache_age}s exceeds ${MAX_SESSION_CACHE_AGE_SECONDS}s; skipping reaping" >&2
  exit 0
fi

ALIVE_SESSION_OWNERS=$(printf '%s' "$sessions_json" | jq -r '
  .sessions[]
  | select((.State != "closed") and (.State != "stopped") and (.State != "gc_swept"))
  | select(
      ((.Template // "") | test("debugger"))
      or ((.Title // "") | test("debugger"))
      or ((.Alias // "") | test("debugger"))
    )
  | .ID, .SessionName, .Name
  | select(. != null and . != "")
' | sort -u)

is_session_alive() {
  local owner="$1"
  [ -n "$owner" ] && printf '%s\n' "$ALIVE_SESSION_OWNERS" | grep -qFx "$owner"
}

release_bug_if_owned() {
  local rig="$1"
  local bug="$2"
  local owner="$3"
  local reason="$4"

  [ -n "$rig" ] || return 0
  [ -n "$bug" ] || return 0
  local bug_json bug_status bug_assignee bug_state
  bug_json=$(gc --rig "$rig" bd show "$bug" --json 2>/dev/null || true)
  bug_status=$(printf '%s' "$bug_json" | jq -r '.[0].status // empty' 2>/dev/null || true)
  bug_assignee=$(printf '%s' "$bug_json" | jq -r '.[0].assignee // empty' 2>/dev/null || true)
  bug_state=$(printf '%s' "$bug_json" | jq -r '.[0].metadata.decision_state // empty' 2>/dev/null || true)

  if [ "$bug_status" = "open" ] && [ "$bug_state" = "in_progress" ] && { [ "$bug_assignee" = "$owner" ] || [ -z "$bug_assignee" ]; }; then
    gc --rig "$rig" bd update "$bug" \
      --assignee="" \
      --set-metadata decision_state=pending \
      --set-metadata debugger_reaped_at="$now_iso" \
      --set-metadata debugger_reap_reason="$reason" \
      --notes "Released debugger claim from abandoned owner ${owner:-missing-owner}: $reason" >/dev/null
    echo "  $bug: released abandoned debugger claim from ${owner:-missing-owner}"
  fi
}

reset_plan() {
  local id="$1"
  local reason="$2"

  gc bd update "$id" \
    --status=open \
    --assignee="" \
    --set-metadata debugger_reaped_at="$now_iso" \
    --set-metadata debugger_reap_reason="$reason" \
    --set-metadata debugger_reaped_by=debugger-plan-reap >/dev/null
}

close_plan() {
  local id="$1"
  local reason="$2"

  gc bd close "$id" --force --reason "$reason" >/dev/null
}

burn_legacy_wisp() {
  local id="$1"
  gc bd mol burn "$id" --force >/dev/null
}

if ! plans_json=$(gc bd list \
  --type=molecule --status=open,in_progress --metadata-field "gc.routed_to=$AGENT" --json 2>/dev/null \
  | jq --arg plan "$PLAN_FORMULA" --arg verify "$VERIFY_FORMULA" '
      [.[] | select(
        .title == $plan
        or .title == $verify
        or .metadata.formula == $plan
        or .metadata.formula == $verify
      )]
    '); then
  echo "debugger-plan-reap: could not list debugger plans; skipping reaping" >&2
  exit 0
fi

plan_count=$(printf '%s' "$plans_json" | jq 'length')
[ "$plan_count" -eq 0 ] && exit 0

reaped=0
kept=0
echoed_header=0

while IFS= read -r plan_json; do
  id=$(printf '%s' "$plan_json" | jq -r '.id // empty')
  created_at=$(printf '%s' "$plan_json" | jq -r '.created_at // empty')
  status=$(printf '%s' "$plan_json" | jq -r '.status // empty')
  assignee=$(printf '%s' "$plan_json" | jq -r '.assignee // empty')
  ephemeral=$(printf '%s' "$plan_json" | jq -r '.ephemeral // false')
  bug=$(printf '%s' "$plan_json" | jq -r '.metadata.bug_id // empty')
  rig=$(printf '%s' "$plan_json" | jq -r '.metadata.rig_name // empty')
  formula=$(printf '%s' "$plan_json" | jq -r '.metadata.formula // .title // empty')

  [ -n "$id" ] || continue

  created_epoch=$(iso_to_epoch "$created_at")
  grace_seconds="$DURABLE_GRACE_SECONDS"
  if [ "$ephemeral" = "true" ]; then
    grace_seconds="$LEGACY_GRACE_SECONDS"
  fi
  grace_cutoff=$((now_epoch - grace_seconds))
  if [ "$created_epoch" -ge "$grace_cutoff" ]; then
    kept=$((kept + 1))
    continue
  fi

  current_json=$(gc bd show "$id" --json 2>/dev/null || true)
  current_status=$(printf '%s' "$current_json" | jq -r '.[0].status // empty' 2>/dev/null || true)
  current_assignee=$(printf '%s' "$current_json" | jq -r '.[0].assignee // empty' 2>/dev/null || true)
  current_ephemeral=$(printf '%s' "$current_json" | jq -r '.[0].ephemeral // false' 2>/dev/null || true)
  if [ "$current_status" != "$status" ] || [ "$current_assignee" != "$assignee" ]; then
    echo "  $id: claim state changed; skipping"
    kept=$((kept + 1))
    continue
  fi
  if [ -n "$current_assignee" ] && is_session_alive "$current_assignee"; then
    echo "  $id: owner is alive; skipping"
    kept=$((kept + 1))
    continue
  fi

  bug_json=""
  bug_status=""
  bug_assignee=""
  bug_state=""
  if [ -n "$rig" ] && [ -n "$bug" ]; then
    bug_json=$(gc --rig "$rig" bd show "$bug" --json 2>/dev/null || true)
    bug_status=$(printf '%s' "$bug_json" | jq -r '.[0].status // empty' 2>/dev/null || true)
    bug_assignee=$(printf '%s' "$bug_json" | jq -r '.[0].assignee // empty' 2>/dev/null || true)
    bug_state=$(printf '%s' "$bug_json" | jq -r '.[0].metadata.decision_state // empty' 2>/dev/null || true)
  fi

  terminal_bug=false
  if [ -z "$bug_status" ] || [ "$bug_status" = "closed" ]; then
    terminal_bug=true
  elif [ -n "$bug_state" ] && [ "$bug_state" != "pending" ] && [ "$bug_state" != "in_progress" ]; then
    terminal_bug=true
  fi

  if $terminal_bug; then
    if [ "$echoed_header" -eq 0 ]; then
      echo "debugger-plan-reap: abandoned debugger plan work found"
      echoed_header=1
    fi

    if [ "$current_ephemeral" = "true" ]; then
      if burn_legacy_wisp "$id"; then
        echo "  $id (age=$((now_epoch - created_epoch))s, $status): burned terminal legacy wisp for $bug"
        reaped=$((reaped + 1))
      else
        echo "  $id: terminal legacy wisp burn failed (non-fatal)" >&2
      fi
    elif close_plan "$id" "Obsolete debugger plan for $bug: bug status=$bug_status decision_state=$bug_state assignee=$bug_assignee"; then
      echo "  $id (age=$((now_epoch - created_epoch))s, $status): closed terminal durable plan for $bug"
      reaped=$((reaped + 1))
    else
      echo "  $id: terminal durable plan close failed (non-fatal)" >&2
    fi
    continue
  fi

  reason=""
  if [ "$status" = "open" ]; then
    if [ -z "$assignee" ]; then
      if [ "$ephemeral" = "true" ]; then
        reason="legacy-wisp-never-claimed"
      else
        kept=$((kept + 1))
        continue
      fi
    else
      reason="claimer-abandoned (status=open owner=$assignee)"
    fi
  elif [ "$status" = "in_progress" ]; then
    if [ -z "$assignee" ]; then
      reason="claimer-missing (status=in_progress without assignee)"
    else
      reason="claimer-abandoned (status=in_progress owner=$assignee)"
    fi
  else
    kept=$((kept + 1))
    continue
  fi

  if [ "$echoed_header" -eq 0 ]; then
    echo "debugger-plan-reap: abandoned debugger plan work found"
    echoed_header=1
  fi

  obsolete=false
  if [ -z "$bug_status" ] || [ "$bug_status" = "closed" ]; then
    obsolete=true
  elif [ "$bug_state" != "pending" ] && [ "$bug_state" != "in_progress" ]; then
    obsolete=true
  elif [ "$bug_state" = "in_progress" ] && [ -n "$bug_assignee" ] && [ "$bug_assignee" != "$assignee" ]; then
    obsolete=true
  fi

  if ! $obsolete; then
    release_bug_if_owned "$rig" "$bug" "$assignee" "$reason"
  fi

  if [ "$current_ephemeral" = "true" ]; then
    if burn_legacy_wisp "$id"; then
      echo "  $id (age=$((now_epoch - created_epoch))s, $status): burned legacy wisp - $reason"
      reaped=$((reaped + 1))
    else
      echo "  $id: legacy wisp burn failed (non-fatal)" >&2
    fi
  elif $obsolete; then
    if close_plan "$id" "Obsolete debugger plan for $bug: bug status=$bug_status decision_state=$bug_state assignee=$bug_assignee"; then
      echo "  $id (age=$((now_epoch - created_epoch))s, $status): closed obsolete durable plan - $reason"
      reaped=$((reaped + 1))
    else
      echo "  $id: obsolete durable plan close failed (non-fatal)" >&2
    fi
  else
    if reset_plan "$id" "$reason"; then
      echo "  $id (age=$((now_epoch - created_epoch))s, $status): reset durable plan - $reason"
      reaped=$((reaped + 1))
    else
      echo "  $id: durable plan reset failed (non-fatal)" >&2
    fi
  fi
done < <(printf '%s' "$plans_json" | jq -c 'sort_by(.created_at) | .[]')

if [ "$reaped" -gt 0 ]; then
  echo "debugger-plan-reap: reaped=$reaped kept=$kept total=$plan_count"
fi

exit 0
