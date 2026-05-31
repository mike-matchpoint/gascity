#!/usr/bin/env bash
# Close the current debugger plan bead and verify that the close landed.
#
# Debugger plan beads are city-scoped. During earlier city-source cutovers the
# same watcher also ran in rig scope, so this helper can resolve either scope,
# closes in the scope where the plan actually exists, and fails loud if it
# cannot prove the terminal close took effect.

set -u

REASON="${1:-Debugger plan completed}"
PLAN_ID="${GC_BEAD_ID:-${PLAN:-}}"
BUG_ID="${BUG:-}"
RIG_NAME="${RIG:-${GC_RIG:-}}"
OWNER="${DEBUGGER_OWNER:-${GC_SESSION_ID:-${GC_SESSION_NAME:-}}}"

log() {
  printf 'debugger-close-plan: %s\n' "$*" >&2
}

mail_mayor() {
  local subject="$1"
  local message="$2"
  gc mail send mayor/ -s "$subject" -m "$message" >/dev/null 2>&1 || true
}

show_status() {
  local scope="$1"
  local id="$2"

  if [ "$scope" = "city" ]; then
    gc bd show "$id" --json 2>/dev/null | jq -r '.[0].status // empty' 2>/dev/null
  else
    gc --rig "$scope" bd show "$id" --json 2>/dev/null | jq -r '.[0].status // empty' 2>/dev/null
  fi
}

close_in_scope() {
  local scope="$1"
  local id="$2"

  if [ "$scope" = "city" ]; then
    gc bd close "$id" --force --reason "$REASON"
  else
    gc --rig "$scope" bd close "$id" --force --reason "$REASON"
  fi
}

burn_in_scope() {
  local scope="$1"
  local id="$2"

  if [ "$scope" = "city" ]; then
    gc bd mol burn "$id" --force
  else
    gc --rig "$scope" bd mol burn "$id" --force
  fi
}

list_candidates() {
  local scope="$1"

  if [ -z "$BUG_ID" ] || [ -z "$RIG_NAME" ]; then
    return 0
  fi

  if [ "$scope" = "city" ]; then
    gc bd list \
      --type=molecule \
      --status=open,in_progress \
      --metadata-field formula=mol-debugger-plan \
      --metadata-field "bug_id=$BUG_ID" \
      --metadata-field "rig_name=$RIG_NAME" \
      --json 2>/dev/null
  else
    gc --rig "$scope" bd list \
      --type=molecule \
      --status=open,in_progress \
      --metadata-field formula=mol-debugger-plan \
      --metadata-field "bug_id=$BUG_ID" \
      --metadata-field "rig_name=$RIG_NAME" \
      --json 2>/dev/null
  fi
}

resolve_candidate() {
  local scope="$1"
  local candidates

  candidates="$(list_candidates "$scope" || true)"
  [ -n "$candidates" ] || return 0

  printf '%s' "$candidates" | jq -r --arg owner "$OWNER" '
    if length == 1 then
      .[0].id
    else
      ([.[] | select((.assignee // "") == $owner)] | if length == 1 then .[0].id else empty end)
    end
  ' 2>/dev/null
}

resolve_scope_for_id() {
  local id="$1"

  if [ -n "$(show_status city "$id")" ]; then
    printf 'city\n'
    return 0
  fi

  if [ -n "$RIG_NAME" ] && [ -n "$(show_status "$RIG_NAME" "$id")" ]; then
    printf '%s\n' "$RIG_NAME"
    return 0
  fi

  return 1
}

SCOPE=""
if [ -n "$PLAN_ID" ]; then
  SCOPE="$(resolve_scope_for_id "$PLAN_ID" || true)"
fi

if [ -z "$PLAN_ID" ] || [ -z "$SCOPE" ]; then
  CITY_CANDIDATE="$(resolve_candidate city || true)"
  RIG_CANDIDATE=""
  if [ -n "$RIG_NAME" ]; then
    RIG_CANDIDATE="$(resolve_candidate "$RIG_NAME" || true)"
  fi

  if [ -n "$CITY_CANDIDATE" ] && [ -z "$RIG_CANDIDATE" ]; then
    PLAN_ID="$CITY_CANDIDATE"
    SCOPE="city"
  elif [ -z "$CITY_CANDIDATE" ] && [ -n "$RIG_CANDIDATE" ]; then
    PLAN_ID="$RIG_CANDIDATE"
    SCOPE="$RIG_NAME"
  elif [ -n "$CITY_CANDIDATE" ] && [ -n "$RIG_CANDIDATE" ]; then
    log "ambiguous plan close: city=$CITY_CANDIDATE rig=$RIG_CANDIDATE bug=${BUG_ID:-unknown} rig=${RIG_NAME:-unknown}"
    mail_mayor "ESCALATION: debugger plan close ambiguous [HIGH]" \
      "Cannot close debugger plan for bug ${BUG_ID:-unknown}; both city plan $CITY_CANDIDATE and rig plan $RIG_CANDIDATE are open/in_progress."
    exit 1
  fi
fi

if [ -z "$PLAN_ID" ] || [ -z "$SCOPE" ]; then
  log "missing plan id; GC_BEAD_ID/PLAN empty and no unique open plan found for bug=${BUG_ID:-unknown} rig=${RIG_NAME:-unknown}"
  mail_mayor "ESCALATION: debugger plan close missing plan id [HIGH]" \
    "Debugger reached terminal close but could not resolve a unique mol-debugger-plan bead. bug=${BUG_ID:-unknown} rig=${RIG_NAME:-unknown} owner=${OWNER:-unknown}"
  exit 1
fi

BURNED=false
if ! close_in_scope "$SCOPE" "$PLAN_ID"; then
  log "close failed for $SCOPE/$PLAN_ID; trying molecule burn fallback"
  if ! burn_in_scope "$SCOPE" "$PLAN_ID"; then
    log "burn fallback failed for $SCOPE/$PLAN_ID"
    mail_mayor "ESCALATION: debugger plan close failed [HIGH]" \
      "Debugger could not close or burn plan $SCOPE/$PLAN_ID for bug ${BUG_ID:-unknown}. Reason attempted: $REASON"
    exit 1
  fi
  BURNED=true
fi

FINAL_STATUS="$(show_status "$SCOPE" "$PLAN_ID" || true)"
if [ "$FINAL_STATUS" != "closed" ]; then
  if $BURNED && [ -z "$FINAL_STATUS" ]; then
    log "burned $SCOPE/$PLAN_ID after close failure: $REASON"
    exit 0
  fi
  log "post-close verification failed for $SCOPE/$PLAN_ID; status=${FINAL_STATUS:-missing}"
  mail_mayor "ESCALATION: debugger plan close did not stick [HIGH]" \
    "Debugger close command returned but plan $SCOPE/$PLAN_ID status is ${FINAL_STATUS:-missing}. bug=${BUG_ID:-unknown} reason=$REASON"
  exit 1
fi

log "closed $SCOPE/$PLAN_ID: $REASON"
