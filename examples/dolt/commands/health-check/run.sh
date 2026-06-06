#!/bin/sh
# gc dolt health-check — Parse `gc dolt health --json` for order outcomes.
#
# Reads a health JSON report from stdin, echoes it to stdout for diagnostics,
# and exits nonzero with a concise stderr message for critical data-plane
# failures. This lets the generic order runner record `order.failed` with a
# useful message without making `gc dolt health --json` itself fail before
# programmatic consumers can parse the report.
set -e

compact_result=false
while [ $# -gt 0 ]; do
  case "$1" in
    --compact-result)
      compact_result=true
      shift
      ;;
    *)
      echo "gc dolt health-check: unknown flag: $1" >&2
      exit 2
      ;;
  esac
done

report=$(cat)
printf '%s\n' "$report"

if [ "$compact_result" = true ]; then
  if printf '%s\n' "$report" | grep -q '^compact: not_applicable reason='; then
    reason=$(printf '%s\n' "$report" | sed -n 's/^compact: not_applicable reason=//p' | head -1)
    echo "Dolt compact not applicable: reason=${reason:-unknown}" >&2
    exit 1
  fi
  exit 0
fi

json_field() {
  field="$1"
  if command -v jq >/dev/null 2>&1; then
    printf '%s\n' "$report" | jq -r "if $field == null then \"\" else $field end" 2>/dev/null || true
    return
  fi
  key=$(printf '%s' "$field" | sed 's/^\.server\.//')
  printf '%s\n' "$report" \
    | sed -n "/\"server\"[[:space:]]*:/,/}/p" \
    | sed -n "s/.*\"$key\"[[:space:]]*:[[:space:]]*\\([^,}]*\\).*/\\1/p" \
    | head -1 \
    | tr -d ' "'
}

reachable=$(json_field ".server.reachable")
running=$(json_field ".server.running")
pid=$(json_field ".server.pid")
port=$(json_field ".server.port")
latency=$(json_field ".server.latency_ms")
ping_latency=$(json_field ".server.ping_latency_ms")
[ -n "$ping_latency" ] || ping_latency="$latency"

fail_threshold() {
  echo "$1" >&2
  exit 1
}

truthy() {
  case "${1:-}" in
    1|true|TRUE|yes|YES|on|ON) return 0 ;;
  esac
  return 1
}

is_nonnegative_int() {
  case "${1:-}" in
    ''|*[!0-9]*)
      return 1
      ;;
  esac
  return 0
}

threshold_value() {
  name="$1"
  value="$2"
  [ -n "$value" ] || return 0
  if ! is_nonnegative_int "$value"; then
    fail_threshold "Dolt health threshold $name must be a non-negative integer, got $value"
  fi
  printf '%s\n' "$value"
}

check_max_int() {
  label="$1"
  value="$2"
  max="$3"
  [ -n "$max" ] || return 0
  [ "$max" != "0" ] || return 0
  is_nonnegative_int "$value" || return 0
  is_nonnegative_int "$max" || return 0
  if [ "$value" -gt "$max" ]; then
    fail_threshold "$label ${value}ms exceeded threshold ${max}ms"
  fi
}

case "$reachable" in
  true)
    max_select_one_ms=$(threshold_value GC_DOLT_HEALTHCHECK_MAX_SELECT_ONE_MS "${GC_DOLT_HEALTHCHECK_MAX_SELECT_ONE_MS:-${GC_DOLT_HEALTHCHECK_MAX_PING_MS:-}}")
    check_max_int "Dolt SELECT 1 latency" "$ping_latency" "$max_select_one_ms"

    max_real_query_ms=$(threshold_value GC_DOLT_HEALTHCHECK_MAX_REAL_QUERY_MS "${GC_DOLT_HEALTHCHECK_MAX_REAL_QUERY_MS:-}")
    max_commits=$(threshold_value GC_DOLT_HEALTHCHECK_MAX_COMMITS "${GC_DOLT_HEALTHCHECK_MAX_COMMITS:-}")
    max_noms="${GC_DOLT_HEALTHCHECK_MAX_NOMS_BYTES:-}"
    max_noms=$(threshold_value GC_DOLT_HEALTHCHECK_MAX_NOMS_BYTES "$max_noms")

    needs_jq=false
    if [ -n "$max_real_query_ms" ] || [ -n "$max_commits" ] || [ -n "$max_noms" ] || truthy "${GC_DOLT_HEALTH_REAL_QUERY:-}"; then
      needs_jq=true
    fi
    if [ "$needs_jq" = true ] && ! command -v jq >/dev/null 2>&1; then
      fail_threshold "Dolt health threshold checks require jq"
    fi

    if command -v jq >/dev/null 2>&1; then
      real_enabled=$(printf '%s\n' "$report" | jq -r '.real_query.enabled // false' 2>/dev/null || echo false)
      if [ "$real_enabled" = "true" ]; then
        real_ok=$(printf '%s\n' "$report" | jq -r '.real_query.ok // false' 2>/dev/null || echo false)
        real_timeout=$(printf '%s\n' "$report" | jq -r '.real_query.timeout // false' 2>/dev/null || echo false)
        real_exit=$(printf '%s\n' "$report" | jq -r '.real_query.exit_code // 0' 2>/dev/null || echo 0)
        real_latency=$(printf '%s\n' "$report" | jq -r '.real_query.latency_ms // 0' 2>/dev/null || echo 0)
        if [ "$real_ok" != "true" ]; then
          fail_threshold "Dolt representative query failed: timeout=${real_timeout:-unknown} exit_code=${real_exit:-unknown}"
        fi
        check_max_int "Dolt representative query latency" "$real_latency" "$max_real_query_ms"
      fi

      if [ -n "$max_commits" ] && [ "$max_commits" != "0" ]; then
        bad_commits=$(printf '%s\n' "$report" | jq -r --argjson max "$max_commits" '
          [.databases[]? | select((.commits // 0) > $max) | "\(.name)=\(.commits)"] | first // ""
        ' 2>/dev/null || true)
        [ -z "$bad_commits" ] || fail_threshold "Dolt commit count exceeded threshold: $bad_commits > $max_commits"
      fi

      if [ -n "$max_noms" ] && [ "$max_noms" != "0" ]; then
        bad_noms=$(printf '%s\n' "$report" | jq -r --argjson max "$max_noms" '
          [.databases[]? | select((.noms_bytes // null) != null and .noms_bytes > $max) | "\(.name)=\(.noms_bytes)"] | first // ""
        ' 2>/dev/null || true)
        [ -z "$bad_noms" ] || fail_threshold "Dolt database noms bytes exceeded threshold: $bad_noms > $max_noms"
        noms_total=$(printf '%s\n' "$report" | jq -r '.storage.noms_bytes_total // 0' 2>/dev/null || true)
        if is_nonnegative_int "$noms_total" && [ "$noms_total" -gt "$max_noms" ]; then
          fail_threshold "Dolt total noms bytes $noms_total exceeded threshold $max_noms"
        fi
      fi
    fi
    exit 0
    ;;
  false)
    echo "Dolt server unreachable: running=${running:-unknown} pid=${pid:-0} port=${port:-unknown} latency_ms=${latency:-0}" >&2
    exit 1
    ;;
  *)
    echo "Dolt health report missing server.reachable" >&2
    exit 1
    ;;
esac
