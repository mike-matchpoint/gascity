#!/usr/bin/env bash
# landing-arbiter-watch — migration cleanup for legacy patrol wisps.
#
# Landing Arbiter now uses pending owned-convoy landing-failure bugs as the
# canonical work item through its typed work_selector. This order no longer
# pours mol-landing-arbiter-patrol wisps; it only runs the legacy reaper so old
# wisps from pre-selector deployments do not accumulate.
set -euo pipefail

# Rigs are enumerated dynamically via `gc rig list --json` so mixed-config
# cities can clean up legacy wisps without requiring any per-rig order wiring.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

RIGS_JSON=$(gc rig list --json)

while read -r RIG_JSON; do
  RIG=$(echo "$RIG_JSON" | jq -r '.name')
  # Reap failures are non-fatal: legacy wisps are migration residue, while
  # current demand is driven directly by the typed bug selector.
  "$SCRIPT_DIR/landing-arbiter-wisp-reap.sh" "$RIG" || \
    echo "landing-arbiter-watch: reap step failed for $RIG (non-fatal)" >&2
done < <(echo "$RIGS_JSON" | jq -c '.rigs[] | select(.hq == false) | select(.suspended == false)')
