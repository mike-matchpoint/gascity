#!/bin/sh
# gc dolt retention-sweep - prune closed operational churn from a quiesced local Dolt store.
#
# This command is intentionally maintenance-only. It refuses non-local Dolt
# endpoints because deleting rows through the live AWS Service would compete
# with GasCity writers and can make an already saturated server worse. Mount the
# existing Dolt PVC at <city>/.beads/dolt, quiesce writers, then run from the
# maintenance job.
set -eu

: "${GC_CITY_PATH:?GC_CITY_PATH must be set}"

retention_class=""
older_than=""
single_commit=false
json=false
max_delete="${GC_DOLT_RETENTION_SWEEP_MAX_DELETE:-200000}"

usage() {
  cat <<'EOF'
Usage: gc dolt retention-sweep --class operational_churn --older-than 48h --single-commit [--json]

Deletes only closed, unpinned operational-churn beads older than the retention
window. --single-commit is required and commits all row deletions with one
bd dolt commit.
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --class)
      retention_class="${2:-}"
      shift 2
      ;;
    --older-than)
      older_than="${2:-}"
      shift 2
      ;;
    --single-commit)
      single_commit=true
      shift
      ;;
    --json)
      json=true
      shift
      ;;
    --max-delete)
      max_delete="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      printf 'retention-sweep: unknown flag: %s\n' "$1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

case "$retention_class" in
  operational_churn)
    ;;
  "")
    printf 'retention-sweep: --class is required\n' >&2
    exit 2
    ;;
  *)
    printf 'retention-sweep: unsupported --class %s\n' "$retention_class" >&2
    exit 2
    ;;
esac

[ -n "$older_than" ] || {
  printf 'retention-sweep: --older-than is required\n' >&2
  exit 2
}

case "$max_delete" in
  ''|*[!0-9]*)
    printf 'retention-sweep: invalid --max-delete %s\n' "$max_delete" >&2
    exit 2
    ;;
esac

if [ "$single_commit" != true ]; then
  printf 'retention-sweep: --single-commit is required\n' >&2
  exit 2
fi

not_applicable() {
  reason="$1"
  printf 'retention-sweep: not_applicable reason=%s\n' "$reason" >&2
  exit 2
}

case "${GC_DOLT_MANAGED_LOCAL:-}" in
  1|true|TRUE|yes|YES)
    ;;
  0|false|FALSE|no|NO)
    not_applicable "managed_local_false"
    ;;
  *)
    not_applicable "managed_local_required"
    ;;
esac

host="${GC_DOLT_HOST:-}"
case "$host" in
  ''|127.0.0.1|localhost|0.0.0.0|::1|::|'[::1]'|'[::]')
    ;;
  *)
    not_applicable "non_local_host"
    ;;
esac

if ! command -v python3 >/dev/null 2>&1; then
  printf 'retention-sweep: python3 is required\n' >&2
  exit 2
fi
if ! command -v bd >/dev/null 2>&1; then
  printf 'retention-sweep: bd is required\n' >&2
  exit 2
fi

canonical_path() {
  python3 - "$1" <<'PY'
import os
import sys

print(os.path.realpath(sys.argv[1]))
PY
}

expected_data_dir="$GC_CITY_PATH/.beads/dolt"
if [ -n "${GC_DOLT_DATA_DIR:-}" ]; then
  if [ "$(canonical_path "$GC_DOLT_DATA_DIR")" != "$(canonical_path "$expected_data_dir")" ]; then
    not_applicable "data_dir_not_city_beads_dolt"
  fi
fi

if [ ! -d "$expected_data_dir" ]; then
  not_applicable "missing_city_beads_dolt"
fi

cutoff_utc=$(python3 - "$older_than" <<'PY'
from datetime import datetime, timedelta, timezone
import re
import sys

value = sys.argv[1].strip()
match = re.fullmatch(r"([1-9][0-9]*)([smhdw]?)", value)
if not match:
    raise SystemExit("invalid --older-than; use values like 48h, 2d, 1w")
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
) || {
  printf 'retention-sweep: invalid --older-than %s\n' "$older_than" >&2
  exit 2
}

cd "$GC_CITY_PATH"
export BEADS_DIR="$GC_CITY_PATH/.beads"
if [ -n "$host" ]; then
  export BEADS_DOLT_SERVER_HOST="$host"
fi
if [ -n "${GC_DOLT_PORT:-}" ]; then
  export BEADS_DOLT_SERVER_PORT="$GC_DOLT_PORT"
fi
if [ -n "${GC_DOLT_USER:-}" ]; then
  export BEADS_DOLT_SERVER_USER="$GC_DOLT_USER"
fi
export BEADS_DOLT_PASSWORD="${GC_DOLT_PASSWORD:-${BEADS_DOLT_PASSWORD:-}}"

required_schema_count=32
schema_count=$(bd sql --json "SELECT COUNT(*) AS c FROM information_schema.columns WHERE table_schema = DATABASE() AND CONCAT(table_name,'.',column_name) IN ('issues.id','issues.status','issues.metadata','issues.closed_at','issues.updated_at','issues.created_at','issues.pinned','labels.issue_id','labels.label','comments.issue_id','events.issue_id','issue_snapshots.issue_id','compaction_snapshots.issue_id','dependencies.issue_id','dependencies.depends_on_issue_id','dependencies.depends_on_wisp_id','child_counters.parent_id','wisps.id','wisps.status','wisps.metadata','wisps.closed_at','wisps.updated_at','wisps.created_at','wisps.pinned','wisp_labels.issue_id','wisp_labels.label','wisp_comments.issue_id','wisp_events.issue_id','wisp_dependencies.issue_id','wisp_dependencies.depends_on_issue_id','wisp_dependencies.depends_on_wisp_id','wisp_child_counters.parent_id');" \
  | python3 -c 'import json,sys; data=json.load(sys.stdin); print(int(data[0]["c"]))') || {
    printf 'retention-sweep: schema validation query failed\n' >&2
    exit 1
  }
if [ "$schema_count" -ne "$required_schema_count" ]; then
  printf 'retention-sweep: bd schema mismatch; saw %s/%s required columns\n' "$schema_count" "$required_schema_count" >&2
  exit 1
fi

issue_predicate="i.status = 'closed' AND COALESCE(i.pinned, 0) = 0 AND COALESCE(i.closed_at, i.updated_at, i.created_at) < '$cutoff_utc' AND (JSON_UNQUOTE(JSON_EXTRACT(COALESCE(i.metadata, JSON_OBJECT()), '$.\"gc.retention_class\"')) = '$retention_class' OR EXISTS (SELECT 1 FROM labels l WHERE l.issue_id = i.id AND (l.label = 'order-tracking' OR l.label LIKE 'order-run:%'))) AND NOT EXISTS (SELECT 1 FROM dependencies d WHERE d.issue_id = i.id OR d.depends_on_issue_id = i.id) AND NOT EXISTS (SELECT 1 FROM issues child_i WHERE child_i.id LIKE CONCAT(i.id, '.%') AND child_i.status <> 'closed') AND NOT EXISTS (SELECT 1 FROM wisps child_w WHERE child_w.id LIKE CONCAT(i.id, '.%') AND child_w.status <> 'closed')"
wisp_predicate="w.status = 'closed' AND COALESCE(w.pinned, 0) = 0 AND COALESCE(w.closed_at, w.updated_at, w.created_at) < '$cutoff_utc' AND (JSON_UNQUOTE(JSON_EXTRACT(COALESCE(w.metadata, JSON_OBJECT()), '$.\"gc.retention_class\"')) = '$retention_class' OR EXISTS (SELECT 1 FROM wisp_labels l WHERE l.issue_id = w.id AND (l.label = 'order-tracking' OR l.label LIKE 'order-run:%'))) AND NOT EXISTS (SELECT 1 FROM wisp_dependencies d WHERE d.issue_id = w.id OR d.depends_on_wisp_id = w.id) AND NOT EXISTS (SELECT 1 FROM dependencies d WHERE d.depends_on_wisp_id = w.id) AND NOT EXISTS (SELECT 1 FROM issues child_i WHERE child_i.id LIKE CONCAT(w.id, '.%') AND child_i.status <> 'closed') AND NOT EXISTS (SELECT 1 FROM wisps child_w WHERE child_w.id LIKE CONCAT(w.id, '.%') AND child_w.status <> 'closed')"

counts_json=$(bd sql --json "SELECT (SELECT COUNT(*) FROM issues i WHERE $issue_predicate) AS issues, (SELECT COUNT(*) FROM wisps w WHERE $wisp_predicate) AS wisps;")
counts=$(printf '%s\n' "$counts_json" | python3 -c 'import json,sys; row=json.load(sys.stdin)[0]; print(str(int(row["issues"]))+" "+str(int(row["wisps"])))') || {
  printf 'retention-sweep: failed to parse candidate counts\n' >&2
  exit 1
}
issue_candidates=${counts%% *}
wisp_candidates=${counts#* }
total_candidates=$((issue_candidates + wisp_candidates))

if [ "$total_candidates" -gt "$max_delete" ]; then
  printf 'retention-sweep: %s candidates exceeds --max-delete %s\n' "$total_candidates" "$max_delete" >&2
  exit 1
fi

if [ "$total_candidates" -eq 0 ]; then
  python3 - "$json" "$retention_class" "$older_than" "$cutoff_utc" "$issue_candidates" "$wisp_candidates" <<'PY'
import json
import sys

as_json = sys.argv[1] == "true"
issues = int(sys.argv[5])
wisps = int(sys.argv[6])
payload = {
    "schema": "gc.dolt.retention_sweep.v1",
    "ok": True,
    "applied": False,
    "class": sys.argv[2],
    "older_than": sys.argv[3],
    "cutoff_utc": sys.argv[4],
    "candidates": {"issues": issues, "wisps": wisps, "total": issues + wisps},
    "deleted": {"issues": 0, "wisps": 0, "total": 0},
}
if as_json:
    print(json.dumps(payload, separators=(",", ":")))
else:
    print(f"retention-sweep: {issues + wisps} candidate(s); nothing to delete")
PY
  exit 0
fi

sql_file=$(mktemp "${TMPDIR:-/tmp}/gc-dolt-retention-sweep.XXXXXX.sql")
trap 'rm -f "$sql_file"' EXIT
cat > "$sql_file" <<SQL
START TRANSACTION;
CREATE TEMPORARY TABLE gc_retention_sweep_issue_ids AS
  SELECT i.id FROM issues i WHERE $issue_predicate;
CREATE TEMPORARY TABLE gc_retention_sweep_wisp_ids AS
  SELECT w.id FROM wisps w WHERE $wisp_predicate;
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
SQL

bd sql "$(cat "$sql_file")" >/dev/null
bd dolt commit -m "gc dolt retention-sweep: $retention_class older than $older_than" >/dev/null

remaining_json=$(bd sql --json "SELECT (SELECT COUNT(*) FROM issues i WHERE $issue_predicate) AS issues, (SELECT COUNT(*) FROM wisps w WHERE $wisp_predicate) AS wisps;")
remaining=$(printf '%s\n' "$remaining_json" | python3 -c 'import json,sys; row=json.load(sys.stdin)[0]; print(str(int(row["issues"]))+" "+str(int(row["wisps"])))') || {
  printf 'retention-sweep: failed to parse remaining counts\n' >&2
  exit 1
}
issue_remaining=${remaining%% *}
wisp_remaining=${remaining#* }
issue_deleted=$((issue_candidates - issue_remaining))
wisp_deleted=$((wisp_candidates - wisp_remaining))
total_deleted=$((issue_deleted + wisp_deleted))

python3 - "$json" "$retention_class" "$older_than" "$cutoff_utc" "$issue_candidates" "$wisp_candidates" "$issue_deleted" "$wisp_deleted" <<'PY'
import json
import sys

as_json = sys.argv[1] == "true"
issues = int(sys.argv[5])
wisps = int(sys.argv[6])
deleted_issues = int(sys.argv[7])
deleted_wisps = int(sys.argv[8])
payload = {
    "schema": "gc.dolt.retention_sweep.v1",
    "ok": True,
    "applied": True,
    "class": sys.argv[2],
    "older_than": sys.argv[3],
    "cutoff_utc": sys.argv[4],
    "candidates": {"issues": issues, "wisps": wisps, "total": issues + wisps},
    "deleted": {
        "issues": deleted_issues,
        "wisps": deleted_wisps,
        "total": deleted_issues + deleted_wisps,
    },
}
if as_json:
    print(json.dumps(payload, separators=(",", ":")))
else:
    print(f"retention-sweep: deleted {deleted_issues + deleted_wisps} operational-churn bead(s)")
PY
