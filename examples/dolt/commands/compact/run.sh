#!/bin/sh
# gc dolt compact — flatten Dolt commit history on managed databases.
#
# Why this exists: every bead mutation creates a Dolt commit. Over time
# this builds an enormous commit graph (thousands of commits/day on busy
# cities). The commit graph IS the storage cost — DOLT_GC alone cannot
# reclaim space when all commits are live history. Flattening squashes
# the graph into a single commit and lets the next DOLT_GC reclaim
# orphaned chunks.
#
# This command replaces the formula-based mol-dog-compactor that was
# routed to the dog pool. Per the formula's own ZFC-exemption notice,
# compaction requires SQL access (database/sql) that agents don't have.
# Running as an exec order gives us direct SQL access via the dolt CLI.
#
# Algorithm (flatten mode):
#   1. Pre-flight: record row counts and value hashes for all user tables and
#      require HEAD to remain stable across a bounded retry loop.
#   2. Soft-reset to the root commit; all data stays staged.
#   3. Commit everything as a single "compaction: flatten history" commit.
#   4. Re-check post-flatten row counts, table value hashes, and database
#      value hash. Row-count decreases fail before full GC. Row-count
#      increases with stable value hashes can continue to full GC. Pure
#      row-count increase plus value-hash drift is the ambiguous concurrent-
#      writer case and is deferred to pending GC instead of quarantined. Any
#      table-list drift, probe failure, same-count value-hash drift, or
#      row-count decrease is quarantined before full GC.
#   4a. Local-verify HEAD-stability gate. The pre-flight stability loop cannot
#      close the residual window between its final HEAD check and the flatten,
#      nor the window during post-flatten verify, so a normal MVCC writer (the
#      beads/mail workload) can still commit inside the flatten window. That
#      legitimately adds rows and shifts value hashes versus the snapshot, which
#      otherwise looks identical to the ambiguous gain+drift corruption signal.
#      Quarantining that false positive blocks all future GC of the db and
#      starves DOLT_GC until host memory is exhausted. So, mirroring the remote-
#      push path's HEAD-stability defer, pure gain+drift is downgraded from a
#      blocking quarantine to a skip-and-retry-next-run. A writer is proven (and
#      distinguished from the flatten's OWN commit) when either HEAD captured
#      immediately before the mutating reset differs from the stable pre-flight
#      HEAD (a writer landed in the preflight->reset window, before the flatten
#      committed), or HEAD captured after verify moved past the flatten's own
#      commit (a writer landed during/after verify). The residual probe->reset
#      window cannot be observed after the fact, so pure gain+drift defers even
#      when HEAD is stable. All stricter failure categories still quarantine.
#   5. Run CALL DOLT_GC('--full') to reclaim chunks orphaned by the flatten.
#
# Remote push failures are recorded in compact-pending-push markers and do not
# fail local compaction. Later runs retry those markers before threshold skips,
# and unverified remote heads must become ancestry-verifiable before push.
# Surgical mode (preserve recent N commits via interactive rebase) is
# intentionally not implemented; flatten is sufficient for bloat recovery
# and avoids the rebase-vs-concurrent-write hazards.
#
# Runs from the dolt pack's mol-dog-compactor order.
#
# Environment:
#   GC_CITY_PATH                          (required) — city root
#   GC_DOLT_PORT                          (required) — managed dolt port
#   GC_DOLT_HOST                          (default: 127.0.0.1)
#   GC_DOLT_USER                          (default: root)
#   GC_DOLT_PASSWORD                      (optional)
#   GC_DOLT_COMPACT_THRESHOLD_COMMITS
#     (default: 2000) — skip databases with fewer commits than this.
#   GC_DOLT_COMPACT_CALL_TIMEOUT_SECS
#     (default: 1800) — wall-clock bound for each SQL CALL.
#   GC_DOLT_COMPACT_PUSH_TIMEOUT_SECS
#     (default: 120) — wall-clock bound for remote compare-and-push
#                     after local compaction. Push failures are recorded for
#                     repair but do not fail local compaction.
#   GC_DOLT_COMPACT_PENDING_PUSH_MAX_AGE_SECS
#     (default: 172800) — maximum age for automatic pending remote-push retry.
#                       Older markers require manual review before push.
#   GC_DOLT_COMPACT_REMOTE               (optional) — remote to fetch/push.
#                                         Defaults to origin when present;
#                                         ambiguous multi-remote stores fail.
#   GC_DOLT_COMPACT_DRY_RUN              (optional) — when set, prints
#                                         what would happen but does not
#                                         execute any DOLT_RESET / COMMIT.
#   GC_DOLT_COMPACT_ONLY_DBS              (optional) — comma-separated list of
#                                         database names to compact. When set,
#                                         all other databases are skipped.
#   GC_DOLT_COMPACT_REQUIRE_APPLICABLE    (optional) — when set to 1, fail
#                                         with exit 2 when this command cannot
#                                         safely apply to the current Dolt
#                                         runtime. Default keeps historical
#                                         skip-and-exit-0 behavior.
#   GC_DOLT_COMPACT_RETENTION_OLDER_THAN  (hosted only, default: 48h) — closed
#                                         operational-churn retention window
#                                         applied before server compaction.
#   GC_DOLT_COMPACT_RETENTION_MAX_DELETE  (hosted only, default: 200000) —
#                                         safety cap for retention candidates.
#   GC_DOLT_COMPACT_RETENTION_DB          (hosted only, default: hq) —
#                                         database that owns bead retention.
#   GC_DOLT_COMPACT_DRAIN_TIMEOUT_MS      (hosted only, default: 120000) —
#                                         server maintenance write-drain bound.
#   GC_DOLT_COMPACT_MAX_DURATION_MS       (hosted only, default: 3600000) —
#                                         server maintenance lease bound.
#   GC_DOLT_REFSPEC_<DB_UPPER>            (optional) — compact remote push
#                                         refspec in <local>:<remote> form.
#                                         DB name is uppercased with '-'
#                                         replaced by '_' to derive the env
#                                         key; DB names that differ only by
#                                         '-' vs '_' share that key.
set -eu

: "${GC_CITY_PATH:?GC_CITY_PATH must be set}"
: "${GC_DOLT_PORT:=}"
gc_dolt_port_input="$GC_DOLT_PORT"
gc_dolt_host_input="${GC_DOLT_HOST:-}"

PACK_DIR="${GC_PACK_DIR:-$(unset CDPATH; cd -- "$(dirname "$0")/.." && pwd)}"
# shellcheck disable=SC1091
. "$PACK_DIR/assets/scripts/runtime.sh"

compact_not_applicable() {
  reason="$1"
  case "${GC_DOLT_COMPACT_REQUIRE_APPLICABLE:-0}" in
    1|true|TRUE|yes|YES)
      printf 'compact: not_applicable reason=%s\n' "$reason" >&2
      exit 2
      ;;
    *)
      printf 'compact: not_applicable reason=%s\n' "$reason"
      exit 0
      ;;
  esac
}

is_local_dolt_host() {
  case "${1:-}" in
    ''|127.0.0.1|localhost|0.0.0.0|::1|::|'[::1]'|'[::]')
      return 0
      ;;
    127.*)
      return 0
      ;;
  esac
  return 1
}

hosted_server_compact=0

case "${GC_DOLT_MANAGED_LOCAL:-}" in
  0|false|FALSE|no|NO)
    if [ -n "$gc_dolt_port_input" ] && ! is_local_dolt_host "$gc_dolt_host_input"; then
      hosted_server_compact=1
      GC_DOLT_PORT="$gc_dolt_port_input"
    else
      compact_not_applicable "managed_local_false"
    fi
    ;;
esac

if [ "$hosted_server_compact" = "1" ]; then
  :
elif [ "${GC_DOLT_MANAGED_LOCAL:-}" = "1" ]; then
  managed_port=$(managed_runtime_port "$DOLT_STATE_FILE" "$DOLT_DATA_DIR" || true)
  if [ -n "$managed_port" ]; then
    if [ -n "$gc_dolt_port_input" ] && [ "$gc_dolt_port_input" != "$managed_port" ]; then
      compact_not_applicable "port_mismatch"
    fi
    GC_DOLT_PORT="$managed_port"
  elif [ -z "$gc_dolt_port_input" ]; then
    compact_not_applicable "managed_runtime_inactive"
  else
    GC_DOLT_PORT="$gc_dolt_port_input"
  fi
elif [ -n "$gc_dolt_port_input" ]; then
  if ! is_local_dolt_host "$gc_dolt_host_input"; then
    hosted_server_compact=1
    GC_DOLT_PORT="$gc_dolt_port_input"
  else
  managed_port=$(managed_runtime_port "$DOLT_STATE_FILE" "$DOLT_DATA_DIR" || true)
  if [ -z "$managed_port" ] || [ "$gc_dolt_port_input" != "$managed_port" ]; then
    compact_not_applicable "port_mismatch"
  fi
  GC_DOLT_PORT="$managed_port"
  fi
elif [ -z "$gc_dolt_port_input" ]; then
  managed_port=$(managed_runtime_port "$DOLT_STATE_FILE" "$DOLT_DATA_DIR" || true)
  if [ -z "$managed_port" ]; then
    compact_not_applicable "managed_runtime_inactive"
  fi
  GC_DOLT_PORT="$managed_port"
fi

: "${GC_DOLT_PORT:?GC_DOLT_PORT must be set}"
: "${GC_DOLT_USER:=root}"

host="${GC_DOLT_HOST:-127.0.0.1}"
threshold_commits="${GC_DOLT_COMPACT_THRESHOLD_COMMITS:-2000}"
call_timeout="${GC_DOLT_COMPACT_CALL_TIMEOUT_SECS:-1800}"
push_timeout="${GC_DOLT_COMPACT_PUSH_TIMEOUT_SECS:-120}"
pending_push_max_age_secs="${GC_DOLT_COMPACT_PENDING_PUSH_MAX_AGE_SECS:-172800}"
compact_remote="${GC_DOLT_COMPACT_REMOTE:-}"
dry_run="${GC_DOLT_COMPACT_DRY_RUN:-}"
only_dbs="${GC_DOLT_COMPACT_ONLY_DBS:-}"
server_retention_older_than="${GC_DOLT_COMPACT_RETENTION_OLDER_THAN:-48h}"
server_retention_max_delete="${GC_DOLT_COMPACT_RETENTION_MAX_DELETE:-${GC_DOLT_RETENTION_SWEEP_MAX_DELETE:-200000}}"
server_drain_timeout_ms="${GC_DOLT_COMPACT_DRAIN_TIMEOUT_MS:-120000}"
server_max_duration_ms="${GC_DOLT_COMPACT_MAX_DURATION_MS:-3600000}"

case "$threshold_commits" in
  ''|*[!0-9]*)
    printf 'compact: invalid GC_DOLT_COMPACT_THRESHOLD_COMMITS=%s (must be a non-negative integer)\n' \
      "$threshold_commits" >&2
    exit 2
    ;;
esac

case "$call_timeout" in
  ''|*[!0-9]*|0)
    printf 'compact: invalid GC_DOLT_COMPACT_CALL_TIMEOUT_SECS=%s (must be a positive integer)\n' \
      "$call_timeout" >&2
    exit 2
    ;;
esac

case "$push_timeout" in
  ''|*[!0-9]*|0)
    printf 'compact: invalid GC_DOLT_COMPACT_PUSH_TIMEOUT_SECS=%s (must be a positive integer)\n' \
      "$push_timeout" >&2
    exit 2
    ;;
esac

case "$pending_push_max_age_secs" in
  ''|*[!0-9]*)
    printf 'compact: invalid GC_DOLT_COMPACT_PENDING_PUSH_MAX_AGE_SECS=%s (must be a non-negative integer)\n' \
      "$pending_push_max_age_secs" >&2
    exit 2
    ;;
esac

case "$server_retention_max_delete" in
  ''|*[!0-9]*)
    printf 'compact: invalid GC_DOLT_COMPACT_RETENTION_MAX_DELETE=%s (must be a non-negative integer)\n' \
      "$server_retention_max_delete" >&2
    exit 2
    ;;
esac

case "$server_drain_timeout_ms" in
  ''|*[!0-9]*|0)
    printf 'compact: invalid GC_DOLT_COMPACT_DRAIN_TIMEOUT_MS=%s (must be a positive integer)\n' \
      "$server_drain_timeout_ms" >&2
    exit 2
    ;;
esac

case "$server_max_duration_ms" in
  ''|*[!0-9]*|0)
    printf 'compact: invalid GC_DOLT_COMPACT_MAX_DURATION_MS=%s (must be a positive integer)\n' \
      "$server_max_duration_ms" >&2
    exit 2
    ;;
esac

case "$compact_remote" in
  ''|[A-Za-z0-9_.-]*)
    case "$compact_remote" in
      *[!A-Za-z0-9_.-]*)
        printf 'compact: invalid GC_DOLT_COMPACT_REMOTE=%s\n' "$compact_remote" >&2
        exit 2
        ;;
    esac
    ;;
  *)
    printf 'compact: invalid GC_DOLT_COMPACT_REMOTE=%s\n' "$compact_remote" >&2
    exit 2
    ;;
esac

# Cross-city flock keyed on host:port so concurrent compactions on the
# same Dolt server don't interleave. Compaction holds open transactions
# and a second compactor running concurrently would race on the
# graph-rewrite step.
lock_host=$(printf '%s' "$host" | tr '[:upper:]' '[:lower:]' | sed 's/^\[\(.*\)\]$/\1/')
case "$lock_host" in
  ''|127.0.0.1|localhost|0.0.0.0|::1|::)
    lock_host="127.0.0.1"
    ;;
esac
lock_key=$(printf '%s-%s' "$lock_host" "$GC_DOLT_PORT" | tr -c 'A-Za-z0-9_.-' '-')
lock_root="/tmp/gc-dolt-compact"
old_umask=$(umask)
umask 077
mkdir -p "$lock_root" || {
  umask "$old_umask"
  printf 'compact: unable to create lock directory %s\n' "$lock_root" >&2
  exit 1
}
umask "$old_umask"
chmod 700 "$lock_root" 2>/dev/null || {
  printf 'compact: unable to secure lock directory %s\n' "$lock_root" >&2
  exit 1
}
lock_path="$lock_root/${lock_key}.lock"
lock_dir="$lock_root/${lock_key}.dir"
lock_pid_path="$lock_dir/pid"
lock_cmd_path="$lock_dir/cmd"
pending_gc_dir="$PACK_STATE_DIR/compact-pending-gc"
pending_push_dir="$PACK_STATE_DIR/compact-pending-push"
quarantine_dir="$PACK_STATE_DIR/compact-quarantine"

# DB discovery uses rig metadata.json files first (authoritative), with a
# filesystem-scan fallback when gc itself is unavailable.
metadata_files() {
  printf '%s\n' "$GC_CITY_PATH/.beads/metadata.json"
  if command -v gc >/dev/null 2>&1; then
    if rig_json=$(run_bounded 5 gc rig list --json 2>/dev/null); then
      rig_paths=$(printf '%s\n' "$rig_json" \
        | if command -v jq >/dev/null 2>&1; then
            jq -r '.rigs[].path' 2>/dev/null
          else
            grep '"path"' | sed 's/.*"path": *"//;s/".*//'
          fi) || true
      if [ -n "$rig_paths" ]; then
        printf '%s\n' "$rig_paths" | while IFS= read -r p; do
          [ -n "$p" ] && printf '%s\n' "$p/.beads/metadata.json"
        done
        return
      fi
    else
      rig_status=$?
      if [ "$rig_status" -eq 124 ]; then
        printf 'compact: gc rig list timed out after 5s; falling back to local filesystem metadata scan\n' >&2
      else
        printf 'compact: gc rig list failed rc=%s; falling back to local filesystem metadata scan\n' "$rig_status" >&2
      fi
    fi
  fi
  find "$GC_CITY_PATH" \
    \( -path "$GC_CITY_PATH/.gc" -o -path "$GC_CITY_PATH/.git" \) -prune -o \
    -path '*/.beads/metadata.json' -print 2>/dev/null || true
}

metadata_db() {
  meta="$1"
  db=""
  if [ ! -f "$meta" ]; then
    printf '%s\n' "beads"
    return 0
  fi
  if command -v jq >/dev/null 2>&1; then
    db=$(jq -r '.dolt_database // empty' "$meta" 2>/dev/null || true)
  else
    db=$(grep -o '"dolt_database"[[:space:]]*:[[:space:]]*"[^"]*"' "$meta" 2>/dev/null \
      | sed 's/.*: *"//;s/"$//' || true)
  fi
  if [ -z "$db" ]; then
    db="beads"
  fi
  printf '%s\n' "$db"
}

valid_database_name() {
  name="$1"
  case "$name" in
    [A-Za-z0-9_]*)
      case "$name" in
        *[!A-Za-z0-9_-]*) return 1 ;;
        *) return 0 ;;
      esac
      ;;
    *) return 1 ;;
  esac
}

valid_table_name() {
  valid_database_name "$1"
}

valid_remote_name() {
  remote_candidate="$1"
  case "$remote_candidate" in
    [A-Za-z0-9_.-]*)
      case "$remote_candidate" in
        *[!A-Za-z0-9_.-]*) return 1 ;;
        *) return 0 ;;
      esac
      ;;
    *) return 1 ;;
  esac
}

valid_branch_name() {
  branch_candidate="$1"
  case "$branch_candidate" in
    -*|.*|*..*|*@{*) return 1 ;;
    [A-Za-z0-9_.-]*)
      case "$branch_candidate" in
        *[!A-Za-z0-9_./-]*) return 1 ;;
        *) return 0 ;;
      esac
      ;;
    *) return 1 ;;
  esac
}

refspec_env_value() {
  db="$1"
  valid_database_name "$db" || return 1
  key=$(printf '%s' "$db" | tr 'a-z-' 'A-Z_')
  case "$key" in
    *[!A-Z0-9_]*) return 0 ;;
  esac
  eval "printf '%s' \"\${GC_DOLT_REFSPEC_$key:-}\""
}

refspec_parts() {
  rs="$1"
  case "$rs" in
    *:*)
      local_branch=${rs%%:*}
      remote_branch=${rs#*:}
      ;;
    *)
      local_branch="$rs"
      remote_branch="$rs"
      ;;
  esac
  [ -z "$local_branch" ] && return 1
  [ -z "$remote_branch" ] && return 1
  valid_branch_name "$local_branch" || return 1
  valid_branch_name "$remote_branch" || return 1
  printf '%s\n%s\n' "$local_branch" "$remote_branch"
}

warn_refspec_fallback() {
  printf 'compact: db=%s WARN: active branch unresolved; falling back to main\n' "$1" >&2
}

is_system_database() {
  system_candidate=$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')
  case "$system_candidate" in
    information_schema|mysql|dolt_cluster|performance_schema|sys|__gc_probe) return 0 ;;
    *) return 1 ;;
  esac
}

emit_database_name() {
  db="$1"
  if ! valid_database_name "$db"; then
    printf 'compact: db=%s invalid database name — skip\n' "$db" >&2
    return 0
  fi
  if is_system_database "$db"; then
    printf 'compact: db=%s system database — skip\n' "$db" >&2
    return 0
  fi
  printf '%s\n' "$db"
}

discover_database_names() {
  while IFS= read -r meta; do
    [ -n "$meta" ] || continue
    db=$(metadata_db "$meta")
    emit_database_name "$db"
  done < "$_meta_tmp"

  if [ -d "$DOLT_DATA_DIR" ]; then
    for d in "$DOLT_DATA_DIR"/*/; do
      [ -d "$d/.dolt" ] || continue
      db=${d%/}
      db=${db##*/}
      is_system_database "$db" && continue
      emit_database_name "$db"
    done
  fi
}

# dolt_query — wrapper that runs a single SQL statement against the
# managed server with the configured port/host/user. Honors the
# per-call timeout. Output is the raw -r result-format-tsv body.
dolt_query() {
  db="$1"
  query="$2"
  export DOLT_CLI_PASSWORD="${GC_DOLT_PASSWORD:-}"
  run_bounded "$call_timeout" \
    dolt --no-tls --host "$host" --port "$GC_DOLT_PORT" \
    --user "$GC_DOLT_USER" \
    --use-db "$db" \
    sql -r tabular -q "$query"
}

dolt_query_global() {
  query="$1"
  export DOLT_CLI_PASSWORD="${GC_DOLT_PASSWORD:-}"
  run_bounded "$call_timeout" \
    dolt --no-tls --host "$host" --port "$GC_DOLT_PORT" \
    --user "$GC_DOLT_USER" \
    sql -r tabular -q "$query"
}

tabular_first_column_rows() {
  awk 'NR>=4 && /^\|/ {
    line=$0
    sub(/^\|[[:space:]]*/, "", line)
    sub(/[[:space:]]*\|.*$/, "", line)
    gsub(/^[[:space:]]+|[[:space:]]+$/, "", line)
    if (line != "") print line
  }'
}

discover_server_database_names() {
  out_tmp=$(mktemp)
  err_tmp=$(mktemp)
  if ! dolt_query_global "SHOW DATABASES" > "$out_tmp" 2>"$err_tmp"; then
    printf 'compact: hosted database discovery failed\n' >&2
    while IFS= read -r err_line; do
      printf 'compact: %s\n' "$err_line" >&2
    done < "$err_tmp"
    rm -f "$out_tmp" "$err_tmp"
    return 1
  fi
  tabular_first_column_rows < "$out_tmp" | while IFS= read -r db; do
    emit_database_name "$db"
  done
  rm -f "$out_tmp" "$err_tmp"
}

emit_error_file() {
  db="$1"
  err_file="$2"
  [ -s "$err_file" ] || return 0
  while IFS= read -r err_line; do
    printf 'compact: db=%s %s\n' "$db" "$err_line" >&2
  done < "$err_file"
}

query_single_cell() {
  db="$1"
  failure_message="$2"
  query="$3"
  out_tmp=$(mktemp)
  err_tmp=$(mktemp)
  if ! dolt_query "$db" "$query" > "$out_tmp" 2>"$err_tmp"; then
    printf 'compact: db=%s %s\n' "$db" "$failure_message" >&2
    emit_error_file "$db" "$err_tmp"
    rm -f "$out_tmp" "$err_tmp"
    return 1
  fi
  awk 'NR==4 {gsub(/[| ]/, ""); print; exit}' "$out_tmp"
  rm -f "$out_tmp" "$err_tmp"
}

resolve_refspec_sql() {
  db="$1"
  if ! valid_database_name "$db"; then
    printf 'compact: db=%s invalid database name — fail\n' "$db" >&2
    return 1
  fi

  active=$(query_single_cell "$db" "active branch probe failed" "SELECT active_branch()" 2>/dev/null || true)
  active_resolved=0
  if [ -n "$active" ] && valid_branch_name "$active"; then
    active_resolved=1
  fi

  override=$(refspec_env_value "$db") || return 1
  if [ -n "$override" ]; then
    parts=$(refspec_parts "$override") || {
      printf 'compact: db=%s invalid refspec override=%s\n' "$db" "$override" >&2
      return 1
    }
    local_branch=$(printf '%s\n' "$parts" | sed -n '1p')
    if [ "$active_resolved" != "1" ]; then
      printf 'compact: db=%s refspec override requires resolved active branch — fail\n' "$db" >&2
      return 1
    fi
    if [ "$local_branch" != "$active" ]; then
      printf 'compact: db=%s refspec override local branch=%s does not match active branch=%s — fail\n' \
        "$db" "$local_branch" "$active" >&2
      return 1
    fi
    printf '%s\n' "$parts"
    return 0
  fi

  if [ "$active_resolved" = "1" ]; then
    printf '%s\n%s\n' "$active" "$active"
    return 0
  fi

  warn_refspec_fallback "$db"
  printf 'main\nmain\n'
}

# commit_count — count of commits reachable from the current branch. Bounded scan
# (LIMIT 200000) so a runaway DB doesn't tie up the connection.
commit_count() {
  db="$1"
  query_single_cell "$db" "commit count probe failed" \
    "SELECT COUNT(*) FROM (SELECT 1 FROM dolt_log LIMIT 200000) AS t"
}

# root_commit — earliest commit hash on the current branch.
root_commit() {
  db="$1"
  query_single_cell "$db" "root commit probe failed" \
    "SELECT commit_hash FROM dolt_log ORDER BY date ASC LIMIT 1"
}

# head_commit — current branch HEAD hash before flattening.
head_commit() {
  db="$1"
  query_single_cell "$db" "HEAD commit probe failed" \
    "SELECT commit_hash FROM dolt_log ORDER BY date DESC LIMIT 1"
}

# user_tables — emit one user-table name per line (excludes dolt_*
# system tables and information_schema views).
user_tables() {
  db="$1"
  out_tmp=$(mktemp)
  err_tmp=$(mktemp)
  if ! dolt_query "$db" \
    "SELECT table_name FROM information_schema.tables WHERE table_schema = '$db' AND table_type = 'BASE TABLE' AND table_name NOT LIKE 'dolt\\_%' ESCAPE '\\\\' ORDER BY table_name" \
    > "$out_tmp" 2>"$err_tmp"; then
    printf 'compact: db=%s table list probe failed\n' "$db" >&2
    emit_error_file "$db" "$err_tmp"
    rm -f "$out_tmp" "$err_tmp"
    return 1
  fi
  awk 'NR>=4 && /^\|/ {gsub(/^\| | \|$/, ""); gsub(/ /, ""); if ($0 != "") print}' "$out_tmp"
  rm -f "$out_tmp" "$err_tmp"
}

# row_count — COUNT(*) for one table. Returns "" on error.
row_count() {
  db="$1"
  table="$2"
  query_single_cell "$db" "row count probe failed for table=$table" \
    "SELECT COUNT(*) FROM \`$table\`"
}

table_value_hash() {
  db="$1"
  table="$2"
  query_single_cell "$db" "table value hash probe failed for table=$table" \
    "SELECT DOLT_HASHOF_TABLE('$table')"
}

db_value_hash() {
  db="$1"
  query_single_cell "$db" "database value hash probe failed" \
    "SELECT DOLT_HASHOF_DB()"
}

remote_count() {
  db="$1"
  query_single_cell "$db" "remote count probe failed" \
    "SELECT COUNT(*) FROM dolt_remotes"
}

remote_exists() {
  db="$1"
  remote="$2"
  query_single_cell "$db" "remote existence probe failed" \
    "SELECT COUNT(*) FROM dolt_remotes WHERE name = '$remote'"
}

single_remote_name() {
  db="$1"
  query_single_cell "$db" "remote probe failed" \
    "SELECT name FROM dolt_remotes ORDER BY name LIMIT 1"
}

select_remote() {
  db="$1"

  if [ -n "$compact_remote" ]; then
    exists=$(remote_exists "$db" "$compact_remote") || return 1
    if [ "$exists" != "1" ]; then
      printf 'compact: db=%s configured remote=%s not found — fail\n' \
        "$db" "$compact_remote" >&2
      return 1
    fi
    printf '%s\n' "$compact_remote"
    return 0
  fi

  count=$(remote_count "$db") || return 1
  case "$count" in
    ''|*[!0-9]*)
      printf 'compact: db=%s remote count probe returned invalid value=%s\n' \
        "$db" "$count" >&2
      return 1
      ;;
  esac

  if [ "$count" -eq 0 ]; then
    printf '\n'
    return 0
  fi
  if [ "$count" -eq 1 ]; then
    single_remote_name "$db"
    return $?
  fi

  origin_exists=$(remote_exists "$db" "origin") || return 1
  if [ "$origin_exists" = "1" ]; then
    printf 'origin\n'
    return 0
  fi
  printf 'compact: db=%s multiple remotes found without origin; set GC_DOLT_COMPACT_REMOTE — fail\n' \
    "$db" >&2
  return 1
}

fetch_remote() {
  db="$1"
  remote="$2"
  dolt_query "$db" "CALL DOLT_FETCH('$remote')"
}

remote_branch_head() {
  db="$1"
  remote="$2"
  branch="$3"
  valid_branch_name "$branch" || return 1
  query_single_cell "$db" "remote HEAD probe failed" \
    "SELECT hash FROM dolt_remote_branches WHERE name = 'remotes/$remote/$branch'"
}

commit_exists_in_local_log() {
  db="$1"
  hash="$2"
  query_single_cell "$db" "remote ancestry probe failed" \
    "SELECT COUNT(*) FROM dolt_log WHERE commit_hash = '$hash'"
}

push_remote_refspec() {
  db="$1"
  remote="$2"
  local_branch="$3"
  remote_branch="$4"
  if [ "$local_branch" = "$remote_branch" ]; then
    refspec_arg="$local_branch"
  else
    refspec_arg="$local_branch:$remote_branch"
  fi
  export DOLT_CLI_PASSWORD="${GC_DOLT_PASSWORD:-}"
  run_bounded "$push_timeout" \
    dolt --no-tls --host "$host" --port "$GC_DOLT_PORT" \
    --user "$GC_DOLT_USER" \
    --use-db "$db" \
    sql -r tabular -q "CALL DOLT_PUSH('--force', '--set-upstream', '$remote', '$refspec_arg')"
}

# preflight_counts — write "<table> <count> <value-hash>" lines for all user tables.
preflight_counts() {
  db="$1"
  out="$2"
  tables_tmp=$(mktemp)
  : > "$out"
  if ! user_tables "$db" > "$tables_tmp"; then
    rm -f "$tables_tmp"
    return 1
  fi
  preflight_failed=0
  while IFS= read -r t; do
    [ -n "$t" ] || continue
    if ! valid_table_name "$t"; then
      printf 'compact: db=%s invalid table name from information_schema table=%s — fail\n' \
        "$db" "$t" >&2
      preflight_failed=1
      break
    fi
    if ! cnt=$(row_count "$db" "$t"); then
      printf 'compact: db=%s pre-flight row count failed for table=%s\n' "$db" "$t" >&2
      preflight_failed=1
      break
    fi
    case "$cnt" in
      ''|*[!0-9]*)
        printf 'compact: db=%s pre-flight row count failed for table=%s\n' "$db" "$t" >&2
        preflight_failed=1
        break
        ;;
    esac
    if ! table_hash=$(table_value_hash "$db" "$t"); then
      printf 'compact: db=%s pre-flight table value hash failed for table=%s\n' "$db" "$t" >&2
      preflight_failed=1
      break
    fi
    if [ -z "$table_hash" ]; then
      printf 'compact: db=%s pre-flight table value hash returned empty value for table=%s\n' "$db" "$t" >&2
      preflight_failed=1
      break
    fi
    printf '%s %s %s\n' "$t" "$cnt" "$table_hash" >> "$out"
  done < "$tables_tmp"
  rm -f "$tables_tmp"
  return "$preflight_failed"
}

# verify_counts — re-count/re-hash and compare against the pre-flight file.
# Row-count decreases fail. Row-count increases with stable hashes can continue.
# Row-count increases plus table hash drift are recorded as the ambiguous writer
# race case so the caller can defer to pending GC. Other hash drift, table-list
# drift, and probe failures quarantine before full GC. Sets category flags plus
# verify_counts_failure_reason and verify_counts_failure_guidance for callers.
verify_counts() {
  db="$1"
  preflight="$2"
  fail=0
  verify_counts_saw_gain=0
  verify_counts_saw_gain_hash_drift=0
  verify_counts_saw_row_decrease=0
  verify_counts_saw_same_count_hash_drift=0
  verify_counts_saw_table_list_change=0
  verify_counts_saw_probe_failure=0
  verify_counts_failure_reason=""
  verify_counts_failure_guidance=""
  preflight_tables=""
  while IFS= read -r line; do
    [ -n "$line" ] || continue
    t=${line%% *}
    preflight_tables="$preflight_tables $t"
    rest=${line#* }
    expected=${rest%% *}
    expected_hash=${rest#* }
    if ! actual=$(row_count "$db" "$t"); then
      printf 'compact: db=%s post-flatten row count failed for table=%s\n' "$db" "$t" >&2
      verify_counts_saw_probe_failure=1
      if [ "$fail" -eq 0 ]; then
        fail=2
        verify_counts_failure_reason="post-flatten row count probe failed"
        verify_counts_failure_guidance="post-flatten row count probe failed; investigate before re-running"
      fi
      continue
    fi
    case "$actual" in
      ''|*[!0-9]*)
        printf 'compact: db=%s post-flatten row count failed for table=%s\n' "$db" "$t" >&2
        verify_counts_saw_probe_failure=1
        if [ "$fail" -eq 0 ]; then
          fail=2
          verify_counts_failure_reason="post-flatten row count probe failed"
          verify_counts_failure_guidance="post-flatten row count probe failed; investigate before re-running"
        fi
        continue
        ;;
    esac
    if ! actual_hash=$(table_value_hash "$db" "$t"); then
      printf 'compact: db=%s post-flatten table value hash failed for table=%s\n' "$db" "$t" >&2
      verify_counts_saw_probe_failure=1
      if [ "$fail" -eq 0 ]; then
        fail=2
        verify_counts_failure_reason="post-flatten table value hash probe failed"
        verify_counts_failure_guidance="post-flatten table value hash probe failed; investigate before re-running"
      fi
      continue
    fi
    if [ -z "$actual_hash" ]; then
      printf 'compact: db=%s post-flatten table value hash returned empty value for table=%s\n' "$db" "$t" >&2
      verify_counts_saw_probe_failure=1
      if [ "$fail" -eq 0 ]; then
        fail=2
        verify_counts_failure_reason="post-flatten table value hash probe failed"
        verify_counts_failure_guidance="post-flatten table value hash probe failed; investigate before re-running"
      fi
      continue
    fi
    table_gained_rows=0
    if [ "$actual" != "$expected" ]; then
      if [ "$actual" -lt "$expected" ]; then
        printf 'compact: db=%s row count decreased after flatten table=%s before=%s after=%s\n' \
          "$db" "$t" "$expected" "$actual" >&2
        verify_counts_saw_row_decrease=1
        if [ "$fail" -ne 1 ]; then
          fail=1
          verify_counts_failure_reason="post-flatten row count decreased"
          verify_counts_failure_guidance="row counts decreased; investigate before re-running"
        fi
      else
        printf 'compact: db=%s table=%s gained rows during flatten before=%s after=%s — pending value-hash verification\n' \
          "$db" "$t" "$expected" "$actual"
        verify_counts_saw_gain=1
        table_gained_rows=1
      fi
    fi
    if [ "$actual_hash" != "$expected_hash" ]; then
      if [ "$table_gained_rows" = "1" ]; then
        verify_counts_saw_gain_hash_drift=1
        printf 'compact: db=%s table=%s value hash changed with row-count increase before=%s after=%s — defer or quarantine after writer-race classification\n' \
          "$db" "$t" "$expected_hash" "$actual_hash" >&2
        if [ "$fail" -ne 1 ]; then
          fail=1
          verify_counts_failure_reason="post-flatten table value hash changed with row-count increase"
          verify_counts_failure_guidance="row-count increase plus table value hash drift cannot prove row preservation; investigate before re-running"
        fi
      else
        printf 'compact: db=%s table=%s value hash changed after flatten without row-count increase before=%s after=%s — quarantine and investigate before GC\n' \
          "$db" "$t" "$expected_hash" "$actual_hash" >&2
        verify_counts_saw_same_count_hash_drift=1
        if [ "$fail" -ne 1 ]; then
          fail=1
          verify_counts_failure_reason="post-flatten table value hash changed without row-count increase"
          verify_counts_failure_guidance="same-count table value hash changed; investigate before re-running"
        fi
      fi
    fi
  done < "$preflight"
  post_tables_tmp=$(mktemp)
  if ! user_tables "$db" > "$post_tables_tmp"; then
    verify_counts_saw_probe_failure=1
    if [ "$fail" -eq 0 ]; then
      fail=2
      verify_counts_failure_reason="post-flatten table list probe failed"
      verify_counts_failure_guidance="post-flatten table list probe failed; investigate before re-running"
    fi
    rm -f "$post_tables_tmp"
    return "$fail"
  fi
  while IFS= read -r post_table; do
    [ -n "$post_table" ] || continue
    if ! valid_table_name "$post_table"; then
      printf 'compact: db=%s invalid table name after flatten table=%s — quarantine and investigate before GC\n' \
        "$db" "$post_table" >&2
      verify_counts_saw_table_list_change=1
      if [ "$fail" -ne 1 ]; then
        fail=1
        verify_counts_failure_reason="post-flatten table list changed"
        verify_counts_failure_guidance="post-flatten table list changed; investigate before re-running"
      fi
      continue
    fi
    case " $preflight_tables " in
      *" $post_table "*) ;;
      *)
        printf 'compact: db=%s table=%s appeared after pre-flight snapshot — quarantine and investigate before GC\n' \
          "$db" "$post_table" >&2
        verify_counts_saw_table_list_change=1
        if [ "$fail" -ne 1 ]; then
          fail=1
          verify_counts_failure_reason="post-flatten table list changed"
          verify_counts_failure_guidance="post-flatten table list changed; investigate before re-running"
        fi
        ;;
    esac
  done < "$post_tables_tmp"
  rm -f "$post_tables_tmp"
  return "$fail"
}

oldgen_has_files() {
  db="$1"
  oldgen_dir="$DOLT_DATA_DIR/$db/.dolt/noms/oldgen"
  [ -d "$oldgen_dir" ] || return 1
  [ -n "$(find "$oldgen_dir" -mindepth 1 -print -quit 2>/dev/null)" ]
}

compact_marker_path() {
  dir="$1"
  db="$2"
  printf '%s/%s\n' "$dir" "$db"
}

has_compact_marker() {
  dir="$1"
  db="$2"
  [ -f "$(compact_marker_path "$dir" "$db")" ]
}

write_compact_marker() {
  dir="$1"
  db="$2"
  reason="$3"
  shift 3

  marker_path=$(compact_marker_path "$dir" "$db")
  created_at=""
  if [ -f "$marker_path" ]; then
    created_at=$(awk 'index($0, "created_at=") == 1 { print substr($0, 12); exit }' "$marker_path" || true)
    case "$created_at" in
      ''|*[!0-9TZ:.-]*)
        created_at=""
        ;;
    esac
  fi
  if [ -z "$created_at" ]; then
    created_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  fi

  old_umask=$(umask)
  umask 077
  if ! mkdir -p "$dir"; then
    umask "$old_umask"
    printf 'compact: db=%s unable to create marker directory %s\n' "$db" "$dir" >&2
    return 1
  fi
  tmp=$(mktemp "$dir/$db.tmp.XXXXXX") || {
    umask "$old_umask"
    printf 'compact: db=%s unable to create marker in %s\n' "$db" "$dir" >&2
    return 1
  }
  umask "$old_umask"

  {
    printf 'db=%s\n' "$db"
    printf 'reason=%s\n' "$reason"
    printf 'created_at=%s\n' "$created_at"
    while [ "$#" -gt 0 ]; do
      printf '%s\n' "$1"
      shift
    done
  } > "$tmp" || {
    rm -f "$tmp"
    printf 'compact: db=%s unable to write marker %s\n' "$db" "$tmp" >&2
    return 1
  }

  if ! mv -f "$tmp" "$marker_path"; then
    rm -f "$tmp"
    printf 'compact: db=%s unable to install marker in %s\n' "$db" "$dir" >&2
    return 1
  fi
  return 0
}

ensure_compact_marker_writable() {
  dir="$1"
  db="$2"

  old_umask=$(umask)
  umask 077
  if ! mkdir -p "$dir"; then
    umask "$old_umask"
    printf 'compact: db=%s unable to create marker directory %s\n' "$db" "$dir" >&2
    return 1
  fi
  probe=$(mktemp "$dir/$db.probe.XXXXXX") || {
    umask "$old_umask"
    printf 'compact: db=%s unable to create marker in %s\n' "$db" "$dir" >&2
    return 1
  }
  umask "$old_umask"

  if ! printf 'probe\n' > "$probe"; then
    rm -f "$probe"
    printf 'compact: db=%s unable to write marker probe %s\n' "$db" "$probe" >&2
    return 1
  fi
  if ! rm -f "$probe"; then
    printf 'compact: db=%s unable to remove marker probe %s\n' "$db" "$probe" >&2
    return 1
  fi
  return 0
}

ensure_repair_marker_paths_writable() {
  db="$1"
  remote="$2"

  ensure_compact_marker_writable "$quarantine_dir" "$db" || return 1
  ensure_compact_marker_writable "$pending_gc_dir" "$db" || return 1
  if [ -n "$remote" ]; then
    ensure_compact_marker_writable "$pending_push_dir" "$db" || return 1
  fi
  return 0
}

write_pending_push_marker() {
  db="$1"
  remote="$2"
  expected_remote_head="${3:-}"
  expected_remote_head_verified="${4:-0}"
  compacted_from_head="${5:-}"
  reason="$6"
  local_branch="${7:-main}"
  remote_branch="${8:-$local_branch}"

  write_compact_marker "$pending_push_dir" "$db" "$reason" \
    "remote=$remote" \
    "expected_remote_head=$expected_remote_head" \
    "expected_remote_head_verified=$expected_remote_head_verified" \
    "compacted_from_head=$compacted_from_head" \
    "local_branch=$local_branch" \
    "remote_branch=$remote_branch"
}

write_pending_gc_marker() {
  _pg_db="$1"
  _pg_reason="$2"
  _pg_remote="${3:-}"
  _pg_expected_remote_head="${4:-}"
  _pg_expected_remote_head_verified="${5:-0}"
  _pg_compacted_from_head="${6:-}"
  _pg_local_branch="${7:-main}"
  _pg_remote_branch="${8:-$_pg_local_branch}"

  write_compact_marker "$pending_gc_dir" "$_pg_db" "$_pg_reason" \
    "remote=$_pg_remote" \
    "expected_remote_head=$_pg_expected_remote_head" \
    "expected_remote_head_verified=$_pg_expected_remote_head_verified" \
    "compacted_from_head=$_pg_compacted_from_head" \
    "local_branch=$_pg_local_branch" \
    "remote_branch=$_pg_remote_branch"
}

compact_marker_value() {
  dir="$1"
  db="$2"
  key="$3"
  marker=$(compact_marker_path "$dir" "$db")
  [ -f "$marker" ] || return 1
  awk -v prefix="$key=" 'index($0, prefix) == 1 { print substr($0, length(prefix) + 1); exit }' "$marker"
}

compact_marker_created_at_epoch() {
  dir="$1"
  db="$2"
  created_at=$(compact_marker_value "$dir" "$db" created_at || true)
  [ -n "$created_at" ] || return 1
  case "$created_at" in
    *[!0-9TZ:.-]*)
      return 1
      ;;
  esac
  date -u -d "$created_at" +%s 2>/dev/null ||
    date -ju -f "%Y-%m-%dT%H:%M:%SZ" "$created_at" +%s 2>/dev/null
}

ensure_remote_push_retry_fresh() {
  dir="$1"
  db="$2"
  marker_label="$3"

  created_epoch=$(compact_marker_created_at_epoch "$dir" "$db" || true)
  if [ -z "$created_epoch" ]; then
    printf 'compact: db=%s %s marker has missing or invalid created_at — manual review required before remote push retry\n' \
      "$db" "$marker_label" >&2
    return 1
  fi
  now_epoch=$(date -u +%s)
  age_secs=$(( now_epoch - created_epoch ))
  if [ "$age_secs" -lt 0 ]; then
    age_secs=0
  fi
  if [ "$age_secs" -gt "$pending_push_max_age_secs" ]; then
    printf 'compact: db=%s %s marker is stale age=%ss max_age=%ss — manual review required before remote push retry\n' \
      "$db" "$marker_label" "$age_secs" "$pending_push_max_age_secs" >&2
    return 1
  fi
  return 0
}

clear_compact_marker() {
  dir="$1"
  db="$2"
  rm -f "$(compact_marker_path "$dir" "$db")"
}

run_full_gc() {
  db="$1"
  failure_prefix="$2"
  success_prefix="$3"
  start="$4"

  printf 'compact: db=%s — running DOLT_GC --full...\n' "$db"
  gc_rc=0
  gc_err_tmp=$(mktemp)
  dolt_query "$db" "CALL DOLT_GC('--full')" >/dev/null 2>"$gc_err_tmp" || gc_rc=$?

  elapsed=$(( $(date +%s) - start ))
  if [ "$gc_rc" -ne 0 ]; then
    printf 'compact: db=%s %s DOLT_GC failed rc=%s duration=%ss\n' \
      "$db" "$failure_prefix" "$gc_rc" "$elapsed" >&2
    emit_error_file "$db" "$gc_err_tmp"
    rm -f "$gc_err_tmp"
    return 1
  fi
  rm -f "$gc_err_tmp"

  printf 'compact: db=%s %s duration=%ss — ok\n' \
    "$db" "$success_prefix" "$elapsed"
  return 0
}

sql_quote() {
  printf '%s' "$1" | sed "s/'/''/g"
}

hosted_retention_cutoff_utc() {
  older_than_value="$1"
  if ! command -v python3 >/dev/null 2>&1; then
    printf 'compact: hosted retention requires python3\n' >&2
    return 1
  fi
  python3 - "$older_than_value" <<'PY'
from datetime import datetime, timedelta, timezone
import re
import sys

value = sys.argv[1].strip()
match = re.fullmatch(r"([1-9][0-9]*)([smhdw]?)", value)
if not match:
    raise SystemExit("invalid retention window")
amount = int(match.group(1))
unit = match.group(2) or "d"
seconds = {
    "s": 1,
    "m": 60,
    "h": 3600,
    "d": 86400,
    "w": 604800,
}[unit] * amount
cutoff = datetime.now(timezone.utc) - timedelta(seconds=seconds)
print(cutoff.strftime("%Y-%m-%d %H:%M:%S"))
PY
}

hosted_retention_plan() {
  db="$1"
  hosted_retention_candidates=0
  hosted_retention_issue_candidates=0
  hosted_retention_wisp_candidates=0
  hosted_retention_cutoff=""
  hosted_retention_issue_candidate_query=""
  hosted_retention_wisp_candidate_query=""
  hosted_retention_issue_delete_limit=0
  hosted_retention_wisp_delete_limit=0

  required_schema_count=32
  schema_count=$(query_single_cell "$db" "retention schema validation query failed" \
    "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND CONCAT(table_name,'.',column_name) IN ('issues.id','issues.status','issues.metadata','issues.closed_at','issues.updated_at','issues.created_at','issues.pinned','labels.issue_id','labels.label','comments.issue_id','events.issue_id','issue_snapshots.issue_id','compaction_snapshots.issue_id','dependencies.issue_id','dependencies.depends_on_issue_id','dependencies.depends_on_wisp_id','child_counters.parent_id','wisps.id','wisps.status','wisps.metadata','wisps.closed_at','wisps.updated_at','wisps.created_at','wisps.pinned','wisp_labels.issue_id','wisp_labels.label','wisp_comments.issue_id','wisp_events.issue_id','wisp_dependencies.issue_id','wisp_dependencies.depends_on_issue_id','wisp_dependencies.depends_on_wisp_id','wisp_child_counters.parent_id');") || return 1
  if [ "$schema_count" != "$required_schema_count" ]; then
    printf 'compact: db=%s retention schema mismatch; saw %s/%s required columns\n' \
      "$db" "$schema_count" "$required_schema_count" >&2
    return 1
  fi

  hosted_retention_cutoff=$(hosted_retention_cutoff_utc "$server_retention_older_than") || {
    printf 'compact: invalid GC_DOLT_COMPACT_RETENTION_OLDER_THAN=%s\n' "$server_retention_older_than" >&2
    return 1
  }

  hosted_retention_count_limit=$((server_retention_max_delete + 1))
  hosted_retention_issue_order_query="SELECT DISTINCT l.issue_id AS id FROM labels l JOIN issues i ON i.id = l.issue_id WHERE (l.label = 'order-tracking' OR l.label LIKE 'order-run:%') AND i.status = 'closed' AND COALESCE(i.pinned, 0) = 0 AND COALESCE(i.closed_at, i.updated_at, i.created_at) < '$hosted_retention_cutoff'"
  hosted_retention_wisp_order_query="SELECT DISTINCT l.issue_id AS id FROM wisp_labels l JOIN wisps w ON w.id = l.issue_id WHERE (l.label = 'order-tracking' OR l.label LIKE 'order-run:%') AND w.status = 'closed' AND COALESCE(w.pinned, 0) = 0 AND COALESCE(w.closed_at, w.updated_at, w.created_at) < '$hosted_retention_cutoff'"
  hosted_retention_issue_candidate_query="SELECT c.id FROM ($hosted_retention_issue_order_query) c WHERE NOT EXISTS (SELECT 1 FROM dependencies d WHERE d.issue_id = c.id OR d.depends_on_issue_id = c.id) AND NOT EXISTS (SELECT 1 FROM issues child_i WHERE child_i.id LIKE CONCAT(c.id, '.%') AND child_i.status <> 'closed') AND NOT EXISTS (SELECT 1 FROM wisps child_w WHERE child_w.id LIKE CONCAT(c.id, '.%') AND child_w.status <> 'closed')"
  hosted_retention_wisp_candidate_query="SELECT c.id FROM ($hosted_retention_wisp_order_query) c WHERE NOT EXISTS (SELECT 1 FROM wisp_dependencies d WHERE d.issue_id = c.id OR d.depends_on_wisp_id = c.id) AND NOT EXISTS (SELECT 1 FROM dependencies d WHERE d.depends_on_wisp_id = c.id) AND NOT EXISTS (SELECT 1 FROM issues child_i WHERE child_i.id LIKE CONCAT(c.id, '.%') AND child_i.status <> 'closed') AND NOT EXISTS (SELECT 1 FROM wisps child_w WHERE child_w.id LIKE CONCAT(c.id, '.%') AND child_w.status <> 'closed')"

  hosted_retention_issue_candidates=$(query_single_cell "$db" "retention issue candidate count failed" \
    "SELECT COUNT(*) FROM ($hosted_retention_issue_candidate_query LIMIT $hosted_retention_count_limit) gc_retention_limited_issues") || return 1

  case "$hosted_retention_issue_candidates" in
    ''|*[!0-9]*)
      printf 'compact: db=%s retention issue candidate count returned invalid value=%s\n' \
        "$db" "$hosted_retention_issue_candidates" >&2
      return 1
      ;;
  esac

  if [ "$hosted_retention_issue_candidates" -gt "$server_retention_max_delete" ]; then
    printf 'compact: db=%s retention issue candidates exceed max_delete=%s\n' \
      "$db" "$server_retention_max_delete" >&2
    return 1
  fi

  hosted_retention_remaining_limit=$((server_retention_max_delete - hosted_retention_issue_candidates + 1))
  hosted_retention_wisp_candidates=$(query_single_cell "$db" "retention wisp candidate count failed" \
    "SELECT COUNT(*) FROM ($hosted_retention_wisp_candidate_query LIMIT $hosted_retention_remaining_limit) gc_retention_limited_wisps") || return 1

  case "$hosted_retention_wisp_candidates" in
    ''|*[!0-9]*)
      printf 'compact: db=%s retention wisp candidate count returned invalid value=%s\n' \
        "$db" "$hosted_retention_wisp_candidates" >&2
      return 1
      ;;
  esac

  hosted_retention_candidates=$((hosted_retention_issue_candidates + hosted_retention_wisp_candidates))
  if [ "$hosted_retention_candidates" -gt "$server_retention_max_delete" ]; then
    printf 'compact: db=%s retention candidates=%s exceeds max_delete=%s\n' \
      "$db" "$hosted_retention_candidates" "$server_retention_max_delete" >&2
    return 1
  fi
  hosted_retention_issue_delete_limit="$hosted_retention_issue_candidates"
  hosted_retention_wisp_delete_limit="$hosted_retention_wisp_candidates"
  return 0
}

hosted_retention_sql() {
  db="$1"
  message=$(sql_quote "gc dolt compact: prune operational_churn older than $server_retention_older_than")
  cat <<SQL
USE \`$db\`;
START TRANSACTION;
CREATE TEMPORARY TABLE gc_retention_sweep_issue_ids AS
  $hosted_retention_issue_candidate_query LIMIT $hosted_retention_issue_delete_limit;
CREATE TEMPORARY TABLE gc_retention_sweep_wisp_ids AS
  $hosted_retention_wisp_candidate_query LIMIT $hosted_retention_wisp_delete_limit;
DELETE FROM labels WHERE issue_id IN (SELECT id FROM gc_retention_sweep_issue_ids);
DELETE FROM comments WHERE issue_id IN (SELECT id FROM gc_retention_sweep_issue_ids);
DELETE FROM events WHERE issue_id IN (SELECT id FROM gc_retention_sweep_issue_ids);
DELETE FROM issue_snapshots WHERE issue_id IN (SELECT id FROM gc_retention_sweep_issue_ids);
DELETE FROM compaction_snapshots WHERE issue_id IN (SELECT id FROM gc_retention_sweep_issue_ids);
DELETE FROM child_counters WHERE parent_id IN (SELECT id FROM gc_retention_sweep_issue_ids);
DELETE FROM dependencies WHERE issue_id IN (SELECT id FROM gc_retention_sweep_issue_ids) OR depends_on_issue_id IN (SELECT id FROM gc_retention_sweep_issue_ids);
DELETE FROM wisp_dependencies WHERE depends_on_issue_id IN (SELECT id FROM gc_retention_sweep_issue_ids);
DELETE FROM issues WHERE id IN (SELECT id FROM gc_retention_sweep_issue_ids);
DELETE FROM wisp_labels WHERE issue_id IN (SELECT id FROM gc_retention_sweep_wisp_ids);
DELETE FROM wisp_comments WHERE issue_id IN (SELECT id FROM gc_retention_sweep_wisp_ids);
DELETE FROM wisp_events WHERE issue_id IN (SELECT id FROM gc_retention_sweep_wisp_ids);
DELETE FROM wisp_child_counters WHERE parent_id IN (SELECT id FROM gc_retention_sweep_wisp_ids);
DELETE FROM wisp_dependencies WHERE issue_id IN (SELECT id FROM gc_retention_sweep_wisp_ids) OR depends_on_wisp_id IN (SELECT id FROM gc_retention_sweep_wisp_ids);
DELETE FROM dependencies WHERE depends_on_wisp_id IN (SELECT id FROM gc_retention_sweep_wisp_ids);
DELETE FROM wisps WHERE id IN (SELECT id FROM gc_retention_sweep_wisp_ids);
COMMIT;
CALL DOLT_COMMIT('-Am', '$message', '--skip-empty');
SQL
}

hosted_database_list_contains() {
  db_file="$1"
  needle="$2"
  while IFS= read -r listed; do
    [ "$listed" = "$needle" ] && return 0
  done < "$db_file"
  return 1
}

run_hosted_server_compact() {
  db_file="$1"

  if [ -n "$only_dbs" ]; then
    printf 'compact: hosted server-side compact does not support GC_DOLT_COMPACT_ONLY_DBS because the server procedure owns the complete maintenance window\n' >&2
    return 1
  fi

  anchor_db=$(sed -n '1p' "$db_file")
  if [ -z "$anchor_db" ]; then
    printf 'compact: hosted server-side compact found no user databases\n' >&2
    return 1
  fi

  capability_err=$(mktemp)
  if ! dolt_query "$anchor_db" "CALL DOLT_MAINTENANCE_STATUS()" >/dev/null 2>"$capability_err"; then
    while IFS= read -r err_line; do
      printf 'compact: hosted maintenance capability check: %s\n' "$err_line" >&2
    done < "$capability_err"
    rm -f "$capability_err"
    compact_not_applicable "server_maintenance_unavailable"
  fi
  rm -f "$capability_err"

  highest_commits=0
  threshold_triggered=0
  db_count=0
  while IFS= read -r db; do
    [ -n "$db" ] || continue
    db_count=$((db_count + 1))
    count=$(commit_count "$db") || return 1
    case "$count" in
      ''|*[!0-9]*)
        printf 'compact: db=%s commit count probe returned invalid value=%s\n' "$db" "$count" >&2
        return 1
        ;;
    esac
    [ "$count" -gt "$highest_commits" ] && highest_commits="$count"
    if [ "$count" -ge "$threshold_commits" ]; then
      threshold_triggered=1
    fi
  done < "$db_file"

  retention_db="${GC_DOLT_COMPACT_RETENTION_DB:-hq}"
  if ! valid_database_name "$retention_db"; then
    printf 'compact: invalid GC_DOLT_COMPACT_RETENTION_DB=%s\n' "$retention_db" >&2
    return 1
  fi
  retention_enabled=0
  hosted_retention_candidates=0
  if hosted_database_list_contains "$db_file" "$retention_db"; then
    hosted_retention_plan "$retention_db" || return 1
    if [ "$hosted_retention_candidates" -gt 0 ]; then
      retention_enabled=1
    fi
    printf 'compact: db=%s retention_candidates=%s issues=%s wisps=%s older_than=%s\n' \
      "$retention_db" "$hosted_retention_candidates" \
      "$hosted_retention_issue_candidates" "$hosted_retention_wisp_candidates" \
      "$server_retention_older_than"
  else
    printf 'compact: db=%s retention database not present — skip retention\n' "$retention_db"
  fi

  if [ "$threshold_triggered" != "1" ] && [ "$retention_enabled" != "1" ]; then
    printf 'compact: hosted server_side databases=%s highest_commits=%s below_threshold=%s retention_candidates=0 — skip\n' \
      "$db_count" "$highest_commits" "$threshold_commits"
    return 0
  fi

  if [ -n "$dry_run" ]; then
    printf 'compact: hosted server_side databases=%s highest_commits=%s retention_candidates=%s — dry-run (would enter maintenance, prune, compact, full GC)\n' \
      "$db_count" "$highest_commits" "$hosted_retention_candidates"
    return 0
  fi

  reason=$(sql_quote "gc dolt compact: hosted server-side maintenance")
  compact_message=$(sql_quote "gc dolt compact: server-side hosted compact")
  exit_message=$(sql_quote "gc dolt compact completed")
  sql_tmp=$(mktemp)
  {
    printf "CALL DOLT_MAINTENANCE_ENTER('%s', '%s', '%s');\n" \
      "$reason" "$server_drain_timeout_ms" "$server_max_duration_ms"
    if [ "$retention_enabled" = "1" ]; then
      hosted_retention_sql "$retention_db"
    fi
    printf "CALL DOLT_SERVER_COMPACT('%s', 'true', '%s', '%s');\n" \
      "$compact_message" "$server_drain_timeout_ms" "$server_max_duration_ms"
    printf "CALL DOLT_MAINTENANCE_EXIT('%s');\n" "$exit_message"
  } > "$sql_tmp"

  compact_err=$(mktemp)
  if ! dolt_query "$anchor_db" "$(cat "$sql_tmp")" >/dev/null 2>"$compact_err"; then
    printf 'compact: hosted server-side maintenance failed\n' >&2
    emit_error_file "$anchor_db" "$compact_err"
    rm -f "$sql_tmp" "$compact_err"
    return 1
  fi
  rm -f "$sql_tmp" "$compact_err"

  printf 'compact: hosted server_side=complete databases=%s highest_commits=%s retention_candidates=%s full_gc=true\n' \
    "$db_count" "$highest_commits" "$hosted_retention_candidates"
  return 0
}

push_remote_after_compaction() {
  db="$1"
  remote="$2"
  expected_remote_head="${3:-}"
  expected_remote_head_verified="${4:-0}"
  push_context="${5:-initial}"
  compacted_from_head="${6:-}"
  local_branch="${7:-main}"
  remote_branch="${8:-$local_branch}"
  [ -n "$remote" ] || return 0
  valid_branch_name "$local_branch" || {
    printf 'compact: db=%s invalid local branch=%s before remote push\n' "$db" "$local_branch" >&2
    return 1
  }
  valid_branch_name "$remote_branch" || {
    printf 'compact: db=%s invalid remote branch=%s before remote push\n' "$db" "$remote_branch" >&2
    return 1
  }

  fetch_rc=0
  fetch_err_tmp=$(mktemp)
  fetch_remote "$db" "$remote" >/dev/null 2>"$fetch_err_tmp" || fetch_rc=$?
  if [ "$fetch_rc" -ne 0 ]; then
    printf 'compact: db=%s remote=%s fetch failed rc=%s before push after local compaction\n' \
      "$db" "$remote" "$fetch_rc" >&2
    emit_error_file "$db" "$fetch_err_tmp"
    rm -f "$fetch_err_tmp"
    write_pending_push_marker "$db" "$remote" "$expected_remote_head" "$expected_remote_head_verified" "$compacted_from_head" \
      "flatten and full GC succeeded but remote fetch before push failed" "$local_branch" "$remote_branch" || return 1
    return 0
  fi
  rm -f "$fetch_err_tmp"

  if ! latest_remote_head=$(remote_branch_head "$db" "$remote" "$remote_branch"); then
    printf 'compact: db=%s remote=%s HEAD probe failed before push after local compaction\n' \
      "$db" "$remote" >&2
    write_pending_push_marker "$db" "$remote" "$expected_remote_head" "$expected_remote_head_verified" "$compacted_from_head" \
      "flatten and full GC succeeded but remote HEAD probe before push failed" "$local_branch" "$remote_branch" || return 1
    return 0
  fi
  if [ -n "$latest_remote_head" ]; then
    case "$latest_remote_head" in
      *[!A-Za-z0-9]*)
        printf 'compact: db=%s remote=%s returned invalid HEAD=%s before push — fail\n' \
          "$db" "$remote" "$latest_remote_head" >&2
        write_pending_push_marker "$db" "$remote" "$expected_remote_head" "$expected_remote_head_verified" "$compacted_from_head" \
          "flatten and full GC succeeded but remote HEAD before push was invalid" "$local_branch" "$remote_branch" || return 1
        return 0
        ;;
    esac
  fi
  if [ "$latest_remote_head" != "$expected_remote_head" ]; then
    if [ -z "$expected_remote_head" ] && [ -n "$latest_remote_head" ]; then
      printf 'compact: db=%s remote=%s recovered HEAD=%s after unknown preflight HEAD — verifying before push\n' \
        "$db" "$remote" "$latest_remote_head"
      expected_remote_head="$latest_remote_head"
      expected_remote_head_verified=0
    elif [ "$push_context" = "retry" ]; then
      if [ -z "$latest_remote_head" ]; then
        printf 'compact: db=%s remote=%s HEAD changed during pending push retry expected_HEAD=%s got_HEAD=<empty> — deferred for next run; manual reconciliation required if this persists\n' \
          "$db" "$remote" "${expected_remote_head:-<empty>}" >&2
        write_pending_push_marker "$db" "$remote" "" 0 "$compacted_from_head" \
          "remote push retry deferred: remote HEAD changed during pending push retry" "$local_branch" "$remote_branch" || return 1
        return 1
      fi
      printf 'compact: db=%s remote=%s HEAD changed during pending push retry expected_HEAD=%s got_HEAD=%s — verifying latest remote HEAD\n' \
        "$db" "$remote" "${expected_remote_head:-<empty>}" "$latest_remote_head" >&2
      expected_remote_head="$latest_remote_head"
      expected_remote_head_verified=0
    else
      printf 'compact: db=%s remote=%s HEAD changed before push expected_HEAD=%s got_HEAD=%s — leaving local compaction pending remote repair\n' \
        "$db" "$remote" "${expected_remote_head:-<empty>}" "${latest_remote_head:-<empty>}" >&2
      write_pending_push_marker "$db" "$remote" "$expected_remote_head" "$expected_remote_head_verified" "$compacted_from_head" \
        "flatten and full GC succeeded but remote HEAD changed before push" "$local_branch" "$remote_branch" || return 1
      return 0
    fi
  fi
  if [ -n "$latest_remote_head" ] && [ "$expected_remote_head_verified" != "1" ]; then
    if [ -n "$compacted_from_head" ] && [ "$latest_remote_head" = "$compacted_from_head" ]; then
      expected_remote_head_verified=1
      printf 'compact: db=%s remote=%s HEAD=%s matches compacted source head — retrying push\n' \
        "$db" "$remote" "$latest_remote_head"
    else
      if ! in_local=$(commit_exists_in_local_log "$db" "$latest_remote_head"); then
        printf 'compact: db=%s remote=%s HEAD=%s ancestry probe failed before push after local compaction\n' \
          "$db" "$remote" "$latest_remote_head" >&2
        write_pending_push_marker "$db" "$remote" "$expected_remote_head" "$expected_remote_head_verified" "$compacted_from_head" \
          "flatten and full GC succeeded but remote ancestry probe before push failed" "$local_branch" "$remote_branch" || return 1
        return 0
      fi
      case "$in_local" in
        1)
          expected_remote_head_verified=1
          printf 'compact: db=%s remote=%s HEAD=%s is now verified in local history — retrying push\n' \
            "$db" "$remote" "$latest_remote_head"
          ;;
        0)
          if [ "$push_context" = "retry" ]; then
            printf 'compact: db=%s remote=%s HEAD=%s remains absent from local history — deferred for next run; manual reconciliation required if this persists\n' \
              "$db" "$remote" "$latest_remote_head" >&2
            write_pending_push_marker "$db" "$remote" "$expected_remote_head" "$expected_remote_head_verified" "$compacted_from_head" \
              "remote push retry deferred: remote has unique commits not in local history" "$local_branch" "$remote_branch" || return 1
            return 1
          fi
          printf 'compact: db=%s remote=%s HEAD=%s was not verified in local history before flatten — leaving local compaction pending remote repair\n' \
            "$db" "$remote" "$latest_remote_head" >&2
          write_pending_push_marker "$db" "$remote" "$expected_remote_head" "$expected_remote_head_verified" "$compacted_from_head" \
            "flatten and full GC succeeded but remote has unique commits not in local history" "$local_branch" "$remote_branch" || return 1
          return 0
          ;;
        *)
          printf 'compact: db=%s remote=%s ancestry probe returned invalid value=%s before push after local compaction\n' \
            "$db" "$remote" "$in_local" >&2
          write_pending_push_marker "$db" "$remote" "$expected_remote_head" "$expected_remote_head_verified" "$compacted_from_head" \
            "flatten and full GC succeeded but remote ancestry probe returned invalid result" "$local_branch" "$remote_branch" || return 1
          return 0
          ;;
      esac
    fi
  fi

  push_rc=0
  push_err_tmp=$(mktemp)
  push_remote_refspec "$db" "$remote" "$local_branch" "$remote_branch" >/dev/null 2>"$push_err_tmp" || push_rc=$?
  if [ "$push_rc" -ne 0 ]; then
    printf 'compact: db=%s remote=%s push failed rc=%s after local compaction\n' \
      "$db" "$remote" "$push_rc" >&2
    emit_error_file "$db" "$push_err_tmp"
    rm -f "$push_err_tmp"
    write_pending_push_marker "$db" "$remote" "$expected_remote_head" "$expected_remote_head_verified" "$compacted_from_head" \
      "flatten and full GC succeeded but remote push failed" "$local_branch" "$remote_branch" || return 1
    return 0
  fi
  rm -f "$push_err_tmp"
  clear_compact_marker "$pending_push_dir" "$db"
  printf 'compact: db=%s remote=%s pushed compacted %s\n' "$db" "$remote" "$remote_branch"
  return 0
}

restore_head_if_current() {
  db="$1"
  head="$2"
  expected_current="$3"
  reason="$4"

  current_head=$(head_commit "$db" || true)
  if [ "$current_head" = "$head" ]; then
    printf 'compact: db=%s already at pre-flatten HEAD=%s after %s\n' \
      "$db" "$head" "$reason" >&2
    return 0
  fi
  if [ "$current_head" != "$expected_current" ]; then
    printf 'compact: db=%s current HEAD=%s is neither pre-flatten HEAD=%s nor expected recovery HEAD=%s after %s — refusing hard reset; manual repair required\n' \
      "$db" "${current_head:-<empty>}" "$head" "$expected_current" "$reason" >&2
    return 1
  fi

  restore_rc=0
  restore_err_tmp=$(mktemp)
  dolt_query "$db" "CALL DOLT_RESET('--hard', '$head')" >/dev/null 2>"$restore_err_tmp" || restore_rc=$?
  if [ "$restore_rc" -ne 0 ]; then
    printf 'compact: db=%s restore to pre-flatten HEAD=%s failed rc=%s — manual repair required\n' \
      "$db" "$head" "$restore_rc" >&2
    emit_error_file "$db" "$restore_err_tmp"
    rm -f "$restore_err_tmp"
    return 1
  fi
  rm -f "$restore_err_tmp"

  restored_head=$(head_commit "$db" || true)
  if [ "$restored_head" != "$head" ]; then
    printf 'compact: db=%s restore verification failed want_HEAD=%s got_HEAD=%s after %s — manual repair required\n' \
      "$db" "$head" "${restored_head:-<empty>}" "$reason" >&2
    return 1
  fi
  printf 'compact: db=%s restored pre-flatten HEAD=%s after %s\n' \
    "$db" "$head" "$reason" >&2
  return 0
}

restore_head_after_flatten_failure() {
  db="$1"
  head="$2"
  root="$3"
  restore_head_if_current "$db" "$head" "$root" "flatten failure"
}

preserve_head_after_integrity_failure() {
  db="$1"
  flatten_head="$2"
  current_head=$(head_commit "$db" || true)
  if [ -z "$current_head" ]; then
    current_head="$flatten_head"
  fi
  printf 'compact: db=%s leaving post-flatten HEAD=%s in place after integrity failure; manual repair required before compaction or GC to avoid discarding possible writer data\n' \
    "$db" "${current_head:-<empty>}" >&2
  return 0
}

preserve_head_after_writer_race_defer() {
  db="$1"
  flatten_head="$2"
  current_head=$(head_commit "$db" || true)
  if [ -z "$current_head" ]; then
    current_head="$flatten_head"
  fi
  printf 'compact: db=%s leaving post-flatten HEAD=%s in place after writer race; pending-GC marker will retry full GC next run\n' \
    "$db" "${current_head:-<empty>}" >&2
  return 0
}

defer_writer_race_after_flatten() {
  db="$1"
  flatten_head="$2"
  defer_remote="$3"
  defer_expected_remote_head="$4"
  defer_expected_remote_head_verified="$5"
  defer_compacted_from_head="$6"
  defer_local_branch="$7"
  defer_remote_branch="$8"
  if ! write_pending_gc_marker "$db" "writer race during flatten deferred full GC" \
    "$defer_remote" "$defer_expected_remote_head" "$defer_expected_remote_head_verified" \
    "$defer_compacted_from_head" "$defer_local_branch" "$defer_remote_branch"; then
    current_head=$(head_commit "$db" || true)
    if [ -z "$current_head" ]; then
      current_head="$flatten_head"
    fi
    printf 'compact: db=%s leaving post-flatten HEAD=%s in place after writer race; pending-GC marker write failed, manual repair required before compaction or GC\n' \
      "$db" "${current_head:-<empty>}" >&2
    return 1
  fi
  preserve_head_after_writer_race_defer "$db" "$flatten_head" || true
  return 0
}

flatten_database() {
  db="$1"
  verify_counts_saw_gain=0
  verify_counts_saw_gain_hash_drift=0
  verify_counts_saw_row_decrease=0
  verify_counts_saw_same_count_hash_drift=0
  verify_counts_saw_table_list_change=0
  verify_counts_saw_probe_failure=0
  verify_counts_failure_reason=""
  verify_counts_failure_guidance=""
  head_before_reset=""
  post_verify_head=""
  writer_race_detected=0

  if [ -n "$only_dbs" ]; then
    case ",$only_dbs," in
      *,"$db",*) ;;
      *)
        printf 'compact: db=%s not in GC_DOLT_COMPACT_ONLY_DBS — skip\n' "$db"
        return 0
        ;;
    esac
  fi

  if has_compact_marker "$quarantine_dir" "$db"; then
    printf 'compact: db=%s integrity quarantine marker exists — manual intervention required before compaction or GC\n' \
      "$db" >&2
    return 1
  fi

  if has_compact_marker "$pending_gc_dir" "$db"; then
    if [ -n "$dry_run" ]; then
      printf 'compact: db=%s pending_gc=present — dry-run (would retry DOLT_GC --full)\n' "$db"
      return 0
    fi
    pending_remote=$(compact_marker_value "$pending_gc_dir" "$db" remote || true)
    pending_expected_remote_head=$(compact_marker_value "$pending_gc_dir" "$db" expected_remote_head || true)
    pending_expected_remote_head_verified=$(compact_marker_value "$pending_gc_dir" "$db" expected_remote_head_verified || true)
    pending_compacted_from_head=$(compact_marker_value "$pending_gc_dir" "$db" compacted_from_head || true)
    pending_local_branch=$(compact_marker_value "$pending_gc_dir" "$db" local_branch || true)
    pending_remote_branch=$(compact_marker_value "$pending_gc_dir" "$db" remote_branch || true)
    [ -n "$pending_local_branch" ] || pending_local_branch="main"
    [ -n "$pending_remote_branch" ] || pending_remote_branch="$pending_local_branch"
    if [ -n "$pending_remote" ] && ! valid_remote_name "$pending_remote"; then
      printf 'compact: db=%s pending_gc marker has invalid remote=%s — manual intervention required\n' \
        "$db" "$pending_remote" >&2
      return 1
    fi
    if ! valid_branch_name "$pending_local_branch"; then
      printf 'compact: db=%s pending_gc marker has invalid local_branch=%s — manual intervention required\n' \
        "$db" "$pending_local_branch" >&2
      return 1
    fi
    if ! valid_branch_name "$pending_remote_branch"; then
      printf 'compact: db=%s pending_gc marker has invalid remote_branch=%s — manual intervention required\n' \
        "$db" "$pending_remote_branch" >&2
      return 1
    fi
    if [ -n "$pending_expected_remote_head" ]; then
      case "$pending_expected_remote_head" in
        *[!A-Za-z0-9]*)
          printf 'compact: db=%s pending_gc marker has invalid expected_remote_head=%s — manual intervention required\n' \
            "$db" "$pending_expected_remote_head" >&2
          return 1
          ;;
      esac
    fi
    case "$pending_expected_remote_head_verified" in
      ''|0|1)
        ;;
      *)
        printf 'compact: db=%s pending_gc marker has invalid expected_remote_head_verified=%s — manual intervention required\n' \
          "$db" "$pending_expected_remote_head_verified" >&2
        return 1
        ;;
    esac
    if [ -n "$pending_compacted_from_head" ]; then
      case "$pending_compacted_from_head" in
        *[!A-Za-z0-9]*)
          printf 'compact: db=%s pending_gc marker has invalid compacted_from_head=%s — manual intervention required\n' \
            "$db" "$pending_compacted_from_head" >&2
          return 1
          ;;
      esac
    fi
    if [ -n "$pending_remote" ]; then
      ensure_remote_push_retry_fresh "$pending_gc_dir" "$db" "pending_gc" || return 1
    fi
    printf 'compact: db=%s pending_gc=present — retrying DOLT_GC --full\n' "$db"
    start=$(date +%s)
    if run_full_gc "$db" "pending-GC retry" "pending-GC retry" "$start"; then
      push_rc=0
      push_remote_after_compaction "$db" "$pending_remote" "$pending_expected_remote_head" "${pending_expected_remote_head_verified:-0}" "retry" "$pending_compacted_from_head" "$pending_local_branch" "$pending_remote_branch" || push_rc=$?
      if [ "$push_rc" -eq 0 ] || { [ -n "$pending_remote" ] && has_compact_marker "$pending_push_dir" "$db"; }; then
        clear_compact_marker "$pending_gc_dir" "$db"
      fi
      return "$push_rc"
    fi
    return 1
  fi

  if has_compact_marker "$pending_push_dir" "$db"; then
    if [ -n "$dry_run" ]; then
      printf 'compact: db=%s pending_push=present — dry-run (would retry remote push)\n' "$db"
      return 0
    fi
    pending_remote=$(compact_marker_value "$pending_push_dir" "$db" remote || true)
    pending_expected_remote_head=$(compact_marker_value "$pending_push_dir" "$db" expected_remote_head || true)
    pending_expected_remote_head_verified=$(compact_marker_value "$pending_push_dir" "$db" expected_remote_head_verified || true)
    pending_compacted_from_head=$(compact_marker_value "$pending_push_dir" "$db" compacted_from_head || true)
    pending_local_branch=$(compact_marker_value "$pending_push_dir" "$db" local_branch || true)
    pending_remote_branch=$(compact_marker_value "$pending_push_dir" "$db" remote_branch || true)
    [ -n "$pending_local_branch" ] || pending_local_branch="main"
    [ -n "$pending_remote_branch" ] || pending_remote_branch="$pending_local_branch"
    if [ -z "$pending_remote" ]; then
      printf 'compact: db=%s pending_push marker is missing remote — manual intervention required\n' \
        "$db" >&2
      return 1
    fi
    if ! valid_branch_name "$pending_local_branch"; then
      printf 'compact: db=%s pending_push marker has invalid local_branch=%s — manual intervention required\n' \
        "$db" "$pending_local_branch" >&2
      return 1
    fi
    if ! valid_branch_name "$pending_remote_branch"; then
      printf 'compact: db=%s pending_push marker has invalid remote_branch=%s — manual intervention required\n' \
        "$db" "$pending_remote_branch" >&2
      return 1
    fi
    if ! valid_remote_name "$pending_remote"; then
      printf 'compact: db=%s pending_push marker has invalid remote=%s — manual intervention required\n' \
        "$db" "$pending_remote" >&2
      return 1
    fi
    if [ -n "$pending_expected_remote_head" ]; then
      case "$pending_expected_remote_head" in
        *[!A-Za-z0-9]*)
          printf 'compact: db=%s pending_push marker has invalid expected_remote_head=%s — manual intervention required\n' \
            "$db" "$pending_expected_remote_head" >&2
          return 1
          ;;
      esac
    fi
    case "$pending_expected_remote_head_verified" in
      ''|0|1)
        ;;
      *)
        printf 'compact: db=%s pending_push marker has invalid expected_remote_head_verified=%s — manual intervention required\n' \
          "$db" "$pending_expected_remote_head_verified" >&2
        return 1
        ;;
    esac
    if [ -n "$pending_compacted_from_head" ]; then
      case "$pending_compacted_from_head" in
        *[!A-Za-z0-9]*)
          printf 'compact: db=%s pending_push marker has invalid compacted_from_head=%s — manual intervention required\n' \
            "$db" "$pending_compacted_from_head" >&2
          return 1
          ;;
      esac
    fi
    ensure_remote_push_retry_fresh "$pending_push_dir" "$db" "pending_push" || return 1
    printf 'compact: db=%s pending_push=present — retrying remote push before threshold check\n' "$db"
    push_remote_after_compaction "$db" "$pending_remote" "$pending_expected_remote_head" "${pending_expected_remote_head_verified:-0}" "retry" "$pending_compacted_from_head" "$pending_local_branch" "$pending_remote_branch"
    return $?
  fi

  if ! count=$(commit_count "$db"); then
    return 1
  fi
  case "$count" in
    ''|*[!0-9]*)
      printf 'compact: db=%s commit count probe returned invalid value=%s\n' "$db" "$count" >&2
      return 1
      ;;
  esac

  if [ "$count" -lt "$threshold_commits" ]; then
    if oldgen_has_files "$db"; then
      printf 'compact: db=%s commits=%s below_threshold=%s oldgen_archives=present pending_gc=absent — skip\n' \
        "$db" "$count" "$threshold_commits"
      return 0
    fi
    printf 'compact: db=%s commits=%s below_threshold=%s — skip\n' \
      "$db" "$count" "$threshold_commits"
    return 0
  fi

  if ! root=$(root_commit "$db"); then
    return 1
  fi
  if [ -z "$root" ]; then
    printf 'compact: db=%s root commit probe returned empty value — fail\n' "$db" >&2
    return 1
  fi

  if ! head=$(head_commit "$db"); then
    return 1
  fi
  if [ -z "$head" ]; then
    printf 'compact: db=%s HEAD commit probe returned empty value — fail\n' "$db" >&2
    return 1
  fi
  compacted_from_head="$head"

  if [ -n "$dry_run" ]; then
    printf 'compact: db=%s commits=%s root=%s — dry-run (would flatten)\n' \
      "$db" "$count" "$root"
    return 0
  fi

  remote=""
  local_branch="main"
  remote_branch="main"
  expected_remote_head=""
  expected_remote_head_verified=0
  if probed_remote=$(select_remote "$db"); then
    remote="$probed_remote"
  else
    printf 'compact: db=%s remote selection failed — fail\n' "$db" >&2
    return 1
  fi
  if [ -n "$remote" ]; then
    if ! valid_remote_name "$remote"; then
      printf 'compact: db=%s invalid remote name=%s — fail\n' "$db" "$remote" >&2
      return 1
    fi

    refspec_pair=$(resolve_refspec_sql "$db") || return 1
    local_branch=$(printf '%s\n' "$refspec_pair" | sed -n '1p')
    remote_branch=$(printf '%s\n' "$refspec_pair" | sed -n '2p')

    printf 'compact: db=%s remote=%s — fetching before flatten...\n' "$db" "$remote"
    fetch_rc=0
    fetch_err_tmp=$(mktemp)
    fetch_remote "$db" "$remote" >/dev/null 2>"$fetch_err_tmp" || fetch_rc=$?
    if [ "$fetch_rc" -ne 0 ]; then
      printf 'compact: db=%s remote=%s fetch failed rc=%s — proceeding from local source of truth\n' \
        "$db" "$remote" "$fetch_rc" >&2
      emit_error_file "$db" "$fetch_err_tmp"
    else
      if ! remote_head=$(remote_branch_head "$db" "$remote" "$remote_branch"); then
        rm -f "$fetch_err_tmp"
        return 1
      fi
      expected_remote_head="$remote_head"
      if [ -n "$remote_head" ] && [ "$remote_head" != "$head" ]; then
        case "$remote_head" in
          *[!A-Za-z0-9]*)
            printf 'compact: db=%s remote=%s returned invalid HEAD=%s — fail\n' \
              "$db" "$remote" "$remote_head" >&2
            rm -f "$fetch_err_tmp"
            return 1
            ;;
        esac
        if ! in_local=$(commit_exists_in_local_log "$db" "$remote_head"); then
          rm -f "$fetch_err_tmp"
          return 1
        fi
        if [ "$in_local" != "1" ]; then
          printf 'compact: db=%s remote=%s remote HEAD=%s is not in local history — proceeding from local source of truth; remote push will remain pending\n' \
            "$db" "$remote" "$remote_head" >&2
        else
          expected_remote_head_verified=1
          printf 'compact: db=%s remote=%s fetch ok\n' "$db" "$remote"
        fi
      elif [ "$remote_head" = "$head" ]; then
        expected_remote_head_verified=1
        printf 'compact: db=%s remote=%s fetch ok\n' "$db" "$remote"
      else
        expected_remote_head_verified=0
        printf 'compact: db=%s remote=%s fetch ok; remote HEAD empty — push will verify after local compaction\n' "$db" "$remote"
      fi
    fi
    rm -f "$fetch_err_tmp"
  fi

  ensure_repair_marker_paths_writable "$db" "$remote" || return 1

  # Race window: between the `head` capture above and the flatten transaction
  # below, a busy database (notably hq, where many writers commit constantly)
  # may move HEAD. The post-flatten value-hash check then fails or defers,
  # depending on the integrity category. Retry preflight up to 3 times with
  # jittered 1-5s sleep,
  # refreshing HEAD between attempts; require HEAD to stay stable across a
  # preflight gather before flattening. This narrows but does not eliminate the
  # race: a writer can still commit between the final HEAD check and DOLT_RESET.
  # Pure row-count gain plus hash drift is deferred to pending GC so the next
  # order can retry; stricter integrity failures still quarantine.
  preflight_tmp=$(mktemp)
  preflight_max_attempts=3
  preflight_attempt=1
  preflight_succeeded=false
  current_head=""
  while [ "$preflight_attempt" -le "$preflight_max_attempts" ]; do
    if [ "$preflight_attempt" -gt 1 ]; then
      if ! head=$(head_commit "$db"); then
        rm -f "$preflight_tmp"
        return 1
      fi
      if [ -z "$head" ]; then
        printf 'compact: db=%s HEAD commit probe returned empty value during retry — fail\n' "$db" >&2
        rm -f "$preflight_tmp"
        return 1
      fi
      compacted_from_head="$head"
    fi

    : > "$preflight_tmp"
    if ! preflight_counts "$db" "$preflight_tmp"; then
      rm -f "$preflight_tmp"
      return 1
    fi
    if ! preflight_hash=$(db_value_hash "$db"); then
      rm -f "$preflight_tmp"
      return 1
    fi
    if [ -z "$preflight_hash" ]; then
      printf 'compact: db=%s pre-flatten value hash probe returned empty value — fail\n' "$db" >&2
      rm -f "$preflight_tmp"
      return 1
    fi

    if ! current_head=$(head_commit "$db"); then
      rm -f "$preflight_tmp"
      return 1
    fi
    if [ -z "$current_head" ]; then
      printf 'compact: db=%s HEAD commit probe returned empty value during preflight verify — fail\n' "$db" >&2
      rm -f "$preflight_tmp"
      return 1
    fi
    if [ "$current_head" = "$head" ]; then
      preflight_succeeded=true
      break
    fi

    if [ "$preflight_attempt" -lt "$preflight_max_attempts" ]; then
      printf 'compact: db=%s HEAD moved during preflight attempt=%s/%s want_HEAD=%s got_HEAD=%s — retrying\n' \
        "$db" "$preflight_attempt" "$preflight_max_attempts" "$head" "${current_head:-<empty>}" >&2
      sleep "$(awk 'BEGIN{srand(); printf "%d", 1 + rand() * 5}')"
    fi
    preflight_attempt=$((preflight_attempt + 1))
  done

  if [ "$preflight_succeeded" != "true" ]; then
    printf 'compact: db=%s HEAD kept moving across %s preflight attempts last_want_HEAD=%s last_got_HEAD=%s — aborting before flatten\n' \
      "$db" "$preflight_max_attempts" "$head" "${current_head:-<empty>}" >&2
    rm -f "$preflight_tmp"
    return 1
  fi

  table_count=$(wc -l < "$preflight_tmp")
  printf 'compact: db=%s commits=%s root=%s tables=%s — flattening...\n' \
    "$db" "$count" "$root" "$table_count"

  start=$(date +%s)

  # Capture HEAD one last time immediately before the mutating flatten. The
  # preflight loop already proved HEAD == "$head" stayed stable across the
  # snapshot gather, so this probe runs strictly BEFORE the flatten's own
  # DOLT_RESET/DOLT_COMMIT — any difference from "$head" here can only be an
  # external writer that committed inside the residual preflight->reset window,
  # never the flatten's own commit (which has not happened yet). An empty/failed
  # probe leaves head_before_reset empty, which keeps the writer-race proof
  # unavailable. Pure gain+drift may still defer; stricter failure categories
  # still fall back to quarantine.
  head_before_reset=$(head_commit "$db" || true)

  # Soft-reset to root + commit-everything is the flatten transaction.
  # Both run in a single dolt sql invocation so the session keeps the
  # USE selection across the two CALLs.
  reset_rc=0
  reset_err_tmp=$(mktemp)
  dolt_query "$db" "
    CALL DOLT_RESET('--soft', '$root');
    CALL DOLT_COMMIT('-Am', 'compaction: flatten history');
  " >/dev/null 2>"$reset_err_tmp" || reset_rc=$?

  if [ "$reset_rc" -ne 0 ]; then
    printf 'compact: db=%s flatten failed rc=%s — restoring pre-flatten HEAD=%s\n' \
      "$db" "$reset_rc" "$head" >&2
    emit_error_file "$db" "$reset_err_tmp"
    rm -f "$preflight_tmp"
    rm -f "$reset_err_tmp"
    restore_head_after_flatten_failure "$db" "$head" "$root" || true
    return 1
  fi
  rm -f "$reset_err_tmp"

  flatten_head=$(head_commit "$db" || true)
  if [ -z "$flatten_head" ]; then
    printf 'compact: db=%s post-flatten HEAD probe failed — quarantine and investigate before GC\n' \
      "$db" >&2
    write_compact_marker "$quarantine_dir" "$db" "post-flatten HEAD probe failed" || {
      rm -f "$preflight_tmp"
      return 1
    }
    rm -f "$preflight_tmp"
    return 1
  fi

  verify_counts_rc=0
  verify_counts "$db" "$preflight_tmp" || verify_counts_rc=$?

  # Writer-race gate (local-verify HEAD-stability). A normal MVCC writer (the
  # beads/mail workload) can commit to this db inside the flatten window, which
  # legitimately adds rows and changes value hashes versus the pre-flight
  # snapshot. That is a benign, self-healing condition — the next scheduled run
  # retries — and must NOT be permanently quarantined (a quarantine marker blocks
  # all future GC of the db and is the production memory-exhaustion bug).
  #
  # We distinguish a writer commit from the flatten's OWN commit using two
  # independent signals, both anchored so the flatten's own commit never trips
  # them:
  #   * head_before_reset != head  — HEAD moved between the stable pre-flight
  #     snapshot and the pre-reset probe. That probe runs before the flatten
  #     mutates anything, so only an external writer can have moved HEAD.
  #   * post_verify_head != flatten_head — HEAD moved past the flatten's own
  #     commit during/after verify_counts. The script issues no commit between
  #     the flatten and this probe, so only an external writer can have moved it.
  # Either signal proves a concurrent writer. If a HEAD probe fails/returns
  # empty we leave the corresponding value empty and the equality below cannot
  # become true. The pure gain+drift case still defers because the final
  # probe->reset window cannot be closed; all stricter failures quarantine.
  post_verify_head=$(head_commit "$db" || true)
  writer_race_detected=0
  if [ -n "$head" ] && [ -n "$head_before_reset" ] && [ "$head_before_reset" != "$head" ]; then
    writer_race_detected=1
    compacted_from_head="$head_before_reset"
  fi
  if [ -n "$flatten_head" ] && [ -n "$post_verify_head" ] && [ "$post_verify_head" != "$flatten_head" ]; then
    writer_race_detected=1
  fi

  if [ "$verify_counts_rc" -ne 0 ]; then
    integrity_reason="${verify_counts_failure_reason:-post-flatten integrity check failed}"
    integrity_guidance="${verify_counts_failure_guidance:-post-flatten integrity check failed; investigate before re-running}"
    # Downgrade quarantine -> defer for the pure ambiguous gain+drift case.
    # A stable HEAD does not prove corruption because a writer can land in the
    # residual window between the last pre-reset HEAD probe and DOLT_RESET.
    # Every stricter integrity failure (row-count decrease, same-count hash
    # drift, table-list drift, probe failure) still quarantines below unchanged.
    if [ "${verify_counts_saw_gain:-0}" = "1" ] && \
       [ "${verify_counts_saw_gain_hash_drift:-0}" = "1" ] && \
       [ "${verify_counts_saw_row_decrease:-0}" != "1" ] && \
       [ "${verify_counts_saw_same_count_hash_drift:-0}" != "1" ] && \
       [ "${verify_counts_saw_table_list_change:-0}" != "1" ] && \
       [ "${verify_counts_saw_probe_failure:-0}" != "1" ]; then
      if [ "$writer_race_detected" = "1" ]; then
        printf 'compact: db=%s writer race detected during flatten (snapshot_HEAD=%s pre_reset_HEAD=%s flatten_HEAD=%s post_verify_HEAD=%s) — table value hash drift with row-count increase is concurrent-writer data, not corruption; deferring, will retry next run\n' \
          "$db" "$head" "${head_before_reset:-<empty>}" "$flatten_head" "${post_verify_head:-<empty>}" >&2
      else
        printf 'compact: db=%s possible writer race during flatten (snapshot_HEAD=%s pre_reset_HEAD=%s flatten_HEAD=%s post_verify_HEAD=%s) — table value hash drift with row-count increase hit the unobservable pre-reset window; deferring, will retry next run\n' \
          "$db" "$head" "${head_before_reset:-<empty>}" "$flatten_head" "${post_verify_head:-<empty>}" >&2
      fi
      if ! defer_writer_race_after_flatten "$db" "$flatten_head" \
        "$remote" "$expected_remote_head" "$expected_remote_head_verified" \
        "$compacted_from_head" "$local_branch" "$remote_branch"; then
        rm -f "$preflight_tmp"
        return 1
      fi
      rm -f "$preflight_tmp"
      return 0
    fi
    if [ "$writer_race_detected" = "1" ] && \
       [ "${verify_counts_saw_gain_hash_drift:-0}" = "1" ]; then
      printf 'compact: db=%s writer race detected during flatten (snapshot_HEAD=%s pre_reset_HEAD=%s flatten_HEAD=%s post_verify_HEAD=%s), but additional integrity failure category prevents defer; quarantine unchanged\n' \
        "$db" "$head" "${head_before_reset:-<empty>}" "$flatten_head" "${post_verify_head:-<empty>}" >&2
    fi
    printf 'compact: db=%s post-flatten INTEGRITY check failed — escalate (%s)\n' \
      "$db" "$integrity_guidance" >&2
    write_compact_marker "$quarantine_dir" "$db" "$integrity_reason" || {
      preserve_head_after_integrity_failure "$db" "$flatten_head" || true
      rm -f "$preflight_tmp"
      return 1
    }
    preserve_head_after_integrity_failure "$db" "$flatten_head" || true
    rm -f "$preflight_tmp"
    return 1
  fi
  pre_db_hash_head=$(head_commit "$db" || true)
  if ! postflight_hash=$(db_value_hash "$db"); then
    printf 'compact: db=%s post-flatten value hash probe failed — quarantine and investigate before GC\n' \
      "$db" >&2
    write_compact_marker "$quarantine_dir" "$db" "post-flatten value hash probe failed" || {
      preserve_head_after_integrity_failure "$db" "$flatten_head" || true
      rm -f "$preflight_tmp"
      return 1
    }
    preserve_head_after_integrity_failure "$db" "$flatten_head" || true
    rm -f "$preflight_tmp"
    return 1
  fi
  if [ -z "$postflight_hash" ]; then
    printf 'compact: db=%s post-flatten value hash probe returned empty value — quarantine and investigate before GC\n' \
      "$db" >&2
    write_compact_marker "$quarantine_dir" "$db" "post-flatten value hash probe returned empty value" || {
      preserve_head_after_integrity_failure "$db" "$flatten_head" || true
      rm -f "$preflight_tmp"
      return 1
    }
    preserve_head_after_integrity_failure "$db" "$flatten_head" || true
    rm -f "$preflight_tmp"
    return 1
  fi
  post_db_hash_head=$(head_commit "$db" || true)
  db_hash_writer_race_detected=0
  if [ -n "$flatten_head" ] && [ -n "$pre_db_hash_head" ] && [ "$pre_db_hash_head" != "$flatten_head" ]; then
    db_hash_writer_race_detected=1
  fi
  if [ -n "$flatten_head" ] && [ -n "$post_db_hash_head" ] && [ "$post_db_hash_head" != "$flatten_head" ]; then
    db_hash_writer_race_detected=1
  fi
  if [ -n "$pre_db_hash_head" ] && [ -n "$post_db_hash_head" ] && [ "$post_db_hash_head" != "$pre_db_hash_head" ]; then
    db_hash_writer_race_detected=1
  fi
  if [ "$db_hash_writer_race_detected" = "1" ]; then
    writer_race_detected=1
  fi
  if [ "$postflight_hash" != "$preflight_hash" ]; then
    if [ "$db_hash_writer_race_detected" = "1" ]; then
      # The DB hash probe runs after table-level verification has already
      # passed. HEAD movement across this probe means an external writer may
      # have changed any value without changing the checked table row counts.
      db_hash_drift_detail="database value hash drift"
      if [ "${verify_counts_saw_gain:-0}" = "1" ]; then
        db_hash_drift_detail="database value hash drift with row-count increase"
      fi
      printf 'compact: db=%s writer race detected during flatten (snapshot_HEAD=%s pre_reset_HEAD=%s flatten_HEAD=%s post_verify_HEAD=%s pre_db_hash_HEAD=%s post_db_hash_HEAD=%s) — %s is concurrent-writer data, not corruption; deferring, will retry next run\n' \
        "$db" "$head" "${head_before_reset:-<empty>}" "$flatten_head" "${post_verify_head:-<empty>}" "${pre_db_hash_head:-<empty>}" "${post_db_hash_head:-<empty>}" "$db_hash_drift_detail" >&2
      if ! defer_writer_race_after_flatten "$db" "$flatten_head" \
        "$remote" "$expected_remote_head" "$expected_remote_head_verified" \
        "$compacted_from_head" "$local_branch" "$remote_branch"; then
        rm -f "$preflight_tmp"
        return 1
      fi
      rm -f "$preflight_tmp"
      return 0
    fi
    if [ "${verify_counts_saw_gain:-0}" = "1" ]; then
      # Same downgrade as the per-table gain+drift case above: a concurrent
      # writer that added rows also shifts the whole-database value hash. The
      # residual pre-reset window can leave HEAD stable even though writer data
      # was included in the flatten, so pure gain+drift defers instead of
      # quarantining.
      if [ "$writer_race_detected" = "1" ]; then
        printf 'compact: db=%s writer race detected during flatten (snapshot_HEAD=%s pre_reset_HEAD=%s flatten_HEAD=%s post_verify_HEAD=%s) — database value hash drift with row-count increase is concurrent-writer data, not corruption; deferring, will retry next run\n' \
          "$db" "$head" "${head_before_reset:-<empty>}" "$flatten_head" "${post_verify_head:-<empty>}" >&2
      else
        printf 'compact: db=%s possible writer race during flatten (snapshot_HEAD=%s pre_reset_HEAD=%s flatten_HEAD=%s post_verify_HEAD=%s) — database value hash drift with row-count increase hit the unobservable pre-reset window; deferring, will retry next run\n' \
          "$db" "$head" "${head_before_reset:-<empty>}" "$flatten_head" "${post_verify_head:-<empty>}" >&2
      fi
      if ! defer_writer_race_after_flatten "$db" "$flatten_head" \
        "$remote" "$expected_remote_head" "$expected_remote_head_verified" \
        "$compacted_from_head" "$local_branch" "$remote_branch"; then
        rm -f "$preflight_tmp"
        return 1
      fi
      rm -f "$preflight_tmp"
      return 0
    else
      printf 'compact: db=%s value hash changed without row-count increase before=%s after=%s — quarantine and investigate before GC\n' \
        "$db" "$preflight_hash" "$postflight_hash" >&2
      write_compact_marker "$quarantine_dir" "$db" "post-flatten value hash changed without row-count increase" || {
        preserve_head_after_integrity_failure "$db" "$flatten_head" || true
        rm -f "$preflight_tmp"
        return 1
      }
      preserve_head_after_integrity_failure "$db" "$flatten_head" || true
      rm -f "$preflight_tmp"
      return 1
    fi
  fi
  if [ "${verify_counts_saw_gain:-0}" = "1" ]; then
    printf 'compact: db=%s row-count increase passed value-hash verification — full GC allowed\n' \
      "$db"
  fi
  rm -f "$preflight_tmp"

  after_count=$(commit_count "$db" || true)

  # CALL DOLT_GC() alone only reclaims working-set chunks — the bulk of
  # the orphaned history lives in noms/oldgen/ archives that require
  # --full to rewrite. Since flatten always orphans the entire prior
  # commit graph, --full is always appropriate here.
  if run_full_gc "$db" "flatten ok commits=$count->${after_count:-?} but" \
    "commits=$count->${after_count:-?}" "$start"; then
    clear_compact_marker "$pending_gc_dir" "$db"
    push_remote_after_compaction "$db" "$remote" "$expected_remote_head" "$expected_remote_head_verified" "initial" "$compacted_from_head" "$local_branch" "$remote_branch"
    return $?
  fi
  write_compact_marker "$pending_gc_dir" "$db" "flatten succeeded but full GC failed" \
    "remote=$remote" "expected_remote_head=$expected_remote_head" \
    "expected_remote_head_verified=$expected_remote_head_verified" \
    "compacted_from_head=$compacted_from_head" \
    "local_branch=$local_branch" \
    "remote_branch=$remote_branch" || return 1
  return 1
}

# shellcheck disable=SC2317
cleanup() {
  if [ "$flock_acquired" = "1" ]; then
    flock -u 9 2>/dev/null || true
    exec 9>&- 2>/dev/null || true
    rm -f "$lock_path" 2>/dev/null || true
  fi
  if [ -n "$lock_cleanup" ]; then
    rm -f "$lock_pid_path" "$lock_cmd_path" 2>/dev/null || true
    rmdir "$lock_cleanup" 2>/dev/null || true
  fi
  if [ -n "${_meta_tmp:-}" ]; then
    rm -f "$_meta_tmp"
  fi
  if [ -n "${_db_tmp:-}" ]; then
    rm -f "$_db_tmp"
  fi
  if [ -n "${_unique_db_tmp:-}" ]; then
    rm -f "$_unique_db_tmp"
  fi
}

lock_process_command() {
  pid="$1"
  command -v ps >/dev/null 2>&1 || return 1
  ps -p "$pid" -o command= 2>/dev/null | sed -n '1p'
}

lock_holder_alive() {
  [ -f "$lock_pid_path" ] || return 1
  pid=$(sed -n '1p' "$lock_pid_path" 2>/dev/null || true)
  case "$pid" in
    ''|*[!0-9]*) return 1 ;;
  esac

  current_cmd=$(lock_process_command "$pid" || true)
  if [ -f "$lock_cmd_path" ]; then
    expected_cmd=$(sed -n '1p' "$lock_cmd_path" 2>/dev/null || true)
    if [ -n "$current_cmd" ] && [ "$current_cmd" = "$expected_cmd" ]; then
      return 0
    fi
    if [ -n "$current_cmd" ]; then
      return 1
    fi
  fi

  if kill -0 "$pid" 2>/dev/null; then
    return 0
  fi
  [ -n "$current_cmd" ]
}

claim_lock_dir() {
  old_umask=$(umask)
  umask 077
  if ! mkdir "$lock_dir" 2>/dev/null; then
    umask "$old_umask"
    return 1
  fi
  if ! printf '%s\n' "$$" > "$lock_pid_path"; then
    umask "$old_umask"
    rmdir "$lock_dir" 2>/dev/null || true
    printf 'compact: unable to write lock metadata %s\n' "$lock_pid_path" >&2
    exit 1
  fi
  lock_cmd=$(lock_process_command "$$" || true)
  if [ -n "$lock_cmd" ]; then
    printf '%s\n' "$lock_cmd" > "$lock_cmd_path" 2>/dev/null || true
  fi
  umask "$old_umask"
  lock_cleanup="$lock_dir"
  return 0
}

clear_stale_lock_dir() {
  [ -d "$lock_dir" ] || return 0
  if [ ! -f "$lock_pid_path" ]; then
    sleep 1
  fi
  if lock_holder_alive; then
    return 1
  fi
  rm -f "$lock_pid_path" "$lock_cmd_path" 2>/dev/null || true
  rmdir "$lock_dir" 2>/dev/null
}

acquire_lock() {
  if command -v flock >/dev/null 2>&1; then
    old_umask=$(umask)
    umask 077
    if ! : >> "$lock_path" 2>/dev/null; then
      umask "$old_umask"
      if [ -d "$lock_path" ]; then
        return 1
      fi
      printf 'compact: unable to create lock file %s\n' "$lock_path" >&2
      exit 1
    fi
    if ! exec 9<>"$lock_path"; then
      umask "$old_umask"
      if [ -d "$lock_path" ]; then
        return 1
      fi
      printf 'compact: unable to open lock file %s\n' "$lock_path" >&2
      exit 1
    fi
    umask "$old_umask"
    chmod 600 "$lock_path" 2>/dev/null || true
    if ! flock -n 9; then
      return 1
    fi
    flock_acquired=1
    if claim_lock_dir; then
      return 0
    fi
    if [ -d "$lock_dir" ] && clear_stale_lock_dir && claim_lock_dir; then
      return 0
    fi
    return 1
  fi

  if claim_lock_dir; then
    return 0
  fi
  if [ -d "$lock_dir" ] && clear_stale_lock_dir && claim_lock_dir; then
    return 0
  fi
  if [ -d "$lock_dir" ]; then
    return 1
  fi

  printf 'compact: unable to create lock directory %s\n' "$lock_dir" >&2
  exit 1
}

main() {
  lock_cleanup=""
  flock_acquired=""
  _meta_tmp=""
  _db_tmp=""
  _unique_db_tmp=""
  trap cleanup EXIT

  # Non-blocking host:port lock. Skip rather than queue up; the other
  # compactor is handling this Dolt server.
  if ! acquire_lock; then
    printf 'compact: another compaction already running for %s:%s — skipping\n' \
      "$host" "$GC_DOLT_PORT"
    exit 0
  fi

  _meta_tmp=$(mktemp)
  if [ "$hosted_server_compact" != "1" ]; then
    metadata_files > "$_meta_tmp"
  fi

  _db_tmp=$(mktemp)
  _unique_db_tmp=$(mktemp)
  if [ "$hosted_server_compact" = "1" ]; then
    discover_server_database_names > "$_db_tmp"
  else
    discover_database_names > "$_db_tmp"
  fi

  seen_dbs=""
  while IFS= read -r db; do
    [ -n "$db" ] || continue
    case " $seen_dbs " in
      *" $db "*) continue ;;
    esac
    seen_dbs="$seen_dbs $db"
    printf '%s\n' "$db" >> "$_unique_db_tmp"
  done < "$_db_tmp"

  if [ "$hosted_server_compact" = "1" ]; then
    run_hosted_server_compact "$_unique_db_tmp"
    exit $?
  fi

  failed_count=0
  while IFS= read -r db; do
    [ -n "$db" ] || continue
    if ! flatten_database "$db"; then
      failed_count=$((failed_count + 1))
    fi
  done < "$_unique_db_tmp"

  if [ "$failed_count" -gt 0 ]; then
    printf 'compact: %s database(s) failed compaction\n' "$failed_count" >&2
    exit 1
  fi
  exit 0
}

main "$@"
