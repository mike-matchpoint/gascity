#!/usr/bin/env bash
# landing-arbiter-wisp-reap — reap orphaned mol-landing-arbiter-patrol wisps.
#
# Landing Arbiter now uses pending landing-failure bugs as canonical typed
# work. This script is retained only as migration cleanup for patrol wisps
# created by older deployments.
#
# This reaper enforces the lifecycle invariant for landing-arbiter wisps
# specifically:
#
#   Old mol-landing-arbiter-patrol wisps should not accumulate after the typed
#   selector cutover. A wisp older than the grace window with no live arbiter
#   session is migration residue and gets burned.
#
# Scoped intentionally to the landing-arbiter template. Other agent
# templates (polecats, refinery, etc.) have different concurrency models
# and lifecycle contracts and are NOT touched here.
#
# Called by `landing-arbiter-watch.sh` during cooldown cleanup. Safe to call
# independently for diagnostics.
#
# Cheap when there are no orphans: one `bd list` returns an empty array.

set -euo pipefail

RIG="${1:-${GC_RIG:-}}"
if [ -z "$RIG" ]; then
  echo "usage: landing-arbiter-wisp-reap.sh <rig-name>" >&2
  exit 2
fi
AGENT="$RIG/codegen-support.landing-arbiter"
FORMULA="mol-landing-arbiter-patrol"

# Tolerance window: a legacy wisp created within the last GRACE_SECONDS is too
# new to reap. Two reasons it needs to be generous:
#
#   1. Pool spawn lag — the controller takes 3-5 minutes to instantiate
#      a fresh arbiter session after the previous one drained. During
#      that window, the wisp sits open with no consumer because no
#      consumer exists yet. Reaping it would deadlock the loop: every
#      tick pours a new wisp and the next tick reaps it before a
#      session ever materializes.
#
#   2. Arbiter thinking time — once spawned, the arbiter takes 5-15
#      minutes for max-effort reasoning. While it's mid-think, the wisp
#      is in_progress; we trust the session-active check for that.
#      But the wisp's `created_at` was at pour time, not claim time,
#      so a long arbiter cycle can leave a 15-min-old wisp legitimately
#      in flight.
#
# Net: 10 minutes (600s) is the floor for routine spawn+claim. We are
# strictly reaping abandoned wisps; earlier reaping risks burning work from a
# pre-cutover session that is still starting.
GRACE_SECONDS=600

now_epoch=$(date -u +%s)
grace_cutoff=$((now_epoch - GRACE_SECONDS))

# Snapshot of current arbiter session state. The arbiter is singleton so
# this is a single row at most. We treat any non-stopped, non-swept
# state as potentially-in-flight: active (running), creating (controller
# spawning), asleep (claimed but idle between iterations), drained
# (cleanly exited). Only fully-stopped/gc_swept slots are confirmed-dead.
#
# This is intentionally conservative: if there's ANY session record
# that the controller might still be working with, we don't reap.
# Reap only when the pool slot is truly empty.
arbiter_session_state=$(gc session list 2>/dev/null \
  | awk -v agent="$AGENT" '$0 ~ agent && $0 !~ /stopped|gc_swept/ {print $3; exit}')

# Any live arbiter session means a pre-cutover patrol may still be in flight.
# Reap only when the pool slot is empty.
if [ -n "$arbiter_session_state" ]; then
  exit 0
fi

# List active wisps with the patrol formula's title. Scope is by title,
# NOT by assignee. A pre-`--assignee` filter would silently skip orphans
# that are (a) unassigned (poured but never reached the watcher's
# post-pour assignment step), or (b) assigned to a stale identity from
# a prior pack-binding (e.g., pre-migration `$RIG/landing-arbiter` vs
# the current `$RIG/codegen-support.landing-arbiter`). Both classes
# accumulate forever and are invisible until the title scope is used.
#
# Newly-poured-but-not-yet-claimed legacy wisps are protected by GRACE_SECONDS
# (currently 600s, 10 min) — the loop below skips any wisp younger than
# the cutoff. The live-session guard above adds a second layer while the
# controller or a pre-cutover session might still be working.
open_wisps_json=$(gc --rig "$RIG" bd list \
  --type=molecule --status=open,in_progress --json 2>/dev/null \
  | jq --arg formula "$FORMULA" '[.[] | select(.title == $formula)]')

wisp_count=$(echo "$open_wisps_json" | jq 'length')

if [ "$wisp_count" -eq 0 ]; then
  exit 0
fi

reaped=0
kept=0
echoed_header=0

while IFS=$'\t' read -r wisp_id created_at wisp_status; do
  [ -z "$wisp_id" ] && continue

  # Convert ISO timestamp to epoch (BSD date on macOS, GNU date on Linux).
  if created_epoch=$(date -j -u -f "%Y-%m-%dT%H:%M:%SZ" "$created_at" +%s 2>/dev/null); then
    :
  else
    created_epoch=$(date -u -d "$created_at" +%s 2>/dev/null || echo 0)
  fi

  # Grace period: skip wisps younger than GRACE_SECONDS.
  if [ "$created_epoch" -ge "$grace_cutoff" ]; then
    kept=$((kept + 1))
    continue
  fi

  # Orphan classification:
  #   - status == "open" (never claimed) -> orphan past grace.
  #   - status == "in_progress" with no live arbiter session -> abandoned.
  #   - a live arbiter session exits the script before this loop.
  #     (Singleton: at most one wisp can be legitimately in flight, and
  #     it's the in_progress one. Any other in_progress wisp older than
  #     grace is from a prior session that drained mid-loop.)
  reap_reason=""
  if [ "$wisp_status" = "open" ]; then
    reap_reason="never-claimed (status=open past grace)"
  elif [ "$wisp_status" = "in_progress" ]; then
    if [ "$arbiter_session_state" != "active" ]; then
      reap_reason="claimer-abandoned (status=in_progress, no active arbiter session)"
    else
      # Multiple in_progress wisps with active session: keep the most
      # recent (highest created_at, processed first in the sorted loop)
      # and reap the rest as session-drained-mid-loop residue.
      if [ "$kept" -eq 0 ]; then
        kept=$((kept + 1))
        continue
      fi
      reap_reason="stale-in-progress (older in_progress wisp; newer one is in flight)"
    fi
  else
    reap_reason="unexpected-status=$wisp_status"
  fi

  if [ "$echoed_header" -eq 0 ]; then
    echo "landing-arbiter-wisp-reap: orphans found, reaping"
    echoed_header=1
  fi

  # Re-check status right before the burn to avoid racing the arbiter's
  # own claim. If the wisp transitioned in the last few milliseconds, skip it.
  current_status=$(gc --rig "$RIG" bd show "$wisp_id" --json 2>/dev/null | jq -r '.[0].status // empty')
  if [ "$current_status" != "$wisp_status" ]; then
    echo "  $wisp_id: status changed ($wisp_status → $current_status) since list; skipping"
    kept=$((kept + 1))
    continue
  fi

  if gc --rig "$RIG" bd mol burn "$wisp_id" --force >/dev/null 2>&1; then
    echo "  $wisp_id (age=$((now_epoch - created_epoch))s, $wisp_status): burned - $reap_reason"
    reaped=$((reaped + 1))
  else
    echo "  $wisp_id: BURN FAILED (non-fatal); continuing" >&2
  fi
done < <(echo "$open_wisps_json" \
  | jq -r 'sort_by(.created_at) | reverse | .[] | [.id, .created_at, .status] | @tsv')

if [ "$reaped" -gt 0 ]; then
  echo "landing-arbiter-wisp-reap: reaped=$reaped kept=$kept total=$wisp_count"
fi

exit 0
