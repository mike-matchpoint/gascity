#!/bin/sh

# Shared lease for Dolt maintenance operations that should not overlap on the
# same managed city. Call enter_dolt_maintenance_lease OPERATION SECONDS and
# the lease is released automatically on process exit.

: "${GC_CITY_PATH:?GC_CITY_PATH must be set}"

DOLT_MAINTENANCE_DIR="${GC_DOLT_MAINTENANCE_DIR:-${GC_CITY_RUNTIME_DIR:-$GC_CITY_PATH/.gc/runtime}/maintenance}"
DOLT_MAINTENANCE_LOCK_FILE="$DOLT_MAINTENANCE_DIR/dolt-maintenance.lock"
DOLT_MAINTENANCE_LEASE_FILE="$DOLT_MAINTENANCE_DIR/dolt-maintenance-lease.json"
DOLT_MAINTENANCE_LOCK_DIR=""
DOLT_MAINTENANCE_LOCK_FD_OPEN=""
DOLT_MAINTENANCE_LEASE_HELD=""

dolt_maintenance_json_escape() {
    printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

dolt_maintenance_epoch() {
    date -u +%s
}

dolt_maintenance_iso_from_epoch() {
    epoch="$1"
    if command -v python3 >/dev/null 2>&1; then
        python3 - "$epoch" <<'PY'
from datetime import datetime, timezone
import sys

print(datetime.fromtimestamp(int(sys.argv[1]), timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"))
PY
        return $?
    fi
    date -u -r "$epoch" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null && return 0
    date -u -d "@$epoch" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null && return 0
    date -u +%Y-%m-%dT%H:%M:%SZ
}

dolt_maintenance_read_json_number() (
    path="$1"
    key="$2"
    [ -f "$path" ] || return 0
    sed -n "s/.*\"$key\"[[:space:]]*:[[:space:]]*\\([0-9][0-9]*\\).*/\\1/p" "$path" 2>/dev/null | head -1 || true
)

dolt_maintenance_read_json_string() (
    path="$1"
    key="$2"
    [ -f "$path" ] || return 0
    sed -n "s/.*\"$key\"[[:space:]]*:[[:space:]]*\"\\([^\"]*\\)\".*/\\1/p" "$path" 2>/dev/null | head -1 || true
)

dolt_maintenance_pid_is_running() (
    pid="$1"
    case "$pid" in
        ''|*[!0-9]*)
            return 1
            ;;
    esac
    kill -0 "$pid" 2>/dev/null
)

dolt_maintenance_lock() {
    mkdir -p "$DOLT_MAINTENANCE_DIR"
    if command -v flock >/dev/null 2>&1; then
        exec 9>"$DOLT_MAINTENANCE_LOCK_FILE"
        if ! flock -n 9; then
            echo "maintenance: another Dolt maintenance operation holds $DOLT_MAINTENANCE_LOCK_FILE" >&2
            return 75
        fi
        DOLT_MAINTENANCE_LOCK_FD_OPEN=1
        return 0
    fi

    DOLT_MAINTENANCE_LOCK_DIR="$DOLT_MAINTENANCE_LOCK_FILE.dir"
    if ! mkdir "$DOLT_MAINTENANCE_LOCK_DIR" 2>/dev/null; then
        echo "maintenance: another Dolt maintenance operation holds $DOLT_MAINTENANCE_LOCK_DIR" >&2
        return 75
    fi
}

dolt_maintenance_unlock() {
    if [ -n "$DOLT_MAINTENANCE_LOCK_FD_OPEN" ]; then
        flock -u 9 2>/dev/null || true
        exec 9>&- 2>/dev/null || true
        DOLT_MAINTENANCE_LOCK_FD_OPEN=""
    fi
    if [ -n "$DOLT_MAINTENANCE_LOCK_DIR" ]; then
        rmdir "$DOLT_MAINTENANCE_LOCK_DIR" 2>/dev/null || true
        DOLT_MAINTENANCE_LOCK_DIR=""
    fi
}

enter_dolt_maintenance_lease() {
    operation="${1:?maintenance operation required}"
    deadline_seconds="${2:-900}"
    case "$deadline_seconds" in
        ''|*[!0-9]*)
            echo "maintenance: invalid lease deadline: $deadline_seconds" >&2
            return 64
            ;;
    esac

    if ! dolt_maintenance_lock; then
        return $?
    fi

    now_epoch=$(dolt_maintenance_epoch)
    if [ -f "$DOLT_MAINTENANCE_LEASE_FILE" ]; then
        existing_expires=$(dolt_maintenance_read_json_number "$DOLT_MAINTENANCE_LEASE_FILE" expires_at_epoch)
        existing_pid=$(dolt_maintenance_read_json_number "$DOLT_MAINTENANCE_LEASE_FILE" pid)
        existing_operation=$(dolt_maintenance_read_json_string "$DOLT_MAINTENANCE_LEASE_FILE" operation)
        if [ -n "$existing_expires" ] && [ "$existing_expires" -gt "$now_epoch" ] && dolt_maintenance_pid_is_running "$existing_pid"; then
            echo "maintenance: active Dolt maintenance lease for ${existing_operation:-unknown} pid ${existing_pid:-unknown} expires at epoch $existing_expires" >&2
            dolt_maintenance_unlock
            return 75
        fi
        echo "maintenance: replacing stale Dolt maintenance lease for ${existing_operation:-unknown}" >&2
    fi

    owner="${GC_DOLT_MAINTENANCE_OWNER:-${GC_SESSION_NAME:-${GC_AGENT:-${USER:-unknown}}}}"
    started_at=$(dolt_maintenance_iso_from_epoch "$now_epoch")
    expires_epoch=$((now_epoch + deadline_seconds))
    expires_at=$(dolt_maintenance_iso_from_epoch "$expires_epoch")
    tmp="$DOLT_MAINTENANCE_LEASE_FILE.tmp.$$"

    cat > "$tmp" <<EOF
{
  "owner": "$(dolt_maintenance_json_escape "$owner")",
  "pid": $$,
  "operation": "$(dolt_maintenance_json_escape "$operation")",
  "city_path": "$(dolt_maintenance_json_escape "$GC_CITY_PATH")",
  "started_at": "$started_at",
  "expires_at": "$expires_at",
  "expires_at_epoch": $expires_epoch,
  "deadline_seconds": $deadline_seconds
}
EOF
    if ! mv -f "$tmp" "$DOLT_MAINTENANCE_LEASE_FILE"; then
        rm -f "$tmp"
        dolt_maintenance_unlock
        return 1
    fi
    DOLT_MAINTENANCE_LEASE_HELD=1
    trap 'release_dolt_maintenance_lease' EXIT
    trap 'release_dolt_maintenance_lease; exit 130' HUP INT TERM
}

release_dolt_maintenance_lease() {
    if [ -n "$DOLT_MAINTENANCE_LEASE_HELD" ]; then
        rm -f "$DOLT_MAINTENANCE_LEASE_FILE" 2>/dev/null || true
        DOLT_MAINTENANCE_LEASE_HELD=""
    fi
    dolt_maintenance_unlock
}
