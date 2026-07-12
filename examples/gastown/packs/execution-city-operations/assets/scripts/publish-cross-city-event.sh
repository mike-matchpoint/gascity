#!/usr/bin/env bash
# Deterministic cross-city event emitter for execution-and-monitoring cities.
#
# Two event classes, both schema-registered below:
#
#   envelope events (city -> city): the script builds the canonical envelope
#     (schemas/events/common-envelope.v1), validates the full event against its
#     versioned schema, and publishes it. The receiving city's ingress adapter
#     turns it into a routed bead.
#
#   flat events (city -> domain runtime): the payload file IS the EventBridge
#     detail. No envelope is added; the event is published with the domain
#     event source (GASCITY_DOMAIN_EVENT_SOURCE, default "GasCity") so
#     deterministic domain bridges can match on Source + DetailType +
#     detail.target_env. Used for execution terminals and domain command
#     requests.
#
# The class is derived from the schema itself: schemas that require
# process_slug are envelope events; all others are flat. This pack registers
# the repo handoff event types below; OTHER packs register their own event
# types by shipping a schema file and passing --schema-file (see the
# domain-handoff pack), so new event families never modify this script.
#
# This is the ONLY approved publish path; do not hand-roll
# `aws events put-events` from agent prompts or order scripts.
#
# Infra (the bus, egress IAM role, and the env below) is provisioned by the
# hosting Kubernetes harness, not by this pack.
#
# Usage:
#   publish-cross-city-event.sh \
#     --event-type RepoBugReported.v1 \
#     --payload-file /path/to/payload.json \
#     [--schema-file /path/to/event.schema.json] \
#     [--target-city sample-code-generation-city-dev] \
#     [--target-city-role execution-monitoring-city] \
#     [--correlation-id UUID] [--idempotency-key KEY | --dedupe-key STR] \
#     [--process-slug SLUG] [--city-pair-slug SLUG] [--dry-run]
#
# Required env (injected by the harness; overridable by flags):
#   GASCITY_EVENT_BUS     EventBridge bus name (e.g. gascity-handoff-dev)
#   AWS_REGION            AWS region
#   GASCITY_SOURCE_CITY   this city's configured name
# Optional env:
#   GASCITY_SOURCE_CITY_ROLE (default: execution-monitoring-city)
#   GASCITY_PROCESS_SLUG, GASCITY_CITY_PAIR_SLUG, GASCITY_EVENT_SOURCE
#   GASCITY_DOMAIN_EVENT_SOURCE (default: GasCity) for flat events
#   GASCITY_CODEGEN_OWNERSHIP_JSON maps payload.repo to its owning code city
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PACK_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
SCHEMA_DIR="$PACK_DIR/schemas/events"

die() { printf 'publish-cross-city-event: %s\n' "$*" >&2; exit 1; }

EVENT_TYPE=""
PAYLOAD_FILE=""
SCHEMA_FILE=""
TARGET_CITY=""
TARGET_CITY_ROLE=""
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
    --schema-file) SCHEMA_FILE="$2"; shift 2 ;;
    --target-city) TARGET_CITY="$2"; shift 2 ;;
    --target-city-role) TARGET_CITY_ROLE="$2"; shift 2 ;;
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

SOURCE_CITY="${GASCITY_SOURCE_CITY:-}"
[ -n "$SOURCE_CITY" ] || die "GASCITY_SOURCE_CITY env (or harness injection) is required"
SOURCE_CITY_ROLE="${GASCITY_SOURCE_CITY_ROLE:-execution-monitoring-city}"
# Event type -> schema file. This pack registers the repo handoff events;
# any other event type must supply --schema-file from its owning pack. The
# envelope-vs-flat class is read from the schema, not hardcoded here.
if [ -z "$SCHEMA_FILE" ]; then
  case "$EVENT_TYPE" in
    RepoBugReported.v1) SCHEMA_FILE="$SCHEMA_DIR/repo-bug-reported.v1.schema.json" ;;
    RepoChangeRequested.v1) SCHEMA_FILE="$SCHEMA_DIR/repo-change-requested.v1.schema.json" ;;
    *) die "unsupported --event-type: $EVENT_TYPE (not registered in $SCHEMA_DIR; pass --schema-file from the owning pack)" ;;
  esac
fi
[ -f "$SCHEMA_FILE" ] || die "schema not found: $SCHEMA_FILE"
SCHEMA_EVENT_TYPE="$(jq -r '.properties.event_type.const // empty' "$SCHEMA_FILE")"
if [ -n "$SCHEMA_EVENT_TYPE" ] && [ "$SCHEMA_EVENT_TYPE" != "$EVENT_TYPE" ]; then
  die "--schema-file declares event_type $SCHEMA_EVENT_TYPE, not $EVENT_TYPE"
fi

# Envelope events require the cross-city envelope fields; flat events are
# domain-addressed and publish the payload as the raw EventBridge detail.
EVENT_CLASS="flat"
if jq -e '(.required // []) | index("process_slug")' "$SCHEMA_FILE" >/dev/null; then
  EVENT_CLASS="envelope"
fi
if [ "$EVENT_CLASS" = "envelope" ]; then
  [ -n "$PROCESS_SLUG" ] || die "--process-slug or GASCITY_PROCESS_SLUG is required for envelope events"
  CITY_PAIR_SLUG="${CITY_PAIR_SLUG:-${PROCESS_SLUG}-exec-codegen}"
else
  PROCESS_SLUG="${PROCESS_SLUG:-domain-runtime}"
  CITY_PAIR_SLUG="${CITY_PAIR_SLUG:-domain-runtime}"
fi

resolve_codegen_owner() {
  local repo="$1"
  local target_city="$2"
  jq -ce \
    --arg repo "$repo" \
    --arg target_city "$target_city" \
    --arg event_type "$EVENT_TYPE" '
      def norm:
        tostring
        | ascii_downcase
        | sub("^git\\+https://github.com/"; "")
        | sub("^https://github.com/"; "")
        | sub("\\.git$"; "");
      def short: norm | split("/")[-1];
      def repo_matches($entry):
        (($entry.repo_name // "" | norm) == ($repo | norm)) or
        (($entry.repo_full_name // "" | norm) == ($repo | norm)) or
        (($entry.repo_url // "" | norm) == ($repo | norm)) or
        (($entry.repo_name // "" | short) == ($repo | short)) or
        (($entry.repo_full_name // "" | short) == ($repo | short)) or
        (($entry.repo_url // "" | short) == ($repo | short));
      first(.[] | select(repo_matches(.) and
        (($target_city | length) == 0 or .code_city == $target_city) and
        ((.supported_event_types // []) | index($event_type))))
    ' <<<"${GASCITY_CODEGEN_OWNERSHIP_JSON:-[]}"
}

CODEGEN_OWNER=""
case "$EVENT_TYPE" in
  RepoBugReported.v1|RepoChangeRequested.v1)
    PAYLOAD_REPO="$(jq -r '.repo // empty' "$PAYLOAD_FILE")"
    [ -n "$PAYLOAD_REPO" ] || die "payload.repo is required for $EVENT_TYPE ownership lookup"
    if jq -e 'has("route") or has("gc_route") or has("gc.routed_to")' "$PAYLOAD_FILE" >/dev/null; then
      die "$EVENT_TYPE payload must not include route, gc_route, or gc.routed_to; routing is derived from the ownership index and receiving adapter"
    fi
    CODEGEN_OWNER="$(resolve_codegen_owner "$PAYLOAD_REPO" "$TARGET_CITY" 2>/dev/null)" || {
      if [ -n "$TARGET_CITY" ]; then
        die "payload.repo=$PAYLOAD_REPO event_type=$EVENT_TYPE is not indexed for target_city=$TARGET_CITY in GASCITY_CODEGEN_OWNERSHIP_JSON"
      fi
      die "payload.repo=$PAYLOAD_REPO event_type=$EVENT_TYPE is not in GASCITY_CODEGEN_OWNERSHIP_JSON; add a repo ownership entry before publishing"
    }
    RESOLVED_TARGET_CITY="$(printf '%s' "$CODEGEN_OWNER" | jq -r '.code_city // empty')"
    [ -n "$RESOLVED_TARGET_CITY" ] || die "matched ownership entry for payload.repo=$PAYLOAD_REPO has no code_city"
    TARGET_CITY="$RESOLVED_TARGET_CITY"
    TARGET_CITY_ROLE="code-generation-city"
    ;;
  *)
    if [ "$EVENT_CLASS" = "envelope" ]; then
      [ -n "$TARGET_CITY" ] || die "--target-city is required for envelope event $EVENT_TYPE"
      if [ -z "$TARGET_CITY_ROLE" ]; then
        TARGET_CITY_ROLE="$(jq -r '.properties.target_city_role.const // empty' "$SCHEMA_FILE")"
      fi
      [ -n "$TARGET_CITY_ROLE" ] || die "--target-city-role is required for envelope event $EVENT_TYPE"
    fi
    ;;
esac

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

TMP_DETAIL="$(mktemp "${TMPDIR:-/tmp}/gascity-xcity-event.XXXXXX.json")"
cleanup() { rm -f "$TMP_DETAIL"; }
trap cleanup EXIT

if [ "$EVENT_CLASS" = "envelope" ]; then
  jq -n \
    --arg event_type "$EVENT_TYPE" \
    --arg process_slug "$PROCESS_SLUG" \
    --arg city_pair_slug "$CITY_PAIR_SLUG" \
    --arg source_city "$SOURCE_CITY" \
    --arg source_city_role "$SOURCE_CITY_ROLE" \
    --arg target_city "$TARGET_CITY" \
    --arg target_city_role "$TARGET_CITY_ROLE" \
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
      target_city_role: $target_city_role,
      correlation_id: $correlation_id,
      idempotency_key: $idempotency_key,
      occurred_at: $occurred_at,
      payload: $payload[0]
    }' > "$TMP_DETAIL"
else
  # Flat events publish the payload as the detail, untouched. Required
  # request-echo fields (correlation, formula_bundle_hash, ...) are the
  # caller's responsibility; the schema validation below enforces presence.
  jq . "$PAYLOAD_FILE" > "$TMP_DETAIL"
fi

# Validate against the versioned schema. Prefer a real JSON Schema validator;
# fall back to a jq required-field check so the script still guards in minimal
# runtimes (the parity test exercises the full validator in CI).
validate_detail() {
  if command -v python3 >/dev/null 2>&1 && python3 -c "import jsonschema" >/dev/null 2>&1; then
    python3 - "$SCHEMA_FILE" "$TMP_DETAIL" <<'PY'
import json, sys
import jsonschema
schema = json.load(open(sys.argv[1]))
instance = json.load(open(sys.argv[2]))
jsonschema.validate(instance=instance, schema=schema)
PY
    return $?
  fi
  # Fallback: top-level + payload required-field presence via jq.
  local k
  for k in $(jq -r '.required[]' "$SCHEMA_FILE"); do
    jq -e "has(\"$k\")" "$TMP_DETAIL" >/dev/null || { echo "missing required field: $k" >&2; return 1; }
  done
  for k in $(jq -r '.properties.payload.required[]? // empty' "$SCHEMA_FILE"); do
    jq -e ".payload | has(\"$k\")" "$TMP_DETAIL" >/dev/null || { echo "missing payload field: $k" >&2; return 1; }
  done
  return 0
}

if ! validate_detail; then
  die "event failed schema validation against $(basename "$SCHEMA_FILE")"
fi

if [ "$EVENT_CLASS" = "envelope" ]; then
  EVENT_SOURCE_VALUE="${GASCITY_EVENT_SOURCE:-gascity.${SOURCE_CITY}}"
else
  EVENT_SOURCE_VALUE="${GASCITY_DOMAIN_EVENT_SOURCE:-GasCity}"
fi

if [ "$DRY_RUN" -eq 1 ]; then
  echo "[dry-run] validated $EVENT_TYPE against $(basename "$SCHEMA_FILE")"
  echo "[dry-run] event_class=$EVENT_CLASS source=$EVENT_SOURCE_VALUE"
  if [ -n "$CODEGEN_OWNER" ]; then
    echo "[dry-run] codegen_owner=$(printf '%s' "$CODEGEN_OWNER" | jq -c '{repo_name, repo_full_name, code_city, code_city_purpose, execution_city, execution_city_purpose}')"
  fi
  echo "[dry-run] idempotency_key=$IDEMPOTENCY_KEY"
  echo "[dry-run] correlation_id=$CORRELATION_ID"
  echo "[dry-run] would put-events to bus=${GASCITY_EVENT_BUS:-<unset>} region=${AWS_REGION:-<unset>}"
  echo "[dry-run] detail:"
  jq . "$TMP_DETAIL"
  exit 0
fi

[ -n "${GASCITY_EVENT_BUS:-}" ] || die "GASCITY_EVENT_BUS env is required for live publish"
[ -n "${AWS_REGION:-}" ] || die "AWS_REGION env is required for live publish"
command -v aws >/dev/null 2>&1 || die "aws CLI is required for live publish"

ENTRY="$(jq -n \
  --arg bus "$GASCITY_EVENT_BUS" \
  --arg source "$EVENT_SOURCE_VALUE" \
  --arg detail_type "$EVENT_TYPE" \
  --rawfile detail "$TMP_DETAIL" \
  '[{ EventBusName: $bus, Source: $source, DetailType: $detail_type, Detail: $detail }]')"

RESULT="$(aws events put-events --region "$AWS_REGION" --entries "$ENTRY" --output json)"
FAILED="$(printf '%s' "$RESULT" | jq -r '.FailedEntryCount // 0')"
if [ "$FAILED" != "0" ]; then
  printf '%s\n' "$RESULT" >&2
  die "put-events reported $FAILED failed entries"
fi
EVENT_ID="$(printf '%s' "$RESULT" | jq -r '.Entries[0].EventId // "unknown"')"
echo "published $EVENT_TYPE event_id=$EVENT_ID idempotency_key=$IDEMPOTENCY_KEY correlation_id=$CORRELATION_ID"
