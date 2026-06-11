#!/usr/bin/env bash
# domain-command-publish — publish GasCityDomainCommandRequested.v1 for open
# domain_command_request beads.
#
# Triggered by orders/domain-command-publish.toml on a cooldown.
#
# Waiter contract: a city agent that needs a deterministic domain action
# (survey run, prompt eval, graph build, decision write, ...) creates one
# bead with:
#   gc.kind  = domain_command_request
#   metadata.payload      = the FLAT GasCityDomainCommandRequested.v1 detail
#                           (JSON string; see the pack schema)
#   metadata.origin_bead  = the molecule step bead the result unblocks
# and wires the dependency `gc bd dep <request> --blocks <origin-step>` so the
# step stays invisible to work queries until the command terminal arrives.
#
# This order publishes the request and stamps handoff.command_published_at;
# the bead stays OPEN as the durable waiter. domain-command-reconcile closes
# it when the domain bridge answers with GasCityDomainCommandTerminal.v1.
#
# Reply injection: if the payload has no reply block, the publisher injects
# the standard one ({this city, command-terminal-reconciler route}) so agents
# never hand-address replies. Idempotent: published waiters carry
# handoff.command_published_at and are filtered out; the publish dedupe key is
# the command_id and the domain router dedupes by idempotency_key.
set -euo pipefail

HANDOFF_SCOPE="command-publish"
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=./handoff-state.sh
. "$SCRIPT_DIR/handoff-state.sh"

if ! handoff_acquire_lock; then
  echo "[domain-command-publish] another run holds the lock — skipping tick"
  exit 0
fi
trap handoff_cleanup EXIT

PUBLISH="$(handoff_publish_script)"
REPLY_ROUTE="${GC_COMMAND_REPLY_ROUTE:-domain-handoff.command-terminal-reconciler}"
REPLY_CITY="${GASCITY_SOURCE_CITY:-}"
REPLY_CITY_ROLE="${GASCITY_SOURCE_CITY_ROLE:-execution-monitoring-city}"

OPEN_JSON=$(gc bd list --status=open --json --limit=0 2>/dev/null || echo '[]')

CANDIDATES=$(printf '%s' "$OPEN_JSON" | jq -r '
  .[]
  | select((.metadata."gc.kind" // "") == "domain_command_request")
  | select((.metadata."handoff.command_published_at" // "") == "")
  | .id
' 2>/dev/null || true)

PUBLISHED=0
INVALID=0

while IFS= read -r BEAD_ID; do
  [ -n "$BEAD_ID" ] || continue

  BEAD_JSON=$(printf '%s' "$OPEN_JSON" | jq -c --arg id "$BEAD_ID" '.[] | select(.id == $id)')
  PAYLOAD_FILE="$HANDOFF_RUN_DIR/command-$BEAD_ID.json"

  if ! printf '%s' "$BEAD_JSON" \
    | jq -e '(.metadata.payload // empty) | fromjson' > "$PAYLOAD_FILE" 2>/dev/null; then
    echo "[domain-command-publish] $BEAD_ID has no parseable metadata.payload — recording blocker" >&2
    gc bd update "$BEAD_ID" --notes "domain-command-publish: metadata.payload is missing or not valid JSON; fix the request bead" >/dev/null 2>&1 || true
    INVALID=$((INVALID + 1))
    continue
  fi

  COMMAND_ID=$(jq -r '.command_id // empty' "$PAYLOAD_FILE")
  if [ -z "$COMMAND_ID" ]; then
    echo "[domain-command-publish] $BEAD_ID payload has no command_id — recording blocker" >&2
    gc bd update "$BEAD_ID" --notes "domain-command-publish: payload.command_id is required" >/dev/null 2>&1 || true
    INVALID=$((INVALID + 1))
    continue
  fi

  # Inject the standard reply block when absent so the domain bridge can
  # address the command terminal back to this city's reconciler.
  if ! jq -e '.reply' "$PAYLOAD_FILE" >/dev/null; then
    [ -n "$REPLY_CITY" ] || { echo "[domain-command-publish] GASCITY_SOURCE_CITY required for reply injection" >&2; exit 1; }
    jq --arg city "$REPLY_CITY" --arg route "$REPLY_ROUTE" --arg role "$REPLY_CITY_ROLE" \
      '.reply = {target_city: $city, route: $route, target_city_role: $role}' \
      "$PAYLOAD_FILE" > "$PAYLOAD_FILE.tmp" && mv "$PAYLOAD_FILE.tmp" "$PAYLOAD_FILE"
  fi

  if "$PUBLISH" --event-type GasCityDomainCommandRequested.v1 \
    --schema-file "$PACK_SCHEMA_DIR/gascity-domain-command-requested.v1.schema.json" \
    --payload-file "$PAYLOAD_FILE" \
    --dedupe-key "$COMMAND_ID"; then
    NOW=$(handoff_now)
    gc bd update "$BEAD_ID" \
      --set-metadata "handoff.command_published_at=$NOW" \
      --set-metadata "handoff.command_id=$COMMAND_ID" >/dev/null
    echo "[domain-command-publish] published $COMMAND_ID from $BEAD_ID (waiter stays open)"
    PUBLISHED=$((PUBLISHED + 1))
  else
    echo "[domain-command-publish] WARN publish failed for $BEAD_ID ($COMMAND_ID); will retry next tick" >&2
  fi
done <<EOF
$CANDIDATES
EOF

echo "[domain-command-publish] done: published=$PUBLISHED invalid=$INVALID"
