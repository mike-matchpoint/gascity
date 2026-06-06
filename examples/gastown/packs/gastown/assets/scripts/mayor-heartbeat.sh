#!/usr/bin/env bash
# mayor-heartbeat — periodic FULL wedge-sweep wake for the city mayor.
#
# Fired by the gastown pack mayor-heartbeat order on a 30m cooldown (controller exec
# order: no LLM, no formula, no wisp). Every tick it takes a cheap mechanical
# pulse of the city (for the log + to hand the mayor a snapshot), then
# UNCONDITIONALLY nudges the mayor to run a full all-agent wedge scan and
# resolve whatever it finds.
#
# WHY UNCONDITIONAL (changed 2026-05-29): this used to wake the mayor only on
# unread mail or Dolt-down and log "ok" on clean ticks WITHOUT waking it. That
# missed SILENT wedges that raise neither signal — e.g. an abandoned
# cartographer planning molecule that gates spec-cartographer-watch and quietly
# starves ALL downstream work-order planning (no mail, no infra fault, just a
# stalled pipeline). Per operator decision the heartbeat now wakes the mayor on
# EVERY tick for a comprehensive sweep: robustness over context frugality. The
# probes below only enrich the nudge + log; the authoritative per-agent wedge
# detection is the mayor's sweep, guided by the checklist in the nudge.
#
# The script mutates no city state; the mayor (authorized per the
# mayor-autonomy directive) does any investigating and resolving.
#
# LIMITATION: a controller-fired order cannot detect controller death — the
# controller is what fires it. Controller liveness is the machine-level
# supervisor's responsibility, not this heartbeat's.
#
# PACK SCOPE: the paired order is declared with scope="city" so it fires
# exactly once even when the gastown pack is imported at both city and rig
# scopes. Do not remove that city scope unless you intend per-rig mayor nudges.
#
# -e is intentionally OFF: a heartbeat must always reach its nudge + log even if
# one probe fails, so every external call is guarded individually instead.
set -uo pipefail

if ! command -v jq >/dev/null 2>&1; then
    echo "mayor-heartbeat: jq is required but not found in PATH" >&2
    exit 1
fi

TS="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

# --- Resolve a state/log dir. Co-locate with city runtime when the controller
#     provides it; otherwise fall back through cwd (=city root for exec orders)
#     to /tmp so the heartbeat never fails just to find somewhere to log. ---
if [ -n "${MAYOR_HEARTBEAT_STATE_DIR:-}" ]; then
    STATE_DIR="$MAYOR_HEARTBEAT_STATE_DIR"
elif [ -n "${GC_CITY_RUNTIME_DIR:-}" ]; then
    STATE_DIR="$GC_CITY_RUNTIME_DIR/mayor-heartbeat"
elif [ -n "${GC_CITY:-}" ]; then
    STATE_DIR="$GC_CITY/.gc/runtime/mayor-heartbeat"
elif [ -d ".gc/runtime" ]; then
    STATE_DIR=".gc/runtime/mayor-heartbeat"
else
    STATE_DIR="/tmp/mayor-heartbeat"
fi
mkdir -p "$STATE_DIR" 2>/dev/null || STATE_DIR="/tmp/mayor-heartbeat"
mkdir -p "$STATE_DIR" 2>/dev/null || true
LOG="$STATE_DIR/heartbeat.log"

# --- Cheap city pulse (all guarded, non-fatal): a snapshot to hand the mayor
#     and to leave in the log. This is NOT the authoritative wedge check — the
#     mayor's sweep is. ---
MAIL_JSON="$(gc mail count mayor --json 2>/dev/null || echo '{}')"
UNREAD="$(printf '%s' "$MAIL_JSON" | jq -r '.unread // 0' 2>/dev/null || echo 0)"
case "$UNREAD" in ''|*[!0-9]*) UNREAD=0 ;; esac

DOLT_JSON="$(gc dolt health --json 2>/dev/null || echo '{}')"
DOLT_REACHABLE="$(printf '%s' "$DOLT_JSON" | jq -r '.server.reachable // false' 2>/dev/null || echo false)"
DOLT_RUNNING="$(printf '%s' "$DOLT_JSON" | jq -r '.server.running // false' 2>/dev/null || echo false)"
DOLT_LAT="$(printf '%s' "$DOLT_JSON" | jq -r '.server.latency_ms // "?"' 2>/dev/null || echo '?')"
if [ "$DOLT_REACHABLE" = "true" ] && [ "$DOLT_RUNNING" = "true" ]; then
    DOLT_TAG="ok"
else
    DOLT_TAG="DOWN"
fi

PULSE="unread=$UNREAD dolt=$DOLT_TAG latency_ms=$DOLT_LAT"

# --- The standing all-agent wedge-sweep checklist. Kept GENERIC (no WO/bead/
#     date specifics) — detailed resolve recipes live in
#     runbooks/observed-system-bugs.md and mayor memory. ---
read -r -d '' CHECKLIST <<'EOF' || true
run a FULL city wedge scan across ALL agents and RESOLVE what you find (you have mayor autonomy to act; validate every finding before acting). Task subagents to help you work in parallel and close them out afterwards. Walk the whole pipeline:
- Controller: are orders firing? `gc order history` newest row should be seconds-to-minutes old; a multi-minute gap = dispatch/controller stall.
- Cartographer: an open spec-cartographer molecule that is STALE (old updated_at, no active cartographer session) gates spec-cartographer-watch and silently starves WO planning. Check it, and check specs/agent-work-orders/ for files with no `source:work-order:<id>` bead. If the molecule's emit is verified complete, close ONLY the scaffold (open steps + molecule) — NEVER the emitted convoy/tasks.
- HOLDING release: scan open HOLDING placeholder beads and their dependents. If every dependent task has already been fully implemented or safely retargeted to concrete implementation beads, verify/repair the real dependency edges first, then close the HOLDING so downstream work can unblock. If any dependent is still partial or ambiguous, leave the HOLDING open and record the blocker.
- Debugger: same stale-plan-molecule check; bugs accumulating un-investigated; debugger-watch firing but not slinging. Identify open bugs with no dependencies not being actioned and ensure they are actioned.
- Refinery: merge queue backed up while the refinery session sits idle/frozen; stuck landing-failure arbitration.
- Polecat dispatch: ready polecat-routed type=task beads with 0 polecats while dispatch ticks fire = spawn stall (ready convoys/bugs/steps with 0 polecats is NORMAL — only type=task routed to gastown.polecat counts).
- Sessions: any registered 'active' but actually frozen (peek the tmux pane — the registry alone lies); boot wedged on API-400/529.
- Orphaned work: beads stuck in_progress with no live owner session.
Drain your inbox (gc mail inbox / read / archive). Record anomalies AND resolutions in runbooks/observed-system-bugs.md following the AGENTS.md protocol for hosted city source edits; consult that runbook + your memory for the resolve recipes. If the sweep finds the city genuinely healthy, log it and stand down — do not invent work.
EOF

SWEEP="HEARTBEAT wedge-sweep ($PULSE) — $CHECKLIST"

# --- ALWAYS wake the mayor (default delivery: queues / waits for a safe
#     boundary, so a 30m tick never interrupts the mayor mid-turn). ---
gc session nudge mayor/ "$SWEEP" 2>/dev/null || true
ACTION="wedge-sweep-nudge"

# --- Always leave an observable trail: one line to the log + a stdout summary
#     captured in `gc order history`. ---
echo "$TS $PULSE action=\"$ACTION\"" >> "$LOG" 2>/dev/null || true
echo "mayor-heartbeat: $PULSE action=$ACTION"

# --- Keep the log bounded (a tick every 30m = ~48 lines/day). ---
if [ -f "$LOG" ]; then
    LINES="$(wc -l < "$LOG" 2>/dev/null || echo 0)"
    case "$LINES" in ''|*[!0-9]*) LINES=0 ;; esac
    if [ "$LINES" -gt 500 ]; then
        tail -n 400 "$LOG" > "$LOG.tmp" 2>/dev/null && mv "$LOG.tmp" "$LOG" 2>/dev/null || true
    fi
fi
