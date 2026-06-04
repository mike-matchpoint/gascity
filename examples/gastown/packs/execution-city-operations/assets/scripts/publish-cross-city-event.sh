#!/usr/bin/env bash
# Deterministic cross-city event emitter for execution-and-monitoring cities.
#
# Builds the canonical envelope (schemas/events/common-envelope.v1), validates
# the full event against its versioned schema, then publishes it to the GasCity
# global event bus via EventBridge. This is the ONLY approved publish path; do
# not hand-roll `aws events put-events` from agent prompts.
#
# Infra (the bus, egress IAM role, and the env below) is provisioned by the
# hosting Kubernetes harness, not by this pack.
#
# Usage:
#   publish-cross-city-event.sh \
#     --event-type RepoBugReported.v1 \
#     --payload-file /path/to/payload.json \
#     --target-city vehicle-graph-code-generation-city-dev \
#     [--correlation-id UUID] [--idempotency-key KEY | --dedupe-key STR] \
#     [--process-slug SLUG] [--city-pair-slug SLUG] [--dry-run]
#
# Required env (injected by the harness; overridable by flags):
#   GASCITY_EVENT_BUS     EventBridge bus name (e.g. GasCity-EventBus-Dev)
#   AWS_REGION            AWS region
#   GASCITY_SOURCE_CITY   this city's name (e.g. vehicle-graph-execution-monitoring-city-dev)
# Optional env:
#   GASCITY_SOURCE_CITY_ROLE (default: execution-monitoring-city)
#   GASCITY_PROCESS_SLUG, GASCITY_CITY_PAIR_SLUG, GASCITY_EVENT_SOURCE
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PACK_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
SCHEMA_DIR="$PACK_DIR/schemas/events"

die() { printf 'publish-cross-city-event: %s\n' "$*" >&2; exit 1; }

EVENT_TYPE=""
PAYLOAD_FILE=""
TARGET_CITY=""
CORRELATION_ID=""
IDEMPOTENCY_KEY=""
DEDUPE_KEY=""
PROCESS_SLUG="${GASCITY_PROCESS_SLUG:-}"
CITY_PAIR_SLUG="${GASCITY_CITY_PAIR_SLUG:-}"
DRY_RUN=0

while [ $# -gt 0 ]; do
  case "$1" in
    --event-type) EVENT_TYPE="$2"; shift 2 ;;
    --payload-file) PAYLOAD_FILE="$2"; shift 2 ;;
    --target-city) TARGET_CITY="$2"; shift 2 ;;
    --correlation-id) CORRELATION_ID="$2"; shift 2 ;;
    --idempotency-key) IDEMPOTENCY_KEY="$2"; shift 2 ;;
    --dedupe-key) DEDUPE_KEY="$2"; shift 2 ;;
    --process-slug) PROCESS_SLUG="$2"; shift 2 ;;
    --city-pair-slug) CITY_PAIR_SLUG="$2"; shift 2 ;;
    --dry-run) DRY_RUN=1; shift ;;
    *) die "unknown argument: $1" ;;
  esac
done

command -v jq >/dev/null 2>&1 || die "jq is required"
[ -n "$EVENT_TYPE" ] || die "--event-type is required"
[ -n "$PAYLOAD_FILE" ] || die "--payload-file is required"
[ -f "$PAYLOAD_FILE" ] || die "payload file not found: $PAYLOAD_FILE"
jq -e . "$PAYLOAD_FILE" >/dev/null 2>&1 || die "payload file is not valid JSON: $PAYLOAD_FILE"
[ -n "$TARGET_CITY" ] || die "--target-city is required"

SOURCE_CITY="${GASCITY_SOURCE_CITY:-}"
[ -n "$SOURCE_CITY" ] || die "GASCITY_SOURCE_CITY env (or harness injection) is required"
SOURCE_CITY_ROLE="${GASCITY_SOURCE_CITY_ROLE:-execution-monitoring-city}"
PROCESS_SLUG="${PROCESS_SLUG:-vehicle-graph}"
CITY_PAIR_SLUG="${CITY_PAIR_SLUG:-${PROCESS_SLUG}-exec-codegen}"

# Map event type -> schema file.
case "$EVENT_TYPE" in
  RepoBugReported.v1) SCHEMA_FILE="$SCHEMA_DIR/repo-bug-reported.v1.schema.json" ;;
  RepoChangeRequested.v1) SCHEMA_FILE="$SCHEMA_DIR/repo-change-requested.v1.schema.json" ;;
  *) die "unsupported --event-type: $EVENT_TYPE (expected RepoBugReported.v1 or RepoChangeRequested.v1)" ;;
esac
[ -f "$SCHEMA_FILE" ] || die "schema not found: $SCHEMA_FILE"

gen_uuid() {
  if command -v uuidgen >/dev/null 2>&1; then uuidgen | tr 'A-Z' 'a-z'; return; fi
  if [ -r /proc/sys/kernel/random/uuid ]; then cat /proc/sys/kernel/random/uuid; return; fi
  # Fallback: hash-based pseudo-uuid.
  printf '%s' "$(date -u +%s%N)-$RANDOM" | shasum -a 256 | cut -c1-32 \
    | sed -E 's/(.{8})(.{4})(.{4})(.{4})(.{12})/\1-\2-\3-\4-\5/'
}

sha256_of() {
  if command -v shasum >/dev/null 2>&1; then shasum -a 256 | cut -d' ' -f1
  else sha256sum | cut -d' ' -f1; fi
}

OCCURRED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
[ -n "$CORRELATION_ID" ] || CORRELATION_ID="$(gen_uuid)"

# Deterministic idempotency key: explicit > dedupe-key > sha256(normalized payload).
if [ -z "$IDEMPOTENCY_KEY" ]; then
  if [ -n "$DEDUPE_KEY" ]; then
    DEDUPE_SCOPE="$DEDUPE_KEY"
  else
    DEDUPE_SCOPE="$(jq -cS . "$PAYLOAD_FILE" | sha256_of)"
  fi
  IDEMPOTENCY_KEY="${PROCESS_SLUG}:${CITY_PAIR_SLUG}:${EVENT_TYPE}:${DEDUPE_SCOPE}"
fi

TMP_ENVELOPE="$(mktemp "${TMPDIR:-/tmp}/gascity-xcity-event.XXXXXX.json")"
cleanup() { rm -f "$TMP_ENVELOPE"; }
trap cleanup EXIT

jq -n \
  --arg event_type "$EVENT_TYPE" \
  --arg process_slug "$PROCESS_SLUG" \
  --arg city_pair_slug "$CITY_PAIR_SLUG" \
  --arg source_city "$SOURCE_CITY" \
  --arg source_city_role "$SOURCE_CITY_ROLE" \
  --arg target_city "$TARGET_CITY" \
  --arg correlation_id "$CORRELATION_ID" \
  --arg idempotency_key "$IDEMPOTENCY_KEY" \
  --arg occurred_at "$OCCURRED_AT" \
  --slurpfile payload "$PAYLOAD_FILE" \
  '{
    event_type: $event_type,
    event_version: "v1",
    process_slug: $process_slug,
    city_pair_slug: $city_pair_slug,
    source_city: $source_city,
    source_city_role: $source_city_role,
    target_city: $target_city,
    target_city_role: "code-generation-city",
    correlation_id: $correlation_id,
    idempotency_key: $idempotency_key,
    occurred_at: $occurred_at,
    payload: $payload[0]
  }' > "$TMP_ENVELOPE"

# Validate against the versioned schema. Prefer a real JSON Schema validator;
# fall back to a jq required-field check so the script still guards in minimal
# runtimes (the parity test exercises the full validator in CI).
validate_envelope() {
  if command -v python3 >/dev/null 2>&1 && python3 -c "import jsonschema" >/dev/null 2>&1; then
    python3 - "$SCHEMA_FILE" "$TMP_ENVELOPE" <<'PY'
import json, sys
import jsonschema
schema = json.load(open(sys.argv[1]))
instance = json.load(open(sys.argv[2]))
jsonschema.validate(instance=instance, schema=schema)
PY
    return $?
  fi
  # Fallback: envelope + payload required-field presence via jq.
  local k
  for k in $(jq -r '.required[]' "$SCHEMA_FILE"); do
    jq -e "has(\"$k\")" "$TMP_ENVELOPE" >/dev/null || { echo "missing envelope field: $k" >&2; return 1; }
  done
  for k in $(jq -r '.properties.payload.required[]? // empty' "$SCHEMA_FILE"); do
    jq -e ".payload | has(\"$k\")" "$TMP_ENVELOPE" >/dev/null || { echo "missing payload field: $k" >&2; return 1; }
  done
  return 0
}

if ! validate_envelope; then
  die "event failed schema validation against $(basename "$SCHEMA_FILE")"
fi

if [ "$DRY_RUN" -eq 1 ]; then
  echo "[dry-run] validated $EVENT_TYPE against $(basename "$SCHEMA_FILE")"
  echo "[dry-run] idempotency_key=$IDEMPOTENCY_KEY"
  echo "[dry-run] correlation_id=$CORRELATION_ID"
  echo "[dry-run] would put-events to bus=${GASCITY_EVENT_BUS:-<unset>} region=${AWS_REGION:-<unset>}"
  echo "[dry-run] envelope:"
  jq . "$TMP_ENVELOPE"
  exit 0
fi

[ -n "${GASCITY_EVENT_BUS:-}" ] || die "GASCITY_EVENT_BUS env is required for live publish"
[ -n "${AWS_REGION:-}" ] || die "AWS_REGION env is required for live publish"
command -v aws >/dev/null 2>&1 || die "aws CLI is required for live publish"

ENTRY="$(jq -n \
  --arg bus "$GASCITY_EVENT_BUS" \
  --arg source "${GASCITY_EVENT_SOURCE:-gascity.${SOURCE_CITY}}" \
  --arg detail_type "$EVENT_TYPE" \
  --rawfile detail "$TMP_ENVELOPE" \
  '[{ EventBusName: $bus, Source: $source, DetailType: $detail_type, Detail: $detail }]')"

RESULT="$(aws events put-events --region "$AWS_REGION" --entries "$ENTRY" --output json)"
FAILED="$(printf '%s' "$RESULT" | jq -r '.FailedEntryCount // 0')"
if [ "$FAILED" != "0" ]; then
  printf '%s\n' "$RESULT" >&2
  die "put-events reported $FAILED failed entries"
fi
EVENT_ID="$(printf '%s' "$RESULT" | jq -r '.Entries[0].EventId // "unknown"')"
echo "published $EVENT_TYPE event_id=$EVENT_ID idempotency_key=$IDEMPOTENCY_KEY correlation_id=$CORRELATION_ID"
