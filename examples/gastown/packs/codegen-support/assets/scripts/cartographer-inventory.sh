#!/usr/bin/env bash
# Step 4 (inventory_beads) — deterministic implementation.
#
# Produces $RUN_DIR/beads_inventory.json (and intermediate artifacts) in a
# single shell process. Replaces the prose-driven LLM step so the only
# wall-time costs are the ~5 `gc bd list` calls below, not LLM reasoning
# or per-bead `gc bd show` fanout.
#
# Required env (source $RUN_DIR/state.env first):
#   RUN_DIR, RIG, WORK_ORDER_ID, REPO_ROOT
#
# Inputs read:
#   $RUN_DIR/inventory.json   — written by step 2 (rig spec inventory)
#                                Optional keys consumed:
#                                  .blocked_by_annotations[]
#                                  .inline_wo_blockers[]
#                                  .blocks_annotations[]
#                                  .family_members[]
#                                  .cohort_label
#                                  .land_together_set[]
#   specs/agent-work-orders/  — for resolving WO-NNN refs to file ids
#
# Outputs written under $RUN_DIR:
#   open_beads_raw.json          — full unfiltered open-bead list
#   open_beads.json              — filtered planning view
#   prior_epoch_beads.json       — all beads (any status) for this WO
#   cross_wo_blockers_raw.json   — single-query result for formal blockers
#   cross_wo_blockers.json       — grouped per blocker_wo_id
#   inline_wo_blockers_raw.json  — single-query result for inline WOs
#   inline_wo_blockers.json      — grouped per inline blocker_wo_id
#   downstream_blocks_raw.json   — single-query result for downstream WOs
#                                   (this WO is `Blocks:` <those WOs>)
#   downstream_blocks.json       — grouped per downstream_wo_id
#   cohort_convoy.json           — cohort convoy lookup result
#   prior_holding_stubs.json     — open HOLDING stubs targeting this WO
#                                   (placeholder-blocker:${WORK_ORDER_ID})
#                                   with their open dependents enumerated
#   candidate_dependents.json    — for step 6 forward/reverse edge detection
#                                   (relevance-filtered: only beads from
#                                    WOs explicitly referenced by this WO,
#                                    plus cohort members + HOLDING dependents)
#   beads_inventory.json         — final aggregate (downstream contract),
#                                  including candidate_dependents mirror
#   unresolved_blocker_refs.json (only if any WO-NNN refs failed to resolve)

set -euo pipefail

: "${RUN_DIR:?RUN_DIR must be set (source state.env first)}"
: "${RIG:?RIG must be set}"
: "${WORK_ORDER_ID:?WORK_ORDER_ID must be set}"
: "${REPO_ROOT:?REPO_ROOT must be set}"

cd "$REPO_ROOT"
mkdir -p "$RUN_DIR"

INVENTORY_JSON="$RUN_DIR/inventory.json"
[ -f "$INVENTORY_JSON" ] || { echo "FATAL: $INVENTORY_JSON missing (step 2 must run first)" >&2; exit 2; }

log() { printf '[inventory_beads] %s\n' "$*" >&2; }

# --- 1. Open beads (single query, raw + filtered views) -----------------
log "querying open beads"
gc --rig "$RIG" bd list --status=open --limit 0 \
  --exclude-type=molecule,session,gate,event,message,rig,role,spec,agent,convergence,step \
  --json \
  > "$RUN_DIR/open_beads_raw.json"

# Filtered planning view: drop order-tracking label and step beads.
jq '[.[]
       | select((.labels // []) | index("order-tracking") | not)
       | select(.issue_type != "step")]' \
  "$RUN_DIR/open_beads_raw.json" \
  > "$RUN_DIR/open_beads.json"

OPEN_COUNT=$(jq 'length' "$RUN_DIR/open_beads.json")
log "open beads (filtered): $OPEN_COUNT"

# --- 2. Prior-epoch beads (this WO) -------------------------------------
log "querying prior-epoch beads for $WORK_ORDER_ID"
gc --rig "$RIG" bd list --all \
  --label="source:work-order:${WORK_ORDER_ID}" --json \
  > "$RUN_DIR/prior_epoch_beads.json"

# --- 3. Cross-WO blockers (single --label-any query, then group) --------
log "resolving Blocked-by annotations"
BLOCKER_WO_IDS=()
BLOCKER_FAMILY_BASES=()  # parallel to BLOCKER_WO_IDS; "" when not a sub-WO
UNRESOLVED_REFS=()

# Sub-WO ids look like `NNN<letter>-<slug>` (e.g. 008a-storage-snapshot).
# Their family convoy is created against the parent `NNN-<slug>` work order
# and carries `family:<parent-basename>` — that's how we find it.
resolve_family_base() {
  local id="$1"
  if [[ "$id" =~ ^([0-9]+)[a-z]+- ]]; then
    local num="${BASH_REMATCH[1]}"
    local match
    match=$(ls "specs/agent-work-orders/${num}"-*.md 2>/dev/null | head -1 || true)
    if [ -n "$match" ]; then
      basename "$match" .md
      return
    fi
  fi
  echo ""
}

while IFS= read -r WO_REF; do
  [ -z "$WO_REF" ] && continue
  # The inventory step (per formula prose lines 299-302) converts WO-NNN
  # refs to full file basenames (e.g. "001-identity-backfill") before
  # writing inventory.json. Try the basename path directly first; only
  # fall back to the numeric-prefix glob for legacy/raw shorthand inputs
  # (e.g. "WO-001" or bare "001"). Without the basename-first check, a
  # full-basename input glob-expands to "<basename>-*.md" which never
  # matches the actual "<basename>.md" file — see WO-010 epoch
  # 2026-05-13T20-40-03Z which had all 9 blocker refs unresolved.
  RESOLVED=""
  if [ -f "specs/agent-work-orders/${WO_REF}.md" ]; then
    RESOLVED="$WO_REF"
  else
    NUM=$(printf '%s' "$WO_REF" | sed -E 's/^WO-//I')
    MATCH=$(ls "specs/agent-work-orders/${NUM}"-*.md 2>/dev/null | head -1 || true)
    if [ -n "$MATCH" ]; then
      RESOLVED=$(basename "$MATCH" .md)
    else
      UNRESOLVED_REFS+=("$WO_REF")
      continue
    fi
  fi
  BLOCKER_WO_IDS+=("$RESOLVED")
  BLOCKER_FAMILY_BASES+=("$(resolve_family_base "$RESOLVED")")
done < <(jq -r '.blocked_by_annotations[]? // empty' "$INVENTORY_JSON")

if [ ${#UNRESOLVED_REFS[@]} -gt 0 ]; then
  printf '%s\n' "${UNRESOLVED_REFS[@]}" | jq -R . | jq -s . \
    > "$RUN_DIR/unresolved_blocker_refs.json"
  log "WARN: ${#UNRESOLVED_REFS[@]} unresolved blocker refs"
fi

if [ ${#BLOCKER_WO_IDS[@]} -gt 0 ]; then
  LABEL_ANY=""
  for id in "${BLOCKER_WO_IDS[@]}"; do
    LABEL_ANY="${LABEL_ANY}source:work-order:${id},"
  done
  # Pull family convoys for sub-WO blockers in the same query (e.g. 008a → app-lke
  # via `family:008-ai-enrichment`). One edge per family beats N edges per sub-WO.
  for fam in "${BLOCKER_FAMILY_BASES[@]}"; do
    [ -n "$fam" ] && LABEL_ANY="${LABEL_ANY}family:${fam},"
  done
  LABEL_ANY="${LABEL_ANY%,}"
  log "single --label-any query for ${#BLOCKER_WO_IDS[@]} blocker WO(s)"
  gc --rig "$RIG" bd list --all --label-any="$LABEL_ANY" --json \
    > "$RUN_DIR/cross_wo_blockers_raw.json"
else
  echo '[]' > "$RUN_DIR/cross_wo_blockers_raw.json"
fi

# Group raw results per blocker_wo_id and classify.
BLOCKERS_JSON_ARR=$(printf '%s\n' "${BLOCKER_WO_IDS[@]:-}" | jq -R . | jq -s '[.[] | select(. != "")]')
FAMILIES_JSON_ARR=$(printf '%s\n' "${BLOCKER_FAMILY_BASES[@]:-}" | jq -R . | jq -s '[.[]]')
jq --argjson wos "$BLOCKERS_JSON_ARR" --argjson fams "$FAMILIES_JSON_ARR" '
  . as $beads
  | [ range(0; $wos | length) as $i
      | $wos[$i] as $wo
      | ($fams[$i] // "") as $fam
      | ($beads | map(select((.labels // []) | index("source:work-order:" + $wo)))) as $owned
      | ($owned | map(select(.status != "closed"))) as $live
      | (
          if $fam != "" then
            ($beads
              | map(select(.status != "closed"
                           and .issue_type == "convoy"
                           and ((.labels // []) | index("family:" + $fam))))
              | .[0].id // null)
          else null
          end
        ) as $family_convoy_id
      | {
          blocker_wo_id: $wo,
          # depends_on candidates must be open — closed beads cannot block.
          # Family convoy (when found) overrides the per-sub-WO grouping.
          convoy_bead_id: (
            $family_convoy_id
            // ($live | map(select(.issue_type == "convoy"))[0].id // null)
          ),
          task_bead_ids: (
            if $family_convoy_id != null then []
            else ($live | map(select(.issue_type != "convoy") | .id))
            end
          ),
          all_closed: (
            if $family_convoy_id != null then false
            elif ($owned | length) == 0 then false
            else ($owned | all(.status == "closed")) end
          ),
          no_beads_known: (
            if $family_convoy_id != null then false
            else (($owned | length) == 0) end
          )
        }
    ]
' "$RUN_DIR/cross_wo_blockers_raw.json" > "$RUN_DIR/cross_wo_blockers.json"

# --- 3b. Inline-WO blockers (runtime gates referenced inline in the WO body
#         but NOT in the formal `Blocked by:` line). Same query shape as
#         cross_wo_blockers, but emitted into a SEPARATE artifact so the
#         step-6 blanket auto-edge rule (which fires on cross_wo_blockers)
#         does not fire on inline gates. The per-candidate audit in step 6
#         and validate-plan check `m` consume this artifact. -------------
log "resolving inline-WO blockers"
INLINE_WO_IDS=()
INLINE_FAMILY_BASES=()
while IFS= read -r WO_REF; do
  [ -z "$WO_REF" ] && continue
  # Same resolution as blocked_by_annotations. inventory.json derives this
  # field from directly_referenced_by_target (already-resolved file paths),
  # so most refs will hit the basename branch directly.
  RESOLVED=""
  if [ -f "specs/agent-work-orders/${WO_REF}.md" ]; then
    RESOLVED="$WO_REF"
  else
    NUM=$(printf '%s' "$WO_REF" | sed -E 's/^WO-//I')
    MATCH=$(ls "specs/agent-work-orders/${NUM}"-*.md 2>/dev/null | head -1 || true)
    if [ -n "$MATCH" ]; then
      RESOLVED=$(basename "$MATCH" .md)
    else
      # Unresolved inline refs are non-fatal — fall through silently.
      # The validator will not require coverage for an entry that never
      # made it into inline_wo_blockers.json.
      continue
    fi
  fi
  INLINE_WO_IDS+=("$RESOLVED")
  INLINE_FAMILY_BASES+=("$(resolve_family_base "$RESOLVED")")
done < <(jq -r '.inline_wo_blockers[]? // empty' "$INVENTORY_JSON")

if [ ${#INLINE_WO_IDS[@]} -gt 0 ]; then
  INLINE_LABEL_ANY=""
  for id in "${INLINE_WO_IDS[@]}"; do
    INLINE_LABEL_ANY="${INLINE_LABEL_ANY}source:work-order:${id},"
  done
  for fam in "${INLINE_FAMILY_BASES[@]}"; do
    [ -n "$fam" ] && INLINE_LABEL_ANY="${INLINE_LABEL_ANY}family:${fam},"
  done
  INLINE_LABEL_ANY="${INLINE_LABEL_ANY%,}"
  log "single --label-any query for ${#INLINE_WO_IDS[@]} inline-WO blocker(s)"
  gc --rig "$RIG" bd list --all --label-any="$INLINE_LABEL_ANY" --json \
    > "$RUN_DIR/inline_wo_blockers_raw.json"
else
  echo '[]' > "$RUN_DIR/inline_wo_blockers_raw.json"
fi

INLINE_JSON_ARR=$(printf '%s\n' "${INLINE_WO_IDS[@]:-}" | jq -R . | jq -s '[.[] | select(. != "")]')
INLINE_FAMS_JSON_ARR=$(printf '%s\n' "${INLINE_FAMILY_BASES[@]:-}" | jq -R . | jq -s '[.[]]')
jq --argjson wos "$INLINE_JSON_ARR" --argjson fams "$INLINE_FAMS_JSON_ARR" '
  . as $beads
  | [ range(0; $wos | length) as $i
      | $wos[$i] as $wo
      | ($fams[$i] // "") as $fam
      | ($beads | map(select((.labels // []) | index("source:work-order:" + $wo)))) as $owned
      | ($owned | map(select(.status != "closed"))) as $live
      | (
          if $fam != "" then
            ($beads
              | map(select(.status != "closed"
                           and .issue_type == "convoy"
                           and ((.labels // []) | index("family:" + $fam))))
              | .[0].id // null)
          else null
          end
        ) as $family_convoy_id
      | {
          blocker_wo_id: $wo,
          convoy_bead_id: (
            $family_convoy_id
            // ($live | map(select(.issue_type == "convoy"))[0].id // null)
          ),
          task_bead_ids: (
            if $family_convoy_id != null then []
            else ($live | map(select(.issue_type != "convoy") | .id))
            end
          ),
          all_closed: (
            if $family_convoy_id != null then false
            elif ($owned | length) == 0 then false
            else ($owned | all(.status == "closed")) end
          ),
          no_beads_known: (
            if $family_convoy_id != null then false
            else (($owned | length) == 0) end
          )
        }
    ]
' "$RUN_DIR/inline_wo_blockers_raw.json" > "$RUN_DIR/inline_wo_blockers.json"

# --- 3c. Downstream-blocks (this WO is listed as `Blocks:` in OUR spec body,
#         which means downstream WOs depend on us). Same query shape as
#         inline_wo_blockers; emitted into a SEPARATE artifact so the
#         step-6 candidate set can include these beads (they are the
#         canonical existing_to_new edge targets) and validate-plan check
#         `n` can require per-candidate audit coverage. ------------------
log "resolving downstream-Blocks annotations"
BLOCKS_WO_IDS=()
BLOCKS_FAMILY_BASES=()
while IFS= read -r WO_REF; do
  [ -z "$WO_REF" ] && continue
  RESOLVED=""
  if [ -f "specs/agent-work-orders/${WO_REF}.md" ]; then
    RESOLVED="$WO_REF"
  else
    NUM=$(printf '%s' "$WO_REF" | sed -E 's/^WO-//I')
    MATCH=$(ls "specs/agent-work-orders/${NUM}"-*.md 2>/dev/null | head -1 || true)
    if [ -n "$MATCH" ]; then
      RESOLVED=$(basename "$MATCH" .md)
    else
      # Unresolved downstream refs are non-fatal — silently skip. The
      # validator will not require coverage for an entry that never made
      # it into downstream_blocks.json.
      continue
    fi
  fi
  BLOCKS_WO_IDS+=("$RESOLVED")
  BLOCKS_FAMILY_BASES+=("$(resolve_family_base "$RESOLVED")")
done < <(jq -r '.blocks_annotations[]? // empty' "$INVENTORY_JSON")

if [ ${#BLOCKS_WO_IDS[@]} -gt 0 ]; then
  BLOCKS_LABEL_ANY=""
  for id in "${BLOCKS_WO_IDS[@]}"; do
    BLOCKS_LABEL_ANY="${BLOCKS_LABEL_ANY}source:work-order:${id},"
  done
  for fam in "${BLOCKS_FAMILY_BASES[@]}"; do
    [ -n "$fam" ] && BLOCKS_LABEL_ANY="${BLOCKS_LABEL_ANY}family:${fam},"
  done
  BLOCKS_LABEL_ANY="${BLOCKS_LABEL_ANY%,}"
  log "single --label-any query for ${#BLOCKS_WO_IDS[@]} downstream-blocks WO(s)"
  gc --rig "$RIG" bd list --all --label-any="$BLOCKS_LABEL_ANY" --json \
    > "$RUN_DIR/downstream_blocks_raw.json"
else
  echo '[]' > "$RUN_DIR/downstream_blocks_raw.json"
fi

BLOCKS_JSON_ARR=$(printf '%s\n' "${BLOCKS_WO_IDS[@]:-}" | jq -R . | jq -s '[.[] | select(. != "")]')
BLOCKS_FAMS_JSON_ARR=$(printf '%s\n' "${BLOCKS_FAMILY_BASES[@]:-}" | jq -R . | jq -s '[.[]]')
jq --argjson wos "$BLOCKS_JSON_ARR" --argjson fams "$BLOCKS_FAMS_JSON_ARR" '
  . as $beads
  | [ range(0; $wos | length) as $i
      | $wos[$i] as $wo
      | ($fams[$i] // "") as $fam
      | ($beads | map(select((.labels // []) | index("source:work-order:" + $wo)))) as $owned
      | ($owned | map(select(.status != "closed"))) as $live
      | (
          if $fam != "" then
            ($beads
              | map(select(.status != "closed"
                           and .issue_type == "convoy"
                           and ((.labels // []) | index("family:" + $fam))))
              | .[0].id // null)
          else null
          end
        ) as $family_convoy_id
      | {
          downstream_wo_id: $wo,
          convoy_bead_id: (
            $family_convoy_id
            // ($live | map(select(.issue_type == "convoy"))[0].id // null)
          ),
          task_bead_ids: (
            if $family_convoy_id != null then []
            else ($live | map(select(.issue_type != "convoy") | .id))
            end
          ),
          all_closed: (
            if $family_convoy_id != null then false
            elif ($owned | length) == 0 then false
            else ($owned | all(.status == "closed")) end
          ),
          no_beads_known: (
            if $family_convoy_id != null then false
            else (($owned | length) == 0) end
          )
        }
    ]
' "$RUN_DIR/downstream_blocks_raw.json" > "$RUN_DIR/downstream_blocks.json"

# --- 4. Cohort convoy lookup --------------------------------------------
COHORT_LABEL=$(jq -r '.cohort_label // empty' "$INVENTORY_JSON")
if [ -n "$COHORT_LABEL" ]; then
  log "cohort label: $COHORT_LABEL"
  gc --rig "$RIG" bd list --all --type=convoy \
    --label="$COHORT_LABEL" --json > "$RUN_DIR/cohort_convoy.json"
else
  echo '[]' > "$RUN_DIR/cohort_convoy.json"
fi

EXISTING_COHORT_CONVOY_ID=$(jq -r '.[0].id // empty' "$RUN_DIR/cohort_convoy.json")
COHORT_DUP=$(jq '[.[1:][]? | .id]' "$RUN_DIR/cohort_convoy.json")
COHORT_TITLE=$(jq -r '.[0].title // empty' "$RUN_DIR/cohort_convoy.json")

# --- 5a. Prior HOLDING stubs targeting this WO ---------------------------
# When an upstream cartographer run encountered `Blocked by: <this WO>` and
# this WO had no beads at the time, that run created a HOLDING stub with
# label `placeholder-blocker:<this-WO>`. This run is the one that lands the
# real beads — enumerate those HOLDINGs and their open dependents so the
# graph step can plan retarget edges and the emit step can release them.
#
# Runs BEFORE §5b so HOLDING-dependent bead IDs can be unioned into
# candidate_dependents (every HOLDING dependent must appear in step 6's
# per-candidate audit; otherwise step 5.7's HOLDING release planning has
# no matching new_to_existing_decisions row to consume).
log "querying prior HOLDING stubs targeting $WORK_ORDER_ID"
gc --rig "$RIG" bd list --status=open \
  --label="placeholder-blocker:${WORK_ORDER_ID}" --json \
  > "$RUN_DIR/prior_holding_stubs_raw.json"

jq --slurpfile beads "$RUN_DIR/open_beads_raw.json" '
  [ .[]
    | . as $h
    | {
        id: $h.id,
        title: $h.title,
        labels: ($h.labels // []),
        placeholder_blocker: (
          ($h.labels // [])
            | map(select(startswith("placeholder-blocker:")))[0]
            | sub("^placeholder-blocker:"; "")
        ),
        placeholder_for_wo: (
          ($h.labels // [])
            | map(select(startswith("placeholder-for-wo:")))[0]
            | sub("^placeholder-for-wo:"; "")
        ),
        dependents: (
          $beads[0]
          | map(select(
              (.dependencies // [])
                | map(.depends_on_id)
                | index($h.id)
            ))
          | map({id, title, description, labels: (.labels // [])})
        )
      }
  ]
' "$RUN_DIR/prior_holding_stubs_raw.json" \
  > "$RUN_DIR/prior_holding_stubs.json"

PRIOR_HOLDING_COUNT=$(jq 'length' "$RUN_DIR/prior_holding_stubs.json")
log "prior HOLDING stubs targeting this WO: $PRIOR_HOLDING_COUNT"

# --- 5b. Candidate dependents (relevance-filtered, not recency-padded) ---
# Selection model: trust the work order. A bead is a valid step-6 candidate
# only if its source WO is explicitly referenced by THIS work order
# (Blocked by / inline body link or bare-text ref / Blocks / family
# cohort sibling), OR it is a cohort convoy member, OR it is a dependent
# of a prior HOLDING that targets this WO. Older selection logic padded
# the set with top-N-by-recency open beads from unrelated WOs, displacing
# real signal — replaced.
#
# Excluded uniformly:
#   - HOLDING stubs (placeholder:cross-wo-blocker) — cartographer-managed
#     scaffolding with no `done_when` content; lifecycle is in §5a.
#   - Order-tracking beads, step beads — never edge candidates.
#
# Capped at 250 as a safety valve for pathological large-cohort WOs; on
# small/medium WOs the relevance filter typically returns 20–60 beads and
# the cap never fires.
RELEVANT_WO_IDS=()
while IFS= read -r ID; do
  [ -n "$ID" ] && RELEVANT_WO_IDS+=("$ID")
done < <(jq -r '
  (
    (.blocked_by_annotations // [])
    + (.inline_wo_blockers // [])
    + (.blocks_annotations // [])
    + (.family_members // [])
  ) | unique | .[]
' "$INVENTORY_JSON")
RELEVANT_WOS_JSON=$(printf '%s\n' "${RELEVANT_WO_IDS[@]:-}" | jq -R . | jq -s '[.[] | select(. != "")]')
log "relevance filter — ${#RELEVANT_WO_IDS[@]} referenced WO(s)"

HOLDING_DEPENDENT_IDS_JSON=$(jq '[.[].dependents[]?.id]' "$RUN_DIR/prior_holding_stubs.json")

jq --arg convoy "${EXISTING_COHORT_CONVOY_ID:-}" \
   --argjson relevant_wos "$RELEVANT_WOS_JSON" \
   --argjson holding_dependent_ids "$HOLDING_DEPENDENT_IDS_JSON" '
  # Build the lookup set of acceptable source:work-order:* labels.
  ($relevant_wos | map("source:work-order:" + .)) as $relevant_labels
  | (
      # 1. Open beads whose source WO is in the relevance set.
      [ .[]
        | select((.labels // []) | any(. as $l | $relevant_labels | index($l)))
      ]
      # 2. Cohort convoy members (parent == cohort_convoy_id).
      + (if $convoy == "" then []
         else [.[] | select(.parent == $convoy)] end)
      # 3. Prior HOLDING dependents — explicit union by ID so step 5.7
      #    has matching audit rows in step 6.
      + [ .[] | select(.id as $i | $holding_dependent_ids | index($i)) ]
    )
  # Standard exclusions.
  | map(select((.labels // []) | index("order-tracking") | not))
  | map(select((.labels // []) | index("placeholder:cross-wo-blocker") | not))
  | map(select(.issue_type != "step"))
  # Issue F fix: deduplicate by id (force-include unions historically
  # produced doubled entries for beads tagged with multiple in-scope WOs).
  | unique_by(.id)
  # Shape for downstream consumers.
  | map({id, title, status, parent, labels: (.labels // []),
         description, updated_at})
  # Recency sort for cartographer reasoning order (most-recent first).
  | sort_by(.updated_at // "1970-01-01T00:00:00Z") | reverse
  # Safety valve cap; relevance filter normally keeps the count well below.
  | .[:250]
' "$RUN_DIR/open_beads_raw.json" > "$RUN_DIR/candidate_dependents.json"

CANDIDATE_COUNT=$(jq 'length' "$RUN_DIR/candidate_dependents.json")
log "candidate_dependents: $CANDIDATE_COUNT bead(s) selected via relevance filter"

# --- 6. Family / land-together bookkeeping (best-effort, deterministic) ---
# unplanned_family_members: peer WOs in the same family that have no beads
# in the bead store yet. The family is derived from cohort_label of the form
# `family:NNN-slug` — peers are spec files matching `specs/agent-work-orders/NNN*-*.md`,
# excluding the current WO. A peer is "unplanned" iff there are zero beads
# carrying `source:work-order:<peer-id>`. `land_together_set` overrides the
# glob heuristic when explicitly set in inventory.json.
UNPLANNED_FAMILY='[]'
PEER_IDS=()

LAND_TOGETHER=$(jq -c '.land_together_set // []' "$INVENTORY_JSON")
if [ "$LAND_TOGETHER" != "[]" ]; then
  while IFS= read -r WO_ID; do
    [ -z "$WO_ID" ] && continue
    [ "$WO_ID" = "$WORK_ORDER_ID" ] && continue
    PEER_IDS+=("$WO_ID")
  done < <(jq -r '.[]' <<<"$LAND_TOGETHER")
elif [ -n "${COHORT_LABEL:-}" ]; then
  # Strip `family:` (or any `prefix:`) then take leading numeric chunk.
  FAMILY_VAL="${COHORT_LABEL#*:}"
  FAMILY_NUM="${FAMILY_VAL%%-*}"
  if [ -n "$FAMILY_NUM" ]; then
    while IFS= read -r PEER_FILE; do
      [ -z "$PEER_FILE" ] && continue
      PEER_ID=$(basename "$PEER_FILE" .md)
      [ "$PEER_ID" = "$WORK_ORDER_ID" ] && continue
      PEER_IDS+=("$PEER_ID")
    done < <(ls "specs/agent-work-orders/${FAMILY_NUM}"*-*.md 2>/dev/null || true)
  fi
fi

if [ ${#PEER_IDS[@]} -gt 0 ]; then
  REAL_UNPLANNED=()
  for PEER_ID in "${PEER_IDS[@]}"; do
    COUNT=$(gc --rig "$RIG" bd list --all \
      --label="source:work-order:${PEER_ID}" --json 2>/dev/null \
      | jq 'length')
    [ "${COUNT:-0}" = "0" ] && REAL_UNPLANNED+=("$PEER_ID")
  done
  if [ ${#REAL_UNPLANNED[@]} -gt 0 ]; then
    UNPLANNED_FAMILY=$(printf '%s\n' "${REAL_UNPLANNED[@]}" | jq -R . | jq -s .)
  fi
fi

# --- 7. Stranded cohort members (placeholder: deterministic check is hard
# without per-WO convoy lookups; leave [] and let downstream LLM steps
# surface this if needed, same as prior runs which produced []) ---
STRANDED='[]'

# --- 8. Assemble final beads_inventory.json ------------------------------
UNRESOLVED_BLOCKERS='[]'
[ -f "$RUN_DIR/unresolved_blocker_refs.json" ] && UNRESOLVED_BLOCKERS=$(cat "$RUN_DIR/unresolved_blocker_refs.json")

LARGE_OPEN_SET=$([ "$OPEN_COUNT" -gt 50 ] && echo true || echo false)

jq -n \
  --slurpfile open       "$RUN_DIR/open_beads.json" \
  --slurpfile prior      "$RUN_DIR/prior_epoch_beads.json" \
  --slurpfile cross      "$RUN_DIR/cross_wo_blockers.json" \
  --slurpfile inline_blk "$RUN_DIR/inline_wo_blockers.json" \
  --slurpfile downstream "$RUN_DIR/downstream_blocks.json" \
  --slurpfile candidates "$RUN_DIR/candidate_dependents.json" \
  --slurpfile convoy     "$RUN_DIR/cohort_convoy.json" \
  --slurpfile holding    "$RUN_DIR/prior_holding_stubs.json" \
  --arg convoy_id        "${EXISTING_COHORT_CONVOY_ID:-}" \
  --arg cohort_label     "${COHORT_LABEL:-}" \
  --arg convoy_title     "${COHORT_TITLE:-}" \
  --argjson cohort_dup   "$COHORT_DUP" \
  --argjson stranded     "$STRANDED" \
  --argjson unplanned    "$UNPLANNED_FAMILY" \
  --argjson unresolved   "$UNRESOLVED_BLOCKERS" \
  --argjson large        "$LARGE_OPEN_SET" \
  '{
    open_beads: (
      $open[0] | map({
        id, title, issue_type, status,
        labels: (.labels // null),
        candidate_role: (
          if .id == $convoy_id then "cohort_convoy_for_this_run"
          else "unrelated" end
        )
      })
    ),
    prior_epoch_beads: (
      $prior[0] | map({
        id, title, status,
        labels: (.labels // null),
        updated_at
      })
    ),
    cross_wo_blockers: $cross[0],
    inline_wo_blockers: $inline_blk[0],
    downstream_blocks: $downstream[0],
    candidate_dependents: $candidates[0],
    existing_cohort_convoy_id: (if $convoy_id == "" then null else $convoy_id end),
    cohort_convoy_label: (if $cohort_label == "" then null else $cohort_label end),
    convoy_title_override: (if $convoy_title == "" then null else $convoy_title end),
    unresolved_cohort_dup: $cohort_dup,
    stranded_cohort_members: $stranded,
    unplanned_family_members: $unplanned,
    unresolved_blocker_refs: $unresolved,
    prior_holding_stubs: $holding[0],
    large_open_set: $large
  }' > "$RUN_DIR/beads_inventory.json"

log "wrote $RUN_DIR/beads_inventory.json"
