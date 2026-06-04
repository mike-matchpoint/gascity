#!/usr/bin/env bash
# stuck-session-sweep — close gc sessions stuck in `creating` state with no
# live tmux pane.
#
# Safety model. A session is closed only when ALL of these hold:
#   1. state == "creating"  (not "alive", not "asleep", not "city-stop")
#   2. age >= STUCK_SESSION_AGE_SECONDS (default 600 = 10 minutes)
#   3. no tmux pane with the session's SessionName exists on any socket
#
# Why this is safe for active sessions and between-step transitions:
#   - Active sessions have state="alive" → filtered out by check 1.
#   - Between-step transitions (a fresh-wake session ends, controller spawns
#     the next step's session): the new session enters state="creating"
#     transiently for a few seconds while its tmux pane is being attached.
#     The 10-minute age floor (check 2) leaves a wide margin around normal
#     spawn times.
#   - If a session genuinely IS spawning (slow controller load), it has a
#     tmux pane being attached — check 3 catches that and skips.
#
# Dry-run by default. Set STUCK_SESSION_DRY_RUN=false to actually close.
# Every real close emits a mayor mail with the affected session IDs.

set -euo pipefail

# jq is required for parsing session JSON.
if ! command -v jq >/dev/null 2>&1; then
  echo "stuck-session-sweep: jq is required but not found in PATH" >&2
  exit 1
fi

AGE_THRESHOLD_SECONDS="${STUCK_SESSION_AGE_SECONDS:-600}"
DRY_RUN="${STUCK_SESSION_DRY_RUN:-true}"
LOG_DIR="${STUCK_SESSION_LOG_DIR:-/tmp/stuck-session-sweep}"
mkdir -p "$LOG_DIR"

TIMESTAMP=$(date -u +%Y-%m-%dT%H-%M-%SZ)
LOG_FILE="$LOG_DIR/sweep-$TIMESTAMP.jsonl"
NOW_EPOCH=$(date -u +%s)

SESSIONS_JSON=$(gc session list --json 2>/dev/null) || {
  echo "stuck-session-sweep: gc session list failed" >&2
  exit 1
}

# Detect creating sessions whose age >= threshold. Age is derived from
# CreatedAt (ISO 8601). Sessions without a parseable CreatedAt are skipped.
CANDIDATES_JSONL=$(echo "$SESSIONS_JSON" | jq -c --argjson now "$NOW_EPOCH" '
  .[] |
  select(.State == "creating") |
  . as $s |
  ($s.CreatedAt // "") as $created |
  ($created | fromdateiso8601 // null) as $created_epoch |
  select($created_epoch != null) |
  ($now - $created_epoch) as $age |
  {ID: $s.ID, SessionName: $s.SessionName, Title: $s.Title, AgentName: $s.AgentName,
   State: $s.State, CreatedAt: $created, AgeSeconds: $age}
' 2>/dev/null || true)

if [ -z "$CANDIDATES_JSONL" ]; then
  exit 0
fi

# Filter by age, then by tmux pane absence.
SWEEPABLE=()
while IFS= read -r cand; do
  [ -z "$cand" ] && continue
  AGE=$(echo "$cand" | jq -r '.AgeSeconds | floor')
  if [ "$AGE" -lt "$AGE_THRESHOLD_SECONDS" ]; then
    continue
  fi
  SESS_NAME=$(echo "$cand" | jq -r '.SessionName // ""')
  # Check all tmux sockets in /tmp/tmux-*/* for any session whose name matches.
  # The default socket is /tmp/tmux-$UID/default; gc may use named sockets too.
  if [ -n "$SESS_NAME" ]; then
    HAS_PANE=0
    for SOCKET_PATH in /tmp/tmux-"$(id -u)"/*; do
      [ -S "$SOCKET_PATH" ] || continue
      SOCKET_NAME=$(basename "$SOCKET_PATH")
      if tmux -L "$SOCKET_NAME" has-session -t "$SESS_NAME" 2>/dev/null; then
        HAS_PANE=1
        break
      fi
    done
    [ "$HAS_PANE" = 1 ] && continue
  fi
  SWEEPABLE+=("$cand")
done <<< "$CANDIDATES_JSONL"

if [ "${#SWEEPABLE[@]}" -eq 0 ]; then
  exit 0
fi

# Log what we found (always).
printf '%s\n' "${SWEEPABLE[@]}" > "$LOG_FILE"

if [ "$DRY_RUN" = "true" ]; then
  COUNT="${#SWEEPABLE[@]}"
  echo "stuck-session-sweep [DRY-RUN]: would close $COUNT session(s); details in $LOG_FILE" >&2
  exit 0
fi

# Real close path: iterate, close each, verify state actually transitioned,
# then mail mayor with results.
#
# Why the post-close re-query matters: per WO-013 cartographer test findings
# Issue C2, `gc session close` returns exit code 0 against `creating`-state
# sessions but does NOT transition them (upstream gascity#1493). Without a
# verify step, the script silently accumulates false-positive CLOSED reports
# while the zombies persist for days.
CLOSED_IDS=()
FAILED=()
STUCK_IDS=()
for cand in "${SWEEPABLE[@]}"; do
  SID=$(echo "$cand" | jq -r '.ID')
  gc session close "$SID" >/dev/null 2>&1 || true
  # Re-query: is the session still present in `creating` state?
  STILL_CREATING=$(gc session list --json 2>/dev/null \
    | jq --arg s "$SID" '[.[] | select(.ID == $s and .State == "creating")] | length' \
    2>/dev/null || echo 1)
  if [ "${STILL_CREATING:-1}" -eq 0 ]; then
    CLOSED_IDS+=("$SID")
  else
    STUCK_IDS+=("$SID")
    FAILED+=("$SID")
  fi
done

if [ "${#CLOSED_IDS[@]}" -gt 0 ] || [ "${#FAILED[@]}" -gt 0 ]; then
  DIAG=""
  if [ "${#STUCK_IDS[@]}" -gt 0 ]; then
    DIAG=$(printf '\nDiagnostic: %d session(s) returned close=0 but stayed in `creating` (likely gascity#1493 — needs `brew install --HEAD gascity` per project_boot_deacon_failed_create memory): %s\n' \
      "${#STUCK_IDS[@]}" "${STUCK_IDS[*]}")
  fi
  BODY=$(printf 'Closed: %s\nFailed: %s\nDetails: %s%s' \
    "${CLOSED_IDS[*]:-none}" "${FAILED[*]:-none}" "$LOG_FILE" "$DIAG")
  gc mail send gastown.mayor \
    --subject "[INFO] stuck-session-sweep closed ${#CLOSED_IDS[@]} ghost session(s)" \
    --body "$BODY" >/dev/null 2>&1 || true
fi
